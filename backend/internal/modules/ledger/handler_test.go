package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
)

const testUserID = "00000000-0000-0000-0000-000000000001"

func newTestHandler(t *testing.T) (*Handler, *testStores) {
	t.Helper()
	return newTestHandlerWithLLM(t, llm.Disabled{})
}

func newTestHandlerWithLLM(t *testing.T, extractor llm.Extractor) (*Handler, *testStores) {
	t.Helper()
	s := newTestStore(t)
	logger := slog.New(slog.DiscardHandler)
	h := &Handler{
		store:        s.Store,
		logger:       logger,
		ledgerImport: NewImportService(s.Store, logger),
		ledgerEmail:  NewEmailService(s.Store, extractor, logger),
	}
	return h, s
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
	mux.HandleFunc("POST /api/v1/ledger/email-accounts", h.CreateLedgerEmailAccount)
	mux.HandleFunc("PUT /api/v1/ledger/email-accounts/{emailAccountId}", h.UpdateLedgerEmailAccount)
	mux.HandleFunc("POST /api/v1/ledger/email-accounts/{emailAccountId}/scan", h.ScanLedgerEmailAccount)
	mux.HandleFunc("GET /api/v1/ledger/email-orders/{emailOrderId}/candidates", h.ListLedgerEmailOrderCandidates)
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

func addAccount(t *testing.T, s *testStores, name string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.New()
	if err := s.CreateLedgerAccount(context.Background(), testUserID, LedgerAccount{
		ID: id, Name: name, Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}
	return id
}

func addCategory(t *testing.T, s *testStores, name string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.New()
	if err := s.CreateLedgerCategory(context.Background(), testUserID, LedgerCategory{
		ID: id, Name: name, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateLedgerCategory: %v", err)
	}
	return id
}

// commitTransactions persists the given transactions through a committed
// import batch, mirroring how transactions enter the store in production.
func commitTransactions(t *testing.T, s *testStores, accountID uuid.UUID, txns ...LedgerTransaction) {
	t.Helper()
	now := time.Now().UTC()
	batch := LedgerImportBatch{
		ID:            uuid.New(),
		AccountID:     accountID,
		SourceType:    "dkb.csv",
		ParserVersion: "1",
		Filename:      "handler-test.csv",
		FileSHA256:    "hash-" + uuid.NewString(),
		Status:        ImportStatusCommitted,
		TotalRows:     len(txns),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	for i := range txns {
		if txns[i].Fingerprint == "" {
			txns[i].Fingerprint = "fp-" + txns[i].ID.String()
		}
		if txns[i].ImportBatchID == uuid.Nil {
			txns[i].ImportBatchID = batch.ID
		}
		if txns[i].CreatedAt.IsZero() {
			txns[i].CreatedAt = now
			txns[i].UpdatedAt = now
		}
	}
	if _, err := s.CommitLedgerImport(context.Background(), testUserID, batch, txns); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}
}

func TestGetLedgerAccount_Success(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountID := addAccount(t, s, "Main account")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+accountID.String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got := decodeJSON[LedgerAccount](t, rec)
	if got.ID != accountID {
		t.Fatalf("ID = %s, want %s", got.ID, accountID)
	}
}

func TestLedgerImportPreview_UsesCamelCaseRowFields(t *testing.T) {
	h, _ := newTestHandler(t)
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
	if first["reviewStatus"] != LedgerTransactionReviewNeedsReview {
		t.Fatalf("reviewStatus = %#v, want %q", first["reviewStatus"], LedgerTransactionReviewNeedsReview)
	}
}

func TestCreateLedgerCategory_Success(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/categories", jsonBody(map[string]any{"name": "Food", "matchWords": []string{"rewe"}}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	category := decodeJSON[LedgerCategory](t, rec)
	if category.Name != "Food" {
		t.Fatalf("name = %q, want Food", category.Name)
	}
	if len(category.MatchWords) != 1 || category.MatchWords[0] != "rewe" {
		t.Fatalf("matchWords = %#v", category.MatchWords)
	}
}

func TestLedgerReviewQueue_OnlyNeedsReview(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountID := addAccount(t, s, "Main")
	commitTransactions(t, s, accountID,
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-03", Currency: "EUR", ReviewStatus: LedgerTransactionReviewNeedsReview},
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR", ReviewStatus: LedgerTransactionReviewConfirmed},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/transactions?limit=10", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	page := decodeJSON[struct {
		Items []LedgerTransaction `json:"items"`
	}](t, rec)
	if len(page.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(page.Items))
	}
	if page.Items[0].ReviewStatus != LedgerTransactionReviewNeedsReview {
		t.Fatalf("reviewStatus = %q", page.Items[0].ReviewStatus)
	}
}

func TestReviewLedgerTransaction_Success(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountID := addAccount(t, s, "Main")
	categoryID := addCategory(t, s, "Food")
	txnID := uuid.New()
	commitTransactions(t, s, accountID, LedgerTransaction{
		ID:           txnID,
		AccountID:    accountID,
		BookingDate:  "2026-04-03",
		Currency:     "EUR",
		ReviewStatus: LedgerTransactionReviewNeedsReview,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/transactions/"+txnID.String()+"/review", jsonBody(map[string]any{"categoryId": categoryID.String()}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	response := decodeJSON[struct {
		Transaction LedgerTransaction `json:"transaction"`
		Category    *LedgerCategory   `json:"category"`
	}](t, rec)
	if response.Transaction.ReviewStatus != LedgerTransactionReviewConfirmed {
		t.Fatalf("reviewStatus = %q", response.Transaction.ReviewStatus)
	}
	if response.Transaction.CategoryID == nil || *response.Transaction.CategoryID != categoryID {
		t.Fatalf("categoryId = %#v", response.Transaction.CategoryID)
	}
}

func TestUpdateLedgerTransactionDetails_Success(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountID := addAccount(t, s, "Main")
	transactionID := uuid.New()
	purchaseID := uuid.New()
	commitTransactions(t, s, accountID, LedgerTransaction{
		ID:          transactionID,
		AccountID:   accountID,
		BookingDate: "2026-04-01",
		Currency:    "EUR",
		Fingerprint: "fp-details",
	})
	now := time.Now().UTC()
	if err := s.CreatePurchase(t.Context(), testUserID, purchases.Purchase{
		ID: purchaseID, CategoryID: uuid.New(), ItemName: "Desk", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreatePurchase: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/ledger/transactions/"+transactionID.String(), jsonBody(map[string]any{
		"note":       "linked invoice",
		"links":      []string{"https://example.com/invoice.pdf"},
		"references": []map[string]string{{"type": LedgerReferencePurchase, "targetId": purchaseID.String()}},
	}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response LedgerTransaction
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
	h, _ := newTestHandler(t)
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
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountA := addAccount(t, s, "Checking")
	accountB := addAccount(t, s, "Savings")
	categoryID := addCategory(t, s, "Household")
	leftID := uuid.New()
	rightID := uuid.New()
	commitTransactions(t, s, accountA,
		LedgerTransaction{ID: leftID, AccountID: accountA, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR"},
		LedgerTransaction{ID: rightID, AccountID: accountB, BookingDate: "2026-04-02", AmountMinor: 1000, Currency: "EUR"},
	)
	if _, err := s.LinkLedgerTransfer(t.Context(), testUserID, leftID, LedgerTransferLinkInput{PairedTransactionID: rightID}); err != nil {
		t.Fatalf("LinkLedgerTransfer: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/transactions/"+leftID.String()+"/review", jsonBody(map[string]string{"categoryId": categoryID.String()}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestLinkLedgerTransfer_Success(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountA := addAccount(t, s, "Checking")
	accountB := addAccount(t, s, "Savings")
	leftID := uuid.New()
	rightID := uuid.New()
	commitTransactions(t, s, accountA,
		LedgerTransaction{ID: leftID, AccountID: accountA, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR"},
		LedgerTransaction{ID: rightID, AccountID: accountB, BookingDate: "2026-04-02", AmountMinor: 1000, Currency: "EUR"},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/transactions/"+leftID.String()+"/transfer-link", jsonBody(map[string]string{"pairedTransactionId": rightID.String()}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestListLedgerTransferCandidates_Success(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountA := addAccount(t, s, "Checking")
	accountB := addAccount(t, s, "Savings")
	sourceID := uuid.New()
	candidateID := uuid.New()
	commitTransactions(t, s, accountA,
		LedgerTransaction{ID: sourceID, AccountID: accountA, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR"},
		LedgerTransaction{ID: candidateID, AccountID: accountB, BookingDate: "2026-04-02", AmountMinor: 1000, Currency: "EUR"},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/transactions/"+sourceID.String()+"/transfer-candidates", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	response := decodeJSON[struct {
		Items []LedgerTransferCandidate `json:"items"`
	}](t, rec)
	if len(response.Items) != 1 || response.Items[0].Transaction.ID != candidateID {
		t.Fatalf("items = %#v, want single candidate %s", response.Items, candidateID)
	}
}

func TestGetLedgerAccount_InvalidUUID(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/accounts/not-a-uuid", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListLedgerTransactions_Paginated(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountID := addAccount(t, s, "Main")
	commitTransactions(t, s, accountID,
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-03", Currency: "EUR"},
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR"},
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR"},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+accountID.String()+"/transactions?limit=2", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	page := decodeJSON[struct {
		Items      []LedgerTransaction `json:"items"`
		NextCursor string              `json:"nextCursor"`
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
		Items      []LedgerTransaction `json:"items"`
		NextCursor string              `json:"nextCursor"`
	}](t, rec)
	if len(page.Items) != 1 {
		t.Fatalf("second page len(items) = %d, want 1", len(page.Items))
	}
}

func TestListLedgerTransactions_InvalidLimit(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountID := addAccount(t, s, "Main")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+accountID.String()+"/transactions?limit=abc", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func newTestHandlerWithEmailKey(t *testing.T) (*Handler, *testStores) {
	t.Helper()
	h, s := newTestHandler(t)
	key, err := cryptoutil.NormalizeEncryptionKey("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NormalizeEncryptionKey: %v", err)
	}
	h.emailEncryptionKey = key
	return h, s
}

func TestEmailAccountHandlers_Folder(t *testing.T) {
	h, _ := newTestHandlerWithEmailKey(t)
	mux := newMux(h)

	create := func(folder string, preClassify bool) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/email-accounts", jsonBody(map[string]any{
			"name": "Mail", "imapHost": "imap.example.com", "imapPort": 993, "username": "u", "password": "p",
			"useTls": true, "scanSince": "2026-01-01", "folder": folder, "preClassify": preClassify,
		}))
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := create("", false); rec.Code != http.StatusBadRequest {
		t.Fatalf("create without folder: status = %d, want 400", rec.Code)
	}
	rec := create("  Orders ", true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var created LedgerEmailAccount
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Folder != "Orders" || !created.PreClassify {
		t.Fatalf("created folder = %q, preClassify = %v", created.Folder, created.PreClassify)
	}

	update := func(folder string, preClassify bool) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/ledger/email-accounts/"+created.ID.String(), jsonBody(map[string]any{
			"name": "Mail", "imapHost": "imap.example.com", "imapPort": 993, "username": "u",
			"useTls": true, "scanSince": "2026-01-01", "folder": folder, "preClassify": preClassify,
		}))
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := update("", false); rec.Code != http.StatusBadRequest {
		t.Fatalf("update without folder: status = %d, want 400", rec.Code)
	}
	rec = update("INBOX", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var updated LedgerEmailAccount
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.Folder != "INBOX" || updated.PreClassify {
		t.Fatalf("updated folder = %q, preClassify = %v", updated.Folder, updated.PreClassify)
	}
}

func TestScanLedgerEmailAccount_WithoutFolderIs409(t *testing.T) {
	h, s := newTestHandlerWithEmailKey(t)
	now := time.Now().UTC()
	account := LedgerEmailAccount{ID: uuid.New(), Name: "Mail", IMAPHost: "imap.invalid", IMAPPort: 993, Username: "u", EncryptedPassword: "x", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}

	rec := httptest.NewRecorder()
	newMux(h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/email-accounts/"+account.ID.String()+"/scan", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateEmailAccount_ConnectionChangesClearTheScanAttempt(t *testing.T) {
	h, s := newTestHandlerWithEmailKey(t)
	mux := newMux(h)
	attempt := time.Now().UTC().Add(-time.Hour)
	now := time.Now().UTC()
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mail", IMAPHost: "imap.example.com", IMAPPort: 993, Username: "u", UseTLS: true,
		EncryptedPassword: "x", Folder: "Orders", ScanSince: "2026-01-01", LastScanAttemptAt: &attempt, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	put := func(mutate func(map[string]any)) {
		body := map[string]any{
			"name": "Mail", "imapHost": "imap.example.com", "imapPort": 993, "username": "u", "useTls": true,
			"scanSince": "2026-01-01", "folder": "Orders", "preClassify": false,
		}
		mutate(body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/ledger/email-accounts/"+account.ID.String(), jsonBody(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
		}
	}
	attemptSet := func() bool {
		got, err := s.GetLedgerEmailAccount(context.Background(), testUserID, account.ID)
		if err != nil {
			t.Fatalf("GetLedgerEmailAccount: %v", err)
		}
		return got.LastScanAttemptAt != nil
	}
	restore := func() {
		if _, err := s.ModifyLedgerEmailAccount(context.Background(), testUserID, account.ID, func(a *LedgerEmailAccount) { a.LastScanAttemptAt = &attempt }); err != nil {
			t.Fatalf("restore: %v", err)
		}
	}

	put(func(b map[string]any) { b["name"] = "Renamed"; b["preClassify"] = true; b["scanSince"] = "2026-02-01" })
	if !attemptSet() {
		t.Error("a change of name, pre-classification or scanSince cleared the attempt")
	}
	for name, mutate := range map[string]func(map[string]any){
		"folder":   func(b map[string]any) { b["folder"] = "Other" },
		"host":     func(b map[string]any) { b["imapHost"] = "imap2.example.com" },
		"port":     func(b map[string]any) { b["imapPort"] = 143 },
		"username": func(b map[string]any) { b["username"] = "other" },
		"tls":      func(b map[string]any) { b["useTls"] = false },
		"password": func(b map[string]any) { b["password"] = "new-secret" },
	} {
		restore()
		put(mutate)
		if attemptSet() {
			t.Errorf("a change of %s did not clear the attempt", name)
		}
		// Put the stored values back so the next case changes exactly one field.
		restore()
		put(func(b map[string]any) {})
	}
}
