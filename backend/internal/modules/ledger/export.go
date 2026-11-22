package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/module"
)

const sectionSchemaVersion = 0

type exportSection struct {
	SchemaVersion uint64               `json:"schemaVersion"`
	Accounts      []LedgerAccount      `json:"accounts"`
	Categories    []LedgerCategory     `json:"categories"`
	Imports       []LedgerImportBatch  `json:"imports"`
	Transactions  []LedgerTransaction  `json:"transactions"`
	EmailAccounts []LedgerEmailAccount `json:"emailAccounts"`
	EmailOrders   []LedgerEmailOrder   `json:"emailOrders"`
	// Balances is optional: exports from before balances existed have none.
	Balances []LedgerBalanceSnapshot `json:"balances,omitempty"`
}

func (m *Module) IsEmpty(ctx context.Context, userID string) (bool, error) {
	accounts, err := m.store.ListLedgerAccounts(ctx, userID)
	if err != nil {
		return false, err
	}
	imports, err := m.store.ListLedgerImports(ctx, userID)
	if err != nil {
		return false, err
	}
	emailAccounts, err := m.store.ListLedgerEmailAccounts(ctx, userID)
	if err != nil {
		return false, err
	}
	emailOrders, err := m.store.ListLedgerEmailOrders(ctx, userID)
	if err != nil {
		return false, err
	}
	return len(accounts) == 0 && len(imports) == 0 && len(emailAccounts) == 0 && len(emailOrders) == 0, nil
}

func (m *Module) ExportSection(ctx context.Context, userID string) (json.RawMessage, error) {
	section := exportSection{SchemaVersion: sectionSchemaVersion, Transactions: []LedgerTransaction{}}
	var err error
	if section.Accounts, err = m.store.ListLedgerAccounts(ctx, userID); err != nil {
		return nil, err
	}
	if section.Categories, err = m.store.ListLedgerCategories(ctx, userID); err != nil {
		return nil, err
	}
	if section.Imports, err = m.store.ListLedgerImports(ctx, userID); err != nil {
		return nil, err
	}
	for _, account := range section.Accounts {
		txns, err := m.store.ListLedgerTransactions(ctx, userID, account.ID)
		if err != nil {
			return nil, err
		}
		section.Transactions = append(section.Transactions, txns...)
	}
	if section.EmailAccounts, err = m.store.ListLedgerEmailAccounts(ctx, userID); err != nil {
		return nil, err
	}
	if section.EmailOrders, err = m.store.ListLedgerEmailOrders(ctx, userID); err != nil {
		return nil, err
	}
	if section.Balances, err = m.store.ListAllLedgerBalanceSnapshots(ctx, userID); err != nil {
		return nil, err
	}
	return json.Marshal(section)
}

func (m *Module) ImportSection(ctx context.Context, userID string, data json.RawMessage, res *module.ImportResult) error {
	var section exportSection
	if err := json.Unmarshal(data, &section); err != nil {
		return fmt.Errorf("%w: invalid ledger section: %v", module.ErrInvalidSection, err)
	}
	if section.SchemaVersion != sectionSchemaVersion {
		return fmt.Errorf("%w: ledger section has schema version %d, this version supports %d", module.ErrInvalidSection, section.SchemaVersion, sectionSchemaVersion)
	}

	if err := m.replaceCategories(ctx, userID, section.Categories, res); err != nil {
		return err
	}

	for _, account := range section.Accounts {
		if err := m.store.CreateLedgerAccount(ctx, userID, account); err != nil {
			return err
		}
	}
	res.Add("ledgerAccounts", len(section.Accounts))
	if err := m.restoreBalances(ctx, userID, section, res); err != nil {
		return err
	}

	// References to items in modules that were not imported are stripped so
	// the transaction data stays internally consistent.
	stripped := 0
	if err := m.store.e.View(func(txn *badger.Txn) error {
		for i := range section.Transactions {
			refs := section.Transactions[i].References
			kept := refs[:0:0]
			for _, ref := range refs {
				target, err := m.store.links.Target(ref.Type)
				if err != nil || !target.Exists(txn, userID, ref.TargetID) {
					stripped++
					continue
				}
				kept = append(kept, ref)
			}
			section.Transactions[i].References = kept
		}
		return nil
	}); err != nil {
		return err
	}
	if stripped > 0 {
		res.Warnf("ledger: removed %d reference(s) to items that were not imported", stripped)
	}
	if dropped := sanitizeImportedSplits(section.Transactions, section.Categories); dropped > 0 {
		res.Warnf("ledger: removed the split of %d transaction(s) with invalid parts", dropped)
	}

	if err := m.restoreTransactions(ctx, userID, section, res); err != nil {
		return err
	}

	// Re-establish the target side of kept references so linked items list
	// their transactions again.
	if err := m.store.e.Update(func(txn *badger.Txn) error {
		for _, t := range section.Transactions {
			for _, ref := range t.References {
				target, err := m.store.links.Target(ref.Type)
				if err != nil {
					continue
				}
				if err := target.AddLink(txn, userID, ref.TargetID, t.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	for _, account := range section.EmailAccounts {
		if err := m.store.CreateLedgerEmailAccount(ctx, userID, account); err != nil {
			return err
		}
	}
	res.Add("ledgerEmailAccounts", len(section.EmailAccounts))
	if len(section.EmailAccounts) > 0 {
		res.Warnf("email account passwords are not part of exports and must be re-entered")
	}
	for _, order := range section.EmailOrders {
		if err := m.store.CreateLedgerEmailOrder(ctx, userID, order); err != nil {
			return err
		}
	}
	res.Add("ledgerEmailOrders", len(section.EmailOrders))
	return nil
}

// sanitizeImportedSplits drops the parts of a transaction when they are not a
// valid split (a part without category is kept: it is what deleting a
// category leaves) or name a category that is not imported; it returns how
// many transactions lost their split.
func sanitizeImportedSplits(txns []LedgerTransaction, categories []LedgerCategory) int {
	known := make(map[uuid.UUID]bool, len(categories))
	for _, category := range categories {
		known[category.ID] = true
	}
	dropped := 0
	for i := range txns {
		parts := txns[i].Splits
		if len(parts) == 0 {
			continue
		}
		valid := txns[i].TransferPairTransactionID == nil
		check := make([]LedgerTransactionSplit, len(parts))
		for j, part := range parts {
			if part.CategoryID != nil && !known[*part.CategoryID] {
				valid = false
			}
			check[j] = part
			if part.CategoryID == nil {
				placeholder := uuid.Max
				check[j].CategoryID = &placeholder
			}
		}
		if _, err := normalizeLedgerSplits(txns[i].AmountMinor, check); err != nil {
			valid = false
		}
		if !valid {
			txns[i].Splits = nil
			dropped++
		}
	}
	return dropped
}

func (m *Module) PruneDeadLinks(context.Context, string, *module.ImportResult) error {
	// Transaction references are already filtered during ImportSection.
	return nil
}

func (m *Module) replaceCategories(ctx context.Context, userID string, cats []LedgerCategory, res *module.ImportResult) error {
	existing, err := m.store.ListLedgerCategories(ctx, userID)
	if err != nil {
		return err
	}
	// children-first deletion: retry until the tree is gone
	for len(existing) > 0 {
		remaining := make([]LedgerCategory, 0, len(existing))
		for _, category := range existing {
			err := m.store.DeleteLedgerCategory(ctx, userID, category.ID)
			if errors.Is(err, ErrLedgerCategoryHasChild) {
				remaining = append(remaining, category)
				continue
			}
			if err != nil {
				return err
			}
		}
		if len(remaining) == len(existing) {
			return errors.New("could not delete existing ledger categories")
		}
		existing = remaining
	}

	// parents-first creation
	created := make(map[uuid.UUID]bool, len(cats))
	pending := cats
	for len(pending) > 0 {
		remaining := make([]LedgerCategory, 0, len(pending))
		for _, category := range pending {
			if category.ParentID != nil && !created[*category.ParentID] {
				remaining = append(remaining, category)
				continue
			}
			if err := m.store.CreateLedgerCategory(ctx, userID, category); err != nil {
				return err
			}
			created[category.ID] = true
			res.Add("ledgerCategories", 1)
		}
		if len(remaining) == len(pending) {
			return fmt.Errorf("%w: ledger categories contain an unresolvable parent reference", module.ErrInvalidSection)
		}
		pending = remaining
	}
	return nil
}

// restoreBalances stores the snapshots of imported accounts; snapshots of an
// account that is not in the export or that are invalid are skipped.
func (m *Module) restoreBalances(ctx context.Context, userID string, section exportSection, res *module.ImportResult) error {
	accounts := make(map[uuid.UUID]bool, len(section.Accounts))
	for _, account := range section.Accounts {
		accounts[account.ID] = true
	}
	skipped := 0
	for _, snapshot := range section.Balances {
		if !accounts[snapshot.AccountID] || snapshot.validate() != nil {
			skipped++
			continue
		}
		if err := m.store.PutLedgerBalanceSnapshot(ctx, userID, snapshot); err != nil {
			return err
		}
		res.Add("ledgerBalances", 1)
	}
	if skipped > 0 {
		res.Warnf("ledger: skipped %d balance(s) of unknown accounts or with invalid data", skipped)
	}
	return nil
}

func (m *Module) restoreTransactions(ctx context.Context, userID string, section exportSection, res *module.ImportResult) error {
	byBatch := make(map[uuid.UUID][]LedgerTransaction)
	for _, txn := range section.Transactions {
		byBatch[txn.ImportBatchID] = append(byBatch[txn.ImportBatchID], txn)
	}
	for _, batch := range section.Imports {
		commit, err := m.store.CommitLedgerImport(ctx, userID, batch, byBatch[batch.ID])
		if err != nil {
			return err
		}
		res.Add("ledgerImports", 1)
		res.Add("ledgerTransactions", commit.ImportedRows)
		delete(byBatch, batch.ID)
	}
	// transactions whose batch is missing from the export get a synthetic batch
	for batchID, txns := range byBatch {
		if len(txns) == 0 {
			continue
		}
		synthetic := LedgerImportBatch{
			ID:         batchID,
			AccountID:  txns[0].AccountID,
			SourceType: "restore",
			Filename:   "restored-orphan-transactions",
			FileSHA256: fmt.Sprintf("restore-%s", batchID),
			Status:     "committed",
			TotalRows:  len(txns),
		}
		commit, err := m.store.CommitLedgerImport(ctx, userID, synthetic, txns)
		if err != nil {
			return err
		}
		res.Add("ledgerTransactions", commit.ImportedRows)
		res.Warnf("%d transaction(s) referenced missing import batch %s", len(txns), batchID)
	}
	return nil
}
