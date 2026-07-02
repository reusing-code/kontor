package ledger

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
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

var (
	ErrLedgerPreviewExpired   = storage.ErrLedgerPreviewExpired
	ErrLedgerFileImported     = storage.ErrLedgerFileImported
	ErrLedgerCategoryHasChild = storage.ErrLedgerCategoryHasChild
	ErrLedgerCategoryHasCycle = storage.ErrLedgerCategoryHasCycle
	ErrLedgerTransferInvalid  = storage.ErrLedgerTransferInvalid
	ErrLedgerTransferLinked   = storage.ErrLedgerTransferLinked
)

const ledgerTransferMatchWindowDays = 3

type Store struct {
	e     *storage.Engine
	links *link.Registry
}

// NewStore builds the ledger store and registers it as the transaction side
// of the link registry.
func NewStore(e *storage.Engine, links *link.Registry) *Store {
	s := &Store{e: e, links: links}
	links.SetTransactionSide(s)
	return s
}

type LedgerTransactionPage struct {
	Items      []LedgerTransaction
	NextCursor string
}

type LedgerTransactionListOptions struct {
	AccountID    *uuid.UUID
	CategoryID   *uuid.UUID
	ReviewStatus string
	Limit        int
	Cursor       string
}

type LedgerReviewResult struct {
	Transaction LedgerTransaction
	Category    *LedgerCategory
}

type LedgerTransferCandidatesResult struct {
	Items []LedgerTransferCandidate
}

// LedgerTransferUnlinkResult carries the unlinked transaction and, when the
// paired side was found, the paired transaction.
type LedgerTransferUnlinkResult struct {
	Transaction       LedgerTransaction
	PairedTransaction *LedgerTransaction
}

type LedgerImportCommitResult struct {
	ImportedRows  int
	DuplicateRows int
}

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

// Ledger email key helpers

func ledEmailAccKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/emailacc/%s", userID, id))
}

func ledEmailAccPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/led/emailacc/", userID))
}

func ledEmailOrderKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/eord/%s", userID, id))
}

func ledEmailOrderPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/led/eord/", userID))
}

func idxLedEmailAccOrderKey(userID string, accountID uuid.UUID, orderID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_emailacc_eord/%s/%s", userID, accountID, orderID))
}

func idxLedEmailMsgIDKey(userID string, messageID string) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_eord_msgid/%s", userID, messageID))
}

func normalizeLedgerTransaction(t LedgerTransaction) LedgerTransaction {
	if t.ReviewStatus == "" {
		t.ReviewStatus = LedgerTransactionReviewNeedsReview
	}
	if t.CategorizationSource == "" {
		t.CategorizationSource = LedgerCategorizationNone
	}
	t.Links = NormalizeLedgerTransactionLinks(t.Links)
	t.References = NormalizeLedgerTransactionReferences(t.References)
	t.EmailOrderIDs = link.NormalizeIDs(t.EmailOrderIDs)
	if t.SpecialCategory == LedgerSpecialCategoryInternalTransfer && t.CategorizationSource == LedgerCategorizationNone {
		t.CategorizationSource = LedgerCategorizationManual
	}
	return t
}

func loadLedgerEmailAccount(txn *badger.Txn, userID string, id uuid.UUID) (LedgerEmailAccount, error) {
	var account LedgerEmailAccount
	item, err := txn.Get(ledEmailAccKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return account, storage.ErrNotFound
		}
		return account, err
	}
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &account)
	})
	return account, err
}

func storeLedgerEmailAccount(txn *badger.Txn, userID string, account LedgerEmailAccount) error {
	data, err := json.Marshal(account)
	if err != nil {
		return err
	}
	return txn.Set(ledEmailAccKey(userID, account.ID), data)
}

func loadLedgerEmailOrder(txn *badger.Txn, userID string, id uuid.UUID) (LedgerEmailOrder, error) {
	var order LedgerEmailOrder
	item, err := txn.Get(ledEmailOrderKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return order, storage.ErrNotFound
		}
		return order, err
	}
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &order)
	})
	if err != nil {
		return order, err
	}
	return NormalizeLedgerEmailOrder(order), nil
}

func storeLedgerEmailOrder(txn *badger.Txn, userID string, order LedgerEmailOrder) error {
	order = NormalizeLedgerEmailOrder(order)
	data, err := json.Marshal(order)
	if err != nil {
		return err
	}
	if err := txn.Set(ledEmailOrderKey(userID, order.ID), data); err != nil {
		return err
	}
	if err := txn.Set(idxLedEmailAccOrderKey(userID, order.EmailAccountID, order.ID), nil); err != nil {
		return err
	}
	if strings.TrimSpace(order.EmailMessageID) != "" {
		if err := txn.Set(idxLedEmailMsgIDKey(userID, order.EmailMessageID), []byte(order.ID.String())); err != nil {
			return err
		}
	}
	return nil
}

func collectLedgerAccounts(txn *badger.Txn, userID string) ([]LedgerAccount, error) {
	var accounts []LedgerAccount
	prefix := ledAccPrefix(userID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var account LedgerAccount
		if err := it.Item().Value(func(val []byte) error {
			return json.Unmarshal(val, &account)
		}); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	if accounts == nil {
		accounts = []LedgerAccount{}
	}
	return accounts, nil
}

func ledgerDateForMatch(t LedgerTransaction) string {
	if t.ValueDate != "" {
		return t.ValueDate
	}
	return t.BookingDate
}

func ledgerDateDeltaDays(left, right string) int {
	leftDate, leftErr := time.Parse("2006-01-02", left)
	rightDate, rightErr := time.Parse("2006-01-02", right)
	if leftErr != nil || rightErr != nil {
		return 999
	}
	delta := int(leftDate.Sub(rightDate).Hours() / 24)
	if delta < 0 {
		return -delta
	}
	return delta
}

func isPotentialLedgerTransferMatch(source, candidate LedgerTransaction) bool {
	if source.ID == candidate.ID {
		return false
	}
	if source.AccountID == candidate.AccountID {
		return false
	}
	if source.Currency != candidate.Currency {
		return false
	}
	if source.AmountMinor != -candidate.AmountMinor {
		return false
	}
	if candidate.TransferPairTransactionID != nil && *candidate.TransferPairTransactionID != source.ID {
		return false
	}
	return ledgerDateDeltaDays(ledgerDateForMatch(source), ledgerDateForMatch(candidate)) <= ledgerTransferMatchWindowDays
}

func applyLedgerTransferState(left *LedgerTransaction, rightID uuid.UUID) {
	left.CategoryID = nil
	left.TransferPairTransactionID = &rightID
	left.SpecialCategory = LedgerSpecialCategoryInternalTransfer
	left.ReviewStatus = LedgerTransactionReviewConfirmed
	left.CategorizationSource = LedgerCategorizationManual
	left.UpdatedAt = time.Now().UTC()
}

func clearLedgerTransferState(t *LedgerTransaction) {
	t.TransferPairTransactionID = nil
	if t.SpecialCategory == LedgerSpecialCategoryInternalTransfer {
		t.SpecialCategory = ""
	}
	if t.CategoryID == nil {
		t.ReviewStatus = LedgerTransactionReviewNeedsReview
		t.CategorizationSource = LedgerCategorizationNone
	}
	t.UpdatedAt = time.Now().UTC()
}

func unlinkLedgerTransferTxn(txn *badger.Txn, userID string, id uuid.UUID) (LedgerTransferUnlinkResult, error) {
	result := LedgerTransferUnlinkResult{}
	ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
	if err != nil {
		return result, err
	}
	if ledgerTxn.TransferPairTransactionID == nil {
		result.Transaction = ledgerTxn
		return result, nil
	}
	paired, err := loadLedgerTransaction(txn, userID, *ledgerTxn.TransferPairTransactionID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return result, err
	}
	clearLedgerTransferState(&ledgerTxn)
	if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
		return result, err
	}
	result.Transaction = ledgerTxn
	if err == nil {
		clearLedgerTransferState(&paired)
		if err := storeLedgerTransaction(txn, userID, paired); err != nil {
			return result, err
		}
		result.PairedTransaction = &paired
	}
	return result, nil
}

func normalizeLedgerCategory(c LedgerCategory) LedgerCategory {
	c.MatchWords = NormalizeLedgerMatchWords(c.MatchWords)
	if c.MatchWords == nil {
		c.MatchWords = []string{}
	}
	return c
}

func loadLedgerCategory(txn *badger.Txn, userID string, id uuid.UUID) (LedgerCategory, error) {
	var cat LedgerCategory
	item, err := txn.Get(ledCatKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return cat, storage.ErrNotFound
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

func storeLedgerCategory(txn *badger.Txn, userID string, cat LedgerCategory) error {
	cat = normalizeLedgerCategory(cat)
	data, err := json.Marshal(cat)
	if err != nil {
		return err
	}
	return txn.Set(ledCatKey(userID, cat.ID), data)
}

func loadLedgerTransaction(txn *badger.Txn, userID string, id uuid.UUID) (LedgerTransaction, error) {
	var t LedgerTransaction
	item, err := txn.Get(ledTxnKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return t, storage.ErrNotFound
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

func storeLedgerTransaction(txn *badger.Txn, userID string, t LedgerTransaction) error {
	t = normalizeLedgerTransaction(t)
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return txn.Set(ledTxnKey(userID, t.ID), data)
}

func collectLedgerCategories(txn *badger.Txn, userID string) ([]LedgerCategory, error) {
	var categories []LedgerCategory
	prefix := ledCatPrefix(userID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var cat LedgerCategory
		if err := it.Item().Value(func(val []byte) error {
			return json.Unmarshal(val, &cat)
		}); err != nil {
			return nil, err
		}
		categories = append(categories, normalizeLedgerCategory(cat))
	}
	if categories == nil {
		categories = []LedgerCategory{}
	}
	return categories, nil
}

func collectLedgerTransactions(txn *badger.Txn, userID string) ([]LedgerTransaction, error) {
	var txns []LedgerTransaction
	prefix := ledTxnPrefix(userID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var t LedgerTransaction
		if err := it.Item().Value(func(val []byte) error {
			return json.Unmarshal(val, &t)
		}); err != nil {
			return nil, err
		}
		txns = append(txns, normalizeLedgerTransaction(t))
	}
	if txns == nil {
		txns = []LedgerTransaction{}
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

func sortLedgerCategories(categories []LedgerCategory) {
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

func sortLedgerTransactions(txns []LedgerTransaction) {
	sort.Slice(txns, func(i, j int) bool {
		if txns[i].BookingDate == txns[j].BookingDate {
			return txns[i].ID.String() > txns[j].ID.String()
		}
		return txns[i].BookingDate > txns[j].BookingDate
	})
}

func containsUUID(ids []uuid.UUID, target uuid.UUID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func withoutUUID(ids []uuid.UUID, target uuid.UUID) []uuid.UUID {
	filtered := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == target {
			continue
		}
		filtered = append(filtered, id)
	}
	return link.NormalizeIDs(filtered)
}

func (s *Store) syncLedgerReferences(txn *badger.Txn, userID string, transactionID uuid.UUID, oldReferences, newReferences []LedgerTransactionReference) error {
	remove := make(map[string]LedgerTransactionReference, len(oldReferences))
	for _, reference := range oldReferences {
		remove[reference.Type+":"+reference.TargetID.String()] = reference
	}
	add := make(map[string]LedgerTransactionReference, len(newReferences))
	for _, reference := range newReferences {
		key := reference.Type + ":" + reference.TargetID.String()
		add[key] = reference
		delete(remove, key)
	}
	for key := range add {
		if _, existed := remove[key]; existed {
			delete(remove, key)
			delete(add, key)
		}
	}
	for _, reference := range remove {
		target, err := s.links.Target(reference.Type)
		if err != nil {
			return err
		}
		if err := target.RemoveLink(txn, userID, reference.TargetID, transactionID); err != nil {
			return err
		}
	}
	for _, reference := range add {
		target, err := s.links.Target(reference.Type)
		if err != nil {
			return err
		}
		if err := target.AddLink(txn, userID, reference.TargetID, transactionID); err != nil {
			return err
		}
	}
	return nil
}

// TransactionExists implements link.TransactionSide.
func (s *Store) TransactionExists(txn *badger.Txn, userID string, id uuid.UUID) bool {
	_, err := loadLedgerTransaction(txn, userID, id)
	return err == nil
}

// RemoveReferences implements link.TransactionSide: strips references to a
// deleted item from the given transactions.
func (s *Store) RemoveReferences(txn *badger.Txn, userID string, transactionIDs []uuid.UUID, referenceType string, targetID uuid.UUID) error {
	for _, transactionID := range link.NormalizeIDs(transactionIDs) {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, transactionID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return err
		}
		nextReferences := make([]LedgerTransactionReference, 0, len(ledgerTxn.References))
		for _, reference := range ledgerTxn.References {
			if reference.Type == referenceType && reference.TargetID == targetID {
				continue
			}
			nextReferences = append(nextReferences, reference)
		}
		if len(nextReferences) == len(ledgerTxn.References) {
			continue
		}
		if err := s.syncLedgerReferences(txn, userID, ledgerTxn.ID, ledgerTxn.References, nextReferences); err != nil {
			return err
		}
		ledgerTxn.References = nextReferences
		ledgerTxn.UpdatedAt = time.Now().UTC()
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
	}
	return nil
}

func paginateLedgerTransactions(txns []LedgerTransaction, limit int, cursor string) LedgerTransactionPage {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	page := LedgerTransactionPage{Items: []LedgerTransaction{}}
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

func (s *Store) ListLedgerAccounts(_ context.Context, userID string) ([]LedgerAccount, error) {
	var accounts []LedgerAccount
	prefix := ledAccPrefix(userID)

	err := s.e.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var a LedgerAccount
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
		accounts = []LedgerAccount{}
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Name == accounts[j].Name {
			return accounts[i].CreatedAt.Before(accounts[j].CreatedAt)
		}
		return strings.ToLower(accounts[i].Name) < strings.ToLower(accounts[j].Name)
	})
	return accounts, nil
}

func (s *Store) GetLedgerAccount(_ context.Context, userID string, id uuid.UUID) (LedgerAccount, error) {
	var a LedgerAccount
	err := s.e.View(func(txn *badger.Txn) error {
		item, err := txn.Get(ledAccKey(userID, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &a)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return a, storage.ErrNotFound
	}
	return a, err
}

func (s *Store) FindLedgerAccountByIBAN(_ context.Context, userID string, iban string) (LedgerAccount, error) {
	var a LedgerAccount
	err := s.e.View(func(txn *badger.Txn) error {
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
		return a, storage.ErrNotFound
	}
	return a, err
}

func (s *Store) CreateLedgerAccount(_ context.Context, userID string, a LedgerAccount) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return s.e.Update(func(txn *badger.Txn) error {
		if a.IBAN != "" {
			if _, err := txn.Get(idxLedAccIBANKey(userID, a.IBAN)); err == nil {
				return storage.ErrConflict
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

func (s *Store) ListLedgerCategories(_ context.Context, userID string) ([]LedgerCategory, error) {
	var categories []LedgerCategory
	err := s.e.View(func(txn *badger.Txn) error {
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

func (s *Store) GetLedgerCategory(_ context.Context, userID string, id uuid.UUID) (LedgerCategory, error) {
	var cat LedgerCategory
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		cat, err = loadLedgerCategory(txn, userID, id)
		return err
	})
	return cat, err
}

func (s *Store) CreateLedgerCategory(_ context.Context, userID string, c LedgerCategory) error {
	c = normalizeLedgerCategory(c)
	return s.e.Update(func(txn *badger.Txn) error {
		if err := ensureLedgerCategoryParentValid(txn, userID, c.ID, c.ParentID); err != nil {
			return err
		}
		return storeLedgerCategory(txn, userID, c)
	})
}

func (s *Store) UpdateLedgerCategory(_ context.Context, userID string, c LedgerCategory) error {
	c = normalizeLedgerCategory(c)
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := loadLedgerCategory(txn, userID, c.ID); err != nil {
			return err
		}
		if err := ensureLedgerCategoryParentValid(txn, userID, c.ID, c.ParentID); err != nil {
			return err
		}
		return storeLedgerCategory(txn, userID, c)
	})
}

func (s *Store) DeleteLedgerCategory(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
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
			if ledgerTxn.TransferPairTransactionID != nil {
				if _, err := unlinkLedgerTransferTxn(txn, userID, ledgerTxn.ID); err != nil {
					return err
				}
				ledgerTxn, err = loadLedgerTransaction(txn, userID, ledgerTxn.ID)
				if err != nil {
					return err
				}
			}
			ledgerTxn.CategoryID = nil
			ledgerTxn.ReviewStatus = LedgerTransactionReviewNeedsReview
			ledgerTxn.CategorizationSource = LedgerCategorizationNone
			ledgerTxn.UpdatedAt = time.Now().UTC()
			if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
				return err
			}
		}

		return txn.Delete(ledCatKey(userID, id))
	})
}

// Ledger Import Batches

func (s *Store) GetLedgerImportByFileHash(_ context.Context, userID string, sha256 string) (LedgerImportBatch, error) {
	var batch LedgerImportBatch
	err := s.e.View(func(txn *badger.Txn) error {
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
		return batch, storage.ErrNotFound
	}
	return batch, err
}

func (s *Store) LedgerTransactionFingerprintExists(_ context.Context, userID string, fingerprint string) (bool, error) {
	exists := false
	err := s.e.View(func(txn *badger.Txn) error {
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

func (s *Store) CommitLedgerImport(_ context.Context, userID string, batch LedgerImportBatch, txns []LedgerTransaction) (LedgerImportCommitResult, error) {
	result := LedgerImportCommitResult{}

	err := s.e.Update(func(txn *badger.Txn) error {
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

func (s *Store) ListLedgerImports(_ context.Context, userID string) ([]LedgerImportBatch, error) {
	var batches []LedgerImportBatch
	prefix := ledImpPrefix(userID)

	err := s.e.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var b LedgerImportBatch
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
		batches = []LedgerImportBatch{}
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

func (s *Store) ListLedgerTransactions(_ context.Context, userID string, accountID uuid.UUID) ([]LedgerTransaction, error) {
	page, err := s.ListLedgerTransactionsPage(context.Background(), userID, accountID, 1000, "")
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Store) ListLedgerTransactionsPage(_ context.Context, userID string, accountID uuid.UUID, limit int, cursor string) (LedgerTransactionPage, error) {
	return s.ListLedgerTransactionsFiltered(context.Background(), userID, LedgerTransactionListOptions{
		AccountID: &accountID,
		Limit:     limit,
		Cursor:    cursor,
	})
}

func (s *Store) ListLedgerTransactionsFiltered(_ context.Context, userID string, options LedgerTransactionListOptions) (LedgerTransactionPage, error) {
	page := LedgerTransactionPage{Items: []LedgerTransaction{}}
	var filtered []LedgerTransaction
	err := s.e.View(func(txn *badger.Txn) error {
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

func (s *Store) GetLedgerTransaction(_ context.Context, userID string, id uuid.UUID) (LedgerTransaction, error) {
	var ledgerTxn LedgerTransaction
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		ledgerTxn, err = loadLedgerTransaction(txn, userID, id)
		return err
	})
	return ledgerTxn, err
}

func (s *Store) ListLedgerTransferCandidates(_ context.Context, userID string, id uuid.UUID) (LedgerTransferCandidatesResult, error) {
	result := LedgerTransferCandidatesResult{Items: []LedgerTransferCandidate{}}
	err := s.e.View(func(txn *badger.Txn) error {
		source, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}
		accounts, err := collectLedgerAccounts(txn, userID)
		if err != nil {
			return err
		}
		accountNameByID := make(map[uuid.UUID]string, len(accounts))
		accountIBANByID := make(map[uuid.UUID]string, len(accounts))
		for _, account := range accounts {
			accountNameByID[account.ID] = account.Name
			accountIBANByID[account.ID] = account.IBAN
		}
		transactions, err := collectLedgerTransactions(txn, userID)
		if err != nil {
			return err
		}
		for _, candidate := range transactions {
			if !isPotentialLedgerTransferMatch(source, candidate) {
				continue
			}
			ibanMatch := false
			if accountIBANByID[candidate.AccountID] != "" && source.CounterpartyIBAN != "" && accountIBANByID[candidate.AccountID] == source.CounterpartyIBAN {
				ibanMatch = true
			}
			if accountIBANByID[source.AccountID] != "" && candidate.CounterpartyIBAN != "" && accountIBANByID[source.AccountID] == candidate.CounterpartyIBAN {
				ibanMatch = true
			}
			result.Items = append(result.Items, LedgerTransferCandidate{
				Transaction:   candidate,
				AccountName:   accountNameByID[candidate.AccountID],
				DateDeltaDays: ledgerDateDeltaDays(ledgerDateForMatch(source), ledgerDateForMatch(candidate)),
				IBANMatch:     ibanMatch,
			})
		}
		sort.Slice(result.Items, func(i, j int) bool {
			if result.Items[i].IBANMatch != result.Items[j].IBANMatch {
				return result.Items[i].IBANMatch
			}
			if result.Items[i].DateDeltaDays != result.Items[j].DateDeltaDays {
				return result.Items[i].DateDeltaDays < result.Items[j].DateDeltaDays
			}
			return result.Items[i].Transaction.BookingDate > result.Items[j].Transaction.BookingDate
		})
		return nil
	})
	return result, err
}

func (s *Store) LinkLedgerTransfer(_ context.Context, userID string, id uuid.UUID, input LedgerTransferLinkInput) (LedgerTransferLinkResult, error) {
	result := LedgerTransferLinkResult{}
	err := s.e.Update(func(txn *badger.Txn) error {
		left, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}
		right, err := loadLedgerTransaction(txn, userID, input.PairedTransactionID)
		if err != nil {
			return err
		}
		if !isPotentialLedgerTransferMatch(left, right) {
			return ErrLedgerTransferInvalid
		}
		if left.TransferPairTransactionID != nil && *left.TransferPairTransactionID != right.ID {
			paired, err := loadLedgerTransaction(txn, userID, *left.TransferPairTransactionID)
			if err == nil {
				clearLedgerTransferState(&paired)
				if err := storeLedgerTransaction(txn, userID, paired); err != nil {
					return err
				}
			}
		}
		if right.TransferPairTransactionID != nil && *right.TransferPairTransactionID != left.ID {
			paired, err := loadLedgerTransaction(txn, userID, *right.TransferPairTransactionID)
			if err == nil {
				clearLedgerTransferState(&paired)
				if err := storeLedgerTransaction(txn, userID, paired); err != nil {
					return err
				}
			}
		}
		applyLedgerTransferState(&left, right.ID)
		applyLedgerTransferState(&right, left.ID)
		if err := storeLedgerTransaction(txn, userID, left); err != nil {
			return err
		}
		if err := storeLedgerTransaction(txn, userID, right); err != nil {
			return err
		}
		result.Transaction = left
		result.PairedTransaction = right
		return nil
	})
	return result, err
}

func (s *Store) UnlinkLedgerTransfer(_ context.Context, userID string, id uuid.UUID) (LedgerTransferUnlinkResult, error) {
	result := LedgerTransferUnlinkResult{}
	err := s.e.Update(func(txn *badger.Txn) error {
		var err error
		result, err = unlinkLedgerTransferTxn(txn, userID, id)
		return err
	})
	return result, err
}

func (s *Store) UpdateLedgerTransactionDetails(_ context.Context, userID string, id uuid.UUID, input LedgerTransactionDetailsInput) (LedgerTransaction, error) {
	updatedAt := time.Now().UTC()
	var updated LedgerTransaction
	err := s.e.Update(func(txn *badger.Txn) error {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}
		if err := s.syncLedgerReferences(txn, userID, ledgerTxn.ID, ledgerTxn.References, input.References); err != nil {
			return err
		}
		ledgerTxn.Note = input.Note
		ledgerTxn.Links = input.Links
		ledgerTxn.References = input.References
		ledgerTxn.UpdatedAt = updatedAt
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
		updated = ledgerTxn
		return nil
	})
	if err != nil {
		return LedgerTransaction{}, err
	}
	return updated, nil
}

func (s *Store) ReviewLedgerTransaction(_ context.Context, userID string, id uuid.UUID, input LedgerTransactionReviewInput) (LedgerReviewResult, error) {
	result := LedgerReviewResult{}
	now := time.Now().UTC()
	err := s.e.Update(func(txn *badger.Txn) error {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}

		var selectedCategory *LedgerCategory
		if input.CategoryID != nil {
			cat, err := loadLedgerCategory(txn, userID, *input.CategoryID)
			if err != nil {
				return err
			}
			selectedCategory = &cat
		}

		if input.NewCategory != nil {
			categoryID := uuid.New()
			newCategory := LedgerCategory{
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
			} else if !errors.Is(err, storage.ErrNotFound) {
				return err
			}
		}

		if len(input.AddMatchWords) > 0 {
			if selectedCategory == nil {
				return storage.ErrNotFound
			}
			selectedCategory.MatchWords = append(selectedCategory.MatchWords, input.AddMatchWords...)
			selectedCategory.MatchWords = NormalizeLedgerMatchWords(selectedCategory.MatchWords)
			selectedCategory.UpdatedAt = now
			if err := storeLedgerCategory(txn, userID, *selectedCategory); err != nil {
				return err
			}
		}

		if selectedCategory != nil {
			if ledgerTxn.TransferPairTransactionID != nil {
				return ErrLedgerTransferLinked
			}
			categoryID := selectedCategory.ID
			ledgerTxn.CategoryID = &categoryID
			ledgerTxn.SpecialCategory = ""
			ledgerTxn.CategorizationSource = LedgerCategorizationManual
		}
		ledgerTxn.ReviewStatus = LedgerTransactionReviewConfirmed
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

// Ledger Email Accounts

func (s *Store) ListLedgerEmailAccounts(_ context.Context, userID string) ([]LedgerEmailAccount, error) {
	items := []LedgerEmailAccount{}
	err := s.e.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := ledEmailAccPrefix(userID)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var item LedgerEmailAccount
			if err := it.Item().Value(func(val []byte) error { return json.Unmarshal(val, &item) }); err != nil {
				return err
			}
			items = append(items, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if strings.EqualFold(items[i].Name, items[j].Name) {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func (s *Store) GetLedgerEmailAccount(_ context.Context, userID string, id uuid.UUID) (LedgerEmailAccount, error) {
	var account LedgerEmailAccount
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		account, err = loadLedgerEmailAccount(txn, userID, id)
		return err
	})
	return account, err
}

func (s *Store) CreateLedgerEmailAccount(_ context.Context, userID string, account LedgerEmailAccount) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailAccKey(userID, account.ID)); err == nil {
			return storage.ErrConflict
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return storeLedgerEmailAccount(txn, userID, account)
	})
}

func (s *Store) UpdateLedgerEmailAccount(_ context.Context, userID string, account LedgerEmailAccount) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailAccKey(userID, account.ID)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return storage.ErrNotFound
			}
			return err
		}
		return storeLedgerEmailAccount(txn, userID, account)
	})
}

func (s *Store) DeleteLedgerEmailAccount(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		account, err := loadLedgerEmailAccount(txn, userID, id)
		if err != nil {
			return err
		}
		orders, err := collectLedgerEmailOrdersByAccount(txn, userID, account.ID)
		if err != nil {
			return err
		}
		for _, order := range orders {
			if err := unlinkLedgerEmailOrderTxn(txn, userID, order.ID); err != nil {
				return err
			}
			if err := txn.Delete(ledEmailOrderKey(userID, order.ID)); err != nil {
				return err
			}
			if err := txn.Delete(idxLedEmailAccOrderKey(userID, order.EmailAccountID, order.ID)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			if strings.TrimSpace(order.EmailMessageID) != "" {
				if err := txn.Delete(idxLedEmailMsgIDKey(userID, order.EmailMessageID)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
					return err
				}
			}
		}
		return txn.Delete(ledEmailAccKey(userID, id))
	})
}

func collectLedgerEmailOrders(txn *badger.Txn, userID string) ([]LedgerEmailOrder, error) {
	items := []LedgerEmailOrder{}
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	prefix := ledEmailOrderPrefix(userID)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var item LedgerEmailOrder
		if err := it.Item().Value(func(val []byte) error { return json.Unmarshal(val, &item) }); err != nil {
			return nil, err
		}
		items = append(items, NormalizeLedgerEmailOrder(item))
	}
	return items, nil
}

func collectLedgerEmailOrdersByAccount(txn *badger.Txn, userID string, accountID uuid.UUID) ([]LedgerEmailOrder, error) {
	all, err := collectLedgerEmailOrders(txn, userID)
	if err != nil {
		return nil, err
	}
	items := make([]LedgerEmailOrder, 0)
	for _, item := range all {
		if item.EmailAccountID == accountID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *Store) ListLedgerEmailOrders(_ context.Context, userID string) ([]LedgerEmailOrder, error) {
	items := []LedgerEmailOrder{}
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		items, err = collectLedgerEmailOrders(txn, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OrderDate == items[j].OrderDate {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].OrderDate > items[j].OrderDate
	})
	return items, nil
}

func (s *Store) ListLedgerEmailOrdersByAccount(_ context.Context, userID string, accountID uuid.UUID) ([]LedgerEmailOrder, error) {
	items := []LedgerEmailOrder{}
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		items, err = collectLedgerEmailOrdersByAccount(txn, userID, accountID)
		return err
	})
	return items, err
}

func (s *Store) ListLedgerEmailOrdersByTransaction(_ context.Context, userID string, transactionID uuid.UUID) ([]LedgerEmailOrder, error) {
	items, err := s.ListLedgerEmailOrders(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	filtered := make([]LedgerEmailOrder, 0)
	for _, item := range items {
		if containsUUID(item.LinkedTransactionIDs, transactionID) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *Store) GetLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID) (LedgerEmailOrder, error) {
	var order LedgerEmailOrder
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		order, err = loadLedgerEmailOrder(txn, userID, id)
		return err
	})
	return order, err
}

func (s *Store) GetLedgerEmailOrderByMessageID(_ context.Context, userID string, messageID string) (LedgerEmailOrder, error) {
	var order LedgerEmailOrder
	err := s.e.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idxLedEmailMsgIDKey(userID, messageID))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return storage.ErrNotFound
			}
			return err
		}
		var idStr string
		if err := item.Value(func(val []byte) error { idStr = string(val); return nil }); err != nil {
			return err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return err
		}
		order, err = loadLedgerEmailOrder(txn, userID, id)
		return err
	})
	return order, err
}

func (s *Store) CreateLedgerEmailOrder(_ context.Context, userID string, order LedgerEmailOrder) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailOrderKey(userID, order.ID)); err == nil {
			return storage.ErrConflict
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return storeLedgerEmailOrder(txn, userID, order)
	})
}

func unlinkLedgerEmailOrderTxn(txn *badger.Txn, userID string, orderID uuid.UUID) error {
	order, err := loadLedgerEmailOrder(txn, userID, orderID)
	if err != nil {
		return err
	}
	for _, transactionID := range order.LinkedTransactionIDs {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, transactionID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return err
		}
		ledgerTxn.EmailOrderIDs = withoutUUID(ledgerTxn.EmailOrderIDs, order.ID)
		ledgerTxn.UpdatedAt = time.Now().UTC()
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
	}
	order.LinkedTransactionIDs = []uuid.UUID{}
	order.MatchStatus = LedgerEmailOrderStatusUnmatched
	order.UpdatedAt = time.Now().UTC()
	return storeLedgerEmailOrder(txn, userID, order)
}

func (s *Store) LinkLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID, input LedgerEmailOrderLinkInput) (LedgerEmailOrder, error) {
	updated := LedgerEmailOrder{}
	err := s.e.Update(func(txn *badger.Txn) error {
		order, err := loadLedgerEmailOrder(txn, userID, id)
		if err != nil {
			return err
		}
		for _, existingTxnID := range order.LinkedTransactionIDs {
			ledgerTxn, err := loadLedgerTransaction(txn, userID, existingTxnID)
			if err == nil {
				ledgerTxn.EmailOrderIDs = withoutUUID(ledgerTxn.EmailOrderIDs, order.ID)
				ledgerTxn.UpdatedAt = time.Now().UTC()
				if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
					return err
				}
			} else if !errors.Is(err, storage.ErrNotFound) {
				return err
			}
		}
		order.LinkedTransactionIDs = link.NormalizeIDs(input.TransactionIDs)
		order.MatchStatus = LedgerEmailOrderStatusMatched
		order.UpdatedAt = time.Now().UTC()
		for _, transactionID := range order.LinkedTransactionIDs {
			ledgerTxn, err := loadLedgerTransaction(txn, userID, transactionID)
			if err != nil {
				return err
			}
			if !containsUUID(ledgerTxn.EmailOrderIDs, order.ID) {
				ledgerTxn.EmailOrderIDs = append(ledgerTxn.EmailOrderIDs, order.ID)
			}
			ledgerTxn.EmailOrderIDs = link.NormalizeIDs(ledgerTxn.EmailOrderIDs)
			ledgerTxn.UpdatedAt = time.Now().UTC()
			if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
				return err
			}
		}
		if err := storeLedgerEmailOrder(txn, userID, order); err != nil {
			return err
		}
		updated = order
		return nil
	})
	return updated, err
}

func (s *Store) RejectLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID) (LedgerEmailOrder, error) {
	updated := LedgerEmailOrder{}
	err := s.e.Update(func(txn *badger.Txn) error {
		if err := unlinkLedgerEmailOrderTxn(txn, userID, id); err != nil {
			return err
		}
		order, err := loadLedgerEmailOrder(txn, userID, id)
		if err != nil {
			return err
		}
		order.MatchStatus = LedgerEmailOrderStatusRejected
		order.UpdatedAt = time.Now().UTC()
		if err := storeLedgerEmailOrder(txn, userID, order); err != nil {
			return err
		}
		updated = order
		return nil
	})
	return updated, err
}
