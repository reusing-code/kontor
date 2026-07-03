package core_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/modules/ledger"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const exportTestUser = "00000000-0000-0000-0000-000000000001"

type exportHarness struct {
	catStore  *categories.Store
	coreStore *core.Store
	contracts *contracts.Module
	purchases *purchases.Module
	auto      *auto.Module
	ledger    *ledger.Module
	mux       http.Handler
}

func newExportHarness(t *testing.T) *exportHarness {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	engine, err := storage.Open(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })

	links := link.NewRegistry()
	catStore := categories.NewStore(engine)
	coreStore := core.NewStore(engine)

	contractsMod := contracts.New(engine, links, catStore, coreStore, nil, logger)
	purchasesMod := purchases.New(engine, links, catStore, logger)
	autoMod := auto.New(engine, links, logger)
	ledgerMod := ledger.New(engine, links, coreStore, ledger.Config{}, logger)
	registry := module.NewRegistry(contractsMod, purchasesMod, autoMod, ledgerMod)

	handler := core.NewHandler(coreStore, logger, []byte("export-test-secret"), nil, nil, registry)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export", handler.Export)
	mux.HandleFunc("POST /api/v1/import", handler.Import)
	mux.HandleFunc("GET /api/v1/modules/{module}/export", handler.ExportModule)
	mux.HandleFunc("POST /api/v1/modules/{module}/import", handler.ImportModule)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), exportTestUser)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})

	return &exportHarness{
		catStore:  catStore,
		coreStore: coreStore,
		contracts: contractsMod,
		purchases: purchasesMod,
		auto:      autoMod,
		ledger:    ledgerMod,
		mux:       wrapped,
	}
}

func (h *exportHarness) do(t *testing.T, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, reader)
	h.mux.ServeHTTP(rec, req)
	return rec
}

type importResultBody struct {
	Restored map[string]int `json:"restored"`
	Warnings []string       `json:"warnings"`
}

type exportEnvelopeBody struct {
	Format        string                     `json:"format"`
	FormatVersion int                        `json:"formatVersion"`
	ExportedAt    time.Time                  `json:"exportedAt"`
	Settings      json.RawMessage            `json:"settings"`
	Modules       map[string]json.RawMessage `json:"modules"`
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return v
}

func hasWarningContaining(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

type seededData struct {
	contractCategory categories.Category
	contract         contracts.Contract
	purchaseCategory categories.Category
	purchase         purchases.Purchase
	vehicle          auto.Vehicle
	costEntry        auto.CostEntry
	ledgerParentCat  ledger.LedgerCategory
	ledgerChildCat   ledger.LedgerCategory
	account          ledger.LedgerAccount
	batch            ledger.LedgerImportBatch
	transaction      ledger.LedgerTransaction
	emailAccount     ledger.LedgerEmailAccount
	emailOrder       ledger.LedgerEmailOrder
}

// seedAllModules fills every module with representative, cross-referencing
// data: a ledger transaction referencing a purchase and an email order linked
// to the transaction.
func seedAllModules(t *testing.T, h *exportHarness) seededData {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	var d seededData

	d.contractCategory = categories.Category{ID: uuid.New(), Name: "Streaming", CreatedAt: now, UpdatedAt: now}
	if err := h.catStore.Create(ctx, exportTestUser, contracts.ModuleID, d.contractCategory); err != nil {
		t.Fatalf("creating contract category: %v", err)
	}
	d.contract = contracts.Contract{ID: uuid.New(), CategoryID: d.contractCategory.ID, Name: "Netflix", StartDate: "2025-01-01", CreatedAt: now, UpdatedAt: now}
	if err := h.contracts.Store().Create(ctx, exportTestUser, d.contract); err != nil {
		t.Fatalf("creating contract: %v", err)
	}

	d.purchaseCategory = categories.Category{ID: uuid.New(), Name: "PC Hardware", CreatedAt: now, UpdatedAt: now}
	if err := h.catStore.Create(ctx, exportTestUser, purchases.ModuleID, d.purchaseCategory); err != nil {
		t.Fatalf("creating purchase category: %v", err)
	}
	d.purchase = purchases.Purchase{ID: uuid.New(), CategoryID: d.purchaseCategory.ID, ItemName: "Monitor", CreatedAt: now, UpdatedAt: now}
	if err := h.purchases.Store().Create(ctx, exportTestUser, d.purchase); err != nil {
		t.Fatalf("creating purchase: %v", err)
	}

	d.vehicle = auto.Vehicle{ID: uuid.New(), Name: "Car", CreatedAt: now, UpdatedAt: now}
	if err := h.auto.Store().CreateVehicle(ctx, exportTestUser, d.vehicle); err != nil {
		t.Fatalf("creating vehicle: %v", err)
	}
	d.costEntry = auto.CostEntry{ID: uuid.New(), VehicleID: d.vehicle.ID, Type: "fuel", Date: "2026-06-01", CreatedAt: now, UpdatedAt: now}
	if err := h.auto.Store().CreateCostEntry(ctx, exportTestUser, d.costEntry); err != nil {
		t.Fatalf("creating cost entry: %v", err)
	}

	d.ledgerParentCat = ledger.LedgerCategory{ID: uuid.New(), Name: "Living", CreatedAt: now, UpdatedAt: now}
	if err := h.ledger.Store().CreateLedgerCategory(ctx, exportTestUser, d.ledgerParentCat); err != nil {
		t.Fatalf("creating ledger parent category: %v", err)
	}
	d.ledgerChildCat = ledger.LedgerCategory{ID: uuid.New(), Name: "Rent", ParentID: &d.ledgerParentCat.ID, CreatedAt: now, UpdatedAt: now}
	if err := h.ledger.Store().CreateLedgerCategory(ctx, exportTestUser, d.ledgerChildCat); err != nil {
		t.Fatalf("creating ledger child category: %v", err)
	}

	d.account = ledger.LedgerAccount{ID: uuid.New(), Name: "Checking", Bank: "DKB", IBAN: "DE111", Currency: "EUR", CreatedAt: now, UpdatedAt: now}
	if err := h.ledger.Store().CreateLedgerAccount(ctx, exportTestUser, d.account); err != nil {
		t.Fatalf("creating ledger account: %v", err)
	}
	d.batch = ledger.LedgerImportBatch{
		ID: uuid.New(), AccountID: d.account.ID, SourceType: "dkb.csv", ParserVersion: "1",
		Filename: "seed.csv", FileSHA256: "seed-hash", Status: ledger.ImportStatusCommitted,
		TotalRows: 1, CreatedAt: now, UpdatedAt: now,
	}
	d.transaction = ledger.LedgerTransaction{
		ID: uuid.New(), AccountID: d.account.ID, BookingDate: "2026-06-01", AmountMinor: -4200,
		Currency: "EUR", Fingerprint: "seed-fp-1", ImportBatchID: d.batch.ID, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := h.ledger.Store().CommitLedgerImport(ctx, exportTestUser, d.batch, []ledger.LedgerTransaction{d.transaction}); err != nil {
		t.Fatalf("committing ledger import: %v", err)
	}
	// Cross-module reference: transaction -> purchase (also links back).
	if _, err := h.ledger.Store().UpdateLedgerTransactionDetails(ctx, exportTestUser, d.transaction.ID, ledger.LedgerTransactionDetailsInput{
		References: []ledger.LedgerTransactionReference{{Type: ledger.LedgerReferencePurchase, TargetID: d.purchase.ID}},
	}); err != nil {
		t.Fatalf("linking transaction to purchase: %v", err)
	}

	d.emailAccount = ledger.LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: "imap.example.com", IMAPPort: 993,
		Username: "user", EncryptedPassword: "super-secret-ciphertext", UseTLS: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := h.ledger.Store().CreateLedgerEmailAccount(ctx, exportTestUser, d.emailAccount); err != nil {
		t.Fatalf("creating email account: %v", err)
	}
	d.emailOrder = ledger.LedgerEmailOrder{
		ID: uuid.New(), EmailAccountID: d.emailAccount.ID, ImporterID: "amazon.de",
		OrderDate: "2026-06-01", TotalMinor: 4200, Currency: "EUR", CreatedAt: now, UpdatedAt: now,
	}
	if err := h.ledger.Store().CreateLedgerEmailOrder(ctx, exportTestUser, d.emailOrder); err != nil {
		t.Fatalf("creating email order: %v", err)
	}
	if _, err := h.ledger.Store().LinkLedgerEmailOrder(ctx, exportTestUser, d.emailOrder.ID, ledger.LedgerEmailOrderLinkInput{
		TransactionIDs: []uuid.UUID{d.transaction.ID},
	}); err != nil {
		t.Fatalf("linking email order: %v", err)
	}

	return d
}

func exportFull(t *testing.T, h *exportHarness) []byte {
	t.Helper()
	rec := h.do(t, "GET", "/api/v1/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d, body: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func TestExportImport_FullRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := newExportHarness(t)
	data := seedAllModules(t, src)
	if err := src.coreStore.UpdateSettings(ctx, exportTestUser, core.UserSettings{RenewalDays: 42, ReminderFrequency: "weekly"}); err != nil {
		t.Fatalf("updating settings: %v", err)
	}

	rec := src.do(t, "GET", "/api/v1/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d, body: %s", rec.Code, rec.Body.String())
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "kontor-export") {
		t.Errorf("Content-Disposition = %q, want attachment with kontor-export filename", disposition)
	}
	if strings.Contains(rec.Body.String(), "super-secret-ciphertext") {
		t.Error("export must not contain encrypted email passwords")
	}
	exported := rec.Body.Bytes()

	var parsed exportEnvelopeBody
	if err := json.Unmarshal(exported, &parsed); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if parsed.Format != "kontor-export" || parsed.FormatVersion != 2 {
		t.Fatalf("format = %q v%d, want kontor-export v2", parsed.Format, parsed.FormatVersion)
	}
	if parsed.ExportedAt.IsZero() {
		t.Error("exportedAt is zero")
	}
	if len(parsed.Modules) != 4 {
		t.Fatalf("modules = %v, want 4 sections", len(parsed.Modules))
	}
	if len(parsed.Settings) == 0 {
		t.Error("full export should include settings")
	}

	dst := newExportHarness(t)
	// Fresh accounts come with seeded default categories; import must replace them.
	now := time.Now().UTC()
	if err := dst.catStore.Create(ctx, exportTestUser, contracts.ModuleID, categories.Category{
		ID: uuid.New(), Name: "Insurance", NameKey: "categoryNames.insurance", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seeding default category: %v", err)
	}

	rec = dst.do(t, "POST", "/api/v1/import", exported)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d, body: %s", rec.Code, rec.Body.String())
	}
	result := decodeBody[importResultBody](t, rec)
	if !hasWarningContaining(result.Warnings, "password") {
		t.Errorf("expected email password warning, got %v", result.Warnings)
	}
	for key, want := range map[string]int{
		"contracts": 1, "purchases": 1, "vehicles": 1, "costEntries": 1,
		"ledgerAccounts": 1, "ledgerCategories": 2, "ledgerImports": 1,
		"ledgerTransactions": 1, "ledgerEmailAccounts": 1, "ledgerEmailOrders": 1,
	} {
		if result.Restored[key] != want {
			t.Errorf("restored[%q] = %d, want %d", key, result.Restored[key], want)
		}
	}

	// Contracts: ID and category preserved, seeded defaults replaced.
	con, err := dst.contracts.Store().Get(ctx, exportTestUser, data.contract.ID)
	if err != nil {
		t.Fatalf("restored contract missing: %v", err)
	}
	if con.Name != "Netflix" || con.CategoryID != data.contractCategory.ID {
		t.Errorf("restored contract = %+v", con)
	}
	conCats, err := dst.catStore.List(ctx, exportTestUser, contracts.ModuleID)
	if err != nil {
		t.Fatalf("listing contract categories: %v", err)
	}
	if len(conCats) != 1 || conCats[0].ID != data.contractCategory.ID {
		t.Errorf("contract categories = %+v, want exported category only", conCats)
	}

	// Purchases: restored with the transaction back-link re-established.
	pur, err := dst.purchases.Store().Get(ctx, exportTestUser, data.purchase.ID)
	if err != nil {
		t.Fatalf("restored purchase missing: %v", err)
	}
	if len(pur.LinkedTransactionIDs) != 1 || pur.LinkedTransactionIDs[0] != data.transaction.ID {
		t.Errorf("purchase linkedTransactionIds = %+v", pur.LinkedTransactionIDs)
	}

	// Auto: vehicle and cost entry preserved.
	if _, err := dst.auto.Store().GetVehicle(ctx, exportTestUser, data.vehicle.ID); err != nil {
		t.Fatalf("restored vehicle missing: %v", err)
	}
	costs, err := dst.auto.Store().ListCostEntries(ctx, exportTestUser, data.vehicle.ID)
	if err != nil {
		t.Fatalf("listing cost entries: %v", err)
	}
	if len(costs) != 1 || costs[0].ID != data.costEntry.ID {
		t.Errorf("restored cost entries = %+v", costs)
	}

	// Ledger: categories with parent link, account, transaction, references.
	ledCats, err := dst.ledger.Store().ListLedgerCategories(ctx, exportTestUser)
	if err != nil {
		t.Fatalf("listing ledger categories: %v", err)
	}
	if len(ledCats) != 2 {
		t.Fatalf("ledger categories = %d, want 2", len(ledCats))
	}
	child, err := dst.ledger.Store().GetLedgerCategory(ctx, exportTestUser, data.ledgerChildCat.ID)
	if err != nil {
		t.Fatalf("restored child category missing: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != data.ledgerParentCat.ID {
		t.Errorf("child parentId = %v, want %s", child.ParentID, data.ledgerParentCat.ID)
	}
	if _, err := dst.ledger.Store().GetLedgerAccount(ctx, exportTestUser, data.account.ID); err != nil {
		t.Fatalf("restored ledger account missing: %v", err)
	}
	txn, err := dst.ledger.Store().GetLedgerTransaction(ctx, exportTestUser, data.transaction.ID)
	if err != nil {
		t.Fatalf("restored transaction missing: %v", err)
	}
	if txn.AmountMinor != -4200 {
		t.Errorf("transaction amount = %d, want -4200", txn.AmountMinor)
	}
	if len(txn.References) != 1 || txn.References[0].TargetID != data.purchase.ID {
		t.Errorf("transaction references = %+v", txn.References)
	}

	// Email account restored without password; order keeps its links.
	emailAccount, err := dst.ledger.Store().GetLedgerEmailAccount(ctx, exportTestUser, data.emailAccount.ID)
	if err != nil {
		t.Fatalf("restored email account missing: %v", err)
	}
	if emailAccount.EncryptedPassword != "" {
		t.Error("restored email account must not regain a password")
	}
	order, err := dst.ledger.Store().GetLedgerEmailOrder(ctx, exportTestUser, data.emailOrder.ID)
	if err != nil {
		t.Fatalf("restored email order missing: %v", err)
	}
	if len(order.LinkedTransactionIDs) != 1 || order.LinkedTransactionIDs[0] != data.transaction.ID {
		t.Errorf("restored email order links = %+v", order.LinkedTransactionIDs)
	}

	// Settings round trip.
	settings, err := dst.coreStore.GetSettings(ctx, exportTestUser)
	if err != nil {
		t.Fatalf("getting settings: %v", err)
	}
	if settings.RenewalDays != 42 || settings.ReminderFrequency != "weekly" {
		t.Errorf("restored settings = %+v", settings)
	}

	// Re-exporting from the target must yield identical module sections.
	reExported := exportFull(t, dst)
	var reParsed exportEnvelopeBody
	if err := json.Unmarshal(reExported, &reParsed); err != nil {
		t.Fatalf("unmarshal re-export: %v", err)
	}
	for id, section := range parsed.Modules {
		var want, got any
		if err := json.Unmarshal(section, &want); err != nil {
			t.Fatalf("unmarshal source %s section: %v", id, err)
		}
		if err := json.Unmarshal(reParsed.Modules[id], &got); err != nil {
			t.Fatalf("unmarshal re-exported %s section: %v", id, err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("re-exported %s section differs:\nwant: %s\ngot:  %s", id, section, reParsed.Modules[id])
		}
	}
}

func TestExport_EmptyUserSucceeds(t *testing.T) {
	h := newExportHarness(t)

	rec := h.do(t, "GET", "/api/v1/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	exported := append([]byte(nil), rec.Body.Bytes()...)
	envelope := decodeBody[exportEnvelopeBody](t, rec)
	if len(envelope.Modules) != 4 {
		t.Fatalf("modules = %d sections, want 4", len(envelope.Modules))
	}
	for id, section := range envelope.Modules {
		var decoded map[string]any
		if err := json.Unmarshal(section, &decoded); err != nil {
			t.Fatalf("section %s is not valid JSON: %v", id, err)
		}
		for key, value := range decoded {
			if key == "schemaVersion" {
				continue
			}
			if _, ok := value.([]any); !ok {
				t.Errorf("section %s field %s = %v, want initialized array", id, key, value)
			}
		}
	}

	// An empty export must import cleanly into another empty account.
	dst := newExportHarness(t)
	rec = dst.do(t, "POST", "/api/v1/import", exported)
	if rec.Code != http.StatusOK {
		t.Fatalf("import of empty export status = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestImport_RejectsNonEmptyModule(t *testing.T) {
	src := newExportHarness(t)
	seedAllModules(t, src)
	exported := exportFull(t, src)

	dst := newExportHarness(t)
	now := time.Now().UTC()
	if err := dst.contracts.Store().Create(context.Background(), exportTestUser, contracts.Contract{
		ID: uuid.New(), CategoryID: uuid.New(), Name: "Existing", StartDate: "2025-01-01", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("creating existing contract: %v", err)
	}

	rec := dst.do(t, "POST", "/api/v1/import", exported)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestImport_UnsupportedFormat(t *testing.T) {
	h := newExportHarness(t)

	cases := []struct {
		name string
		body string
	}{
		{"wrong version", `{"format":"kontor-export","formatVersion":1,"modules":{"contracts":{}}}`},
		{"wrong format", `{"format":"other-tool","formatVersion":2,"modules":{"contracts":{}}}`},
		{"unknown module", `{"format":"kontor-export","formatVersion":2,"modules":{"bogus":{}}}`},
		{"invalid json", `{not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(t, "POST", "/api/v1/import", []byte(tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestImport_ContractsOnly_PrunesDeadTransactionLinks(t *testing.T) {
	h := newExportHarness(t)
	ctx := context.Background()
	now := time.Now().UTC()

	cat := categories.Category{ID: uuid.New(), Name: "Streaming", CreatedAt: now, UpdatedAt: now}
	con := contracts.Contract{
		ID:                   uuid.New(),
		CategoryID:           cat.ID,
		Name:                 "Netflix",
		StartDate:            "2025-01-01",
		LinkedTransactionIDs: []uuid.UUID{uuid.New()}, // ledger data not part of the file
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	envelope := map[string]any{
		"format":        "kontor-export",
		"formatVersion": 2,
		"modules": map[string]any{
			"contracts": map[string]any{
				"schemaVersion": 0,
				"categories":    []categories.Category{cat},
				"contracts":     []contracts.Contract{con},
			},
		},
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	rec := h.do(t, "POST", "/api/v1/import", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	result := decodeBody[importResultBody](t, rec)
	if !hasWarningContaining(result.Warnings, "missing ledger transactions") {
		t.Errorf("expected dead-link warning, got %v", result.Warnings)
	}

	restored, err := h.contracts.Store().Get(ctx, exportTestUser, con.ID)
	if err != nil {
		t.Fatalf("restored contract missing: %v", err)
	}
	if len(restored.LinkedTransactionIDs) != 0 {
		t.Errorf("linkedTransactionIds = %+v, want pruned", restored.LinkedTransactionIDs)
	}
}

func TestExportModule_ContainsOnlyThatModule(t *testing.T) {
	src := newExportHarness(t)
	seedAllModules(t, src)

	rec := src.do(t, "GET", "/api/v1/modules/purchases/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if disposition := rec.Header().Get("Content-Disposition"); !strings.Contains(disposition, "kontor-export-purchases") {
		t.Errorf("Content-Disposition = %q, want kontor-export-purchases filename", disposition)
	}
	envelope := decodeBody[exportEnvelopeBody](t, rec)
	if len(envelope.Modules) != 1 {
		t.Fatalf("modules = %d sections, want only purchases", len(envelope.Modules))
	}
	if _, ok := envelope.Modules["purchases"]; !ok {
		t.Fatalf("missing purchases section, got %v", envelope.Modules)
	}
	if len(envelope.Settings) != 0 {
		t.Error("single-module export must not include settings")
	}

	rec = src.do(t, "GET", "/api/v1/modules/nope/export", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown module export status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestImportModule_FromFullExport_ImportsOnlyThatModule(t *testing.T) {
	ctx := context.Background()
	src := newExportHarness(t)
	data := seedAllModules(t, src)
	exported := exportFull(t, src)

	dst := newExportHarness(t)
	rec := dst.do(t, "POST", "/api/v1/modules/contracts/import", exported)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	result := decodeBody[importResultBody](t, rec)
	if result.Restored["contracts"] != 1 {
		t.Errorf("restored[contracts] = %d, want 1", result.Restored["contracts"])
	}
	if result.Restored["purchases"] != 0 {
		t.Errorf("restored[purchases] = %d, want 0", result.Restored["purchases"])
	}

	if _, err := dst.contracts.Store().Get(ctx, exportTestUser, data.contract.ID); err != nil {
		t.Fatalf("contract not imported: %v", err)
	}
	purs, err := dst.purchases.Store().List(ctx, exportTestUser)
	if err != nil {
		t.Fatalf("listing purchases: %v", err)
	}
	if len(purs) != 0 {
		t.Errorf("purchases = %d, want none imported", len(purs))
	}
	accounts, err := dst.ledger.Store().ListLedgerAccounts(ctx, exportTestUser)
	if err != nil {
		t.Fatalf("listing ledger accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("ledger accounts = %d, want none imported", len(accounts))
	}
}

func TestImport_OrphanTransactionsGetSyntheticBatch(t *testing.T) {
	ctx := context.Background()
	src := newExportHarness(t)
	now := time.Now().UTC()

	account := ledger.LedgerAccount{ID: uuid.New(), Name: "Checking", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}
	if err := src.ledger.Store().CreateLedgerAccount(ctx, exportTestUser, account); err != nil {
		t.Fatalf("creating account: %v", err)
	}
	batch := ledger.LedgerImportBatch{
		ID: uuid.New(), AccountID: account.ID, SourceType: "dkb.csv", ParserVersion: "1",
		Filename: "orphan.csv", FileSHA256: "orphan-hash", Status: ledger.ImportStatusCommitted,
		TotalRows: 1, CreatedAt: now, UpdatedAt: now,
	}
	orphanBatchID := uuid.New() // transaction points at a batch missing from the export
	txn := ledger.LedgerTransaction{
		ID: uuid.New(), AccountID: account.ID, BookingDate: "2026-06-01", AmountMinor: -100,
		Currency: "EUR", Fingerprint: "orphan-fp", ImportBatchID: orphanBatchID, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := src.ledger.Store().CommitLedgerImport(ctx, exportTestUser, batch, []ledger.LedgerTransaction{txn}); err != nil {
		t.Fatalf("committing import: %v", err)
	}

	exported := exportFull(t, src)

	dst := newExportHarness(t)
	rec := dst.do(t, "POST", "/api/v1/import", exported)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	result := decodeBody[importResultBody](t, rec)
	if !hasWarningContaining(result.Warnings, "missing import batch") {
		t.Errorf("expected orphan batch warning, got %v", result.Warnings)
	}

	restored, err := dst.ledger.Store().GetLedgerTransaction(ctx, exportTestUser, txn.ID)
	if err != nil {
		t.Fatalf("orphan transaction not restored: %v", err)
	}
	if restored.AmountMinor != -100 {
		t.Errorf("restored amount = %d, want -100", restored.AmountMinor)
	}
	imports, err := dst.ledger.Store().ListLedgerImports(ctx, exportTestUser)
	if err != nil {
		t.Fatalf("listing imports: %v", err)
	}
	if len(imports) != 2 {
		t.Fatalf("imports = %d, want original batch plus synthetic one", len(imports))
	}
	foundSynthetic := false
	for _, b := range imports {
		if b.ID == orphanBatchID {
			foundSynthetic = true
		}
	}
	if !foundSynthetic {
		t.Error("synthetic batch with the orphan batch ID not found")
	}
}
