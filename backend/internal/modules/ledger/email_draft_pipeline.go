package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/extraction"
)

const (
	operationEmailDraft  = "ledger.email_draft"
	maxEmailDraftsPerRun = 10
	emailDraftRunTimeout = 15 * time.Minute
	// emailDraftRejectPrefix keeps the refusal counters of the drafts folder
	// apart from those of the order folder: UIDs are per folder.
	emailDraftRejectPrefix = "drafts/"
)

var (
	ErrEmailDraftsBusy        = errors.New("the drafts folders are already being checked")
	ErrEmailDraftsUnavailable = errors.New("email drafts are not available")
	ErrEmailDraftsNoTargets   = extraction.ErrNoTargets

	// errEmailDraftsStop ends a pass early: the LLM is unavailable.
	errEmailDraftsStop = errors.New("stop the pass")
)

// EmailDraftRunResult counts one pass over a user's drafts folders. Failed
// messages are retried on the next pass. Warnings name accounts and carry
// fixed texts, never email content.
type EmailDraftRunResult struct {
	Found    int      `json:"found"`
	Created  int      `json:"created"`
	Skipped  int      `json:"skipped"`
	Failed   int      `json:"failed"`
	Warnings []string `json:"warnings"`
}

// EmailDraftPipeline turns the messages of each account's drafts folder into
// drafts of the draft inbox, classified by the shared classifier.
type EmailDraftPipeline struct {
	store         *Store
	drafts        *drafts.Store
	classifier    *extraction.Classifier
	core          *core.Store
	encryptionKey []byte
	interval      time.Duration
	logger        *slog.Logger
	now           func() time.Time

	mu      sync.Mutex
	running map[string]bool
}

func NewEmailDraftPipeline(store *Store, draftStore *drafts.Store, classifier *extraction.Classifier, coreStore *core.Store,
	encryptionKey []byte, interval time.Duration, logger *slog.Logger) *EmailDraftPipeline {
	return &EmailDraftPipeline{
		store:         store,
		drafts:        draftStore,
		classifier:    classifier,
		core:          coreStore,
		encryptionKey: encryptionKey,
		interval:      interval,
		logger:        logger.With("component", "ledger-email-drafts"),
		now:           time.Now,
		running:       map[string]bool{},
	}
}

func (p *EmailDraftPipeline) Start(ctx context.Context) {
	switch {
	case p.interval <= 0:
		p.logger.Info("email draft pipeline disabled", "reason", "LEDGER_EMAIL_SCAN_INTERVAL is 0")
		return
	case len(p.encryptionKey) == 0:
		p.logger.Info("email draft pipeline disabled", "reason", "no valid encryption key")
		return
	case !p.classifier.Enabled():
		p.logger.Info("email draft pipeline disabled", "reason", "LLM is not configured")
		return
	}
	go func() {
		p.logger.Info("email draft pipeline started", "interval", p.interval)
		p.runAll(ctx)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				p.logger.Info("email draft pipeline stopped")
				return
			case <-ticker.C:
				p.runAll(ctx)
			}
		}
	}()
}

func (p *EmailDraftPipeline) runAll(ctx context.Context) {
	users, err := p.core.ListUsers(ctx)
	if err != nil {
		p.logger.Error("listing users for the email draft pipeline", "error", err)
		return
	}
	for _, u := range users {
		if ctx.Err() != nil {
			return
		}
		userID := u.ID.String()
		enabled, err := p.core.ModuleEnabled(ctx, userID, ModuleID)
		if err != nil {
			p.logger.Error("checking module enablement", "userID", userID, "error", err)
			continue
		}
		if !enabled {
			continue
		}
		userCtx, cancel := context.WithTimeout(ctx, emailDraftRunTimeout)
		res, err := p.RunUser(userCtx, userID)
		cancel()
		switch {
		case errors.Is(err, ErrEmailDraftsNoTargets), errors.Is(err, ErrEmailDraftsBusy), errors.Is(err, ErrEmailLLMDisabled):
		case err != nil:
			p.logger.Error("email draft pipeline run failed", "userID", userID, "error", err)
		case res.Created > 0 || res.Failed > 0:
			p.logger.Info("email draft pipeline run finished", "userID", userID,
				"found", res.Found, "created", res.Created, "skipped", res.Skipped, "failed", res.Failed)
		}
	}
}

func (p *EmailDraftPipeline) lock(userID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running[userID] {
		return false
	}
	p.running[userID] = true
	return true
}

func (p *EmailDraftPipeline) unlock(userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.running, userID)
}

// emailDraftRef is the draft source ref of a message: account and dedup key.
func emailDraftRef(accountID uuid.UUID, key string) string {
	return accountID.String() + ":" + key
}

func parseEmailDraftRef(ref string) (uuid.UUID, string, bool) {
	id, key, ok := strings.Cut(ref, ":")
	accountID, err := uuid.Parse(id)
	if !ok || err != nil || key == "" {
		return uuid.UUID{}, "", false
	}
	return accountID, key, true
}

// emailDraftRun is the state of one pass over a user's accounts.
type emailDraftRun struct {
	userID  string
	targets []string
	catalog *extraction.Catalog
	budget  int
	res     EmailDraftRunResult
}

func (r *emailDraftRun) warn(account LedgerEmailAccount, reason string) {
	r.res.Warnings = append(r.res.Warnings, fmt.Sprintf("%s: %s", account.Name, reason))
}

// RunUser processes up to maxEmailDraftsPerRun new messages of the drafts
// folders of the user's accounts. A transient LLM failure ends the pass
// early; the messages are picked up again next time.
func (p *EmailDraftPipeline) RunUser(ctx context.Context, userID string) (EmailDraftRunResult, error) {
	run := &emailDraftRun{userID: userID, budget: maxEmailDraftsPerRun, res: EmailDraftRunResult{Warnings: []string{}}}
	if !p.classifier.Enabled() {
		return run.res, ErrEmailLLMDisabled
	}
	if !p.lock(userID) {
		return run.res, ErrEmailDraftsBusy
	}
	defer p.unlock(userID)

	var err error
	if run.targets, err = p.classifier.EnabledTargets(ctx, userID); err != nil {
		return run.res, err
	}
	if len(run.targets) == 0 {
		return run.res, ErrEmailDraftsNoTargets
	}
	accounts, err := p.store.ListLedgerEmailAccounts(ctx, userID)
	if err != nil {
		return run.res, err
	}
	for _, account := range accounts {
		if account.DraftsFolder == "" {
			continue
		}
		if ctx.Err() != nil {
			return run.res, ctx.Err()
		}
		err := p.runAccount(ctx, run, account)
		switch {
		case errors.Is(err, errEmailDraftsStop):
			return run.res, nil
		case errors.Is(err, ErrIMAP):
			p.logger.Warn("email draft pipeline: mailbox failed", "userID", userID, "accountID", account.ID)
			run.warn(account, "could not read the drafts folder")
		case err != nil:
			return run.res, err
		}
	}
	return run.res, nil
}

func (p *EmailDraftPipeline) runAccount(ctx context.Context, run *emailDraftRun, account LedgerEmailAccount) error {
	if account.EncryptedPassword == "" {
		run.warn(account, "no stored password")
		return nil
	}
	password, err := cryptoutil.DecryptString(account.EncryptedPassword, p.encryptionKey)
	if err != nil {
		p.logger.Error("decrypting email account password", "userID", run.userID, "accountID", account.ID, "error", err)
		run.warn(account, "could not decrypt the stored password")
		return nil
	}
	client, err := openMailbox(account, password, account.DraftsFolder)
	if err != nil {
		return err
	}
	defer client.Close()
	defer client.Logout()
	uids, err := client.SearchSince(account.ScanSince)
	if err != nil {
		return imapError(err)
	}
	if len(uids) > maxFetchedMessages {
		run.warn(account, fmt.Sprintf("only the newest %d messages of the drafts folder are checked", maxFetchedMessages))
		uids = uids[len(uids)-maxFetchedMessages:]
	}
	msgs, err := listMessages(ctx, client, uids, headerListing{
		ignored: func(uidKey string) (bool, error) {
			return p.store.HasLedgerEmailDraftSeen(ctx, run.userID, account.ID, uidKey)
		},
		rejected: func(uidKey, _ string) error {
			run.res.Failed++
			return p.countRejection(ctx, run.userID, account, uidKey)
		},
		warn:    func(string, string) { run.res.Failed++ },
		fetched: func() { run.res.Found++ },
	})
	if err != nil {
		return err
	}
	queued := make(map[string]bool, len(msgs))
	for _, m := range msgs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if queued[m.Key] {
			run.res.Skipped++
			continue
		}
		queued[m.Key] = true
		known, err := p.known(ctx, run.userID, account, m.Key)
		if err != nil {
			return err
		}
		if known {
			run.res.Skipped++
			continue
		}
		if run.budget == 0 {
			return nil
		}
		run.budget--
		if err := p.processMessage(ctx, run, account, m); err != nil {
			return err
		}
	}
	return nil
}

// known reports a message that was decided or ignored, or has a pending draft.
func (p *EmailDraftPipeline) known(ctx context.Context, userID string, account LedgerEmailAccount, key string) (bool, error) {
	seen, err := p.store.HasLedgerEmailDraftSeen(ctx, userID, account.ID, key)
	if err != nil || seen {
		return seen, err
	}
	return p.drafts.HasSource(ctx, userID, drafts.SourceEmail, emailDraftRef(account.ID, key))
}

// countRejection counts a fetch the server refused; after maxFetchRejections
// the message is ignored.
func (p *EmailDraftPipeline) countRejection(ctx context.Context, userID string, account LedgerEmailAccount, key string) error {
	count, err := p.store.RecordLedgerEmailRejection(ctx, userID, account.ID, emailDraftRejectPrefix+key)
	if err != nil || count < maxFetchRejections {
		return err
	}
	return p.store.MarkLedgerEmailDraftSeen(ctx, userID, account.ID, EmailSeenUnreadable, key)
}

// processMessage creates one draft. errEmailDraftsStop reports a transient
// LLM failure, an ErrIMAP an unusable connection.
func (p *EmailDraftPipeline) processMessage(ctx context.Context, run *emailDraftRun, account LedgerEmailAccount, m emailMessage) error {
	parsed, err := m.Load()
	switch {
	case errors.Is(err, errMessageTooLarge):
		run.res.Failed++
		return p.store.MarkLedgerEmailDraftSeen(ctx, run.userID, account.ID, EmailSeenTooLarge, m.Key)
	case errors.Is(err, errMessageParse):
		run.res.Failed++
		return p.store.MarkLedgerEmailDraftSeen(ctx, run.userID, account.ID, EmailSeenUnreadable, m.Key)
	case errors.Is(err, errIMAPRejected):
		run.res.Failed++
		return p.countRejection(ctx, run.userID, account, m.Key)
	case err != nil:
		return imapError(err)
	}
	if run.catalog == nil {
		loaded, err := p.classifier.LoadCatalog(ctx, run.userID, run.targets)
		if err != nil {
			return err
		}
		run.catalog = &loaded
	}
	title := truncateRunes(collapseSpaces(parsed.Subject), maxEmailHeaderRunes)
	prop, err := p.classifier.Classify(ctx, operationEmailDraft, *run.catalog,
		extraction.Input{Kind: extraction.InputEmail, Title: title, Text: buildEmailInput(parsed)}, p.now())
	if err != nil {
		class := "request"
		if extraction.IsTimeout(err) {
			class = "timeout"
		}
		p.logger.Warn("classifying email", "userID", run.userID, "accountID", account.ID, "message", m.Label, "class", class)
		run.res.Failed++
		return errEmailDraftsStop
	}

	from := truncateRunes(collapseSpaces(parsed.From), maxEmailHeaderRunes)
	if title == "" {
		title = from
	}
	msg := ledgerEmailDraftMessage{Subject: truncateRunes(collapseSpaces(parsed.Subject), maxEmailHeaderRunes), From: from}
	// a hashed dedup key means the Message-ID is missing or unusable in a link
	if id := strings.TrimSpace(parsed.MessageID); id != "" && id == m.Key {
		msg.MessageID = id
	}
	if !parsed.Date.IsZero() {
		msg.Date = &parsed.Date
	}
	if m.Mailbox != nil {
		msg.emailMailboxRef = *m.Mailbox
	}
	if err := p.store.putLedgerEmailDraftMessage(run.userID, account.ID, m.Key, msg); err != nil {
		return err
	}
	source := drafts.Source{Kind: drafts.SourceEmail, Ref: emailDraftRef(account.ID, m.Key), Title: title}
	d, err := drafts.New(source, prop.Target, prop.ParentID, prop.Input, prop.Note)
	if err == nil {
		err = p.drafts.Create(ctx, run.userID, d)
	}
	switch {
	case errors.Is(err, drafts.ErrDuplicateSource):
		run.res.Skipped++
	case err != nil:
		p.logger.Error("storing email draft", "userID", run.userID, "accountID", account.ID, "message", m.Label, "error", err)
		run.res.Failed++
		if err := p.store.deleteLedgerEmailDraftMessage(run.userID, account.ID, m.Key); err != nil {
			return err
		}
	default:
		run.res.Created++
	}
	return nil
}
