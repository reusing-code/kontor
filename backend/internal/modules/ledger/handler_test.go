package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
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
		ledgerEmail:  NewEmailService(s.Store, extractor, nil, logger),
		suggester:    NewCategorySuggester(s.Store, extractor, SuggestConfig{}, logger),
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
	mux.HandleFunc("POST /api/v1/ledger/accounts", h.CreateLedgerAccount)
	mux.HandleFunc("GET /api/v1/ledger/accounts/{accountId}", h.GetLedgerAccount)
	mux.HandleFunc("PUT /api/v1/ledger/accounts/{accountId}", h.UpdateLedgerAccount)
	mux.HandleFunc("DELETE /api/v1/ledger/accounts/{accountId}", h.DeleteLedgerAccount)
	mux.HandleFunc("GET /api/v1/ledger/accounts/{accountId}/delete-summary", h.GetLedgerAccountDeleteSummary)
	mux.HandleFunc("GET /api/v1/ledger/accounts/{accountId}/transactions", h.ListLedgerTransactions)
	mux.HandleFunc("POST /api/v1/ledger/accounts/{accountId}/balance", h.SetLedgerAccountBalance)
	mux.HandleFunc("DELETE /api/v1/ledger/accounts/{accountId}/balance/{asOf}", h.DeleteLedgerAccountBalance)
	mux.HandleFunc("GET /api/v1/ledger/imports", h.ListLedgerImports)
	mux.HandleFunc("GET /api/v1/ledger/review-queue", h.ListLedgerTransactionsReviewQueue)
	mux.HandleFunc("GET /api/v1/ledger/transactions", h.ListAllLedgerTransactions)
	mux.HandleFunc("GET /api/v1/ledger/transactions/{transactionId}", h.GetLedgerTransaction)
	mux.HandleFunc("PUT /api/v1/ledger/transactions/{transactionId}", h.UpdateLedgerTransactionDetails)
	mux.HandleFunc("POST /api/v1/ledger/transactions/{transactionId}/references", h.AddLedgerTransactionReference)
	mux.HandleFunc("PUT /api/v1/ledger/transactions/{transactionId}/splits", h.SetLedgerTransactionSplits)
	mux.HandleFunc("GET /api/v1/ledger/transactions/{transactionId}/transfer-candidates", h.ListLedgerTransferCandidates)
	mux.HandleFunc("POST /api/v1/ledger/transactions/{transactionId}/transfer-link", h.LinkLedgerTransfer)
	mux.HandleFunc("DELETE /api/v1/ledger/transactions/{transactionId}/transfer-link", h.UnlinkLedgerTransfer)
	mux.HandleFunc("POST /api/v1/ledger/transactions/{transactionId}/review", h.ReviewLedgerTransaction)
	mux.HandleFunc("POST /api/v1/ledger/transactions/review-batch", h.ReviewLedgerTransactionsBatch)
	mux.HandleFunc("POST /api/v1/ledger/category-suggestions/run", h.RunLedgerCategorySuggestions)
	mux.HandleFunc("GET /api/v1/ledger/category-suggestions/status", h.GetLedgerCategorySuggestionStatus)
	mux.HandleFunc("POST /api/v1/ledger/recurring/run", h.RunLedgerRecurringDetection)
	mux.HandleFunc("GET /api/v1/ledger/analysis/spending", h.GetLedgerSpendingAnalysis)
	mux.HandleFunc("GET /api/v1/ledger/analysis/cash-flow", h.GetLedgerCashFlowAnalysis)
	mux.HandleFunc("GET /api/v1/ledger/analysis/ranking", h.GetLedgerRankingAnalysis)
	mux.HandleFunc("GET /api/v1/ledger/analysis/net-worth", h.GetLedgerNetWorthAnalysis)
	mux.HandleFunc("POST /api/v1/ledger/imports/{previewId}/commit", h.LedgerImportCommit)
	mux.HandleFunc("POST /api/v1/ledger/email-accounts/folders", h.ListLedgerEmailFolders)
	mux.HandleFunc("POST /api/v1/ledger/email-accounts", h.CreateLedgerEmailAccount)
	mux.HandleFunc("PUT /api/v1/ledger/email-accounts/{emailAccountId}", h.UpdateLedgerEmailAccount)
	mux.HandleFunc("POST /api/v1/ledger/email-accounts/{emailAccountId}/test", h.TestLedgerEmailAccount)
	mux.HandleFunc("POST /api/v1/ledger/email-accounts/{emailAccountId}/scan", h.ScanLedgerEmailAccount)
	mux.HandleFunc("GET /api/v1/ledger/email-accounts/{emailAccountId}/scan", h.GetLedgerEmailScanStatus)
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
	req := httptest.NewRequest("GET", "/api/v1/ledger/review-queue?limit=10", nil)
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

func TestLedgerReviewQueue_TotalCountsTheWholeQueue(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountID := addAccount(t, s, "Main")
	commitTransactions(t, s, accountID,
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-04", Currency: "EUR", ReviewStatus: LedgerTransactionReviewNeedsReview},
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-03", Currency: "EUR", ReviewStatus: LedgerTransactionReviewNeedsReview},
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR", ReviewStatus: LedgerTransactionReviewNeedsReview},
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR", ReviewStatus: LedgerTransactionReviewConfirmed},
	)

	for _, query := range []string{"limit=1", "limit=all", ""} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/review-queue?"+query, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d; body: %s", query, rec.Code, rec.Body.String())
		}
		page := decodeJSON[struct {
			Items []LedgerTransaction `json:"items"`
			Total int                 `json:"total"`
		}](t, rec)
		if page.Total != 3 {
			t.Fatalf("%s: total = %d, want 3", query, page.Total)
		}
		if query == "limit=1" && len(page.Items) != 1 {
			t.Fatalf("limit=1: len(items) = %d, want 1", len(page.Items))
		}
	}
}

type ledgerListPage struct {
	Items      []LedgerTransaction `json:"items"`
	NextCursor string              `json:"nextCursor"`
	Total      int                 `json:"total"`
}

func listAllTransactions(t *testing.T, mux http.Handler, query string) ledgerListPage {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/transactions"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	return decodeJSON[ledgerListPage](t, rec)
}

func transactionIDs(txns []LedgerTransaction) []uuid.UUID {
	ids := make([]uuid.UUID, len(txns))
	for i, txn := range txns {
		ids[i] = txn.ID
	}
	return ids
}

func TestLedgerAllTransactions_CombinesAccounts(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	main := addAccount(t, s, "Main")
	savings := addAccount(t, s, "Savings")
	a := LedgerTransaction{ID: uuid.New(), AccountID: main, BookingDate: "2026-04-01", Currency: "EUR", ReviewStatus: LedgerTransactionReviewConfirmed}
	b := LedgerTransaction{ID: uuid.New(), AccountID: savings, BookingDate: "2026-04-03", Currency: "EUR", ReviewStatus: LedgerTransactionReviewNeedsReview}
	c := LedgerTransaction{ID: uuid.New(), AccountID: main, BookingDate: "2026-04-02", Currency: "EUR", ReviewStatus: LedgerTransactionReviewNeedsReview}
	commitTransactions(t, s, main, a, c)
	commitTransactions(t, s, savings, b)

	page := listAllTransactions(t, mux, "")
	if got, want := transactionIDs(page.Items), []uuid.UUID{b.ID, c.ID, a.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if page.NextCursor != "" || page.Total != 3 {
		t.Fatalf("nextCursor = %q, total = %d, want none and 3", page.NextCursor, page.Total)
	}

	page = listAllTransactions(t, mux, "?accountId="+main.String())
	if got, want := transactionIDs(page.Items), []uuid.UUID{c.ID, a.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("filtered items = %v, want %v", got, want)
	}

	page = listAllTransactions(t, mux, "?accountId="+main.String()+"&accountId="+savings.String())
	if len(page.Items) != 3 {
		t.Fatalf("len(items) with both accounts = %d, want 3", len(page.Items))
	}

	page = listAllTransactions(t, mux, "?accountId="+uuid.NewString())
	if len(page.Items) != 0 {
		t.Fatalf("len(items) for an unknown account = %d, want 0", len(page.Items))
	}
}

func TestLedgerAllTransactions_Paging(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	main := addAccount(t, s, "Main")
	savings := addAccount(t, s, "Savings")
	var mainTxns, savingsTxns []LedgerTransaction
	for i := range 5 {
		txn := LedgerTransaction{ID: uuid.New(), BookingDate: fmt.Sprintf("2026-04-%02d", i+1), Currency: "EUR", ReviewStatus: LedgerTransactionReviewConfirmed}
		if i%2 == 0 {
			txn.AccountID = main
			mainTxns = append(mainTxns, txn)
		} else {
			txn.AccountID = savings
			savingsTxns = append(savingsTxns, txn)
		}
	}
	commitTransactions(t, s, main, mainTxns...)
	commitTransactions(t, s, savings, savingsTxns...)

	var seen []LedgerTransaction
	query := "?limit=2"
	for range 5 {
		page := listAllTransactions(t, mux, query)
		seen = append(seen, page.Items...)
		if page.NextCursor == "" {
			break
		}
		query = "?limit=2&cursor=" + page.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("paged through %d transactions, want 5", len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i-1].BookingDate < seen[i].BookingDate {
			t.Fatalf("not newest first at %d: %s before %s", i, seen[i-1].BookingDate, seen[i].BookingDate)
		}
	}
}

func TestLedgerAllTransactions_InvalidParameters(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	for _, query := range []string{"?accountId=nope", "?limit=0", "?limit=x"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/transactions"+query, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want %d", query, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestLedgerReviewQueue_LimitAll(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	accountID := addAccount(t, s, "Main")
	txns := make([]LedgerTransaction, 0, 601)
	for i := range 600 {
		txns = append(txns, LedgerTransaction{
			ID:           uuid.New(),
			AccountID:    accountID,
			BookingDate:  fmt.Sprintf("2026-%02d-%02d", i%12+1, i%28+1),
			Currency:     "EUR",
			ReviewStatus: LedgerTransactionReviewNeedsReview,
		})
	}
	txns = append(txns, LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR", ReviewStatus: LedgerTransactionReviewConfirmed})
	commitTransactions(t, s, accountID, txns...)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/review-queue?limit=all&cursor="+txns[0].ID.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	page := decodeJSON[struct {
		Items      []LedgerTransaction `json:"items"`
		NextCursor string              `json:"nextCursor"`
	}](t, rec)
	if len(page.Items) != 600 {
		t.Fatalf("len(items) = %d, want 600", len(page.Items))
	}
	if page.NextCursor != "" {
		t.Fatalf("nextCursor = %q, want none", page.NextCursor)
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

	get := func(query string) accountPageBody {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+accountID.String()+"/transactions?"+query, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want %d; body: %s", query, rec.Code, http.StatusOK, rec.Body.String())
		}
		return decodeJSON[accountPageBody](t, rec)
	}
	dates := func(page accountPageBody) []string {
		var out []string
		for _, txn := range page.Items {
			out = append(out, txn.BookingDate)
		}
		return out
	}

	first := get("limit=2")
	if got := dates(first); !slices.Equal(got, []string{"2026-04-03", "2026-04-02"}) || first.Offset != 0 || first.Total != 3 {
		t.Fatalf("first page = %v (offset %d, total %d)", got, first.Offset, first.Total)
	}
	second := get("limit=2&offset=2")
	if got := dates(second); !slices.Equal(got, []string{"2026-04-01"}) || second.Offset != 2 || second.Total != 3 {
		t.Fatalf("second page = %v (offset %d, total %d)", got, second.Offset, second.Total)
	}
	middle := get("limit=2&offset=1")
	if got := dates(middle); !slices.Equal(got, []string{"2026-04-02", "2026-04-01"}) {
		t.Fatalf("page at offset 1 = %v", got)
	}
	past := get("limit=2&offset=10")
	if past.Items == nil || len(past.Items) != 0 || past.Offset != 3 || past.Total != 3 {
		t.Fatalf("page past the end = %+v", past)
	}
}

func TestListLedgerTransactions_InvalidOffset(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")

	for _, offset := range []string{"abc", "-1", "1.5"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+accountID.String()+"/transactions?offset="+offset, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("offset %q: status = %d, want %d", offset, rec.Code, http.StatusBadRequest)
		}
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
	h.encryptionKey = key
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

func TestEmailAccountHandlers_DraftsFolder(t *testing.T) {
	h, _ := newTestHandlerWithEmailKey(t)
	mux := newMux(h)
	body := func(draftsFolder string) map[string]any {
		return map[string]any{
			"name": "Mail", "imapHost": "imap.example.com", "imapPort": 993, "username": "u", "password": "p",
			"useTls": true, "scanSince": "2026-01-01", "folder": "Orders", "draftsFolder": draftsFolder,
		}
	}
	send := func(method, path, draftsFolder string) (*httptest.ResponseRecorder, LedgerEmailAccount) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, jsonBody(body(draftsFolder))))
		var account LedgerEmailAccount
		_ = json.Unmarshal(rec.Body.Bytes(), &account)
		return rec, account
	}

	if rec, _ := send(http.MethodPost, "/api/v1/ledger/email-accounts", "Orders"); rec.Code != http.StatusBadRequest {
		t.Fatalf("create with the order folder as drafts folder: status = %d, want 400", rec.Code)
	}
	rec, created := send(http.MethodPost, "/api/v1/ledger/email-accounts", " Kontor ")
	if rec.Code != http.StatusCreated || created.DraftsFolder != "Kontor" {
		t.Fatalf("create: status = %d, draftsFolder = %q", rec.Code, created.DraftsFolder)
	}
	path := "/api/v1/ledger/email-accounts/" + created.ID.String()
	if rec, _ := send(http.MethodPut, path, "Orders"); rec.Code != http.StatusBadRequest {
		t.Fatalf("update with the order folder as drafts folder: status = %d, want 400", rec.Code)
	}
	if rec, updated := send(http.MethodPut, path, ""); rec.Code != http.StatusOK || updated.DraftsFolder != "" {
		t.Fatalf("update: status = %d, draftsFolder = %q", rec.Code, updated.DraftsFolder)
	}
}

func TestEmailAccountHandlers_ProcessedActions(t *testing.T) {
	h, _ := newTestHandlerWithEmailKey(t)
	mux := newMux(h)
	send := func(method, path string, actions map[string]any) (*httptest.ResponseRecorder, LedgerEmailAccount) {
		body := map[string]any{
			"name": "Mail", "imapHost": "imap.example.com", "imapPort": 993, "username": "u", "password": "p",
			"useTls": true, "scanSince": "2026-01-01", "folder": "Orders", "draftsFolder": "Kontor",
		}
		for k, v := range actions {
			body[k] = v
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, jsonBody(body)))
		var account LedgerEmailAccount
		_ = json.Unmarshal(rec.Body.Bytes(), &account)
		return rec, account
	}

	rec, created := send(http.MethodPost, "/api/v1/ledger/email-accounts", nil)
	if rec.Code != http.StatusCreated || created.ProcessedAction != EmailActionKeep || created.DraftsProcessedAction != EmailActionKeep {
		t.Fatalf("create: status = %d, actions = %q/%q, want keep", rec.Code, created.ProcessedAction, created.DraftsProcessedAction)
	}
	path := "/api/v1/ledger/email-accounts/" + created.ID.String()
	if rec, _ := send(http.MethodPut, path, map[string]any{"processedAction": "move", "processedFolder": "Kontor"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("move into the drafts folder: status = %d, want 400", rec.Code)
	}
	rec, updated := send(http.MethodPut, path, map[string]any{
		"processedAction": "move", "processedFolder": "Done", "draftsProcessedAction": "delete",
	})
	if rec.Code != http.StatusOK || updated.ProcessedAction != EmailActionMove || updated.ProcessedFolder != "Done" ||
		updated.DraftsProcessedAction != EmailActionDelete {
		t.Fatalf("update: status = %d, account = %+v", rec.Code, updated)
	}
	stored, err := h.store.GetLedgerEmailAccount(t.Context(), testUserID, created.ID)
	if err != nil || stored.ProcessedFolder != "Done" || stored.DraftsProcessedAction != EmailActionDelete {
		t.Fatalf("stored = %+v, err = %v", stored, err)
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
