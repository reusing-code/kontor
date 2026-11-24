package ledger

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/storage"
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
		RedirectURL:     s.RedirectURL(),
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
			BalancesRequested:  true,
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
				account.Unmapped = p.Unmapped
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
