package reminder

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
	"github.com/tobi/contracts/backend/internal/store"
)

type mockStore struct {
	users     []model.User
	settings  map[string]model.UserSettings
	contracts map[string][]model.Contract
}

func (m *mockStore) CreateUser(_ context.Context, _ model.User) error { return nil }
func (m *mockStore) GetUserByEmail(_ context.Context, _ string) (model.User, error) {
	return model.User{}, store.ErrNotFound
}
func (m *mockStore) GetUserByID(_ context.Context, _ string) (model.User, error) {
	return model.User{}, store.ErrNotFound
}
func (m *mockStore) UpdateUser(_ context.Context, _ model.User) error { return nil }
func (m *mockStore) ListUsers(_ context.Context) ([]model.User, error) {
	return m.users, nil
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
func (m *mockStore) ListContracts(_ context.Context, userID string) ([]model.Contract, error) {
	return m.contracts[userID], nil
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
	return model.Vehicle{}, store.ErrNotFound
}
func (m *mockStore) CreateVehicle(_ context.Context, _ string, _ model.Vehicle) error { return nil }
func (m *mockStore) UpdateVehicle(_ context.Context, _ string, _ model.Vehicle) error { return nil }
func (m *mockStore) DeleteVehicle(_ context.Context, _ string, _ uuid.UUID) error     { return nil }

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
	return nil, nil
}
func (m *mockStore) GetLedgerAccount(_ context.Context, _ string, _ uuid.UUID) (model.LedgerAccount, error) {
	return model.LedgerAccount{}, store.ErrNotFound
}
func (m *mockStore) FindLedgerAccountByIBAN(_ context.Context, _ string, _ string) (model.LedgerAccount, error) {
	return model.LedgerAccount{}, store.ErrNotFound
}
func (m *mockStore) CreateLedgerAccount(_ context.Context, _ string, _ model.LedgerAccount) error {
	return nil
}
func (m *mockStore) ListLedgerCategories(_ context.Context, _ string) ([]model.LedgerCategory, error) {
	return nil, nil
}
func (m *mockStore) GetLedgerCategory(_ context.Context, _ string, _ uuid.UUID) (model.LedgerCategory, error) {
	return model.LedgerCategory{}, store.ErrNotFound
}
func (m *mockStore) CreateLedgerCategory(_ context.Context, _ string, _ model.LedgerCategory) error {
	return nil
}
func (m *mockStore) UpdateLedgerCategory(_ context.Context, _ string, _ model.LedgerCategory) error {
	return nil
}
func (m *mockStore) DeleteLedgerCategory(_ context.Context, _ string, _ uuid.UUID) error {
	return nil
}
func (m *mockStore) GetLedgerImportByFileHash(_ context.Context, _ string, _ string) (model.LedgerImportBatch, error) {
	return model.LedgerImportBatch{}, store.ErrNotFound
}
func (m *mockStore) LedgerTransactionFingerprintExists(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}
func (m *mockStore) CommitLedgerImport(_ context.Context, _ string, _ model.LedgerImportBatch, _ []model.LedgerTransaction) (store.LedgerImportCommitResult, error) {
	return store.LedgerImportCommitResult{}, nil
}
func (m *mockStore) ListLedgerImports(_ context.Context, _ string) ([]model.LedgerImportBatch, error) {
	return nil, nil
}
func (m *mockStore) ListLedgerTransactions(_ context.Context, _ string, _ uuid.UUID) ([]model.LedgerTransaction, error) {
	return nil, nil
}
func (m *mockStore) ListLedgerTransactionsPage(_ context.Context, _ string, _ uuid.UUID, _ int, _ string) (store.LedgerTransactionPage, error) {
	return store.LedgerTransactionPage{}, nil
}
func (m *mockStore) ListLedgerTransactionsFiltered(_ context.Context, _ string, _ store.LedgerTransactionListOptions) (store.LedgerTransactionPage, error) {
	return store.LedgerTransactionPage{}, nil
}
func (m *mockStore) GetLedgerTransaction(_ context.Context, _ string, _ uuid.UUID) (model.LedgerTransaction, error) {
	return model.LedgerTransaction{}, store.ErrNotFound
}
func (m *mockStore) ReviewLedgerTransaction(_ context.Context, _ string, _ uuid.UUID, _ model.LedgerTransactionReviewInput) (store.LedgerReviewResult, error) {
	return store.LedgerReviewResult{}, store.ErrNotFound
}

func (m *mockStore) Close() error { return nil }

func newTestUser() model.User {
	return model.User{
		ID:    uuid.New(),
		Email: "test@example.com",
	}
}

func TestCheckUser_SkipsDisabled(t *testing.T) {
	user := newTestUser()
	uid := user.ID.String()

	ms := &mockStore{
		settings: map[string]model.UserSettings{
			uid: {RenewalDays: 90, ReminderFrequency: "disabled"},
		},
	}

	sched := &Scheduler{store: ms, email: nil, logger: testLogger()}
	err := sched.checkUser(context.Background(), user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckUser_SkipsEmptyFrequency(t *testing.T) {
	user := newTestUser()
	uid := user.ID.String()

	ms := &mockStore{
		settings: map[string]model.UserSettings{
			uid: {RenewalDays: 90, ReminderFrequency: ""},
		},
	}

	sched := &Scheduler{store: ms, email: nil, logger: testLogger()}
	err := sched.checkUser(context.Background(), user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckUser_SkipsWhenNotEnoughTimeElapsed(t *testing.T) {
	user := newTestUser()
	uid := user.ID.String()

	ms := &mockStore{
		settings: map[string]model.UserSettings{
			uid: {
				RenewalDays:       90,
				ReminderFrequency: "weekly",
				LastReminderSent:  time.Now().Add(-1 * time.Hour), // sent 1 hour ago
			},
		},
	}

	sched := &Scheduler{store: ms, email: nil, logger: testLogger()}
	err := sched.checkUser(context.Background(), user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckUser_DefaultSettingsSkips(t *testing.T) {
	user := newTestUser()

	ms := &mockStore{
		settings: map[string]model.UserSettings{},
	}

	sched := &Scheduler{store: ms, email: nil, logger: testLogger()}
	err := sched.checkUser(context.Background(), user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildEmail_FormatsCorrectly(t *testing.T) {
	matches := []upcomingContract{
		{
			contract:         model.Contract{Name: "Phone Plan", Company: "Telco Inc"},
			cancellationDate: "2025-08-15",
		},
		{
			contract:         model.Contract{Name: "Gym Membership"},
			cancellationDate: "2025-09-01",
		},
	}

	body := buildEmail(matches)

	if !strings.Contains(body, "Phone Plan (Telco Inc)") {
		t.Errorf("expected body to contain 'Phone Plan (Telco Inc)', got:\n%s", body)
	}
	if !strings.Contains(body, "cancellation by 2025-08-15") {
		t.Errorf("expected body to contain cancellation date 2025-08-15")
	}
	if !strings.Contains(body, "- Gym Membership — cancellation by 2025-09-01") {
		t.Errorf("expected body to contain Gym Membership without company, got:\n%s", body)
	}
	if strings.Contains(body, "Gym Membership (") {
		t.Errorf("Gym Membership should not have company parentheses")
	}
}

func TestBuildEmail_SingleContract(t *testing.T) {
	matches := []upcomingContract{
		{
			contract:         model.Contract{Name: "Insurance", Company: "ACME"},
			cancellationDate: "2025-07-01",
		},
	}

	body := buildEmail(matches)
	lines := strings.Split(body, "\n")

	found := false
	for _, line := range lines {
		if strings.HasPrefix(line, "- Insurance") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected line starting with '- Insurance', got:\n%s", body)
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
