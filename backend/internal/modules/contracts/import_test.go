package contracts

import (
	"bytes"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func TestImportContracts_RefusesOversizedUpload(t *testing.T) {
	env := newTestEnv(t)
	h := &Handler{store: env.store, categories: env.categories, logger: slog.New(slog.DiscardHandler)}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(bytes.Repeat([]byte(" "), maxImportBytes+1))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(middleware.SetUserID(req.Context(), testUserID))
	rec := httptest.NewRecorder()
	h.ImportContracts(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "body_too_large") {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestImportContracts_ValidatesRows(t *testing.T) {
	env := newTestEnv(t)
	h := &Handler{store: env.store, categories: env.categories, logger: slog.New(slog.DiscardHandler)}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(`[
		{"category": "Telecom", "name": " Phone ", "startDate": "2026-01-01"},
		{"category": "Telecom", "name": "Bad", "startDate": "2026-01-01", "price": -1},
		{"category": "Telecom", "name": "Bad date", "startDate": "01.01.2026"}
	]`))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(middleware.SetUserID(req.Context(), testUserID))
	rec := httptest.NewRecorder()
	h.ImportContracts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	res := decodeJSON[importResult](t, rec)
	if res.Created != 1 || len(res.Errors) != 2 ||
		res.Errors[0] != (importError{Row: 2, Error: "price must not be negative"}) ||
		res.Errors[1] != (importError{Row: 3, Error: "startDate must be a date in YYYY-MM-DD format"}) {
		t.Errorf("result = %+v", res)
	}
	list, _ := env.store.List(t.Context(), testUserID)
	if len(list) != 1 || list[0].Name != "Phone" {
		t.Errorf("stored = %+v", list)
	}
}
