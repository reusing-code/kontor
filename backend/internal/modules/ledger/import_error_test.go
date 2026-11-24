package ledger

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const importErrorTestCSV = "\xEF\xBB\xBF\"Girokonto\";\"DE12345678901234567890\"\n\"\"\n\"Kontostand vom DD.MM.YYYY:\";\"111.111,11 €\"\n\"\"\n\"Buchungsdatum\";\"Wertstellung\";\"Status\";\"Zahlungspflichtige*r\";\"Zahlungsempfänger*in\";\"Verwendungszweck\";\"Umsatztyp\";\"IBAN\";\"Betrag (€)\";\"Gläubiger-ID\";\"Mandatsreferenz\";\"Kundenreferenz\"\n\"07.04.26\";\"02.04.26\";\"Gebucht\";\"DKB AG\";\"Mustermann,Fred\";\"Depot 0123 Wertpapierertrag\";\"Eingang\";\"0000000000\";\"800,23\";\"\";\"\";\"\"\n"

func postPreview(t *testing.T, mux http.Handler, sourceType string) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := multipartBody(t, map[string]string{"sourceType": sourceType}, "file", "dkb.csv", []byte(importErrorTestCSV))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/imports/preview", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestImportErrors_InputFailuresAre400(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	if rec := postPreview(t, mux, "nope.csv"); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown source type") {
		t.Fatalf("unknown source: status = %d, body: %s", rec.Code, rec.Body.String())
	}

	rec := postPreview(t, mux, "dkb.csv")
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	preview := decodeJSON[PreviewResult](t, rec)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/imports/"+preview.PreviewID+"/commit", jsonBody(map[string]string{"accountId": "not-a-uuid"}))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid accountId") {
		t.Fatalf("invalid accountId: status = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestImportErrors_UnknownPreviewIs404(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/imports/unknown/commit", nil)
	rec := httptest.NewRecorder()
	newMux(h).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestImportErrors_StoreFailuresAre500(t *testing.T) {
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	store := NewStore(engine, link.NewRegistry())
	logger := slog.New(slog.DiscardHandler)
	h := &Handler{store: store, logger: logger, ledgerImport: NewImportService(store, logger)}
	engine.Close()

	rec := postPreview(t, newMux(h), "dkb.csv")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("status = %d, body: %s; want 500 without the store error", rec.Code, rec.Body.String())
	}
}
