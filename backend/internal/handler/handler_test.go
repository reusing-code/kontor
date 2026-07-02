package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
	"github.com/reusing-code/kontor/backend/internal/store"
)

// mockStore implements store.Store in memory for handler tests.
type mockStore struct {
	categories          map[string]map[uuid.UUID]model.Category // keyed by module, then ID
	contracts           map[uuid.UUID]model.Contract
	purchases           map[uuid.UUID]model.Purchase
	vehicles            map[uuid.UUID]model.Vehicle
	users               map[string]model.User // keyed by email
	usersById           map[string]model.User // keyed by ID
	settings            map[string]model.UserSettings
	ledgerAccounts      map[uuid.UUID]model.LedgerAccount
	ledgerEmailAccounts map[uuid.UUID]model.LedgerEmailAccount
	ledgerCategories    map[uuid.UUID]model.LedgerCategory
	ledgerImports       []model.LedgerImportBatch
	ledgerTransactions  map[uuid.UUID][]model.LedgerTransaction
	ledgerEmailOrders   map[uuid.UUID]model.LedgerEmailOrder
}

func newMockStore() *mockStore {
	return &mockStore{
		categories:          make(map[string]map[uuid.UUID]model.Category),
		contracts:           make(map[uuid.UUID]model.Contract),
		purchases:           make(map[uuid.UUID]model.Purchase),
		vehicles:            make(map[uuid.UUID]model.Vehicle),
		users:               make(map[string]model.User),
		usersById:           make(map[string]model.User),
		settings:            make(map[string]model.UserSettings),
		ledgerAccounts:      make(map[uuid.UUID]model.LedgerAccount),
		ledgerEmailAccounts: make(map[uuid.UUID]model.LedgerEmailAccount),
		ledgerCategories:    make(map[uuid.UUID]model.LedgerCategory),
		ledgerTransactions:  make(map[uuid.UUID][]model.LedgerTransaction),
		ledgerEmailOrders:   make(map[uuid.UUID]model.LedgerEmailOrder),
	}
}

func (m *mockStore) addCategory(module string, c model.Category) {
	if m.categories[module] == nil {
		m.categories[module] = make(map[uuid.UUID]model.Category)
	}
	m.categories[module][c.ID] = c
}

func (m *mockStore) CreateUser(_ context.Context, u model.User) error {
	if _, ok := m.users[u.Email]; ok {
		return store.ErrConflict
	}
	m.users[u.Email] = u
	m.usersById[u.ID.String()] = u
	return nil
}

func (m *mockStore) GetUserByEmail(_ context.Context, email string) (model.User, error) {
	u, ok := m.users[email]
	if !ok {
		return u, store.ErrNotFound
	}
	return u, nil
}

func (m *mockStore) GetUserByID(_ context.Context, id string) (model.User, error) {
	u, ok := m.usersById[id]
	if !ok {
		return u, store.ErrNotFound
	}
	return u, nil
}

func (m *mockStore) UpdateUser(_ context.Context, u model.User) error {
	if _, ok := m.usersById[u.ID.String()]; !ok {
		return store.ErrNotFound
	}
	m.usersById[u.ID.String()] = u
	m.users[u.Email] = u
	return nil
}

func (m *mockStore) GetSettings(_ context.Context, userID string) (model.UserSettings, error) {
	s, ok := m.settings[userID]
	if !ok {
		return model.DefaultUserSettings(), nil
	}
	return s, nil
}

func (m *mockStore) UpdateSettings(_ context.Context, userID string, s model.UserSettings) error {
	m.settings[userID] = s
	return nil
}

func (m *mockStore) ListCategories(_ context.Context, _ string, module string) ([]model.Category, error) {
	modCats := m.categories[module]
	out := make([]model.Category, 0, len(modCats))
	for _, c := range modCats {
		out = append(out, c)
	}
	return out, nil
}

func (m *mockStore) GetCategory(_ context.Context, _ string, module string, id uuid.UUID) (model.Category, error) {
	if modCats, ok := m.categories[module]; ok {
		if c, ok := modCats[id]; ok {
			return c, nil
		}
	}
	return model.Category{}, store.ErrNotFound
}

func (m *mockStore) CreateCategory(_ context.Context, _ string, module string, c model.Category) error {
	if m.categories[module] == nil {
		m.categories[module] = make(map[uuid.UUID]model.Category)
	}
	m.categories[module][c.ID] = c
	return nil
}

func (m *mockStore) UpdateCategory(_ context.Context, _ string, module string, c model.Category) error {
	if modCats, ok := m.categories[module]; ok {
		if _, ok := modCats[c.ID]; ok {
			m.categories[module][c.ID] = c
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *mockStore) DeleteCategory(_ context.Context, _ string, module string, id uuid.UUID) error {
	if modCats, ok := m.categories[module]; ok {
		if _, ok := modCats[id]; ok {
			delete(m.categories[module], id)
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *mockStore) ListContracts(_ context.Context, _ string) ([]model.Contract, error) {
	out := make([]model.Contract, 0, len(m.contracts))
	for _, c := range m.contracts {
		out = append(out, c)
	}
	return out, nil
}

func (m *mockStore) ListContractsByCategory(_ context.Context, _ string, catID uuid.UUID) ([]model.Contract, error) {
	var out []model.Contract
	for _, c := range m.contracts {
		if c.CategoryID == catID {
			out = append(out, c)
		}
	}
	if out == nil {
		out = []model.Contract{}
	}
	return out, nil
}

func (m *mockStore) GetContract(_ context.Context, _ string, id uuid.UUID) (model.Contract, error) {
	c, ok := m.contracts[id]
	if !ok {
		return c, store.ErrNotFound
	}
	return c, nil
}

func (m *mockStore) CreateContract(_ context.Context, _ string, c model.Contract) error {
	m.contracts[c.ID] = c
	return nil
}

func (m *mockStore) UpdateContract(_ context.Context, _ string, c model.Contract) error {
	if _, ok := m.contracts[c.ID]; !ok {
		return store.ErrNotFound
	}
	m.contracts[c.ID] = c
	return nil
}

func (m *mockStore) DeleteContract(_ context.Context, _ string, id uuid.UUID) error {
	if _, ok := m.contracts[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.contracts, id)
	return nil
}

func (m *mockStore) ListUsers(_ context.Context) ([]model.User, error) {
	out := make([]model.User, 0, len(m.usersById))
	for _, u := range m.usersById {
		out = append(out, u)
	}
	return out, nil
}

func (m *mockStore) Close() error { return nil }

func (m *mockStore) ListPurchases(_ context.Context, _ string) ([]model.Purchase, error) {
	out := make([]model.Purchase, 0, len(m.purchases))
	for _, purchase := range m.purchases {
		out = append(out, purchase)
	}
	return out, nil
}
func (m *mockStore) ListPurchasesByCategory(_ context.Context, _ string, categoryID uuid.UUID) ([]model.Purchase, error) {
	var out []model.Purchase
	for _, purchase := range m.purchases {
		if purchase.CategoryID == categoryID {
			out = append(out, purchase)
		}
	}
	if out == nil {
		out = []model.Purchase{}
	}
	return out, nil
}
func (m *mockStore) GetPurchase(_ context.Context, _ string, id uuid.UUID) (model.Purchase, error) {
	purchase, ok := m.purchases[id]
	if !ok {
		return model.Purchase{}, store.ErrNotFound
	}
	return purchase, nil
}
func (m *mockStore) CreatePurchase(_ context.Context, _ string, purchase model.Purchase) error {
	m.purchases[purchase.ID] = purchase
	return nil
}
func (m *mockStore) UpdatePurchase(_ context.Context, _ string, purchase model.Purchase) error {
	if _, ok := m.purchases[purchase.ID]; !ok {
		return store.ErrNotFound
	}
	m.purchases[purchase.ID] = purchase
	return nil
}
func (m *mockStore) DeletePurchase(_ context.Context, _ string, id uuid.UUID) error {
	if _, ok := m.purchases[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.purchases, id)
	return nil
}

func (m *mockStore) ListVehicles(_ context.Context, _ string) ([]model.Vehicle, error) {
	out := make([]model.Vehicle, 0, len(m.vehicles))
	for _, vehicle := range m.vehicles {
		out = append(out, vehicle)
	}
	return out, nil
}
func (m *mockStore) GetVehicle(_ context.Context, _ string, id uuid.UUID) (model.Vehicle, error) {
	vehicle, ok := m.vehicles[id]
	if !ok {
		return model.Vehicle{}, store.ErrNotFound
	}
	return vehicle, nil
}
func (m *mockStore) CreateVehicle(_ context.Context, _ string, vehicle model.Vehicle) error {
	m.vehicles[vehicle.ID] = vehicle
	return nil
}
func (m *mockStore) UpdateVehicle(_ context.Context, _ string, vehicle model.Vehicle) error {
	if _, ok := m.vehicles[vehicle.ID]; !ok {
		return store.ErrNotFound
	}
	m.vehicles[vehicle.ID] = vehicle
	return nil
}
func (m *mockStore) DeleteVehicle(_ context.Context, _ string, id uuid.UUID) error {
	if _, ok := m.vehicles[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.vehicles, id)
	return nil
}

func (m *mockStore) ListCostEntries(_ context.Context, _ string, _ uuid.UUID) ([]model.CostEntry, error) {
	return nil, nil
}
func (m *mockStore) GetCostEntry(_ context.Context, _ string, _ uuid.UUID) (model.CostEntry, error) {
	return model.CostEntry{}, store.ErrNotFound
}
func (m *mockStore) CreateCostEntry(_ context.Context, _ string, _ model.CostEntry) error { return nil }
func (m *mockStore) UpdateCostEntry(_ context.Context, _ string, _ model.CostEntry) error { return nil }
func (m *mockStore) DeleteCostEntry(_ context.Context, _ string, _ uuid.UUID) error       { return nil }

func (m *mockStore) ListLedgerAccounts(_ context.Context, _ string) ([]model.LedgerAccount, error) {
	out := make([]model.LedgerAccount, 0, len(m.ledgerAccounts))
	for _, a := range m.ledgerAccounts {
		out = append(out, a)
	}
	return out, nil
}
func (m *mockStore) GetLedgerAccount(_ context.Context, _ string, id uuid.UUID) (model.LedgerAccount, error) {
	a, ok := m.ledgerAccounts[id]
	if !ok {
		return model.LedgerAccount{}, store.ErrNotFound
	}
	return a, nil
}
func (m *mockStore) FindLedgerAccountByIBAN(_ context.Context, _ string, _ string) (model.LedgerAccount, error) {
	return model.LedgerAccount{}, store.ErrNotFound
}
func (m *mockStore) CreateLedgerAccount(_ context.Context, _ string, a model.LedgerAccount) error {
	m.ledgerAccounts[a.ID] = a
	return nil
}
func (m *mockStore) ListLedgerCategories(_ context.Context, _ string) ([]model.LedgerCategory, error) {
	out := make([]model.LedgerCategory, 0, len(m.ledgerCategories))
	for _, category := range m.ledgerCategories {
		out = append(out, category)
	}
	return out, nil
}
func (m *mockStore) GetLedgerCategory(_ context.Context, _ string, id uuid.UUID) (model.LedgerCategory, error) {
	category, ok := m.ledgerCategories[id]
	if !ok {
		return model.LedgerCategory{}, store.ErrNotFound
	}
	return category, nil
}
func (m *mockStore) CreateLedgerCategory(_ context.Context, _ string, c model.LedgerCategory) error {
	m.ledgerCategories[c.ID] = c
	return nil
}
func (m *mockStore) UpdateLedgerCategory(_ context.Context, _ string, c model.LedgerCategory) error {
	if _, ok := m.ledgerCategories[c.ID]; !ok {
		return store.ErrNotFound
	}
	m.ledgerCategories[c.ID] = c
	return nil
}
func (m *mockStore) DeleteLedgerCategory(_ context.Context, _ string, id uuid.UUID) error {
	if _, ok := m.ledgerCategories[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.ledgerCategories, id)
	return nil
}
func (m *mockStore) GetLedgerImportByFileHash(_ context.Context, _ string, _ string) (model.LedgerImportBatch, error) {
	return model.LedgerImportBatch{}, store.ErrNotFound
}
func (m *mockStore) LedgerTransactionFingerprintExists(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}
func (m *mockStore) CommitLedgerImport(_ context.Context, _ string, batch model.LedgerImportBatch, txns []model.LedgerTransaction) (store.LedgerImportCommitResult, error) {
	m.ledgerImports = append(m.ledgerImports, batch)
	for _, txn := range txns {
		m.ledgerTransactions[txn.AccountID] = append(m.ledgerTransactions[txn.AccountID], txn)
	}
	return store.LedgerImportCommitResult{ImportedRows: len(txns)}, nil
}
func (m *mockStore) ListLedgerImports(_ context.Context, _ string) ([]model.LedgerImportBatch, error) {
	return append([]model.LedgerImportBatch(nil), m.ledgerImports...), nil
}
func (m *mockStore) ListLedgerTransactions(_ context.Context, _ string, accountID uuid.UUID) ([]model.LedgerTransaction, error) {
	return append([]model.LedgerTransaction(nil), m.ledgerTransactions[accountID]...), nil
}
func (m *mockStore) ListLedgerTransactionsPage(_ context.Context, _ string, accountID uuid.UUID, limit int, cursor string) (store.LedgerTransactionPage, error) {
	items := append([]model.LedgerTransaction(nil), m.ledgerTransactions[accountID]...)
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	start := 0
	if cursor != "" {
		for i, txn := range items {
			if txn.ID.String() == cursor {
				start = i + 1
				break
			}
		}
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := store.LedgerTransactionPage{Items: items[start:end]}
	if end < len(items) {
		page.NextCursor = items[end-1].ID.String()
	}
	return page, nil
}
func (m *mockStore) ListLedgerTransactionsFiltered(_ context.Context, _ string, options store.LedgerTransactionListOptions) (store.LedgerTransactionPage, error) {
	var items []model.LedgerTransaction
	for accountID, txns := range m.ledgerTransactions {
		if options.AccountID != nil && accountID != *options.AccountID {
			continue
		}
		for _, txn := range txns {
			if options.ReviewStatus != "" && txn.ReviewStatus != options.ReviewStatus {
				continue
			}
			items = append(items, txn)
		}
	}
	if options.Limit <= 0 || options.Limit > len(items) {
		options.Limit = len(items)
	}
	start := 0
	if options.Cursor != "" {
		for i, txn := range items {
			if txn.ID.String() == options.Cursor {
				start = i + 1
				break
			}
		}
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + options.Limit
	if end > len(items) {
		end = len(items)
	}
	page := store.LedgerTransactionPage{Items: items[start:end]}
	if end < len(items) && end > start {
		page.NextCursor = items[end-1].ID.String()
	}
	return page, nil
}
func (m *mockStore) GetLedgerTransaction(_ context.Context, _ string, id uuid.UUID) (model.LedgerTransaction, error) {
	for _, txns := range m.ledgerTransactions {
		for _, txn := range txns {
			if txn.ID == id {
				return txn, nil
			}
		}
	}
	return model.LedgerTransaction{}, store.ErrNotFound
}
func (m *mockStore) ListLedgerTransferCandidates(_ context.Context, _ string, id uuid.UUID) (store.LedgerTransferCandidatesResult, error) {
	source, err := m.GetLedgerTransaction(context.Background(), "", id)
	if err != nil {
		return store.LedgerTransferCandidatesResult{}, err
	}
	accountNameByID := make(map[uuid.UUID]string, len(m.ledgerAccounts))
	for _, account := range m.ledgerAccounts {
		accountNameByID[account.ID] = account.Name
	}
	var items []model.LedgerTransferCandidate
	for _, txns := range m.ledgerTransactions {
		for _, txn := range txns {
			if txn.ID == source.ID || txn.AccountID == source.AccountID {
				continue
			}
			if txn.Currency != source.Currency || txn.AmountMinor != -source.AmountMinor {
				continue
			}
			items = append(items, model.LedgerTransferCandidate{Transaction: txn, AccountName: accountNameByID[txn.AccountID]})
		}
	}
	return store.LedgerTransferCandidatesResult{Items: items}, nil
}
func (m *mockStore) LinkLedgerTransfer(_ context.Context, _ string, id uuid.UUID, input model.LedgerTransferLinkInput) (model.LedgerTransferLinkResult, error) {
	var result model.LedgerTransferLinkResult
	var leftFound bool
	var rightFound bool
	for accountID, txns := range m.ledgerTransactions {
		for i, txn := range txns {
			switch txn.ID {
			case id:
				pairID := input.PairedTransactionID
				txn.TransferPairTransactionID = &pairID
				txn.SpecialCategory = model.LedgerSpecialCategoryInternalTransfer
				m.ledgerTransactions[accountID][i] = txn
				result.Transaction = txn
				leftFound = true
			case input.PairedTransactionID:
				pairID := id
				txn.TransferPairTransactionID = &pairID
				txn.SpecialCategory = model.LedgerSpecialCategoryInternalTransfer
				m.ledgerTransactions[accountID][i] = txn
				result.PairedTransaction = txn
				rightFound = true
			}
		}
	}
	if !leftFound || !rightFound {
		return model.LedgerTransferLinkResult{}, store.ErrNotFound
	}
	return result, nil
}
func (m *mockStore) UnlinkLedgerTransfer(_ context.Context, _ string, id uuid.UUID) (store.LedgerTransferLinkResult, error) {
	var result store.LedgerTransferLinkResult
	var pairID *uuid.UUID
	for accountID, txns := range m.ledgerTransactions {
		for i, txn := range txns {
			if txn.ID != id {
				continue
			}
			pairID = txn.TransferPairTransactionID
			txn.TransferPairTransactionID = nil
			txn.SpecialCategory = ""
			m.ledgerTransactions[accountID][i] = txn
			result.Transaction = txn
		}
	}
	if pairID != nil {
		for accountID, txns := range m.ledgerTransactions {
			for i, txn := range txns {
				if txn.ID != *pairID {
					continue
				}
				txn.TransferPairTransactionID = nil
				txn.SpecialCategory = ""
				m.ledgerTransactions[accountID][i] = txn
				copyTxn := txn
				result.PairedTransaction = &copyTxn
			}
		}
	}
	return result, nil
}
func (m *mockStore) UpdateLedgerTransactionDetails(_ context.Context, _ string, id uuid.UUID, input model.LedgerTransactionDetailsInput) (model.LedgerTransaction, error) {
	for accountID, txns := range m.ledgerTransactions {
		for i, txn := range txns {
			if txn.ID != id {
				continue
			}
			txn.Note = input.Note
			txn.Links = append([]string(nil), input.Links...)
			txn.References = append([]model.LedgerTransactionReference(nil), input.References...)
			m.ledgerTransactions[accountID][i] = txn
			return txn, nil
		}
	}
	return model.LedgerTransaction{}, store.ErrNotFound
}
func (m *mockStore) ReviewLedgerTransaction(_ context.Context, _ string, id uuid.UUID, input model.LedgerTransactionReviewInput) (store.LedgerReviewResult, error) {
	for accountID, txns := range m.ledgerTransactions {
		for i, txn := range txns {
			if txn.ID != id {
				continue
			}
			if input.CategoryID != nil && txn.TransferPairTransactionID != nil {
				return store.LedgerReviewResult{}, store.ErrLedgerTransferLinked
			}
			if input.CategoryID != nil {
				categoryID := *input.CategoryID
				txn.CategoryID = &categoryID
			}
			txn.ReviewStatus = model.LedgerTransactionReviewConfirmed
			txn.CategorizationSource = model.LedgerCategorizationManual
			m.ledgerTransactions[accountID][i] = txn
			result := store.LedgerReviewResult{Transaction: txn}
			if txn.CategoryID != nil {
				if category, ok := m.ledgerCategories[*txn.CategoryID]; ok {
					result.Category = &category
				}
			}
			return result, nil
		}
	}
	return store.LedgerReviewResult{}, store.ErrNotFound
}

func (m *mockStore) ListLedgerEmailAccounts(_ context.Context, _ string) ([]model.LedgerEmailAccount, error) {
	out := make([]model.LedgerEmailAccount, 0, len(m.ledgerEmailAccounts))
	for _, item := range m.ledgerEmailAccounts {
		out = append(out, item)
	}
	return out, nil
}

func (m *mockStore) GetLedgerEmailAccount(_ context.Context, _ string, id uuid.UUID) (model.LedgerEmailAccount, error) {
	item, ok := m.ledgerEmailAccounts[id]
	if !ok {
		return model.LedgerEmailAccount{}, store.ErrNotFound
	}
	return item, nil
}

func (m *mockStore) CreateLedgerEmailAccount(_ context.Context, _ string, account model.LedgerEmailAccount) error {
	m.ledgerEmailAccounts[account.ID] = account
	return nil
}

func (m *mockStore) UpdateLedgerEmailAccount(_ context.Context, _ string, account model.LedgerEmailAccount) error {
	m.ledgerEmailAccounts[account.ID] = account
	return nil
}

func (m *mockStore) DeleteLedgerEmailAccount(_ context.Context, _ string, id uuid.UUID) error {
	delete(m.ledgerEmailAccounts, id)
	return nil
}

func (m *mockStore) ListLedgerEmailOrders(_ context.Context, _ string) ([]model.LedgerEmailOrder, error) {
	out := make([]model.LedgerEmailOrder, 0, len(m.ledgerEmailOrders))
	for _, item := range m.ledgerEmailOrders {
		out = append(out, item)
	}
	return out, nil
}

func (m *mockStore) ListLedgerEmailOrdersByAccount(_ context.Context, _ string, accountID uuid.UUID) ([]model.LedgerEmailOrder, error) {
	out := make([]model.LedgerEmailOrder, 0)
	for _, item := range m.ledgerEmailOrders {
		if item.EmailAccountID == accountID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *mockStore) ListLedgerEmailOrdersByTransaction(_ context.Context, _ string, transactionID uuid.UUID) ([]model.LedgerEmailOrder, error) {
	out := make([]model.LedgerEmailOrder, 0)
	for _, item := range m.ledgerEmailOrders {
		for _, linkedID := range item.LinkedTransactionIDs {
			if linkedID == transactionID {
				out = append(out, item)
				break
			}
		}
	}
	return out, nil
}

func (m *mockStore) GetLedgerEmailOrder(_ context.Context, _ string, id uuid.UUID) (model.LedgerEmailOrder, error) {
	item, ok := m.ledgerEmailOrders[id]
	if !ok {
		return model.LedgerEmailOrder{}, store.ErrNotFound
	}
	return item, nil
}

func (m *mockStore) GetLedgerEmailOrderByMessageID(_ context.Context, _ string, messageID string) (model.LedgerEmailOrder, error) {
	for _, item := range m.ledgerEmailOrders {
		if item.EmailMessageID == messageID {
			return item, nil
		}
	}
	return model.LedgerEmailOrder{}, store.ErrNotFound
}

func (m *mockStore) CreateLedgerEmailOrder(_ context.Context, _ string, order model.LedgerEmailOrder) error {
	m.ledgerEmailOrders[order.ID] = order
	return nil
}

func (m *mockStore) LinkLedgerEmailOrder(_ context.Context, _ string, id uuid.UUID, input model.LedgerEmailOrderLinkInput) (model.LedgerEmailOrder, error) {
	order, ok := m.ledgerEmailOrders[id]
	if !ok {
		return model.LedgerEmailOrder{}, store.ErrNotFound
	}
	order.LinkedTransactionIDs = append([]uuid.UUID(nil), input.TransactionIDs...)
	order.MatchStatus = model.LedgerEmailOrderStatusMatched
	m.ledgerEmailOrders[id] = order
	return order, nil
}

func (m *mockStore) RejectLedgerEmailOrder(_ context.Context, _ string, id uuid.UUID) (model.LedgerEmailOrder, error) {
	order, ok := m.ledgerEmailOrders[id]
	if !ok {
		return model.LedgerEmailOrder{}, store.ErrNotFound
	}
	order.MatchStatus = model.LedgerEmailOrderStatusRejected
	order.LinkedTransactionIDs = nil
	m.ledgerEmailOrders[id] = order
	return order, nil
}

var testJWTSecret = []byte("test-secret-key")

const testUserID = "00000000-0000-0000-0000-000000000001"

func newTestHandler() (*Handler, *mockStore) {
	ms := newMockStore()
	h := New(ms, slog.Default(), testJWTSecret, nil)
	return h, ms
}

func newMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/ledger/imports/preview", h.LedgerImportPreview)
	mux.HandleFunc("GET /api/v1/ledger/categories", h.ListLedgerCategories)
	mux.HandleFunc("POST /api/v1/ledger/categories", h.CreateLedgerCategory)
	mux.HandleFunc("GET /api/v1/ledger/categories/{id}", h.GetLedgerCategory)
	mux.HandleFunc("PUT /api/v1/ledger/categories/{id}", h.UpdateLedgerCategory)
	mux.HandleFunc("DELETE /api/v1/ledger/categories/{id}", h.DeleteLedgerCategory)
	mux.HandleFunc("GET /api/v1/ledger/accounts", h.ListLedgerAccounts)
	mux.HandleFunc("GET /api/v1/ledger/accounts/{accountId}", h.GetLedgerAccount)
	mux.HandleFunc("GET /api/v1/ledger/accounts/{accountId}/transactions", h.ListLedgerTransactions)
	mux.HandleFunc("GET /api/v1/ledger/imports", h.ListLedgerImports)
	mux.HandleFunc("GET /api/v1/ledger/transactions", h.ListLedgerTransactionsReviewQueue)
	mux.HandleFunc("GET /api/v1/ledger/transactions/{transactionId}", h.GetLedgerTransaction)
	mux.HandleFunc("PUT /api/v1/ledger/transactions/{transactionId}", h.UpdateLedgerTransactionDetails)
	mux.HandleFunc("GET /api/v1/ledger/transactions/{transactionId}/transfer-candidates", h.ListLedgerTransferCandidates)
	mux.HandleFunc("POST /api/v1/ledger/transactions/{transactionId}/transfer-link", h.LinkLedgerTransfer)
	mux.HandleFunc("DELETE /api/v1/ledger/transactions/{transactionId}/transfer-link", h.UnlinkLedgerTransfer)
	mux.HandleFunc("POST /api/v1/ledger/transactions/{transactionId}/review", h.ReviewLedgerTransaction)
	// Inject test user into context for all requests
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

func multipartBody(t *testing.T, fields map[string]string, fileField string, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return v
}


func TestGetLedgerAccount_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	account := model.LedgerAccount{
		ID:       uuid.New(),
		Name:     "Main account",
		Bank:     "DKB",
		Currency: "EUR",
	}
	ms.ledgerAccounts[account.ID] = account

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+account.ID.String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got := decodeJSON[model.LedgerAccount](t, rec)
	if got.ID != account.ID {
		t.Fatalf("ID = %s, want %s", got.ID, account.ID)
	}
}

func TestLedgerImportPreview_UsesCamelCaseRowFields(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	csv := []byte("\xEF\xBB\xBF\"Girokonto\";\"DE12345678901234567890\"\n\"\"\n\"Kontostand vom DD.MM.YYYY:\";\"111.111,11 €\"\n\"\"\n\"Buchungsdatum\";\"Wertstellung\";\"Status\";\"Zahlungspflichtige*r\";\"Zahlungsempfänger*in\";\"Verwendungszweck\";\"Umsatztyp\";\"IBAN\";\"Betrag (€)\";\"Gläubiger-ID\";\"Mandatsreferenz\";\"Kundenreferenz\"\n\"07.04.26\";\"02.04.26\";\"Gebucht\";\"DKB AG\";\"Mustermann,Fred\";\"Depot 0123 Wertpapierertrag\";\"Eingang\";\"0000000000\";\"800,23\";\"\";\"\";\"\"\n")
	body, contentType := multipartBody(t, map[string]string{"sourceType": "dkb.csv"}, "file", "dkb.csv", csv)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/imports/preview", body)
	req.Header.Set("Content-Type", contentType)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode preview response: %v", err)
	}
	transactions, ok := raw["transactions"].([]any)
	if !ok || len(transactions) != 1 {
		t.Fatalf("transactions malformed: %#v", raw["transactions"])
	}
	first, ok := transactions[0].(map[string]any)
	if !ok {
		t.Fatalf("transaction malformed: %#v", transactions[0])
	}
	row, ok := first["row"].(map[string]any)
	if !ok {
		t.Fatalf("row malformed: %#v", first["row"])
	}

	if row["bookingDate"] != "2026-04-07" {
		t.Fatalf("bookingDate = %#v, want %q", row["bookingDate"], "2026-04-07")
	}
	if row["valueDate"] != "2026-04-02" {
		t.Fatalf("valueDate = %#v, want %q", row["valueDate"], "2026-04-02")
	}
	if row["amountMinor"] != float64(80023) {
		t.Fatalf("amountMinor = %#v, want %v", row["amountMinor"], float64(80023))
	}
	if row["counterpartyName"] != "Mustermann,Fred" {
		t.Fatalf("counterpartyName = %#v, want %q", row["counterpartyName"], "Mustermann,Fred")
	}
	if row["purpose"] != "Depot 0123 Wertpapierertrag" {
		t.Fatalf("purpose = %#v, want %q", row["purpose"], "Depot 0123 Wertpapierertrag")
	}
	if first["reviewStatus"] != model.LedgerTransactionReviewNeedsReview {
		t.Fatalf("reviewStatus = %#v, want %q", first["reviewStatus"], model.LedgerTransactionReviewNeedsReview)
	}
}

func TestCreateLedgerCategory_Success(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/categories", jsonBody(map[string]any{"name": "Food", "matchWords": []string{"rewe"}}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	category := decodeJSON[model.LedgerCategory](t, rec)
	if category.Name != "Food" {
		t.Fatalf("name = %q, want Food", category.Name)
	}
	if len(category.MatchWords) != 1 || category.MatchWords[0] != "rewe" {
		t.Fatalf("matchWords = %#v", category.MatchWords)
	}
}

func TestLedgerReviewQueue_OnlyNeedsReview(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	accountID := uuid.New()
	ms.ledgerAccounts[accountID] = model.LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR"}
	ms.ledgerTransactions[accountID] = []model.LedgerTransaction{
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-03", Currency: "EUR", ReviewStatus: model.LedgerTransactionReviewNeedsReview},
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR", ReviewStatus: model.LedgerTransactionReviewConfirmed},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/transactions?limit=10", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	page := decodeJSON[struct {
		Items []model.LedgerTransaction `json:"items"`
	}](t, rec)
	if len(page.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(page.Items))
	}
	if page.Items[0].ReviewStatus != model.LedgerTransactionReviewNeedsReview {
		t.Fatalf("reviewStatus = %q", page.Items[0].ReviewStatus)
	}
}

func TestReviewLedgerTransaction_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	accountID := uuid.New()
	categoryID := uuid.New()
	txnID := uuid.New()
	ms.ledgerAccounts[accountID] = model.LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR"}
	ms.ledgerCategories[categoryID] = model.LedgerCategory{ID: categoryID, Name: "Food"}
	ms.ledgerTransactions[accountID] = []model.LedgerTransaction{{
		ID:           txnID,
		AccountID:    accountID,
		BookingDate:  "2026-04-03",
		Currency:     "EUR",
		ReviewStatus: model.LedgerTransactionReviewNeedsReview,
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/transactions/"+txnID.String()+"/review", jsonBody(map[string]any{"categoryId": categoryID.String()}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	response := decodeJSON[struct {
		Transaction model.LedgerTransaction `json:"transaction"`
		Category    *model.LedgerCategory   `json:"category"`
	}](t, rec)
	if response.Transaction.ReviewStatus != model.LedgerTransactionReviewConfirmed {
		t.Fatalf("reviewStatus = %q", response.Transaction.ReviewStatus)
	}
	if response.Transaction.CategoryID == nil || *response.Transaction.CategoryID != categoryID {
		t.Fatalf("categoryId = %#v", response.Transaction.CategoryID)
	}
}

func TestUpdateLedgerTransactionDetails_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	accountID := uuid.New()
	transactionID := uuid.New()
	purchaseID := uuid.New()
	ms.ledgerTransactions[accountID] = []model.LedgerTransaction{{
		ID:            transactionID,
		AccountID:     accountID,
		BookingDate:   "2026-04-01",
		Currency:      "EUR",
		Fingerprint:   "fp-details",
		ImportBatchID: uuid.New(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}}
	ms.purchases[purchaseID] = model.Purchase{ID: purchaseID, CategoryID: uuid.New(), ItemName: "Desk", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/ledger/transactions/"+transactionID.String(), jsonBody(map[string]any{
		"note":       "linked invoice",
		"links":      []string{"https://example.com/invoice.pdf"},
		"references": []map[string]string{{"type": model.LedgerReferencePurchase, "targetId": purchaseID.String()}},
	}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response model.LedgerTransaction
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Note != "linked invoice" {
		t.Fatalf("note = %q", response.Note)
	}
	if len(response.Links) != 1 || response.Links[0] != "https://example.com/invoice.pdf" {
		t.Fatalf("links = %#v", response.Links)
	}
	if len(response.References) != 1 || response.References[0].TargetID != purchaseID {
		t.Fatalf("references = %#v", response.References)
	}
}

func TestUpdateLedgerTransactionDetails_InvalidURL(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/ledger/transactions/"+uuid.New().String(), jsonBody(map[string]any{
		"links": []string{"notaurl"},
	}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReviewLedgerTransaction_LinkedTransferRequiresExplicitUnlink(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	accountA := uuid.New()
	accountB := uuid.New()
	leftID := uuid.New()
	rightID := uuid.New()
	categoryID := uuid.New()
	ms.ledgerAccounts[accountA] = model.LedgerAccount{ID: accountA, Name: "Checking"}
	ms.ledgerAccounts[accountB] = model.LedgerAccount{ID: accountB, Name: "Savings"}
	ms.ledgerCategories[categoryID] = model.LedgerCategory{ID: categoryID, Name: "Household"}
	ms.ledgerTransactions[accountA] = []model.LedgerTransaction{{ID: leftID, AccountID: accountA, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR", TransferPairTransactionID: &rightID, SpecialCategory: model.LedgerSpecialCategoryInternalTransfer}}
	ms.ledgerTransactions[accountB] = []model.LedgerTransaction{{ID: rightID, AccountID: accountB, BookingDate: "2026-04-02", AmountMinor: 1000, Currency: "EUR", TransferPairTransactionID: &leftID, SpecialCategory: model.LedgerSpecialCategoryInternalTransfer}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/transactions/"+leftID.String()+"/review", jsonBody(map[string]string{"categoryId": categoryID.String()}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestLinkLedgerTransfer_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	accountA := uuid.New()
	accountB := uuid.New()
	leftID := uuid.New()
	rightID := uuid.New()
	ms.ledgerAccounts[accountA] = model.LedgerAccount{ID: accountA, Name: "Checking"}
	ms.ledgerAccounts[accountB] = model.LedgerAccount{ID: accountB, Name: "Savings"}
	ms.ledgerTransactions[accountA] = []model.LedgerTransaction{{ID: leftID, AccountID: accountA, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR"}}
	ms.ledgerTransactions[accountB] = []model.LedgerTransaction{{ID: rightID, AccountID: accountB, BookingDate: "2026-04-02", AmountMinor: 1000, Currency: "EUR"}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/transactions/"+leftID.String()+"/transfer-link", jsonBody(map[string]string{"pairedTransactionId": rightID.String()}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestListLedgerTransferCandidates_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	accountA := uuid.New()
	accountB := uuid.New()
	sourceID := uuid.New()
	candidateID := uuid.New()
	ms.ledgerAccounts[accountA] = model.LedgerAccount{ID: accountA, Name: "Checking"}
	ms.ledgerAccounts[accountB] = model.LedgerAccount{ID: accountB, Name: "Savings"}
	ms.ledgerTransactions[accountA] = []model.LedgerTransaction{{ID: sourceID, AccountID: accountA, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR"}}
	ms.ledgerTransactions[accountB] = []model.LedgerTransaction{{ID: candidateID, AccountID: accountB, BookingDate: "2026-04-02", AmountMinor: 1000, Currency: "EUR"}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/transactions/"+sourceID.String()+"/transfer-candidates", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetLedgerAccount_InvalidUUID(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/accounts/not-a-uuid", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListLedgerTransactions_Paginated(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	accountID := uuid.New()
	ms.ledgerAccounts[accountID] = model.LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR"}
	firstID := uuid.New()
	secondID := uuid.New()
	thirdID := uuid.New()
	ms.ledgerTransactions[accountID] = []model.LedgerTransaction{
		{ID: firstID, AccountID: accountID, BookingDate: "2026-04-03", Currency: "EUR"},
		{ID: secondID, AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR"},
		{ID: thirdID, AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR"},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+accountID.String()+"/transactions?limit=2", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var page struct {
		Items      []model.LedgerTransaction `json:"items"`
		NextCursor string                    `json:"nextCursor"`
	}
	page = decodeJSON[struct {
		Items      []model.LedgerTransaction `json:"items"`
		NextCursor string                    `json:"nextCursor"`
	}](t, rec)
	if len(page.Items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(page.Items))
	}
	if page.NextCursor == "" {
		t.Fatal("expected nextCursor")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+accountID.String()+"/transactions?limit=2&cursor="+page.NextCursor, nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second page status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	page = decodeJSON[struct {
		Items      []model.LedgerTransaction `json:"items"`
		NextCursor string                    `json:"nextCursor"`
	}](t, rec)
	if len(page.Items) != 1 {
		t.Fatalf("second page len(items) = %d, want 1", len(page.Items))
	}
}

func TestListLedgerTransactions_InvalidLimit(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	accountID := uuid.New()
	ms.ledgerAccounts[accountID] = model.LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+accountID.String()+"/transactions?limit=abc", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
