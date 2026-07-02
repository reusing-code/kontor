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
	"github.com/reusing-code/kontor/backend/internal/model"
)

var (
	ErrLedgerPreviewExpired   = errors.New("ledger preview expired")
	ErrLedgerFileImported     = errors.New("ledger file already imported")
	ErrLedgerCategoryHasChild = errors.New("ledger category has children")
	ErrLedgerCategoryHasCycle = errors.New("ledger category cycle")
	ErrLedgerTransferInvalid  = errors.New("invalid transfer pair")
	ErrLedgerTransferLinked   = errors.New("linked internal transfers must be unlinked explicitly before assigning a category")
)

const ledgerTransferMatchWindowDays = 3

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

func normalizeLedgerTransaction(t model.LedgerTransaction) model.LedgerTransaction {
	if t.ReviewStatus == "" {
		t.ReviewStatus = model.LedgerTransactionReviewNeedsReview
	}
	if t.CategorizationSource == "" {
		t.CategorizationSource = model.LedgerCategorizationNone
	}
	t.Links = model.NormalizeLedgerTransactionLinks(t.Links)
	t.References = model.NormalizeLedgerTransactionReferences(t.References)
	t.EmailOrderIDs = model.NormalizeLinkedTransactionIDs(t.EmailOrderIDs)
	if t.SpecialCategory == model.LedgerSpecialCategoryInternalTransfer && t.CategorizationSource == model.LedgerCategorizationNone {
		t.CategorizationSource = model.LedgerCategorizationManual
	}
	return t
}

func loadLedgerEmailAccount(txn *badger.Txn, userID string, id uuid.UUID) (model.LedgerEmailAccount, error) {
	var account model.LedgerEmailAccount
	item, err := txn.Get(ledEmailAccKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return account, ErrNotFound
		}
		return account, err
	}
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &account)
	})
	return account, err
}

func storeLedgerEmailAccount(txn *badger.Txn, userID string, account model.LedgerEmailAccount) error {
	data, err := json.Marshal(account)
	if err != nil {
		return err
	}
	return txn.Set(ledEmailAccKey(userID, account.ID), data)
}

func loadLedgerEmailOrder(txn *badger.Txn, userID string, id uuid.UUID) (model.LedgerEmailOrder, error) {
	var order model.LedgerEmailOrder
	item, err := txn.Get(ledEmailOrderKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return order, ErrNotFound
		}
		return order, err
	}
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &order)
	})
	if err != nil {
		return order, err
	}
	return model.NormalizeLedgerEmailOrder(order), nil
}

func storeLedgerEmailOrder(txn *badger.Txn, userID string, order model.LedgerEmailOrder) error {
	order = model.NormalizeLedgerEmailOrder(order)
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

func collectLedgerAccounts(txn *badger.Txn, userID string) ([]model.LedgerAccount, error) {
	var accounts []model.LedgerAccount
	prefix := ledAccPrefix(userID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var account model.LedgerAccount
		if err := it.Item().Value(func(val []byte) error {
			return json.Unmarshal(val, &account)
		}); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	if accounts == nil {
		accounts = []model.LedgerAccount{}
	}
	return accounts, nil
}

func ledgerDateForMatch(t model.LedgerTransaction) string {
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

func isPotentialLedgerTransferMatch(source, candidate model.LedgerTransaction) bool {
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

func applyLedgerTransferState(left *model.LedgerTransaction, rightID uuid.UUID) {
	left.CategoryID = nil
	left.TransferPairTransactionID = &rightID
	left.SpecialCategory = model.LedgerSpecialCategoryInternalTransfer
	left.ReviewStatus = model.LedgerTransactionReviewConfirmed
	left.CategorizationSource = model.LedgerCategorizationManual
	left.UpdatedAt = time.Now().UTC()
}

func clearLedgerTransferState(t *model.LedgerTransaction) {
	t.TransferPairTransactionID = nil
	if t.SpecialCategory == model.LedgerSpecialCategoryInternalTransfer {
		t.SpecialCategory = ""
	}
	if t.CategoryID == nil {
		t.ReviewStatus = model.LedgerTransactionReviewNeedsReview
		t.CategorizationSource = model.LedgerCategorizationNone
	}
	t.UpdatedAt = time.Now().UTC()
}

func unlinkLedgerTransferTxn(txn *badger.Txn, userID string, id uuid.UUID) (LedgerTransferLinkResult, error) {
	result := LedgerTransferLinkResult{}
	ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
	if err != nil {
		return result, err
	}
	if ledgerTxn.TransferPairTransactionID == nil {
		result.Transaction = ledgerTxn
		return result, nil
	}
	paired, err := loadLedgerTransaction(txn, userID, *ledgerTxn.TransferPairTransactionID)
	if err != nil && !errors.Is(err, ErrNotFound) {
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

func normalizePurchase(p model.Purchase) model.Purchase {
	p.LinkedTransactionIDs = model.NormalizeLinkedTransactionIDs(p.LinkedTransactionIDs)
	return p
}

func normalizeContract(c model.Contract) model.Contract {
	c.LinkedTransactionIDs = model.NormalizeLinkedTransactionIDs(c.LinkedTransactionIDs)
	return c
}

func normalizeVehicle(v model.Vehicle) model.Vehicle {
	v.LinkedTransactionIDs = model.NormalizeLinkedTransactionIDs(v.LinkedTransactionIDs)
	return v
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
	return model.NormalizeLinkedTransactionIDs(filtered)
}

func storePurchase(txn *badger.Txn, userID string, purchase model.Purchase) error {
	purchase = normalizePurchase(purchase)
	data, err := json.Marshal(purchase)
	if err != nil {
		return err
	}
	return txn.Set(purKey(userID, purchase.ID), data)
}

func storeContract(txn *badger.Txn, userID string, contract model.Contract) error {
	contract = normalizeContract(contract)
	data, err := json.Marshal(contract)
	if err != nil {
		return err
	}
	return txn.Set(conKey(userID, contract.ID), data)
}

func storeVehicle(txn *badger.Txn, userID string, vehicle model.Vehicle) error {
	vehicle = normalizeVehicle(vehicle)
	data, err := json.Marshal(vehicle)
	if err != nil {
		return err
	}
	return txn.Set(vehKey(userID, vehicle.ID), data)
}

func syncLedgerReferences(txn *badger.Txn, userID string, transactionID uuid.UUID, oldReferences, newReferences []model.LedgerTransactionReference) error {
	remove := make(map[string]model.LedgerTransactionReference, len(oldReferences))
	for _, reference := range oldReferences {
		remove[reference.Type+":"+reference.TargetID.String()] = reference
	}
	add := make(map[string]model.LedgerTransactionReference, len(newReferences))
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
		switch reference.Type {
		case model.LedgerReferencePurchase:
			item, err := txn.Get(purKey(userID, reference.TargetID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				return err
			}
			var purchase model.Purchase
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &purchase) }); err != nil {
				return err
			}
			purchase = normalizePurchase(purchase)
			purchase.LinkedTransactionIDs = withoutUUID(purchase.LinkedTransactionIDs, transactionID)
			if err := storePurchase(txn, userID, purchase); err != nil {
				return err
			}
		case model.LedgerReferenceContract:
			item, err := txn.Get(conKey(userID, reference.TargetID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				return err
			}
			var contract model.Contract
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &contract) }); err != nil {
				return err
			}
			contract = normalizeContract(contract)
			contract.LinkedTransactionIDs = withoutUUID(contract.LinkedTransactionIDs, transactionID)
			if err := storeContract(txn, userID, contract); err != nil {
				return err
			}
		case model.LedgerReferenceVehicle:
			item, err := txn.Get(vehKey(userID, reference.TargetID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				return err
			}
			var vehicle model.Vehicle
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &vehicle) }); err != nil {
				return err
			}
			vehicle = normalizeVehicle(vehicle)
			vehicle.LinkedTransactionIDs = withoutUUID(vehicle.LinkedTransactionIDs, transactionID)
			if err := storeVehicle(txn, userID, vehicle); err != nil {
				return err
			}
		}
	}
	for _, reference := range add {
		switch reference.Type {
		case model.LedgerReferencePurchase:
			item, err := txn.Get(purKey(userID, reference.TargetID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					return ErrNotFound
				}
				return err
			}
			var purchase model.Purchase
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &purchase) }); err != nil {
				return err
			}
			purchase = normalizePurchase(purchase)
			if !containsUUID(purchase.LinkedTransactionIDs, transactionID) {
				purchase.LinkedTransactionIDs = append(purchase.LinkedTransactionIDs, transactionID)
			}
			purchase.LinkedTransactionIDs = model.NormalizeLinkedTransactionIDs(purchase.LinkedTransactionIDs)
			if err := storePurchase(txn, userID, purchase); err != nil {
				return err
			}
		case model.LedgerReferenceContract:
			item, err := txn.Get(conKey(userID, reference.TargetID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					return ErrNotFound
				}
				return err
			}
			var contract model.Contract
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &contract) }); err != nil {
				return err
			}
			contract = normalizeContract(contract)
			if !containsUUID(contract.LinkedTransactionIDs, transactionID) {
				contract.LinkedTransactionIDs = append(contract.LinkedTransactionIDs, transactionID)
			}
			contract.LinkedTransactionIDs = model.NormalizeLinkedTransactionIDs(contract.LinkedTransactionIDs)
			if err := storeContract(txn, userID, contract); err != nil {
				return err
			}
		case model.LedgerReferenceVehicle:
			item, err := txn.Get(vehKey(userID, reference.TargetID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					return ErrNotFound
				}
				return err
			}
			var vehicle model.Vehicle
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &vehicle) }); err != nil {
				return err
			}
			vehicle = normalizeVehicle(vehicle)
			if !containsUUID(vehicle.LinkedTransactionIDs, transactionID) {
				vehicle.LinkedTransactionIDs = append(vehicle.LinkedTransactionIDs, transactionID)
			}
			vehicle.LinkedTransactionIDs = model.NormalizeLinkedTransactionIDs(vehicle.LinkedTransactionIDs)
			if err := storeVehicle(txn, userID, vehicle); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeLedgerTransactionLinks(txn *badger.Txn, userID string, transactionIDs []uuid.UUID, referenceType string, targetID uuid.UUID) error {
	for _, transactionID := range model.NormalizeLinkedTransactionIDs(transactionIDs) {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, transactionID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return err
		}
		nextReferences := make([]model.LedgerTransactionReference, 0, len(ledgerTxn.References))
		for _, reference := range ledgerTxn.References {
			if reference.Type == referenceType && reference.TargetID == targetID {
				continue
			}
			nextReferences = append(nextReferences, reference)
		}
		if len(nextReferences) == len(ledgerTxn.References) {
			continue
		}
		if err := syncLedgerReferences(txn, userID, ledgerTxn.ID, ledgerTxn.References, nextReferences); err != nil {
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

func (s *BadgerStore) ListLedgerTransferCandidates(_ context.Context, userID string, id uuid.UUID) (LedgerTransferCandidatesResult, error) {
	result := LedgerTransferCandidatesResult{Items: []model.LedgerTransferCandidate{}}
	err := s.db.View(func(txn *badger.Txn) error {
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
			result.Items = append(result.Items, model.LedgerTransferCandidate{
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

func (s *BadgerStore) LinkLedgerTransfer(_ context.Context, userID string, id uuid.UUID, input model.LedgerTransferLinkInput) (model.LedgerTransferLinkResult, error) {
	result := model.LedgerTransferLinkResult{}
	err := s.db.Update(func(txn *badger.Txn) error {
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

func (s *BadgerStore) UnlinkLedgerTransfer(_ context.Context, userID string, id uuid.UUID) (LedgerTransferLinkResult, error) {
	result := LedgerTransferLinkResult{}
	err := s.db.Update(func(txn *badger.Txn) error {
		var err error
		result, err = unlinkLedgerTransferTxn(txn, userID, id)
		return err
	})
	return result, err
}

func (s *BadgerStore) UpdateLedgerTransactionDetails(_ context.Context, userID string, id uuid.UUID, input model.LedgerTransactionDetailsInput) (model.LedgerTransaction, error) {
	updatedAt := time.Now().UTC()
	var updated model.LedgerTransaction
	err := s.db.Update(func(txn *badger.Txn) error {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}
		if err := syncLedgerReferences(txn, userID, ledgerTxn.ID, ledgerTxn.References, input.References); err != nil {
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
		return model.LedgerTransaction{}, err
	}
	return updated, nil
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
			if ledgerTxn.TransferPairTransactionID != nil {
				return ErrLedgerTransferLinked
			}
			categoryID := selectedCategory.ID
			ledgerTxn.CategoryID = &categoryID
			ledgerTxn.SpecialCategory = ""
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

// Ledger Email Accounts

func (s *BadgerStore) ListLedgerEmailAccounts(_ context.Context, userID string) ([]model.LedgerEmailAccount, error) {
	items := []model.LedgerEmailAccount{}
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := ledEmailAccPrefix(userID)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var item model.LedgerEmailAccount
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

func (s *BadgerStore) GetLedgerEmailAccount(_ context.Context, userID string, id uuid.UUID) (model.LedgerEmailAccount, error) {
	var account model.LedgerEmailAccount
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		account, err = loadLedgerEmailAccount(txn, userID, id)
		return err
	})
	return account, err
}

func (s *BadgerStore) CreateLedgerEmailAccount(_ context.Context, userID string, account model.LedgerEmailAccount) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailAccKey(userID, account.ID)); err == nil {
			return ErrConflict
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return storeLedgerEmailAccount(txn, userID, account)
	})
}

func (s *BadgerStore) UpdateLedgerEmailAccount(_ context.Context, userID string, account model.LedgerEmailAccount) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailAccKey(userID, account.ID)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}
		return storeLedgerEmailAccount(txn, userID, account)
	})
}

func (s *BadgerStore) DeleteLedgerEmailAccount(_ context.Context, userID string, id uuid.UUID) error {
	return s.db.Update(func(txn *badger.Txn) error {
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

func collectLedgerEmailOrders(txn *badger.Txn, userID string) ([]model.LedgerEmailOrder, error) {
	items := []model.LedgerEmailOrder{}
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	prefix := ledEmailOrderPrefix(userID)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var item model.LedgerEmailOrder
		if err := it.Item().Value(func(val []byte) error { return json.Unmarshal(val, &item) }); err != nil {
			return nil, err
		}
		items = append(items, model.NormalizeLedgerEmailOrder(item))
	}
	return items, nil
}

func collectLedgerEmailOrdersByAccount(txn *badger.Txn, userID string, accountID uuid.UUID) ([]model.LedgerEmailOrder, error) {
	all, err := collectLedgerEmailOrders(txn, userID)
	if err != nil {
		return nil, err
	}
	items := make([]model.LedgerEmailOrder, 0)
	for _, item := range all {
		if item.EmailAccountID == accountID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *BadgerStore) ListLedgerEmailOrders(_ context.Context, userID string) ([]model.LedgerEmailOrder, error) {
	items := []model.LedgerEmailOrder{}
	err := s.db.View(func(txn *badger.Txn) error {
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

func (s *BadgerStore) ListLedgerEmailOrdersByAccount(_ context.Context, userID string, accountID uuid.UUID) ([]model.LedgerEmailOrder, error) {
	items := []model.LedgerEmailOrder{}
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		items, err = collectLedgerEmailOrdersByAccount(txn, userID, accountID)
		return err
	})
	return items, err
}

func (s *BadgerStore) ListLedgerEmailOrdersByTransaction(_ context.Context, userID string, transactionID uuid.UUID) ([]model.LedgerEmailOrder, error) {
	items, err := s.ListLedgerEmailOrders(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	filtered := make([]model.LedgerEmailOrder, 0)
	for _, item := range items {
		if containsUUID(item.LinkedTransactionIDs, transactionID) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *BadgerStore) GetLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID) (model.LedgerEmailOrder, error) {
	var order model.LedgerEmailOrder
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		order, err = loadLedgerEmailOrder(txn, userID, id)
		return err
	})
	return order, err
}

func (s *BadgerStore) GetLedgerEmailOrderByMessageID(_ context.Context, userID string, messageID string) (model.LedgerEmailOrder, error) {
	var order model.LedgerEmailOrder
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idxLedEmailMsgIDKey(userID, messageID))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
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

func (s *BadgerStore) CreateLedgerEmailOrder(_ context.Context, userID string, order model.LedgerEmailOrder) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailOrderKey(userID, order.ID)); err == nil {
			return ErrConflict
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
			if errors.Is(err, ErrNotFound) {
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
	order.MatchStatus = model.LedgerEmailOrderStatusUnmatched
	order.UpdatedAt = time.Now().UTC()
	return storeLedgerEmailOrder(txn, userID, order)
}

func (s *BadgerStore) LinkLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID, input model.LedgerEmailOrderLinkInput) (model.LedgerEmailOrder, error) {
	updated := model.LedgerEmailOrder{}
	err := s.db.Update(func(txn *badger.Txn) error {
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
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
		}
		order.LinkedTransactionIDs = model.NormalizeLinkedTransactionIDs(input.TransactionIDs)
		order.MatchStatus = model.LedgerEmailOrderStatusMatched
		order.UpdatedAt = time.Now().UTC()
		for _, transactionID := range order.LinkedTransactionIDs {
			ledgerTxn, err := loadLedgerTransaction(txn, userID, transactionID)
			if err != nil {
				return err
			}
			if !containsUUID(ledgerTxn.EmailOrderIDs, order.ID) {
				ledgerTxn.EmailOrderIDs = append(ledgerTxn.EmailOrderIDs, order.ID)
			}
			ledgerTxn.EmailOrderIDs = model.NormalizeLinkedTransactionIDs(ledgerTxn.EmailOrderIDs)
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

func (s *BadgerStore) RejectLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID) (model.LedgerEmailOrder, error) {
	updated := model.LedgerEmailOrder{}
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := unlinkLedgerEmailOrderTxn(txn, userID, id); err != nil {
			return err
		}
		order, err := loadLedgerEmailOrder(txn, userID, id)
		if err != nil {
			return err
		}
		order.MatchStatus = model.LedgerEmailOrderStatusRejected
		order.UpdatedAt = time.Now().UTC()
		if err := storeLedgerEmailOrder(txn, userID, order); err != nil {
			return err
		}
		updated = order
		return nil
	})
	return updated, err
}
