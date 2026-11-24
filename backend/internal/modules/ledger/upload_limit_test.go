package ledger

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

// zeroReader yields endless bytes without allocating them.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// streamedMultipart writes a form with fields and one file of size bytes
// through a pipe, so the test never holds the whole body in memory.
func streamedMultipart(fields map[string]string, fileField string, size int64) (io.Reader, string) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		for k, v := range fields {
			_ = writer.WriteField(k, v)
		}
		part, err := writer.CreateFormFile(fileField, "big")
		if err == nil {
			_, err = io.Copy(part, io.LimitReader(zeroReader{}, size))
		}
		if err == nil {
			err = writer.Close()
		}
		pw.CloseWithError(err)
	}()
	return pr, writer.FormDataContentType()
}

func expectTooLarge(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "body_too_large") {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestLedgerImportPreview_RefusesOversizedUpload(t *testing.T) {
	h, _ := newTestHandler(t)
	body, contentType := streamedMultipart(map[string]string{"sourceType": "dkb.csv"}, "file", maxLedgerImportBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/imports/preview", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	newMux(h).ServeHTTP(rec, req)
	expectTooLarge(t, rec)
}

func TestScanHandler_RefusesOversizedEmailUpload(t *testing.T) {
	h, s := newTestHandlerWithLLM(t, llmtest.New())
	now := time.Now().UTC()
	account := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	body, contentType := streamedMultipart(nil, "files", maxEmailUploadBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/email-accounts/"+account.ID.String()+"/scan", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	newMux(h).ServeHTTP(rec, req)
	expectTooLarge(t, rec)
}

func TestLedgerHandlers_RefuseOversizedJSON(t *testing.T) {
	h, _ := newTestHandler(t)
	big := `{"name":"` + strings.Repeat("a", 1<<20) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/categories", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newMux(h).ServeHTTP(rec, req)
	expectTooLarge(t, rec)
}
