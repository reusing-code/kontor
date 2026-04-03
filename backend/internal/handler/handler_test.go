package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/middleware"
	"github.com/tobi/contracts/backend/internal/model"
	"github.com/tobi/contracts/backend/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// mockStore implements store.Store in memory for handler tests.
type mockStore struct {
	categories map[string]map[uuid.UUID]model.Category // keyed by module, then ID
	contracts  map[uuid.UUID]model.Contract
	users      map[string]model.User // keyed by email
	usersById  map[string]model.User // keyed by ID
	settings   map[string]model.UserSettings
}

func newMockStore() *mockStore {
	return &mockStore{
		categories: make(map[string]map[uuid.UUID]model.Category),
		contracts:  make(map[uuid.UUID]model.Contract),
		users:      make(map[string]model.User),
		usersById:  make(map[string]model.User),
		settings:   make(map[string]model.UserSettings),
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
	return nil, nil
}
func (m *mockStore) ListPurchasesByCategory(_ context.Context, _ string, _ uuid.UUID) ([]model.Purchase, error) {
	return nil, nil
}
func (m *mockStore) GetPurchase(_ context.Context, _ string, _ uuid.UUID) (model.Purchase, error) {
	return model.Purchase{}, store.ErrNotFound
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
func (m *mockStore) GetLedgerImportByFileHash(_ context.Context, _ string, _ string) (model.LedgerImportBatch, error) {
	return model.LedgerImportBatch{}, store.ErrNotFound
}
func (m *mockStore) LedgerTransactionFingerprintExists(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}
func (m *mockStore) CommitLedgerImport(_ context.Context, _ string, _ model.LedgerImportBatch, _ []model.LedgerTransaction) error {
	return nil
}
func (m *mockStore) ListLedgerImports(_ context.Context, _ string) ([]model.LedgerImportBatch, error) {
	return nil, nil
}
func (m *mockStore) ListLedgerTransactions(_ context.Context, _ string, _ uuid.UUID) ([]model.LedgerTransaction, error) {
	return nil, nil
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
	mux.HandleFunc("GET /api/v1/modules/{module}/categories", h.ListCategories)
	mux.HandleFunc("POST /api/v1/modules/{module}/categories", h.CreateCategory)
	mux.HandleFunc("GET /api/v1/modules/{module}/categories/{id}", h.GetCategory)
	mux.HandleFunc("PUT /api/v1/modules/{module}/categories/{id}", h.UpdateCategory)
	mux.HandleFunc("DELETE /api/v1/modules/{module}/categories/{id}", h.DeleteCategory)
	mux.HandleFunc("GET /api/v1/categories/{id}/contracts", h.ListContractsByCategory)
	mux.HandleFunc("POST /api/v1/categories/{id}/contracts", h.CreateContractInCategory)
	mux.HandleFunc("GET /api/v1/contracts/upcoming-renewals", h.UpcomingRenewals)
	mux.HandleFunc("GET /api/v1/contracts", h.ListContracts)
	mux.HandleFunc("GET /api/v1/contracts/{id}", h.GetContract)
	mux.HandleFunc("PUT /api/v1/contracts/{id}", h.UpdateContract)
	mux.HandleFunc("DELETE /api/v1/contracts/{id}", h.DeleteContract)
	mux.HandleFunc("GET /api/v1/summary", h.Summary)
	mux.HandleFunc("GET /api/v1/settings", h.GetSettings)
	mux.HandleFunc("PUT /api/v1/settings", h.UpdateSettings)
	mux.HandleFunc("PUT /api/v1/settings/password", h.ChangePassword)
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

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return v
}

// Auth handler tests

func newAuthMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", h.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	return mux
}

func TestRegister_SeedsDefaultCategories(t *testing.T) {
	h, ms := newTestHandler()
	mux := newAuthMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(map[string]string{"email": "seed@test.com", "password": "pass"}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	totalCategories := 0
	for _, modCats := range ms.categories {
		totalCategories += len(modCats)
	}
	if totalCategories != 8 {
		t.Fatalf("expected 8 default categories (3 contracts + 5 purchases), got %d", totalCategories)
	}
}

func TestRegisterThenLogin(t *testing.T) {
	h, _ := newTestHandler()
	mux := newAuthMux(h)

	creds := map[string]string{"email": "test@example.com", "password": "secret123"}

	// Register
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(creds))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	regResp := decodeJSON[authResponse](t, rec)
	if regResp.Token == "" {
		t.Fatal("register: expected token")
	}
	if regResp.User.Email != "test@example.com" {
		t.Fatalf("register: email = %q, want %q", regResp.User.Email, "test@example.com")
	}

	// Login with same credentials
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/auth/login", jsonBody(creds))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	loginResp := decodeJSON[authResponse](t, rec)
	if loginResp.Token == "" {
		t.Fatal("login: expected token")
	}
	if loginResp.User.Email != "test@example.com" {
		t.Fatalf("login: email = %q, want %q", loginResp.User.Email, "test@example.com")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	h, _ := newTestHandler()
	mux := newAuthMux(h)

	// Register
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(map[string]string{"email": "a@b.com", "password": "correct"}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// Login with wrong password
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/auth/login", jsonBody(map[string]string{"email": "a@b.com", "password": "wrong"}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login wrong pw: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	h, _ := newTestHandler()
	mux := newAuthMux(h)

	creds := map[string]string{"email": "dup@test.com", "password": "pass"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(creds))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first register: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(creds))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

// Category handler tests

func TestCreateCategory_Success(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/modules/contracts/categories", jsonBody(map[string]string{"name": "Test"}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	cat := decodeJSON[model.Category](t, rec)
	if cat.Name != "Test" {
		t.Errorf("Name = %q, want %q", cat.Name, "Test")
	}
	if cat.ID == uuid.Nil {
		t.Error("ID should not be nil")
	}
}

func TestCreateCategory_EmptyName(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/modules/contracts/categories", jsonBody(map[string]string{"name": ""}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateCategory_InvalidJSON(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/modules/contracts/categories", bytes.NewBufferString("{bad"))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateCategory_UnknownField(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	body := `{"name":"Test","bogus":"field"}`
	req := httptest.NewRequest("POST", "/api/v1/modules/contracts/categories", bytes.NewBufferString(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (unknown fields should be rejected)", rec.Code, http.StatusBadRequest)
	}
}

func TestGetCategory_NotFound(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/modules/contracts/categories/"+uuid.New().String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetCategory_InvalidUUID(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/modules/contracts/categories/not-a-uuid", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListCategories_Empty(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/modules/contracts/categories", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	cats := decodeJSON[[]model.Category](t, rec)
	if len(cats) != 0 {
		t.Errorf("expected empty list, got %d", len(cats))
	}
}

func TestUpdateCategory_NotFound(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/modules/contracts/categories/"+uuid.New().String(), jsonBody(map[string]string{"name": "X"}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteCategory_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	cat := model.Category{ID: uuid.New(), Name: "X"}
	ms.addCategory("contracts", cat)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/modules/contracts/categories/"+cat.ID.String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestDeleteCategory_NotFound(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/modules/contracts/categories/"+uuid.New().String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// Contract handler tests

func TestCreateContract_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	cat := model.Category{ID: uuid.New(), Name: "Cat"}
	ms.addCategory("contracts", cat)

	body := map[string]any{
		"name":                    "Phone",
		"startDate":               "2025-01-01",
		"minimumDurationMonths":   12,
		"extensionDurationMonths": 12,
		"noticePeriodMonths":      3,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/categories/"+cat.ID.String()+"/contracts", jsonBody(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	con := decodeJSON[model.Contract](t, rec)
	if con.Name != "Phone" {
		t.Errorf("Name = %q, want %q", con.Name, "Phone")
	}
	if con.CategoryID != cat.ID {
		t.Errorf("CategoryID = %s, want %s", con.CategoryID, cat.ID)
	}
}

func TestCreateContract_MissingName(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	cat := model.Category{ID: uuid.New(), Name: "Cat"}
	ms.addCategory("contracts", cat)

	body := map[string]any{"startDate": "2025-01-01"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/categories/"+cat.ID.String()+"/contracts", jsonBody(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateContract_MissingStartDate(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	cat := model.Category{ID: uuid.New(), Name: "Cat"}
	ms.addCategory("contracts", cat)

	body := map[string]any{"name": "X"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/categories/"+cat.ID.String()+"/contracts", jsonBody(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateContract_CategoryNotFound(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	body := map[string]any{"name": "X", "startDate": "2025-01-01"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/categories/"+uuid.New().String()+"/contracts", jsonBody(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetContract_NotFound(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/contracts/"+uuid.New().String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetContract_InvalidUUID(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/contracts/not-valid", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteContract_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	con := model.Contract{ID: uuid.New(), Name: "X"}
	ms.contracts[con.ID] = con

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/contracts/"+con.ID.String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestDeleteContract_NotFound(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/contracts/"+uuid.New().String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateContract_NotFound(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	body := map[string]any{"name": "X", "startDate": "2025-01-01"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/contracts/"+uuid.New().String(), jsonBody(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// Content-Type check

func TestResponses_HaveJSONContentType(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/modules/contracts/categories", nil)
	mux.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

// Settings handler tests

func TestGetSettings_ReturnsDefaults(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/settings", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	s := decodeJSON[model.SettingsResponse](t, rec)
	if s.RenewalDays != 90 {
		t.Errorf("RenewalDays = %d, want 90", s.RenewalDays)
	}
	if s.ReminderFrequency != "disabled" {
		t.Errorf("ReminderFrequency = %q, want %q", s.ReminderFrequency, "disabled")
	}
}

func TestGetSettings_OmitsLastReminderSent(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	ms.settings[testUserID] = model.UserSettings{
		RenewalDays:       90,
		ReminderFrequency: "weekly",
		LastReminderSent:  time.Now(),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/settings", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var raw map[string]any
	json.NewDecoder(rec.Body).Decode(&raw)
	if _, ok := raw["lastReminderSent"]; ok {
		t.Error("response should not contain lastReminderSent")
	}
}

func TestUpdateSettings_Success(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{"renewalDays": 30, "reminderFrequency": "weekly"}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	s := decodeJSON[model.SettingsResponse](t, rec)
	if s.RenewalDays != 30 {
		t.Errorf("RenewalDays = %d, want 30", s.RenewalDays)
	}
	if s.ReminderFrequency != "weekly" {
		t.Errorf("ReminderFrequency = %q, want %q", s.ReminderFrequency, "weekly")
	}

	// Verify persisted
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/v1/settings", nil)
	mux.ServeHTTP(rec, req)

	s = decodeJSON[model.SettingsResponse](t, rec)
	if s.RenewalDays != 30 {
		t.Errorf("persisted RenewalDays = %d, want 30", s.RenewalDays)
	}
}

func TestUpdateSettings_InvalidRange(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	for _, days := range []int{0, -1, 366} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]int{"renewalDays": days}))
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("days=%d: status = %d, want %d", days, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestChangePassword_Success(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	// Create a user with known ID matching testUserID
	uid, _ := uuid.Parse(testUserID)
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpass"), bcrypt.DefaultCost)
	ms.usersById[testUserID] = model.User{ID: uid, Email: "test@test.com", PasswordHash: string(hash)}
	ms.users["test@test.com"] = ms.usersById[testUserID]

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings/password", jsonBody(map[string]string{
		"currentPassword": "oldpass",
		"newPassword":     "newpass",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Verify new password works
	updated := ms.usersById[testUserID]
	if err := bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("newpass")); err != nil {
		t.Error("new password should be valid after change")
	}
}

func TestChangePassword_WrongCurrent(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	uid, _ := uuid.Parse(testUserID)
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.DefaultCost)
	ms.usersById[testUserID] = model.User{ID: uid, Email: "test@test.com", PasswordHash: string(hash)}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings/password", jsonBody(map[string]string{
		"currentPassword": "wrong",
		"newPassword":     "newpass",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestChangePassword_MissingFields(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings/password", jsonBody(map[string]string{
		"currentPassword": "old",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateSettings_InvalidReminderFrequency(t *testing.T) {
	h, _ := newTestHandler()
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays":       30,
		"reminderFrequency": "daily",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateSettings_PreservesLastReminderSent(t *testing.T) {
	h, ms := newTestHandler()
	mux := newMux(h)

	sent := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	ms.settings[testUserID] = model.UserSettings{
		RenewalDays:       90,
		ReminderFrequency: "weekly",
		LastReminderSent:  sent,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays":       60,
		"reminderFrequency": "monthly",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	persisted := ms.settings[testUserID]
	if !persisted.LastReminderSent.Equal(sent) {
		t.Errorf("LastReminderSent = %v, want %v", persisted.LastReminderSent, sent)
	}
	if persisted.RenewalDays != 60 {
		t.Errorf("RenewalDays = %d, want 60", persisted.RenewalDays)
	}
	if persisted.ReminderFrequency != "monthly" {
		t.Errorf("ReminderFrequency = %q, want %q", persisted.ReminderFrequency, "monthly")
	}
}
