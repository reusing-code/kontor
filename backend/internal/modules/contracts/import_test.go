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
