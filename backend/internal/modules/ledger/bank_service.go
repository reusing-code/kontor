package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

var (
	ErrBankImportDisabled   = errors.New("bank import is not enabled")
	ErrBankPublicURLMissing = errors.New("PUBLIC_URL is not configured")
	ErrBankUnknownASPSP     = errors.New("unknown bank")
	ErrBankInvalidCountry   = errors.New("invalid country code")
	ErrBankRestricted       = errors.New("the Enable Banking app is restricted to its owner's accounts")
	ErrBankUnavailable      = errors.New("bank service unavailable")
	ErrBankRateLimited      = errors.New("bank service rate limit reached")
	ErrBankSyncRunning      = errors.New("a bank sync is already running")
	ErrBankConsentExpired   = errors.New("bank consent expired")
	ErrBankInvalidMapping   = errors.New("invalid account mapping")
	ErrBankCurrencyMismatch = errors.New("currencies differ")
)

const (
	bankASPSPCacheTTL   = 10 * time.Minute
	bankASPSPFailureTTL = 5 * time.Second
	bankDefaultValidity = 90 * 24 * time.Hour
	bankMaxValidity     = 180 * 24 * time.Hour
	bankValidityMargin  = 5 * time.Minute
	bankSessionTimeout  = 30 * time.Second
	bankCleanupTimeout  = 15 * time.Second
)

// Reasons the public callback reports to the result page (a fixed set).
const (
	BankReasonDenied       = "denied"
	BankReasonBankError    = "bank_error"
	BankReasonInvalidState = "invalid_state"
	BankReasonMissingCode  = "missing_code"
	BankReasonRestricted   = "restricted"
	BankReasonFailed       = "failed"
	BankReasonDisabled     = "disabled"
)

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

type BankServiceConfig struct {
	Client        *enablebanking.Client
	Store         *Store
	Enabled       module.EnablementSource
	EncryptionKey []byte
	PublicURL     string
	AfterImport   func(userID string)
	Logger        *slog.Logger
	Now           func() time.Time
}

// BankService connects banks through Enable Banking. A nil *BankService
// behaves as disabled.
type BankService struct {
	client      *enablebanking.Client
	store       *Store
	enabled     module.EnablementSource
	key         []byte
	publicURL   string
	afterImport func(userID string)
	logger      *slog.Logger
	now         func() time.Time

	// connMu serializes the find-then-write of a consent callback, so two
	// callbacks for one bank cannot create two connections.
	connMu sync.Mutex

	cacheMu sync.Mutex
	cache   map[string]aspspCacheEntry
}

type aspspCacheEntry struct {
	items   []BankInfo
	err     error
	fetched time.Time
}

func NewBankService(cfg BankServiceConfig) *BankService {
	s := &BankService{
		client:      cfg.Client,
		store:       cfg.Store,
		enabled:     cfg.Enabled,
		key:         cfg.EncryptionKey,
		publicURL:   strings.TrimRight(cfg.PublicURL, "/"),
		afterImport: cfg.AfterImport,
		logger:      cfg.Logger,
		now:         cfg.Now,
		cache:       map[string]aspspCacheEntry{},
	}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

func (s *BankService) active() bool { return s != nil && s.client != nil }

// BankInfo is a bank offered for connecting.
type BankInfo struct {
	Name           string `json:"name"`
	Country        string `json:"country"`
	Logo           string `json:"logo"`
	Verified       bool   `json:"verified"`
	MaxConsentDays int    `json:"maxConsentDays"`
	// maxConsentSeconds keeps the bank's exact maximum; the day count above is
	// only for display and would round a short maximum down to zero.
	maxConsentSeconds int64
}

type BankAuthorizeInput struct {
	ASPSPName    string `json:"aspspName"`
	ASPSPCountry string `json:"aspspCountry"`
}

// BankCallbackResult is what the callback tells the result page: success, or a
// fixed reason code. It never carries upstream data.
type BankCallbackResult struct {
	Connected bool
	Reason    string
}

// bankErrCode describes an upstream error without anything but its status and
// machine-readable code.
func bankErrCode(err error) string {
	var apiErr *enablebanking.APIError
	switch {
	case errors.As(err, &apiErr):
		return fmt.Sprintf("%d %s", apiErr.Status, apiErr.Code)
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		var transport *enablebanking.TransportError
		if errors.As(err, &transport) {
			return "transport"
		}
		return "error"
	}
}

func isBankRestricted(err error) bool {
	var apiErr *enablebanking.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case "ACCESS_DENIED", "NO_ACCOUNTS_ADDED", "ASPSP_ACCOUNT_NOT_ACCESSIBLE":
		return true
	}
	return false
}

func mapBankUpstreamError(err error) error {
	switch {
	case isBankRestricted(err):
		return ErrBankRestricted
	case enablebanking.IsRateLimited(err):
		return ErrBankRateLimited
	default:
		return ErrBankUnavailable
	}
}

func isVerifiedBank(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "dkb") || strings.Contains(lower, "comdirect")
}

func (s *BankService) ListASPSPs(ctx context.Context, country string) ([]BankInfo, error) {
	if !s.active() {
		return nil, ErrBankImportDisabled
	}
	if !countryPattern.MatchString(country) {
		return nil, ErrBankInvalidCountry
	}
	now := s.now()
	s.cacheMu.Lock()
	entry, ok := s.cache[country]
	s.cacheMu.Unlock()
	if ok && entry.err == nil && now.Sub(entry.fetched) < bankASPSPCacheTTL {
		return entry.items, nil
	}
	// A failure is remembered briefly so a failing upstream is not hit by
	// every request.
	if ok && entry.err != nil && now.Sub(entry.fetched) < bankASPSPFailureTTL {
		return nil, entry.err
	}

	list, err := s.client.ASPSPs(ctx, country)
	if err != nil {
		s.logger.Warn("bank list failed", "country", country, "upstream", bankErrCode(err))
		mapped := mapBankUpstreamError(err)
		// the caller's own cancellation says nothing about the upstream
		if ctx.Err() == nil {
			s.cacheMu.Lock()
			s.cache[country] = aspspCacheEntry{err: mapped, fetched: now}
			s.cacheMu.Unlock()
		}
		return nil, mapped
	}
	items := make([]BankInfo, 0, len(list))
	for _, a := range list {
		items = append(items, BankInfo{
			Name:           a.Name,
			Country:        a.Country,
			Logo:           a.Logo,
			Verified:       isVerifiedBank(a.Name),
			MaxConsentDays: int(a.MaximumConsentValidity / 86400),

			maxConsentSeconds: a.MaximumConsentValidity,
		})
	}
	s.cacheMu.Lock()
	s.cache[country] = aspspCacheEntry{items: items, fetched: now}
	s.cacheMu.Unlock()
	return items, nil
}

// consentValidUntil is the end of the requested consent: the bank's maximum,
// capped at 180 days, minus a margin because Enable Banking rejects values
// above its own "now + maximum".
func consentValidUntil(now time.Time, maxSeconds int64) time.Time {
	validity := bankDefaultValidity
	if maxSeconds > 0 {
		validity = min(time.Duration(maxSeconds)*time.Second, bankMaxValidity)
	}
	return now.Add(validity - bankValidityMargin)
}

// StartAuthorization stores a single-use state and returns the bank's
// authorization URL.
func (s *BankService) StartAuthorization(ctx context.Context, userID string, in BankAuthorizeInput) (string, error) {
	if !s.active() {
		return "", ErrBankImportDisabled
	}
	if s.publicURL == "" {
		return "", ErrBankPublicURLMissing
	}
	banks, err := s.ListASPSPs(ctx, strings.ToUpper(in.ASPSPCountry))
	if err != nil {
		return "", err
	}
	var bank *BankInfo
	for i := range banks {
		if strings.EqualFold(banks[i].Name, in.ASPSPName) {
			bank = &banks[i]
			break
		}
	}
	if bank == nil {
		return "", ErrBankUnknownASPSP
	}

	now := s.now()
	state, err := newBankAuthState(userID, s.key)
	if err != nil {
		return "", err
	}
	hash := bankStateHash(state)
	if err := s.store.CreateLedgerBankAuthState(ctx, userID, hash, BankAuthState{
		ASPSPName: bank.Name, ASPSPCountry: bank.Country, CreatedAt: now, ExpiresAt: now.Add(bankAuthStateTTL),
	}); err != nil {
		return "", err
	}

	url, err := s.client.StartAuth(ctx, enablebanking.AuthRequest{
		ASPSPName:       bank.Name,
		ASPSPCountry:    bank.Country,
		State:           state,
		RedirectURL:     s.publicURL + "/api/v1/ledger/bank/callback",
		ValidUntil:      consentValidUntil(now, bank.maxConsentSeconds),
		ExtendedHistory: true,
	})
	if err != nil {
		s.logger.Warn("bank authorization failed", "user_id", userID, "upstream", bankErrCode(err))
		if delErr := s.store.DeleteLedgerBankAuthState(context.WithoutCancel(ctx), userID, hash); delErr != nil {
			s.logger.Error("deleting bank auth state failed", "error", delErr)
		}
		return "", mapBankUpstreamError(err)
	}
	return url, nil
}

// CompleteAuthorization handles the public callback. The state is the only
// credential; code, state and error text never reach the result, logs or
// errors.
func (s *BankService) CompleteAuthorization(ctx context.Context, code, state, bankError string) BankCallbackResult {
	fail := func(reason string) BankCallbackResult { return BankCallbackResult{Reason: reason} }
	if !s.active() {
		return fail(BankReasonDisabled)
	}
	userID, err := openBankAuthState(state, s.key)
	if err != nil {
		return fail(BankReasonInvalidState)
	}
	now := s.now()
	pending, err := s.store.ConsumeLedgerBankAuthState(ctx, userID, bankStateHash(state), now)
	if err != nil {
		if !errors.Is(err, ErrBankStateInvalid) {
			s.logger.Error("consuming bank auth state failed", "error", err)
			return fail(BankReasonFailed)
		}
		return fail(BankReasonInvalidState)
	}
	if s.enabled != nil {
		on, err := s.enabled.ModuleEnabled(ctx, userID, ModuleID)
		if err != nil || !on {
			return fail(BankReasonFailed)
		}
	}
	if bankError != "" {
		if bankError == "access_denied" {
			return fail(BankReasonDenied)
		}
		return fail(BankReasonBankError)
	}
	if code == "" {
		return fail(BankReasonMissingCode)
	}

	sessionCtx, cancel := context.WithTimeout(ctx, bankSessionTimeout)
	defer cancel()
	session, err := s.client.CreateSession(sessionCtx, code)
	if err != nil {
		s.logger.Warn("bank session creation failed", "user_id", userID, "upstream", bankErrCode(err))
		if isBankRestricted(err) {
			return fail(BankReasonRestricted)
		}
		return fail(BankReasonFailed)
	}
	if session.ValidUntil.IsZero() {
		s.logger.Warn("bank session without validity", "user_id", userID)
		s.closeSessionID(ctx, userID, session.ID)
		return fail(BankReasonFailed)
	}

	if err := s.storeSession(ctx, userID, pending, session, now); err != nil {
		s.logger.Error("storing bank connection failed", "user_id", userID, "error", err)
		// the new remote session would stay open with its ID lost
		s.closeSessionID(ctx, userID, session.ID)
		return fail(BankReasonFailed)
	}
	return BankCallbackResult{Connected: true}
}

// storeSession creates the connection or replaces the existing one for the
// bank in place, carrying mappings and sync state over. The replacement is a
// read-modify-write of the stored record, so a state write of a running sync is
// not overwritten.
func (s *BankService) storeSession(ctx context.Context, userID string, pending BankAuthState, session enablebanking.Session, now time.Time) error {
	encrypted, err := cryptoutil.EncryptString(session.ID, s.key)
	if err != nil {
		return errors.New("encrypting session failed")
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()

	build := func(prev LedgerBankConnection) LedgerBankConnection {
		conn := LedgerBankConnection{
			ID:                 uuid.New(),
			ASPSPName:          pending.ASPSPName,
			ASPSPCountry:       pending.ASPSPCountry,
			EncryptedSessionID: encrypted,
			AuthorizedAt:       now,
			ValidUntil:         session.ValidUntil,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if prev.ID != uuid.Nil {
			conn.ID, conn.CreatedAt = prev.ID, prev.CreatedAt
		}
		for _, a := range session.Accounts {
			account := LedgerBankAccount{
				UID: a.UID, IBAN: normalizeIBAN(a.IBAN), Currency: strings.ToUpper(a.Currency), Name: a.Name, IdentificationHash: a.IdentificationHash,
			}
			if p := matchPreviousBankAccount(prev.Accounts, account); p != nil {
				account.LedgerAccountID = p.LedgerAccountID
				account.LastSyncAt = p.LastSyncAt
				// A consent_expired status and its attempt described the old
				// consent: keeping only the attempt would show a stale "Failed"
				// and the account is due on the next tick without it.
				if p.LastSyncStatus != BankSyncConsentExpired {
					account.LastSyncStatus = p.LastSyncStatus
					account.LastSyncAttemptAt = p.LastSyncAttemptAt
				}
				account.LastImported = p.LastImported
				account.LastDuplicates = p.LastDuplicates
			}
			conn.Accounts = append(conn.Accounts, account)
		}
		return conn
	}

	old, err := s.store.FindLedgerBankConnectionByASPSP(ctx, userID, pending.ASPSPName, pending.ASPSPCountry)
	var stored, replaced LedgerBankConnection
	switch {
	case err == nil:
		stored, err = s.store.ModifyLedgerBankConnection(ctx, userID, old.ID, func(c *LedgerBankConnection) {
			replaced = *c
			*c = build(*c)
		})
		if err != nil {
			return err
		}
	case errors.Is(err, storage.ErrNotFound):
		stored = build(LedgerBankConnection{})
		if err := s.store.PutLedgerBankConnection(ctx, userID, stored); err != nil {
			return err
		}
	default:
		return err
	}
	// The record is stored; mappings are also resolved lazily on every list and
	// sync, so a failure here must not make the callback fail (it would close
	// the session the stored record points at).
	if _, err := s.resolveMappings(ctx, userID, stored); err != nil {
		s.logger.Warn("resolving bank account mappings failed", "user_id", userID, "connection_id", stored.ID)
	}

	if replaced.ID != uuid.Nil && replaced.EncryptedSessionID != "" {
		s.closeSession(ctx, userID, replaced, session.ID)
	}
	return nil
}

func matchPreviousBankAccount(previous []LedgerBankAccount, a LedgerBankAccount) *LedgerBankAccount {
	for i := range previous {
		if a.IdentificationHash != "" && previous[i].IdentificationHash == a.IdentificationHash {
			return &previous[i]
		}
	}
	for i := range previous {
		if a.IBAN != "" && previous[i].IBAN == a.IBAN {
			return &previous[i]
		}
	}
	return nil
}

// closeSession ends the remote session of a connection best effort and
// reports whether it worked. A session equal to keep (a bank that hands out
// the same session on re-authorization) is left open.
func (s *BankService) closeSession(ctx context.Context, userID string, c LedgerBankConnection, keep ...string) bool {
	sessionID, err := cryptoutil.DecryptString(c.EncryptedSessionID, s.key)
	if err != nil {
		s.logger.Warn("decrypting bank session failed", "user_id", userID, "connection_id", c.ID)
		return false
	}
	if slices.Contains(keep, sessionID) {
		return true
	}
	return s.closeSessionID(ctx, userID, sessionID)
}

func (s *BankService) closeSessionID(ctx context.Context, userID, sessionID string) bool {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bankCleanupTimeout)
	defer cancel()
	if err := s.client.DeleteSession(closeCtx, sessionID); err != nil {
		s.logger.Warn("closing bank session failed", "user_id", userID, "upstream", bankErrCode(err))
		return false
	}
	return true
}

// mapByIBAN maps unmapped bank accounts to the ledger account with the same
// IBAN and currency, unless another bank account of the user already uses it.
func (s *BankService) mapByIBAN(ctx context.Context, userID string, conn *LedgerBankConnection) error {
	all, err := s.store.ListLedgerBankConnections(ctx, userID)
	if err != nil {
		return err
	}
	used := map[uuid.UUID]bool{}
	for _, c := range all {
		if c.ID == conn.ID {
			continue
		}
		for _, a := range c.Accounts {
			if a.LedgerAccountID != nil {
				used[*a.LedgerAccountID] = true
			}
		}
	}
	for _, a := range conn.Accounts {
		if a.LedgerAccountID != nil {
			used[*a.LedgerAccountID] = true
		}
	}
	for i := range conn.Accounts {
		a := &conn.Accounts[i]
		if a.LedgerAccountID != nil || a.IBAN == "" {
			continue
		}
		acc, err := s.store.FindLedgerAccountByIBAN(ctx, userID, a.IBAN)
		if errors.Is(err, storage.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if !strings.EqualFold(acc.Currency, a.Currency) || used[acc.ID] {
			continue
		}
		id := acc.ID
		a.LedgerAccountID = &id
		used[id] = true
	}
	return nil
}

// Connections lists the user's connections as views, mapping accounts whose
// IBAN has a ledger account since the last look.
func (s *BankService) Connections(ctx context.Context, userID string) ([]BankConnectionView, error) {
	if !s.active() {
		return nil, ErrBankImportDisabled
	}
	conns, err := s.store.ListLedgerBankConnections(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	views := make([]BankConnectionView, 0, len(conns))
	for _, c := range conns {
		c, err = s.resolveMappings(ctx, userID, c)
		if err != nil {
			return nil, err
		}
		views = append(views, NewBankConnectionView(c, now))
	}
	return views, nil
}

// resolveMappings persists IBAN-based mappings found for unmapped accounts.
func (s *BankService) resolveMappings(ctx context.Context, userID string, c LedgerBankConnection) (LedgerBankConnection, error) {
	probe := c
	probe.Accounts = append([]LedgerBankAccount(nil), c.Accounts...)
	if err := s.mapByIBAN(ctx, userID, &probe); err != nil {
		return c, err
	}
	mapped := map[string]uuid.UUID{}
	for i, a := range probe.Accounts {
		if c.Accounts[i].LedgerAccountID == nil && a.LedgerAccountID != nil {
			mapped[a.UID] = *a.LedgerAccountID
		}
	}
	if len(mapped) == 0 {
		return c, nil
	}
	return s.store.ModifyLedgerBankConnection(ctx, userID, c.ID, func(stored *LedgerBankConnection) {
		for i := range stored.Accounts {
			if id, ok := mapped[stored.Accounts[i].UID]; ok && stored.Accounts[i].LedgerAccountID == nil {
				stored.Accounts[i].LedgerAccountID = &id
			}
		}
	})
}

// Disconnect closes the remote session best effort and deletes the connection
// regardless; remoteClosed tells whether the bank confirmed.
func (s *BankService) Disconnect(ctx context.Context, userID string, id uuid.UUID) (bool, error) {
	if !s.active() {
		return false, ErrBankImportDisabled
	}
	conn, err := s.store.GetLedgerBankConnection(ctx, userID, id)
	if err != nil {
		return false, err
	}
	remoteClosed := s.closeSession(ctx, userID, conn)
	if err := s.store.DeleteLedgerBankConnection(ctx, userID, id); err != nil {
		return remoteClosed, err
	}
	return remoteClosed, nil
}

// MapAccount maps a bank account to an existing ledger account.
func (s *BankService) MapAccount(ctx context.Context, userID string, id uuid.UUID, uid string, ledgerAccountID uuid.UUID) (BankConnectionView, error) {
	if !s.active() {
		return BankConnectionView{}, ErrBankImportDisabled
	}
	conn, err := s.store.GetLedgerBankConnection(ctx, userID, id)
	if err != nil {
		return BankConnectionView{}, err
	}
	idx := -1
	for i, a := range conn.Accounts {
		if a.UID == uid {
			idx = i
		}
	}
	if idx < 0 {
		return BankConnectionView{}, storage.ErrNotFound
	}
	bankAccount := conn.Accounts[idx]

	ledgerAccount, err := s.store.GetLedgerAccount(ctx, userID, ledgerAccountID)
	if errors.Is(err, storage.ErrNotFound) {
		return BankConnectionView{}, ErrBankInvalidMapping
	}
	if err != nil {
		return BankConnectionView{}, err
	}
	if !strings.EqualFold(ledgerAccount.Currency, bankAccount.Currency) {
		return BankConnectionView{}, ErrBankCurrencyMismatch
	}
	if ledgerAccount.IBAN != "" && normalizeIBAN(ledgerAccount.IBAN) != bankAccount.IBAN {
		return BankConnectionView{}, ErrBankInvalidMapping
	}

	all, err := s.store.ListLedgerBankConnections(ctx, userID)
	if err != nil {
		return BankConnectionView{}, err
	}
	for _, c := range all {
		for _, a := range c.Accounts {
			if a.LedgerAccountID != nil && *a.LedgerAccountID == ledgerAccountID && (c.ID != id || a.UID != uid) {
				return BankConnectionView{}, ErrBankInvalidMapping
			}
		}
	}

	updated, err := s.store.ModifyLedgerBankConnection(ctx, userID, id, func(c *LedgerBankConnection) {
		for i := range c.Accounts {
			if c.Accounts[i].UID == uid {
				c.Accounts[i].LedgerAccountID = &ledgerAccountID
			}
		}
	})
	if err != nil {
		return BankConnectionView{}, err
	}
	return NewBankConnectionView(updated, s.now()), nil
}
