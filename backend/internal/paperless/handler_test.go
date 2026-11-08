package paperless

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/paperless/paperlesstest"
)

const testUserID = "00000000-0000-0000-0000-000000000001"

// testEncryptionKey is 32 bytes.
const testEncryptionKey = "0123456789abcdef0123456789abcdef"

func newHandlerMux(t *testing.T, encryptionKey string) (http.Handler, *Store) {
	t.Helper()
	s := newTestStore(t)
	h := NewHandler(NewService(s, slog.New(slog.DiscardHandler), encryptionKey), slog.New(slog.DiscardHandler))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/paperless/config", h.GetConfig)
	mux.HandleFunc("PUT /api/v1/paperless/config", h.PutConfig)
	mux.HandleFunc("DELETE /api/v1/paperless/config", h.DeleteConfig)
	mux.HandleFunc("POST /api/v1/paperless/config/test", h.TestConfig)
	mux.HandleFunc("GET /api/v1/paperless/search", h.Search)
	mux.HandleFunc("GET /api/v1/paperless/documents/{documentId}/thumb", h.Thumbnail)
	mux.HandleFunc("GET /api/v1/paperless/links/{entityType}/{entityId}", h.ListLinks)
	mux.HandleFunc("POST /api/v1/paperless/links/{entityType}/{entityId}", h.AttachLinks)
	mux.HandleFunc("DELETE /api/v1/paperless/links/{entityType}/{entityId}/{documentId}", h.DetachLink)
	mux.HandleFunc("GET /api/v1/paperless/pipeline", h.GetPipelineSettings)
	mux.HandleFunc("PUT /api/v1/paperless/pipeline", h.PutPipelineSettings)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	}), s
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

func do(mux http.Handler, method, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	mux.ServeHTTP(rec, req)
	return rec
}

func putConfig(t *testing.T, mux http.Handler, baseURL, token string) {
	t.Helper()
	rec := do(mux, "PUT", "/api/v1/paperless/config", jsonBody(map[string]string{"baseUrl": baseURL, "token": token}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT config status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetConfig_Unconfigured(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	rec := do(mux, "GET", "/api/v1/paperless/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	resp := decodeJSON[configResponse](t, rec)
	if resp.Configured {
		t.Error("expected configured=false")
	}
}

func TestPutConfig_RoundtripAndTokenHidden(t *testing.T) {
	mux, store := newHandlerMux(t, testEncryptionKey)
	putConfig(t, mux, "https://paperless.example/", "secret-token")

	rec := do(mux, "GET", "/api/v1/paperless/config", nil)
	if bytes.Contains(rec.Body.Bytes(), []byte("secret-token")) || bytes.Contains(rec.Body.Bytes(), []byte("oken\"")) {
		t.Errorf("token leaked in response: %s", rec.Body.String())
	}
	resp := decodeJSON[configResponse](t, rec)
	if !resp.Configured || resp.BaseURL != "https://paperless.example" {
		t.Errorf("resp = %+v (trailing slash should be trimmed)", resp)
	}

	cfg, err := store.GetConfig(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg.EncryptedToken == "" || cfg.EncryptedToken == "secret-token" {
		t.Errorf("token should be stored encrypted, got %q", cfg.EncryptedToken)
	}
}

func TestPutConfig_TokenRequiredOnFirstSave(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	rec := do(mux, "PUT", "/api/v1/paperless/config", jsonBody(map[string]string{"baseUrl": "https://p.example"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPutConfig_TokenOptionalOnUpdate(t *testing.T) {
	mux, store := newHandlerMux(t, testEncryptionKey)
	putConfig(t, mux, "https://p.example", "secret")
	before, _ := store.GetConfig(context.Background(), testUserID)

	rec := do(mux, "PUT", "/api/v1/paperless/config", jsonBody(map[string]string{"baseUrl": "https://p.example"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	after, _ := store.GetConfig(context.Background(), testUserID)
	if after.EncryptedToken != before.EncryptedToken {
		t.Error("token should be kept when omitted on update")
	}
}

func TestPutConfig_BaseURLChangeResetsCustomFieldID(t *testing.T) {
	mux, store := newHandlerMux(t, testEncryptionKey)
	putConfig(t, mux, "https://p.example", "secret")
	cfg, _ := store.GetConfig(context.Background(), testUserID)
	cfg.CustomFieldID = 42
	if err := store.PutConfig(context.Background(), testUserID, cfg); err != nil {
		t.Fatalf("PutConfig: %v", err)
	}

	putConfig(t, mux, "https://other.example", "secret")
	after, _ := store.GetConfig(context.Background(), testUserID)
	if after.CustomFieldID != 0 {
		t.Errorf("CustomFieldID = %d, want reset to 0", after.CustomFieldID)
	}
}

func TestPutConfig_InvalidBaseURL(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	for _, baseURL := range []string{"", "not-a-url", "ftp://x.example"} {
		rec := do(mux, "PUT", "/api/v1/paperless/config", jsonBody(map[string]string{"baseUrl": baseURL, "token": "t"}))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("baseUrl %q: status = %d, want 400", baseURL, rec.Code)
		}
	}
}

func TestPutConfig_NoEncryptionKey(t *testing.T) {
	mux, _ := newHandlerMux(t, "")
	rec := do(mux, "PUT", "/api/v1/paperless/config", jsonBody(map[string]string{"baseUrl": "https://p.example", "token": "t"}))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestSearch_Unconfigured(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	rec := do(mux, "GET", "/api/v1/paperless/search?query=x", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestTestConfig_UpstreamDown(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()
	putConfig(t, mux, server.URL, "bad-token")

	rec := do(mux, "POST", "/api/v1/paperless/config/test", jsonBody(map[string]string{}))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestTestConfigAndSearch_Success(t *testing.T) {
	f := paperlesstest.New(t)
	f.AddDocument(paperlesstest.Document{ID: 1, Title: "Invoice", Created: "2026-01-01"})
	mux, _ := newHandlerMux(t, testEncryptionKey)
	putConfig(t, mux, f.URL, "test-token")

	rec := do(mux, "POST", "/api/v1/paperless/config/test", jsonBody(map[string]string{}))
	if rec.Code != http.StatusOK {
		t.Fatalf("test status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(mux, "GET", "/api/v1/paperless/search?query=invoice", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("search status = %d", rec.Code)
	}
	page := decodeJSON[SearchPage](t, rec)
	if page.Count != 1 || page.Results[0].Title != "Invoice" {
		t.Errorf("page = %+v", page)
	}
}

func TestListLinks_InvalidEntity(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	rec := do(mux, "GET", "/api/v1/paperless/links/bogus/"+uuid.NewString(), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for invalid entityType", rec.Code)
	}
	rec = do(mux, "GET", "/api/v1/paperless/links/contract/not-a-uuid", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for invalid entityId", rec.Code)
	}
}

func TestListLinks_EmptyWithoutConfig(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	rec := do(mux, "GET", "/api/v1/paperless/links/contract/"+uuid.NewString(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	links := decodeJSON[[]Link](t, rec)
	if len(links) != 0 {
		t.Errorf("links = %+v", links)
	}
}

func TestAttachAndDetach_WithBacklink(t *testing.T) {
	f := paperlesstest.New(t)
	f.AddDocument(paperlesstest.Document{ID: 7, Title: "Invoice", Created: "2026-01-01"})
	mux, store := newHandlerMux(t, testEncryptionKey)
	putConfig(t, mux, f.URL, "test-token")

	entityID := uuid.NewString()
	entityURL := "https://kontor.example/contracts/" + entityID
	rec := do(mux, "POST", "/api/v1/paperless/links/contract/"+entityID, jsonBody(AttachInput{
		EntityURL: entityURL,
		Documents: []AttachDocument{{ID: 7, Title: "Invoice"}},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("attach status = %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[attachResponse](t, rec)
	if len(resp.Links) != 1 || len(resp.Warnings) != 0 {
		t.Fatalf("resp = %+v", resp)
	}

	doc, _ := f.Document(7)
	if len(doc.CustomFields) != 1 {
		t.Fatalf("doc fields = %+v", doc.CustomFields)
	}
	if doc.CustomFields[0].Value != entityURL {
		t.Errorf("back-link = %v, want %s", doc.CustomFields[0].Value, entityURL)
	}
	if !f.HasTag(7, DefaultDoneTag) {
		t.Errorf("attach should add the done tag %q", DefaultDoneTag)
	}

	cfg, _ := store.GetConfig(context.Background(), testUserID)
	if cfg.CustomFieldID == 0 {
		t.Error("expected cached CustomFieldID")
	}

	rec = do(mux, "DELETE", "/api/v1/paperless/links/contract/"+entityID+"/7", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d", rec.Code)
	}
	doc, _ = f.Document(7)
	if len(doc.CustomFields) != 0 {
		t.Errorf("back-link should be cleared: %+v", doc.CustomFields)
	}
	if f.HasTag(7, DefaultDoneTag) {
		t.Error("detaching the last link should remove the done tag")
	}
	links, _ := store.ListLinks(context.Background(), testUserID, EntityContract, entityID)
	if len(links) != 0 {
		t.Errorf("links = %+v", links)
	}
}

func TestDetach_KeepsDoneTagWhileOtherLinksRemain(t *testing.T) {
	f := paperlesstest.New(t)
	f.AddDocument(paperlesstest.Document{ID: 7, Title: "Invoice"})
	mux, _ := newHandlerMux(t, testEncryptionKey)
	putConfig(t, mux, f.URL, "test-token")
	rec := do(mux, "PUT", "/api/v1/paperless/pipeline", jsonBody(PipelineSettings{InboxTag: "in", DoneTag: "linked"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT pipeline status = %d: %s", rec.Code, rec.Body.String())
	}

	first, second := uuid.NewString(), uuid.NewString()
	for _, id := range []string{first, second} {
		rec := do(mux, "POST", "/api/v1/paperless/links/purchase/"+id, jsonBody(AttachInput{
			EntityURL: "https://kontor.example/purchases/" + id,
			Documents: []AttachDocument{{ID: 7, Title: "Invoice"}},
		}))
		if rec.Code != http.StatusOK {
			t.Fatalf("attach status = %d", rec.Code)
		}
	}
	if !f.HasTag(7, "linked") {
		t.Fatal("configured done tag should be added")
	}

	do(mux, "DELETE", "/api/v1/paperless/links/purchase/"+first+"/7", nil)
	if !f.HasTag(7, "linked") {
		t.Error("done tag must stay while another entity links the document")
	}
	do(mux, "DELETE", "/api/v1/paperless/links/purchase/"+second+"/7", nil)
	if f.HasTag(7, "linked") {
		t.Error("done tag should be removed with the last link")
	}
}

func TestPipelineSettings_DefaultsAndValidation(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	rec := do(mux, "GET", "/api/v1/paperless/pipeline", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decodeJSON[PipelineSettings](t, rec); got != DefaultPipelineSettings() {
		t.Fatalf("defaults = %+v", got)
	}

	for _, bad := range []PipelineSettings{
		{InboxTag: "", DoneTag: "kontor"},
		{InboxTag: "a", DoneTag: " "},
		{InboxTag: "Kontor", DoneTag: "kontor"},
	} {
		rec := do(mux, "PUT", "/api/v1/paperless/pipeline", jsonBody(bad))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%+v: status = %d, want 400", bad, rec.Code)
		}
	}

	rec = do(mux, "PUT", "/api/v1/paperless/pipeline", jsonBody(PipelineSettings{Enabled: true, InboxTag: " inbox ", DoneTag: "done"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(mux, "GET", "/api/v1/paperless/pipeline", nil)
	want := PipelineSettings{Enabled: true, InboxTag: "inbox", DoneTag: "done"}
	if got := decodeJSON[PipelineSettings](t, rec); got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
}

func TestAttach_StoresLinkDespiteBacklinkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer server.Close()
	mux, store := newHandlerMux(t, testEncryptionKey)
	putConfig(t, mux, server.URL, "test-token")

	entityID := uuid.NewString()
	rec := do(mux, "POST", "/api/v1/paperless/links/transaction/"+entityID, jsonBody(AttachInput{
		EntityURL: "https://kontor.example/ledger/transactions/" + entityID,
		Documents: []AttachDocument{{ID: 3, Title: "Receipt"}},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("attach status = %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[attachResponse](t, rec)
	if len(resp.Warnings) == 0 {
		t.Error("expected warnings when paperless is down")
	}
	links, _ := store.ListLinks(context.Background(), testUserID, EntityTransaction, entityID)
	if len(links) != 1 {
		t.Errorf("link should be stored despite back-link failure, got %+v", links)
	}
}

func TestAttach_InvalidInput(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	entityID := uuid.NewString()
	cases := []AttachInput{
		{EntityURL: "", Documents: []AttachDocument{{ID: 1}}},
		{EntityURL: "not-a-url", Documents: []AttachDocument{{ID: 1}}},
		{EntityURL: "https://kontor.example/x", Documents: nil},
		{EntityURL: "https://kontor.example/x", Documents: []AttachDocument{{ID: 0}}},
	}
	for i, input := range cases {
		rec := do(mux, "POST", "/api/v1/paperless/links/contract/"+entityID, jsonBody(input))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: status = %d, want 400", i, rec.Code)
		}
	}
}

func TestDetach_NotFound(t *testing.T) {
	mux, _ := newHandlerMux(t, testEncryptionKey)
	rec := do(mux, "DELETE", "/api/v1/paperless/links/contract/"+uuid.NewString()+"/5", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
