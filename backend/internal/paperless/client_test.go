package paperless

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/paperless/paperlesstest"
)

func newFake(t *testing.T) (*paperlesstest.Server, *Client) {
	t.Helper()
	f := paperlesstest.New(t)
	return f, NewClient(f.URL, "test-token")
}

func TestClientSendsAuthAndVersionHeaders(t *testing.T) {
	f, c := newFake(t)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	auth, accept := f.LastHeaders()
	if auth != "Token test-token" {
		t.Errorf("Authorization = %q, want %q", auth, "Token test-token")
	}
	if accept != "application/json; version="+apiVersion {
		t.Errorf("Accept = %q", accept)
	}
}

func TestSearchDocuments(t *testing.T) {
	f, c := newFake(t)
	f.AddDocument(paperlesstest.Document{ID: 1, Title: "Invoice 2026", Created: "2026-01-01"})

	page, err := c.SearchDocuments(context.Background(), "invoice", 1)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if page.Count != 1 || len(page.Results) != 1 {
		t.Fatalf("page = %+v", page)
	}
	res := page.Results[0]
	if res.Title != "Invoice 2026" {
		t.Errorf("Title = %q", res.Title)
	}
	if res.Snippet != "match context" {
		t.Errorf("Snippet = %q, want HTML stripped %q", res.Snippet, "match context")
	}
}

func TestSearchDocuments_EmptyQueryListsRecent(t *testing.T) {
	f, c := newFake(t)
	f.AddDocument(paperlesstest.Document{ID: 1, Title: "Doc", Created: "2026-01-01"})

	page, err := c.SearchDocuments(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if len(page.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(page.Results))
	}
	if page.Results[0].Snippet != "" {
		t.Errorf("Snippet = %q, want empty for non-search listing", page.Results[0].Snippet)
	}
}

func TestEnsureCustomField_CreatesThenFinds(t *testing.T) {
	f, c := newFake(t)

	id, err := c.EnsureCustomField(context.Background())
	if err != nil {
		t.Fatalf("EnsureCustomField: %v", err)
	}
	if id != 1 {
		t.Fatalf("id = %d, want 1", id)
	}
	fields := f.CustomFields()
	if len(fields) != 1 || fields[0].DataType != "url" {
		t.Fatalf("customFields = %+v", fields)
	}

	id2, err := c.EnsureCustomField(context.Background())
	if err != nil {
		t.Fatalf("EnsureCustomField (second): %v", err)
	}
	if id2 != id {
		t.Errorf("second call id = %d, want %d (no duplicate)", id2, id)
	}
	if len(f.CustomFields()) != 1 {
		t.Errorf("expected 1 custom field, got %d", len(f.CustomFields()))
	}
}

func TestSetCustomField_PreservesOtherFields(t *testing.T) {
	f, c := newFake(t)
	f.AddDocument(paperlesstest.Document{ID: 5, Title: "Doc", CustomFields: []paperlesstest.CustomFieldValue{{Field: 9, Value: "keep"}}})

	if err := c.SetCustomField(context.Background(), 5, 1, "https://kontor.example/a"); err != nil {
		t.Fatalf("SetCustomField: %v", err)
	}
	doc, _ := f.Document(5)
	if len(doc.CustomFields) != 2 {
		t.Fatalf("fields = %+v", doc.CustomFields)
	}
	if err := c.SetCustomField(context.Background(), 5, 1, "https://kontor.example/b"); err != nil {
		t.Fatalf("SetCustomField (update): %v", err)
	}
	doc, _ = f.Document(5)
	if len(doc.CustomFields) != 2 {
		t.Fatalf("expected value update not append, fields = %+v", doc.CustomFields)
	}
	for _, fv := range doc.CustomFields {
		if fv.Field == 1 && fv.Value != "https://kontor.example/b" {
			t.Errorf("field 1 value = %v", fv.Value)
		}
	}
}

func TestClearCustomFieldIfMatches(t *testing.T) {
	f, c := newFake(t)
	f.AddDocument(paperlesstest.Document{ID: 5, CustomFields: []paperlesstest.CustomFieldValue{
		{Field: 1, Value: "https://kontor.example/other"},
		{Field: 9, Value: "keep"},
	}})

	if err := c.ClearCustomFieldIfMatches(context.Background(), 5, 1, "https://kontor.example/mine"); err != nil {
		t.Fatalf("ClearCustomFieldIfMatches: %v", err)
	}
	if doc, _ := f.Document(5); len(doc.CustomFields) != 2 {
		t.Fatalf("field should not be cleared on mismatch: %+v", doc.CustomFields)
	}

	if err := c.ClearCustomFieldIfMatches(context.Background(), 5, 1, "https://kontor.example/other"); err != nil {
		t.Fatalf("ClearCustomFieldIfMatches: %v", err)
	}
	doc, _ := f.Document(5)
	if len(doc.CustomFields) != 1 || doc.CustomFields[0].Field != 9 {
		t.Fatalf("fields = %+v", doc.CustomFields)
	}
}

func TestClientUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	defer server.Close()

	err := NewClient(server.URL, "bad").Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestFindAndEnsureTag(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	f.AddTag("Other")

	if _, found, err := c.FindTag(ctx, "kontor"); err != nil || found {
		t.Fatalf("FindTag missing: found=%v err=%v", found, err)
	}
	id, err := c.EnsureTag(ctx, "kontor")
	if err != nil {
		t.Fatalf("EnsureTag: %v", err)
	}
	if id != f.TagID("kontor") {
		t.Fatalf("EnsureTag id = %d, want %d", id, f.TagID("kontor"))
	}
	again, err := c.EnsureTag(ctx, "KONTOR")
	if err != nil || again != id {
		t.Fatalf("EnsureTag (case-insensitive) = %d, %v; want %d", again, err, id)
	}
}

func TestListDocumentsWithTag_PagesAndCaps(t *testing.T) {
	f, c := newFake(t)
	inbox := f.AddTag("kontor-inbox")
	for i := 1; i <= 250; i++ {
		tags := []int{}
		if i%2 == 0 {
			tags = append(tags, inbox)
		}
		f.AddDocument(paperlesstest.Document{ID: i, Title: "Doc", Tags: tags})
	}

	docs, err := c.ListDocumentsWithTag(context.Background(), inbox, 1000)
	if err != nil {
		t.Fatalf("ListDocumentsWithTag: %v", err)
	}
	if len(docs) != 125 {
		t.Fatalf("got %d documents, want 125 across pages", len(docs))
	}
	capped, err := c.ListDocumentsWithTag(context.Background(), inbox, 30)
	if err != nil {
		t.Fatalf("ListDocumentsWithTag (capped): %v", err)
	}
	if len(capped) != 30 || capped[0].ID != 2 {
		t.Fatalf("capped = %d docs starting at %d", len(capped), capped[0].ID)
	}
}

func TestGetDocumentAndModifyTags(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	inbox := f.AddTag("kontor-inbox")
	done := f.AddTag("kontor")
	f.AddDocument(paperlesstest.Document{ID: 3, Title: "Invoice", Content: "Total 12.00", Tags: []int{inbox, 99}})

	doc, err := c.GetDocument(ctx, 3)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc.Content != "Total 12.00" || doc.Title != "Invoice" {
		t.Fatalf("doc = %+v", doc)
	}

	if err := c.ModifyTags(ctx, []int{3}, []int{done}, []int{inbox}); err != nil {
		t.Fatalf("ModifyTags: %v", err)
	}
	if f.HasTag(3, "kontor-inbox") || !f.HasTag(3, "kontor") {
		t.Fatalf("tags not swapped")
	}
	got, _ := f.Document(3)
	if len(got.Tags) != 2 {
		t.Fatalf("other tags must be kept: %v", got.Tags)
	}
}
