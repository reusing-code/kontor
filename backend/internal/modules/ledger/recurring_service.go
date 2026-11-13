package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/module"
)

var (
	ErrRecurringRunning     = errors.New("a recurring payment detection is already in progress")
	ErrRecurringUnavailable = errors.New("recurring payment detection is not available")
)

const (
	recurringMaxNewPerRun   = 20
	recurringMaxPending     = 50
	recurringRunTimeout     = 2 * time.Minute
	recurringContractModule = "contracts"

	noteRecurringPayment      = "recurring_payment"
	noteRecurringPaymentEnded = "recurring_payment_ended"
)

// RecurringRunResult counts the detected groups, the drafts created and the
// groups skipped because they have a pending draft or were rejected.
type RecurringRunResult struct {
	Groups  int `json:"groups"`
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

// recurringContractInput has the shape of the contracts module's create input.
type recurringContractInput struct {
	Name            string  `json:"name"`
	Company         string  `json:"company,omitempty"`
	Price           float64 `json:"price"`
	BillingInterval string  `json:"billingInterval"`
	StartDate       string  `json:"startDate"`
	EndDate         string  `json:"endDate,omitempty"`
}

// RecurringService proposes detected recurring payments as contract drafts.
// One run per user at a time.
type RecurringService struct {
	store   *Store
	drafts  *drafts.Store
	enabled module.EnablementSource
	logger  *slog.Logger

	mu      sync.Mutex
	running map[string]bool
	baseCtx context.Context
	wg      sync.WaitGroup
}

// NewRecurringService builds the service; enabled may be nil (no module
// check).
func NewRecurringService(store *Store, draftStore *drafts.Store, enabled module.EnablementSource, logger *slog.Logger) *RecurringService {
	return &RecurringService{
		store:   store,
		drafts:  draftStore,
		enabled: enabled,
		logger:  logger,
		running: map[string]bool{},
		baseCtx: context.Background(),
	}
}

// SetBackgroundContext bounds background runs by the server's lifetime.
func (s *RecurringService) SetBackgroundContext(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseCtx = ctx
}

func (s *RecurringService) tryLock(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[userID] {
		return false
	}
	s.running[userID] = true
	return true
}

func (s *RecurringService) unlock(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, userID)
}

// RunUser detects recurring payments and creates drafts for new groups.
// ErrTargetModuleDisabled when the contracts module is disabled.
func (s *RecurringService) RunUser(ctx context.Context, userID string) (RecurringRunResult, error) {
	if s == nil || s.drafts == nil {
		return RecurringRunResult{}, ErrRecurringUnavailable
	}
	if !s.tryLock(userID) {
		return RecurringRunResult{}, ErrRecurringRunning
	}
	defer s.unlock(userID)
	return s.run(ctx, userID)
}

// RunInBackground starts a best-effort run unless the service has no draft
// store or a run is already going for the user.
func (s *RecurringService) RunInBackground(userID string) {
	if s == nil || s.drafts == nil || !s.tryLock(userID) {
		return
	}
	s.mu.Lock()
	base := s.baseCtx
	s.mu.Unlock()
	s.wg.Go(func() {
		defer s.unlock(userID)
		ctx, cancel := context.WithTimeout(base, recurringRunTimeout)
		defer cancel()
		_, err := s.run(ctx, userID)
		if err != nil && !errors.Is(err, ErrTargetModuleDisabled) {
			s.logger.Warn("background recurring payment detection failed", "userId", userID, "error", err)
		}
	})
}

func checkContractsEnabled(ctx context.Context, enabled module.EnablementSource, userID string) error {
	if enabled == nil {
		return nil
	}
	on, err := enabled.ModuleEnabled(ctx, userID, recurringContractModule)
	if err != nil {
		return err
	}
	if !on {
		return fmt.Errorf("%w: the %s module is disabled", ErrTargetModuleDisabled, recurringContractModule)
	}
	return nil
}

func (s *RecurringService) run(ctx context.Context, userID string) (RecurringRunResult, error) {
	var result RecurringRunResult
	if err := checkContractsEnabled(ctx, s.enabled, userID); err != nil {
		return result, err
	}
	var txns []LedgerTransaction
	if err := s.store.e.View(func(txn *badger.Txn) error {
		var err error
		txns, err = collectLedgerTransactions(txn, userID)
		return err
	}); err != nil {
		return result, err
	}
	groups := DetectRecurring(txns, time.Now())
	result.Groups = len(groups)

	for _, g := range groups {
		if result.Created >= recurringMaxNewPerRun {
			break
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		pending, err := s.drafts.HasSource(ctx, userID, drafts.SourceLedger, g.Ref)
		if err != nil {
			return result, err
		}
		rec, found, err := s.store.getRecurringRecord(userID, g.Ref)
		if err != nil {
			return result, err
		}
		if pending || (found && rec.Status == recurringStatusRejected) {
			result.Skipped++
			continue
		}
		d, err := recurringDraft(g)
		if err != nil {
			return result, err
		}
		// The record is written first: a record without a draft (the create
		// failed) is refreshed by the next run, a draft never lacks its record.
		if err := s.store.putRecurringRecord(userID, g.Ref, recurringRecord{
			TransactionIDs: g.TransactionIDs, Status: recurringStatusPending, CreatedAt: time.Now().UTC(),
		}); err != nil {
			return result, err
		}
		err = s.drafts.CreateLimited(ctx, userID, d, recurringMaxPending)
		switch {
		case errors.Is(err, drafts.ErrDuplicateSource):
			result.Skipped++
		case errors.Is(err, drafts.ErrTooManyPending):
			s.logger.Info("recurring payment drafts capped", "userId", userID, "created", result.Created)
			return result, nil
		case err != nil:
			return result, err
		default:
			result.Created++
		}
	}
	if result.Created > 0 {
		s.logger.Info("recurring payment drafts created", "userId", userID, "groups", result.Groups, "created", result.Created)
	}
	return result, nil
}

func recurringDraft(g RecurringGroup) (drafts.Draft, error) {
	in := recurringContractInput{
		Name:            g.Name,
		Company:         g.Name,
		Price:           float64(g.PriceMinor) / 100,
		BillingInterval: g.Interval,
		StartDate:       g.StartDate,
	}
	note := noteRecurringPayment
	if g.Ended {
		in.EndDate = g.LastDate
		note = noteRecurringPaymentEnded
	}
	input, err := json.Marshal(in)
	if err != nil {
		return drafts.Draft{}, err
	}
	source := drafts.Source{
		Kind:  drafts.SourceLedger,
		Ref:   g.Ref,
		Title: fmt.Sprintf("%s · %d× %s EUR", g.Name, g.Count, formatMinor(g.PriceMinor)),
	}
	return drafts.New(source, drafts.TargetContract, "", input, note)
}

func formatMinor(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

// RecurringDraftHooks link the group's transactions to the contract created
// from a ledger draft and remember rejected groups.
type RecurringDraftHooks struct {
	store   *Store
	enabled module.EnablementSource
	logger  *slog.Logger
}

var _ drafts.SourceHooks = RecurringDraftHooks{}

func (h RecurringDraftHooks) OnAccept(ctx context.Context, userID string, d drafts.Draft, in drafts.AcceptInput) ([]string, error) {
	if !validRecurringRef(d.Source.Ref) {
		return nil, errors.New("invalid recurring payment ref")
	}
	if err := checkContractsEnabled(ctx, h.enabled, userID); err != nil {
		return nil, err
	}
	contractID, err := uuid.Parse(in.EntityID)
	if err != nil {
		return nil, err
	}
	rec, found, err := h.store.getRecurringRecord(userID, d.Source.Ref)
	if err != nil {
		return nil, err
	}
	if !found {
		h.logger.Warn("recurring payment record missing on accept", "draftId", d.ID)
		return []string{"the payments of this draft were not found, no transactions were linked"}, nil
	}
	_, missing, err := h.store.LinkLedgerTransactionsToTarget(ctx, userID, rec.TransactionIDs,
		LedgerTransactionReference{Type: LedgerReferenceContract, TargetID: contractID})
	if err != nil {
		return nil, err
	}
	warnings := []string{}
	if len(missing) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d transactions no longer exist and were not linked", len(missing)))
	}
	if err := h.store.deleteRecurringRecord(userID, d.Source.Ref); err != nil {
		h.logger.Warn("deleting recurring payment record", "draftId", d.ID, "error", err)
	}
	return warnings, nil
}

func (h RecurringDraftHooks) OnReject(ctx context.Context, userID string, d drafts.Draft) []string {
	if !validRecurringRef(d.Source.Ref) {
		return []string{"invalid recurring payment ref"}
	}
	if err := h.store.rejectRecurringRecord(ctx, userID, d.Source.Ref); err != nil {
		h.logger.Warn("storing recurring payment rejection", "draftId", d.ID, "error", err)
		return []string{"could not remember the rejection, the payments may be proposed again"}
	}
	return []string{}
}
