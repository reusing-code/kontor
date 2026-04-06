package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
)

var (
	ErrLedgerPreviewExpired   = errors.New("ledger preview expired")
	ErrLedgerFileImported     = errors.New("ledger file already imported")
	ErrLedgerCategoryHasChild = errors.New("ledger category has children")
	ErrLedgerCategoryHasCycle = errors.New("ledger category cycle")
)

// Ledger account key helpers

func ledAccKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/acc/%s", userID, id))
}

func ledAccPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/led/acc/", userID))
}

func idxLedAccIBANKey(userID string, iban string) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_acc_iban/%s", userID, iban))
}

// Ledger category key helpers

func ledCatKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/cat/%s", userID, id))
}

func ledCatPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/led/cat/", userID))
}

// Ledger transaction key helpers

func ledTxnKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/txn/%s", userID, id))
}

func ledTxnPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/led/txn/", userID))
}

func idxLedAccTxnKey(userID string, accountID uuid.UUID, bookingDate string, txnID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_acc_txn/%s/%s/%s", userID, accountID, bookingDate, txnID))
}

func idxLedAccTxnPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_acc_txn/%s/", userID, accountID))
}

func idxLedTxnFPKey(userID string, fingerprint string) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_txn_fp/%s", userID, fingerprint))
}

func idxLedImpTxnKey(userID string, batchID uuid.UUID, txnID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_imp_txn/%s/%s", userID, batchID, txnID))
}

// Ledger import batch key helpers

func ledImpKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/imp/%s", userID, id))
}

func ledImpPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/led/imp/", userID))
}

func idxLedFileHashKey(userID string, sha256 string) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_file_hash/%s", userID, sha256))
}

func normalizeLedgerTransaction(t model.LedgerTransaction) model.LedgerTransaction {
	if t.ReviewStatus == "" {
		t.ReviewStatus = model.LedgerTransactionReviewNeedsReview
	}
	if t.CategorizationSource == "" {
		t.CategorizationSource = model.LedgerCategorizationNone
	}
	return t
}

func normalizeLedgerCategory(c model.LedgerCategory) model.LedgerCategory {
	c.MatchWords = model.NormalizeLedgerMatchWords(c.MatchWords)
	if c.MatchWords == nil {
		c.MatchWords = []string{}
	}
	return c
}

func loadLedgerCategory(txn *badger.Txn, userID string, id uuid.UUID) (model.LedgerCategory, error) {
	var cat model.LedgerCategory
	item, err := txn.Get(ledCatKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return cat, ErrNotFound
		}
		return cat, err
	}
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &cat)
	})
	if err != nil {
		return cat, err
	}
	return normalizeLedgerCategory(cat), nil
}

func storeLedgerCategory(txn *badger.Txn, userID string, cat model.LedgerCategory) error {
	cat = normalizeLedgerCategory(cat)
	data, err := json.Marshal(cat)
	if err != nil {
		return err
	}
	return txn.Set(ledCatKey(userID, cat.ID), data)
}

func loadLedgerTransaction(txn *badger.Txn, userID string, id uuid.UUID) (model.LedgerTransaction, error) {
	var t model.LedgerTransaction
	item, err := txn.Get(ledTxnKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return t, ErrNotFound
		}
		return t, err
	}
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &t)
	})
	if err != nil {
		return t, err
	}
	return normalizeLedgerTransaction(t), nil
}

func storeLedgerTransaction(txn *badger.Txn, userID string, t model.LedgerTransaction) error {
	t = normalizeLedgerTransaction(t)
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return txn.Set(ledTxnKey(userID, t.ID), data)
}

func collectLedgerCategories(txn *badger.Txn, userID string) ([]model.LedgerCategory, error) {
	var categories []model.LedgerCategory
	prefix := ledCatPrefix(userID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var cat model.LedgerCategory
		if err := it.Item().Value(func(val []byte) error {
			return json.Unmarshal(val, &cat)
		}); err != nil {
			return nil, err
		}
		categories = append(categories, normalizeLedgerCategory(cat))
	}
	if categories == nil {
		categories = []model.LedgerCategory{}
	}
	return categories, nil
}

func collectLedgerTransactions(txn *badger.Txn, userID string) ([]model.LedgerTransaction, error) {
	var txns []model.LedgerTransaction
	prefix := ledTxnPrefix(userID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var t model.LedgerTransaction
		if err := it.Item().Value(func(val []byte) error {
			return json.Unmarshal(val, &t)
		}); err != nil {
			return nil, err
		}
		txns = append(txns, normalizeLedgerTransaction(t))
	}
	if txns == nil {
		txns = []model.LedgerTransaction{}
	}
	return txns, nil
}

func ensureLedgerCategoryParentValid(txn *badger.Txn, userID string, categoryID uuid.UUID, parentID *uuid.UUID) error {
	if parentID == nil {
		return nil
	}
	if *parentID == categoryID {
		return ErrLedgerCategoryHasCycle
	}
	current, err := loadLedgerCategory(txn, userID, *parentID)
	if err != nil {
		return err
	}
	for current.ParentID != nil {
		if *current.ParentID == categoryID {
			return ErrLedgerCategoryHasCycle
		}
		next, err := loadLedgerCategory(txn, userID, *current.ParentID)
		if err != nil {
			return err
		}
		current = next
	}
	return nil
}

func sortLedgerCategories(categories []model.LedgerCategory) {
	sort.Slice(categories, func(i, j int) bool {
		leftParent := ""
		rightParent := ""
		if categories[i].ParentID != nil {
			leftParent = categories[i].ParentID.String()
		}
		if categories[j].ParentID != nil {
			rightParent = categories[j].ParentID.String()
		}
		if leftParent != rightParent {
			return leftParent < rightParent
		}
		leftName := strings.ToLower(categories[i].Name)
		rightName := strings.ToLower(categories[j].Name)
		if leftName != rightName {
			return leftName < rightName
		}
		return categories[i].CreatedAt.Before(categories[j].CreatedAt)
	})
}

func sortLedgerTransactions(txns []model.LedgerTransaction) {
	sort.Slice(txns, func(i, j int) bool {
		if txns[i].BookingDate == txns[j].BookingDate {
			return txns[i].ID.String() > txns[j].ID.String()
		}
		return txns[i].BookingDate > txns[j].BookingDate
	})
}

func paginateLedgerTransactions(txns []model.LedgerTransaction, limit int, cursor string) LedgerTransactionPage {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	page := LedgerTransactionPage{Items: []model.LedgerTransaction{}}
	start := 0
	if cursor != "" {
		for i, txn := range txns {
			if txn.ID.String() == cursor {
				start = i + 1
				break
			}
		}
	}
	if start > len(txns) {
		start = len(txns)
	}
	end := start + limit
	if end > len(txns) {
		end = len(txns)
	}
	page.Items = txns[start:end]
	if end < len(txns) && end > start {
		page.NextCursor = txns[end-1].ID.String()
	}
	return page
}

// Ledger Accounts

func (s *BadgerStore) ListLedgerAccounts(_ context.Context, userID string) ([]model.LedgerAccount, error) {
	var accounts []model.LedgerAccount
	prefix := ledAccPrefix(userID)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var a model.LedgerAccount
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &a)
			}); err != nil {
				return err
			}
			accounts = append(accounts, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if accounts == nil {
		accounts = []model.LedgerAccount{}
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Name == accounts[j].Name {
			return accounts[i].CreatedAt.Before(accounts[j].CreatedAt)
		}
		return strings.ToLower(accounts[i].Name) < strings.ToLower(accounts[j].Name)
	})
	return accounts, nil
}

func (s *BadgerStore) GetLedgerAccount(_ context.Context, userID string, id uuid.UUID) (model.LedgerAccount, error) {
	var a model.LedgerAccount
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(ledAccKey(userID, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &a)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *BadgerStore) FindLedgerAccountByIBAN(_ context.Context, userID string, iban string) (model.LedgerAccount, error) {
	var a model.LedgerAccount
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idxLedAccIBANKey(userID, iban))
		if err != nil {
			return err
		}
		var idStr string
		if err := item.Value(func(val []byte) error {
			idStr = string(val)
			return nil
		}); err != nil {
			return err
		}
		accID, err := uuid.Parse(idStr)
		if err != nil {
			return err
		}
		accItem, err := txn.Get(ledAccKey(userID, accID))
		if err != nil {
			return err
		}
		return accItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &a)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *BadgerStore) CreateLedgerAccount(_ context.Context, userID string, a model.LedgerAccount) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if a.IBAN != "" {
			if _, err := txn.Get(idxLedAccIBANKey(userID, a.IBAN)); err == nil {
				return ErrConflict
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		if err := txn.Set(ledAccKey(userID, a.ID), data); err != nil {
			return err
		}
		if a.IBAN != "" {
			if err := txn.Set(idxLedAccIBANKey(userID, a.IBAN), []byte(a.ID.String())); err != nil {
				return err
			}
		}
		return nil
	})
}

// Ledger Categories

func (s *BadgerStore) ListLedgerCategories(_ context.Context, userID string) ([]model.LedgerCategory, error) {
	var categories []model.LedgerCategory
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		categories, err = collectLedgerCategories(txn, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	sortLedgerCategories(categories)
	return categories, nil
}

func (s *BadgerStore) GetLedgerCategory(_ context.Context, userID string, id uuid.UUID) (model.LedgerCategory, error) {
	var cat model.LedgerCategory
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		cat, err = loadLedgerCategory(txn, userID, id)
		return err
	})
	return cat, err
}

func (s *BadgerStore) CreateLedgerCategory(_ context.Context, userID string, c model.LedgerCategory) error {
	c = normalizeLedgerCategory(c)
	return s.db.Update(func(txn *badger.Txn) error {
		if err := ensureLedgerCategoryParentValid(txn, userID, c.ID, c.ParentID); err != nil {
			return err
		}
		return storeLedgerCategory(txn, userID, c)
	})
}

func (s *BadgerStore) UpdateLedgerCategory(_ context.Context, userID string, c model.LedgerCategory) error {
	c = normalizeLedgerCategory(c)
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := loadLedgerCategory(txn, userID, c.ID); err != nil {
			return err
		}
		if err := ensureLedgerCategoryParentValid(txn, userID, c.ID, c.ParentID); err != nil {
			return err
		}
		return storeLedgerCategory(txn, userID, c)
	})
}

func (s *BadgerStore) DeleteLedgerCategory(_ context.Context, userID string, id uuid.UUID) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := loadLedgerCategory(txn, userID, id); err != nil {
			return err
		}

		categories, err := collectLedgerCategories(txn, userID)
		if err != nil {
			return err
		}
		for _, category := range categories {
			if category.ParentID != nil && *category.ParentID == id {
				return ErrLedgerCategoryHasChild
			}
		}

		transactions, err := collectLedgerTransactions(txn, userID)
		if err != nil {
			return err
		}
		for _, ledgerTxn := range transactions {
			if ledgerTxn.CategoryID == nil || *ledgerTxn.CategoryID != id {
				continue
			}
			ledgerTxn.CategoryID = nil
			ledgerTxn.ReviewStatus = model.LedgerTransactionReviewNeedsReview
			ledgerTxn.CategorizationSource = model.LedgerCategorizationNone
			ledgerTxn.UpdatedAt = time.Now().UTC()
			if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
				return err
			}
		}

		return txn.Delete(ledCatKey(userID, id))
	})
}

// Ledger Import Batches

func (s *BadgerStore) GetLedgerImportByFileHash(_ context.Context, userID string, sha256 string) (model.LedgerImportBatch, error) {
	var batch model.LedgerImportBatch
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idxLedFileHashKey(userID, sha256))
		if err != nil {
			return err
		}
		var idStr string
		if err := item.Value(func(val []byte) error {
			idStr = string(val)
			return nil
		}); err != nil {
			return err
		}
		batchID, err := uuid.Parse(idStr)
		if err != nil {
			return err
		}
		batchItem, err := txn.Get(ledImpKey(userID, batchID))
		if err != nil {
			return err
		}
		return batchItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &batch)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return batch, ErrNotFound
	}
	return batch, err
}

func (s *BadgerStore) LedgerTransactionFingerprintExists(_ context.Context, userID string, fingerprint string) (bool, error) {
	exists := false
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(idxLedTxnFPKey(userID, fingerprint))
		if err == nil {
			exists = true
			return nil
		}
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	})
	return exists, err
}

func (s *BadgerStore) CommitLedgerImport(_ context.Context, userID string, batch model.LedgerImportBatch, txns []model.LedgerTransaction) (LedgerImportCommitResult, error) {
	result := LedgerImportCommitResult{}

	err := s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(idxLedFileHashKey(userID, batch.FileSHA256)); err == nil {
			return ErrLedgerFileImported
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		for i := range txns {
			t := normalizeLedgerTransaction(txns[i])
			if _, err := txn.Get(idxLedTxnFPKey(userID, t.Fingerprint)); err == nil {
				result.DuplicateRows++
				continue
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			if err := storeLedgerTransaction(txn, userID, t); err != nil {
				return err
			}
			if err := txn.Set(idxLedAccTxnKey(userID, t.AccountID, t.BookingDate, t.ID), []byte{}); err != nil {
				return err
			}
			if err := txn.Set(idxLedTxnFPKey(userID, t.Fingerprint), []byte(t.ID.String())); err != nil {
				return err
			}
			if err := txn.Set(idxLedImpTxnKey(userID, batch.ID, t.ID), []byte{}); err != nil {
				return err
			}
			result.ImportedRows++
		}

		batch.ImportedRows = result.ImportedRows
		batch.DuplicateRows = result.DuplicateRows
		batchData, err := json.Marshal(batch)
		if err != nil {
			return err
		}
		if err := txn.Set(ledImpKey(userID, batch.ID), batchData); err != nil {
			return err
		}
		if err := txn.Set(idxLedFileHashKey(userID, batch.FileSHA256), []byte(batch.ID.String())); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return LedgerImportCommitResult{}, err
	}
	return result, nil
}

func (s *BadgerStore) ListLedgerImports(_ context.Context, userID string) ([]model.LedgerImportBatch, error) {
	var batches []model.LedgerImportBatch
	prefix := ledImpPrefix(userID)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var b model.LedgerImportBatch
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &b)
			}); err != nil {
				return err
			}
			batches = append(batches, b)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if batches == nil {
		batches = []model.LedgerImportBatch{}
	}
	sort.Slice(batches, func(i, j int) bool {
		if batches[i].CreatedAt.Equal(batches[j].CreatedAt) {
			return batches[i].ID.String() > batches[j].ID.String()
		}
		return batches[i].CreatedAt.After(batches[j].CreatedAt)
	})
	return batches, nil
}

// Ledger Transactions

func (s *BadgerStore) ListLedgerTransactions(_ context.Context, userID string, accountID uuid.UUID) ([]model.LedgerTransaction, error) {
	page, err := s.ListLedgerTransactionsPage(context.Background(), userID, accountID, 1000, "")
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *BadgerStore) ListLedgerTransactionsPage(_ context.Context, userID string, accountID uuid.UUID, limit int, cursor string) (LedgerTransactionPage, error) {
	return s.ListLedgerTransactionsFiltered(context.Background(), userID, LedgerTransactionListOptions{
		AccountID: &accountID,
		Limit:     limit,
		Cursor:    cursor,
	})
}

func (s *BadgerStore) ListLedgerTransactionsFiltered(_ context.Context, userID string, options LedgerTransactionListOptions) (LedgerTransactionPage, error) {
	page := LedgerTransactionPage{Items: []model.LedgerTransaction{}}
	var filtered []model.LedgerTransaction
	err := s.db.View(func(txn *badger.Txn) error {
		all, err := collectLedgerTransactions(txn, userID)
		if err != nil {
			return err
		}
		for _, ledgerTxn := range all {
			if options.AccountID != nil && ledgerTxn.AccountID != *options.AccountID {
				continue
			}
			if options.CategoryID != nil {
				if ledgerTxn.CategoryID == nil || *ledgerTxn.CategoryID != *options.CategoryID {
					continue
				}
			}
			if options.ReviewStatus != "" && ledgerTxn.ReviewStatus != options.ReviewStatus {
				continue
			}
			filtered = append(filtered, ledgerTxn)
		}
		return nil
	})
	if err != nil {
		return page, err
	}
	sortLedgerTransactions(filtered)
	return paginateLedgerTransactions(filtered, options.Limit, options.Cursor), nil
}

func (s *BadgerStore) GetLedgerTransaction(_ context.Context, userID string, id uuid.UUID) (model.LedgerTransaction, error) {
	var ledgerTxn model.LedgerTransaction
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		ledgerTxn, err = loadLedgerTransaction(txn, userID, id)
		return err
	})
	return ledgerTxn, err
}

func (s *BadgerStore) ReviewLedgerTransaction(_ context.Context, userID string, id uuid.UUID, input model.LedgerTransactionReviewInput) (LedgerReviewResult, error) {
	result := LedgerReviewResult{}
	now := time.Now().UTC()
	err := s.db.Update(func(txn *badger.Txn) error {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}

		var selectedCategory *model.LedgerCategory
		if input.CategoryID != nil {
			cat, err := loadLedgerCategory(txn, userID, *input.CategoryID)
			if err != nil {
				return err
			}
			selectedCategory = &cat
		}

		if input.NewCategory != nil {
			categoryID := uuid.New()
			newCategory := model.LedgerCategory{
				ID:         categoryID,
				Name:       input.NewCategory.Name,
				ParentID:   input.NewCategory.ParentID,
				MatchWords: input.NewCategory.MatchWords,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := ensureLedgerCategoryParentValid(txn, userID, categoryID, newCategory.ParentID); err != nil {
				return err
			}
			if err := storeLedgerCategory(txn, userID, newCategory); err != nil {
				return err
			}
			selectedCategory = &newCategory
		}

		if selectedCategory == nil && ledgerTxn.CategoryID != nil {
			cat, err := loadLedgerCategory(txn, userID, *ledgerTxn.CategoryID)
			if err == nil {
				selectedCategory = &cat
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
		}

		if len(input.AddMatchWords) > 0 {
			if selectedCategory == nil {
				return ErrNotFound
			}
			selectedCategory.MatchWords = append(selectedCategory.MatchWords, input.AddMatchWords...)
			selectedCategory.MatchWords = model.NormalizeLedgerMatchWords(selectedCategory.MatchWords)
			selectedCategory.UpdatedAt = now
			if err := storeLedgerCategory(txn, userID, *selectedCategory); err != nil {
				return err
			}
		}

		if selectedCategory != nil {
			categoryID := selectedCategory.ID
			ledgerTxn.CategoryID = &categoryID
			ledgerTxn.CategorizationSource = model.LedgerCategorizationManual
		}
		ledgerTxn.ReviewStatus = model.LedgerTransactionReviewConfirmed
		ledgerTxn.UpdatedAt = now
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}

		result.Transaction = ledgerTxn
		if selectedCategory != nil {
			catCopy := *selectedCategory
			result.Category = &catCopy
		}
		return nil
	})
	if err != nil {
		return LedgerReviewResult{}, err
	}
	return result, nil
}
