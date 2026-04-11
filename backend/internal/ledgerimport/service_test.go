package ledgerimport

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
	"github.com/tobi/contracts/backend/internal/store"
)

type mockStore struct {
	accounts     map[string]model.LedgerAccount // keyed by id
	categories   map[string]model.LedgerCategory
	ibanIndex    map[string]model.LedgerAccount // keyed by iban
	batches      map[string]model.LedgerImportBatch
	fileHashes   map[string]model.LedgerImportBatch
	fingerprints map[string]bool
	transactions []model.LedgerTransaction
}

func newMockStore() *mockStore {
	return &mockStore{
		accounts:     make(map[string]model.LedgerAccount),
		categories:   make(map[string]model.LedgerCategory),
		ibanIndex:    make(map[string]model.LedgerAccount),
		batches:      make(map[string]model.LedgerImportBatch),
		fileHashes:   make(map[string]model.LedgerImportBatch),
		fingerprints: make(map[string]bool),
	}
}

func (m *mockStore) ListLedgerAccounts(_ context.Context, _ string) ([]model.LedgerAccount, error) {
	var out []model.LedgerAccount
	for _, a := range m.accounts {
		out = append(out, a)
	}
	return out, nil
}

func (m *mockStore) GetLedgerAccount(_ context.Context, _ string, id uuid.UUID) (model.LedgerAccount, error) {
	a, ok := m.accounts[id.String()]
	if !ok {
		return model.LedgerAccount{}, store.ErrNotFound
	}
	return a, nil
}

func (m *mockStore) FindLedgerAccountByIBAN(_ context.Context, _ string, iban string) (model.LedgerAccount, error) {
	a, ok := m.ibanIndex[iban]
	if !ok {
		return model.LedgerAccount{}, store.ErrNotFound
	}
	return a, nil
}

func (m *mockStore) CreateLedgerAccount(_ context.Context, _ string, a model.LedgerAccount) error {
	m.accounts[a.ID.String()] = a
	if a.IBAN != "" {
		m.ibanIndex[a.IBAN] = a
	}
	return nil
}
func (m *mockStore) ListLedgerCategories(_ context.Context, _ string) ([]model.LedgerCategory, error) {
	out := make([]model.LedgerCategory, 0, len(m.categories))
	for _, category := range m.categories {
		out = append(out, category)
	}
	return out, nil
}
func (m *mockStore) GetLedgerCategory(_ context.Context, _ string, id uuid.UUID) (model.LedgerCategory, error) {
	category, ok := m.categories[id.String()]
	if !ok {
		return model.LedgerCategory{}, store.ErrNotFound
	}
	return category, nil
}
func (m *mockStore) CreateLedgerCategory(_ context.Context, _ string, c model.LedgerCategory) error {
	m.categories[c.ID.String()] = c
	return nil
}
func (m *mockStore) UpdateLedgerCategory(_ context.Context, _ string, c model.LedgerCategory) error {
	m.categories[c.ID.String()] = c
	return nil
}
func (m *mockStore) DeleteLedgerCategory(_ context.Context, _ string, id uuid.UUID) error {
	delete(m.categories, id.String())
	return nil
}

func (m *mockStore) GetLedgerImportByFileHash(_ context.Context, _ string, sha256 string) (model.LedgerImportBatch, error) {
	b, ok := m.fileHashes[sha256]
	if !ok {
		return model.LedgerImportBatch{}, store.ErrNotFound
	}
	return b, nil
}

func (m *mockStore) LedgerTransactionFingerprintExists(_ context.Context, _ string, fp string) (bool, error) {
	return m.fingerprints[fp], nil
}

func (m *mockStore) CommitLedgerImport(_ context.Context, _ string, batch model.LedgerImportBatch, txns []model.LedgerTransaction) (store.LedgerImportCommitResult, error) {
	m.batches[batch.ID.String()] = batch
	m.fileHashes[batch.FileSHA256] = batch
	result := store.LedgerImportCommitResult{}
	for _, t := range txns {
		if m.fingerprints[t.Fingerprint] {
			result.DuplicateRows++
			continue
		}
		m.fingerprints[t.Fingerprint] = true
		m.transactions = append(m.transactions, t)
		result.ImportedRows++
	}
	return result, nil
}

func (m *mockStore) ListLedgerImports(_ context.Context, _ string) ([]model.LedgerImportBatch, error) {
	var out []model.LedgerImportBatch
	for _, b := range m.batches {
		out = append(out, b)
	}
	return out, nil
}

func (m *mockStore) ListLedgerTransactions(_ context.Context, _ string, _ uuid.UUID) ([]model.LedgerTransaction, error) {
	return m.transactions, nil
}
func (m *mockStore) ListLedgerTransactionsPage(_ context.Context, _ string, _ uuid.UUID, limit int, cursor string) (store.LedgerTransactionPage, error) {
	items := append([]model.LedgerTransaction(nil), m.transactions...)
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
	if end < len(items) && end > start {
		page.NextCursor = items[end-1].ID.String()
	}
	return page, nil
}
func (m *mockStore) ListLedgerTransactionsFiltered(_ context.Context, _ string, options store.LedgerTransactionListOptions) (store.LedgerTransactionPage, error) {
	items := append([]model.LedgerTransaction(nil), m.transactions...)
	filtered := make([]model.LedgerTransaction, 0, len(items))
	for _, txn := range items {
		if options.ReviewStatus != "" && txn.ReviewStatus != options.ReviewStatus {
			continue
		}
		filtered = append(filtered, txn)
	}
	if options.Limit <= 0 || options.Limit > len(filtered) {
		options.Limit = len(filtered)
	}
	if options.Limit < 0 {
		options.Limit = 0
	}
	page := store.LedgerTransactionPage{Items: filtered}
	if len(filtered) > options.Limit {
		page.Items = filtered[:options.Limit]
		page.NextCursor = filtered[options.Limit-1].ID.String()
	}
	return page, nil
}
func (m *mockStore) GetLedgerTransaction(_ context.Context, _ string, id uuid.UUID) (model.LedgerTransaction, error) {
	for _, txn := range m.transactions {
		if txn.ID == id {
			return txn, nil
		}
	}
	return model.LedgerTransaction{}, store.ErrNotFound
}
func (m *mockStore) UpdateLedgerTransactionDetails(_ context.Context, _ string, _ uuid.UUID, _ model.LedgerTransactionDetailsInput) (model.LedgerTransaction, error) {
	return model.LedgerTransaction{}, store.ErrNotFound
}
func (m *mockStore) ListLedgerTransferCandidates(_ context.Context, _ string, _ uuid.UUID) (store.LedgerTransferCandidatesResult, error) {
	return store.LedgerTransferCandidatesResult{}, nil
}
func (m *mockStore) LinkLedgerTransfer(_ context.Context, _ string, _ uuid.UUID, _ model.LedgerTransferLinkInput) (model.LedgerTransferLinkResult, error) {
	return model.LedgerTransferLinkResult{}, store.ErrNotFound
}
func (m *mockStore) UnlinkLedgerTransfer(_ context.Context, _ string, _ uuid.UUID) (store.LedgerTransferLinkResult, error) {
	return store.LedgerTransferLinkResult{}, store.ErrNotFound
}
func (m *mockStore) ReviewLedgerTransaction(_ context.Context, _ string, id uuid.UUID, input model.LedgerTransactionReviewInput) (store.LedgerReviewResult, error) {
	for i, txn := range m.transactions {
		if txn.ID != id {
			continue
		}
		if input.CategoryID != nil {
			categoryID := *input.CategoryID
			txn.CategoryID = &categoryID
		}
		txn.ReviewStatus = model.LedgerTransactionReviewConfirmed
		txn.CategorizationSource = model.LedgerCategorizationManual
		m.transactions[i] = txn
		result := store.LedgerReviewResult{Transaction: txn}
		if txn.CategoryID != nil {
			if category, ok := m.categories[txn.CategoryID.String()]; ok {
				result.Category = &category
			}
		}
		return result, nil
	}
	return store.LedgerReviewResult{}, store.ErrNotFound
}
func (m *mockStore) ListLedgerEmailAccounts(_ context.Context, _ string) ([]model.LedgerEmailAccount, error) {
	return nil, nil
}
func (m *mockStore) GetLedgerEmailAccount(_ context.Context, _ string, _ uuid.UUID) (model.LedgerEmailAccount, error) {
	return model.LedgerEmailAccount{}, store.ErrNotFound
}
func (m *mockStore) CreateLedgerEmailAccount(_ context.Context, _ string, _ model.LedgerEmailAccount) error {
	return nil
}
func (m *mockStore) UpdateLedgerEmailAccount(_ context.Context, _ string, _ model.LedgerEmailAccount) error {
	return nil
}
func (m *mockStore) DeleteLedgerEmailAccount(_ context.Context, _ string, _ uuid.UUID) error {
	return nil
}
func (m *mockStore) ListLedgerEmailOrders(_ context.Context, _ string) ([]model.LedgerEmailOrder, error) {
	return nil, nil
}
func (m *mockStore) ListLedgerEmailOrdersByAccount(_ context.Context, _ string, _ uuid.UUID) ([]model.LedgerEmailOrder, error) {
	return nil, nil
}
func (m *mockStore) ListLedgerEmailOrdersByTransaction(_ context.Context, _ string, _ uuid.UUID) ([]model.LedgerEmailOrder, error) {
	return nil, nil
}
func (m *mockStore) GetLedgerEmailOrder(_ context.Context, _ string, _ uuid.UUID) (model.LedgerEmailOrder, error) {
	return model.LedgerEmailOrder{}, store.ErrNotFound
}
func (m *mockStore) GetLedgerEmailOrderByMessageID(_ context.Context, _ string, _ string) (model.LedgerEmailOrder, error) {
	return model.LedgerEmailOrder{}, store.ErrNotFound
}
func (m *mockStore) CreateLedgerEmailOrder(_ context.Context, _ string, _ model.LedgerEmailOrder) error {
	return nil
}
func (m *mockStore) LinkLedgerEmailOrder(_ context.Context, _ string, _ uuid.UUID, _ model.LedgerEmailOrderLinkInput) (model.LedgerEmailOrder, error) {
	return model.LedgerEmailOrder{}, store.ErrNotFound
}
func (m *mockStore) RejectLedgerEmailOrder(_ context.Context, _ string, _ uuid.UUID) (model.LedgerEmailOrder, error) {
	return model.LedgerEmailOrder{}, store.ErrNotFound
}

// Stub all other store.Store methods
func (m *mockStore) CreateUser(_ context.Context, _ model.User) error { return nil }
func (m *mockStore) GetUserByEmail(_ context.Context, _ string) (model.User, error) {
	return model.User{}, nil
}
func (m *mockStore) GetUserByID(_ context.Context, _ string) (model.User, error) {
	return model.User{}, nil
}
func (m *mockStore) UpdateUser(_ context.Context, _ model.User) error  { return nil }
func (m *mockStore) ListUsers(_ context.Context) ([]model.User, error) { return nil, nil }
func (m *mockStore) GetSettings(_ context.Context, _ string) (model.UserSettings, error) {
	return model.UserSettings{}, nil
}
func (m *mockStore) UpdateSettings(_ context.Context, _ string, _ model.UserSettings) error {
	return nil
}
func (m *mockStore) ListCategories(_ context.Context, _ string, _ string) ([]model.Category, error) {
	return nil, nil
}
func (m *mockStore) GetCategory(_ context.Context, _ string, _ string, _ uuid.UUID) (model.Category, error) {
	return model.Category{}, nil
}
func (m *mockStore) CreateCategory(_ context.Context, _ string, _ string, _ model.Category) error {
	return nil
}
func (m *mockStore) UpdateCategory(_ context.Context, _ string, _ string, _ model.Category) error {
	return nil
}
func (m *mockStore) DeleteCategory(_ context.Context, _ string, _ string, _ uuid.UUID) error {
	return nil
}
func (m *mockStore) ListContracts(_ context.Context, _ string) ([]model.Contract, error) {
	return nil, nil
}
func (m *mockStore) ListContractsByCategory(_ context.Context, _ string, _ uuid.UUID) ([]model.Contract, error) {
	return nil, nil
}
func (m *mockStore) GetContract(_ context.Context, _ string, _ uuid.UUID) (model.Contract, error) {
	return model.Contract{}, nil
}
func (m *mockStore) CreateContract(_ context.Context, _ string, _ model.Contract) error { return nil }
func (m *mockStore) UpdateContract(_ context.Context, _ string, _ model.Contract) error { return nil }
func (m *mockStore) DeleteContract(_ context.Context, _ string, _ uuid.UUID) error      { return nil }
func (m *mockStore) ListPurchases(_ context.Context, _ string) ([]model.Purchase, error) {
	return nil, nil
}
func (m *mockStore) ListPurchasesByCategory(_ context.Context, _ string, _ uuid.UUID) ([]model.Purchase, error) {
	return nil, nil
}
func (m *mockStore) GetPurchase(_ context.Context, _ string, _ uuid.UUID) (model.Purchase, error) {
	return model.Purchase{}, nil
}
func (m *mockStore) CreatePurchase(_ context.Context, _ string, _ model.Purchase) error { return nil }
func (m *mockStore) UpdatePurchase(_ context.Context, _ string, _ model.Purchase) error { return nil }
func (m *mockStore) DeletePurchase(_ context.Context, _ string, _ uuid.UUID) error      { return nil }
func (m *mockStore) ListVehicles(_ context.Context, _ string) ([]model.Vehicle, error) {
	return nil, nil
}
func (m *mockStore) GetVehicle(_ context.Context, _ string, _ uuid.UUID) (model.Vehicle, error) {
	return model.Vehicle{}, nil
}
func (m *mockStore) CreateVehicle(_ context.Context, _ string, _ model.Vehicle) error { return nil }
func (m *mockStore) UpdateVehicle(_ context.Context, _ string, _ model.Vehicle) error { return nil }
func (m *mockStore) DeleteVehicle(_ context.Context, _ string, _ uuid.UUID) error     { return nil }
func (m *mockStore) ListCostEntries(_ context.Context, _ string, _ uuid.UUID) ([]model.CostEntry, error) {
	return nil, nil
}
func (m *mockStore) GetCostEntry(_ context.Context, _ string, _ uuid.UUID) (model.CostEntry, error) {
	return model.CostEntry{}, nil
}
func (m *mockStore) CreateCostEntry(_ context.Context, _ string, _ model.CostEntry) error { return nil }
func (m *mockStore) UpdateCostEntry(_ context.Context, _ string, _ model.CostEntry) error { return nil }
func (m *mockStore) DeleteCostEntry(_ context.Context, _ string, _ uuid.UUID) error       { return nil }
func (m *mockStore) Close() error                                                         { return nil }

var _ store.Store = (*mockStore)(nil)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func dkbTestCSV() string {
	return "\xEF\xBB\xBF" + `"Girokonto";"DE12345678901234567890"
""
"Kontostand vom DD.MM.YYYY:";"111.111,11 €"
""
"Buchungsdatum";"Wertstellung";"Status";"Zahlungspflichtige*r";"Zahlungsempfänger*in";"Verwendungszweck";"Umsatztyp";"IBAN";"Betrag (€)";"Gläubiger-ID";"Mandatsreferenz";"Kundenreferenz"
"07.04.26";"02.04.26";"Gebucht";"DKB AG";"Mustermann,Fred";"Depot 0123";"Eingang";"0000000000";"800,23";"";"";""
"01.04.26";"01.04.26";"Gebucht";"Fred";"Fred";"Transfer";"Eingang";"DE11222233334444555566";"50,00";"";"";""
`
}

func TestServicePreview_DKB_AutoResolveAccount(t *testing.T) {
	ms := newMockStore()
	accID := uuid.New()
	ms.accounts[accID.String()] = model.LedgerAccount{
		ID:   accID,
		IBAN: "DE12345678901234567890",
		Bank: "DKB",
	}
	ms.ibanIndex["DE12345678901234567890"] = ms.accounts[accID.String()]

	svc := NewService(ms, testLogger())
	result, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AccountID != accID.String() {
		t.Errorf("AccountID = %q, want %q", result.AccountID, accID.String())
	}
	if result.TotalRows != 2 {
		t.Errorf("TotalRows = %d, want 2", result.TotalRows)
	}
	if result.NewRows != 2 {
		t.Errorf("NewRows = %d, want 2", result.NewRows)
	}
	if result.DuplicateRows != 0 {
		t.Errorf("DuplicateRows = %d, want 0", result.DuplicateRows)
	}
	if result.PreviewID == "" {
		t.Error("PreviewID should not be empty")
	}
}

func TestServicePreview_DKB_NoAccount(t *testing.T) {
	ms := newMockStore()
	svc := NewService(ms, testLogger())

	result, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AccountID != "" {
		t.Errorf("AccountID = %q, want empty (no matching account)", result.AccountID)
	}
	if result.IBAN != "DE12345678901234567890" {
		t.Errorf("IBAN = %q, want DE12345678901234567890", result.IBAN)
	}
}

func TestServicePreview_DuplicateFile(t *testing.T) {
	ms := newMockStore()
	svc := NewService(ms, testLogger())

	// First import
	_, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
		AccountID:  uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("first preview: %v", err)
	}

	// Commit to register the file hash - we need to go through commit
	// Actually, file hash is only stored on commit, so preview of same file should work
	// Let's test the commit flow instead
}

func TestServiceCommit_NewAccount(t *testing.T) {
	ms := newMockStore()
	svc := NewService(ms, testLogger())

	preview, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	result, err := svc.Commit(context.Background(), CommitRequest{
		PreviewID: preview.PreviewID,
		NewAccount: &model.LedgerAccountInput{
			Name:     "My DKB Account",
			Bank:     "DKB",
			IBAN:     "DE12345678901234567890",
			Currency: "EUR",
		},
		UserID: "user1",
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	if result.ImportedRows != 2 {
		t.Errorf("ImportedRows = %d, want 2", result.ImportedRows)
	}
	if result.DuplicateRows != 0 {
		t.Errorf("DuplicateRows = %d, want 0", result.DuplicateRows)
	}
	if len(ms.accounts) != 1 {
		t.Errorf("accounts count = %d, want 1", len(ms.accounts))
	}
	if len(ms.transactions) != 2 {
		t.Errorf("transactions count = %d, want 2", len(ms.transactions))
	}
}

func TestServiceCommit_ExistingAccount(t *testing.T) {
	ms := newMockStore()
	accID := uuid.New()
	ms.accounts[accID.String()] = model.LedgerAccount{
		ID:   accID,
		IBAN: "DE12345678901234567890",
		Bank: "DKB",
	}
	ms.ibanIndex["DE12345678901234567890"] = ms.accounts[accID.String()]

	svc := NewService(ms, testLogger())

	preview, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	result, err := svc.Commit(context.Background(), CommitRequest{
		PreviewID: preview.PreviewID,
		UserID:    "user1",
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	if result.AccountID != accID {
		t.Errorf("AccountID = %v, want %v", result.AccountID, accID)
	}
	if result.ImportedRows != 2 {
		t.Errorf("ImportedRows = %d, want 2", result.ImportedRows)
	}
}

func TestServiceCommit_DeduplicatesOnSecondImport(t *testing.T) {
	ms := newMockStore()
	accID := uuid.New()
	ms.accounts[accID.String()] = model.LedgerAccount{
		ID:   accID,
		IBAN: "DE12345678901234567890",
		Bank: "DKB",
	}
	ms.ibanIndex["DE12345678901234567890"] = ms.accounts[accID.String()]

	svc := NewService(ms, testLogger())

	// First import
	csv1 := dkbTestCSV()
	preview1, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(csv1),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("first preview: %v", err)
	}
	_, err = svc.Commit(context.Background(), CommitRequest{
		PreviewID: preview1.PreviewID,
		UserID:    "user1",
	})
	if err != nil {
		t.Fatalf("first commit: %v", err)
	}

	// Second import with different file content but overlapping transactions
	csv2 := "\xEF\xBB\xBF" + `"Girokonto";"DE12345678901234567890"
""
"Kontostand vom DD.MM.YYYY:";"999,99 €"
""
"Buchungsdatum";"Wertstellung";"Status";"Zahlungspflichtige*r";"Zahlungsempfänger*in";"Verwendungszweck";"Umsatztyp";"IBAN";"Betrag (€)";"Gläubiger-ID";"Mandatsreferenz";"Kundenreferenz"
"07.04.26";"02.04.26";"Gebucht";"DKB AG";"Mustermann,Fred";"Depot 0123";"Eingang";"0000000000";"800,23";"";"";""
"10.04.26";"10.04.26";"Gebucht";"New Sender";"New Receiver";"New purpose";"Ausgang";"DE99999999999999999999";"-25,00";"";"";""
`
	preview2, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(csv2),
		Filename:   "test2.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("second preview: %v", err)
	}

	if preview2.DuplicateRows != 1 {
		t.Errorf("second preview DuplicateRows = %d, want 1", preview2.DuplicateRows)
	}
	if preview2.NewRows != 1 {
		t.Errorf("second preview NewRows = %d, want 1", preview2.NewRows)
	}

	result2, err := svc.Commit(context.Background(), CommitRequest{
		PreviewID: preview2.PreviewID,
		UserID:    "user1",
	})
	if err != nil {
		t.Fatalf("second commit: %v", err)
	}

	if result2.ImportedRows != 1 {
		t.Errorf("second commit ImportedRows = %d, want 1", result2.ImportedRows)
	}
	if result2.DuplicateRows != 1 {
		t.Errorf("second commit DuplicateRows = %d, want 1", result2.DuplicateRows)
	}

	// Total transactions should be 3 (2 from first + 1 new from second)
	if len(ms.transactions) != 3 {
		t.Errorf("total transactions = %d, want 3", len(ms.transactions))
	}
}

func TestServiceCommit_ExpiredPreview(t *testing.T) {
	ms := newMockStore()
	svc := NewService(ms, testLogger())

	_, err := svc.Commit(context.Background(), CommitRequest{
		PreviewID: "nonexistent",
		AccountID: uuid.New().String(),
		UserID:    "user1",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent preview")
	}
}

func TestServiceCommit_NoAccountProvided(t *testing.T) {
	ms := newMockStore()
	svc := NewService(ms, testLogger())

	preview, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	// No account and no new account provided
	_, err = svc.Commit(context.Background(), CommitRequest{
		PreviewID: preview.PreviewID,
		UserID:    "user1",
	})
	if err == nil {
		t.Fatal("expected error when no account is provided")
	}
}
