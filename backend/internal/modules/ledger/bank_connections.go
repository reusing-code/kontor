package ledger

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

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
		if a.LedgerAccountID != nil || a.Unmapped || a.IBAN == "" {
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
			a := &c.Accounts[i]
			if a.UID != uid {
				continue
			}
			if a.LedgerAccountID == nil || *a.LedgerAccountID != ledgerAccountID {
				// the sync state belongs to the old target; the next sync runs at once
				resetBankAccountSyncState(a)
			}
			a.LedgerAccountID = &ledgerAccountID
			a.Unmapped = false
		}
	})
	if err != nil {
		return BankConnectionView{}, err
	}
	return NewBankConnectionView(updated, s.now()), nil
}

// UnmapAccount removes the mapping of a bank account. Transactions it already
// synced stay in their ledger account; the bank account is not synced and not
// mapped by IBAN again until it is mapped by hand.
func (s *BankService) UnmapAccount(ctx context.Context, userID string, id uuid.UUID, uid string) (BankConnectionView, error) {
	if !s.active() {
		return BankConnectionView{}, ErrBankImportDisabled
	}
	found := false
	updated, err := s.store.ModifyLedgerBankConnection(ctx, userID, id, func(c *LedgerBankConnection) {
		for i := range c.Accounts {
			if a := &c.Accounts[i]; a.UID == uid {
				found = true
				a.LedgerAccountID = nil
				a.Unmapped = true
				resetBankAccountSyncState(a)
			}
		}
	})
	if err != nil {
		return BankConnectionView{}, err
	}
	if !found {
		return BankConnectionView{}, storage.ErrNotFound
	}
	return NewBankConnectionView(updated, s.now()), nil
}
