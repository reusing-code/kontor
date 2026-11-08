// Package paperlesstest provides an in-memory paperless-ngx API double.
package paperlesstest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type CustomFieldValue struct {
	Field int `json:"field"`
	Value any `json:"value"`
}

type CustomField struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Document struct {
	ID           int                `json:"id"`
	Title        string             `json:"title"`
	Created      string             `json:"created"`
	Content      string             `json:"content"`
	Tags         []int              `json:"tags"`
	CustomFields []CustomFieldValue `json:"custom_fields"`
}

// Server serves the subset of the paperless-ngx API Kontor uses. All state
// is guarded by a mutex; use the accessors from tests.
type Server struct {
	URL string

	mu           sync.Mutex
	documents    map[int]*Document
	customFields []CustomField
	tags         []Tag
	lastAuth     string
	lastAccept   string
	failAll      int
	contentReads int
}

func New(t *testing.T) *Server {
	t.Helper()
	s := &Server{documents: map[int]*Document{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/documents/", s.listDocuments)
	mux.HandleFunc("GET /api/documents/{id}/", s.getDocument)
	mux.HandleFunc("PATCH /api/documents/{id}/", s.patchDocument)
	mux.HandleFunc("POST /api/documents/bulk_edit/", s.bulkEdit)
	mux.HandleFunc("GET /api/documents/{id}/thumb/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		w.Write([]byte("thumb-bytes"))
	})
	mux.HandleFunc("GET /api/custom_fields/", s.listCustomFields)
	mux.HandleFunc("POST /api/custom_fields/", s.createCustomField)
	mux.HandleFunc("GET /api/tags/", s.listTags)
	mux.HandleFunc("POST /api/tags/", s.createTag)
	server := httptest.NewServer(s.record(mux))
	t.Cleanup(server.Close)
	s.URL = server.URL
	return s
}

func (s *Server) record(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.lastAuth = r.Header.Get("Authorization")
		s.lastAccept = r.Header.Get("Accept")
		fail := s.failAll
		s.mu.Unlock()
		if fail != 0 {
			http.Error(w, "fake failure", fail)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// FailAll makes every request answer with the given status; 0 restores
// normal operation.
func (s *Server) FailAll(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failAll = status
}

func (s *Server) LastHeaders() (auth, accept string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastAuth, s.lastAccept
}

func (s *Server) AddDocument(d Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[d.ID] = &d
}

// Document returns a copy of the stored document.
func (s *Server) Document(id int) (Document, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.documents[id]
	if !ok {
		return Document{}, false
	}
	cp := *d
	cp.Tags = slices.Clone(d.Tags)
	cp.CustomFields = slices.Clone(d.CustomFields)
	return cp, true
}

// ContentReads counts single-document fetches (the only call returning content).
func (s *Server) ContentReads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contentReads
}

func (s *Server) CustomFields() []CustomField {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.customFields)
}

// AddTag creates a tag and returns its ID.
func (s *Server) AddTag(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addTagLocked(name)
}

func (s *Server) addTagLocked(name string) int {
	id := len(s.tags) + 1
	s.tags = append(s.tags, Tag{ID: id, Name: name})
	return id
}

// TagID returns the ID of the named tag, or 0.
func (s *Server) TagID(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tags {
		if strings.EqualFold(t.Name, name) {
			return t.ID
		}
	}
	return 0
}

// HasTag reports whether the document carries the named tag.
func (s *Server) HasTag(documentID int, name string) bool {
	id := s.TagID(name)
	d, ok := s.Document(documentID)
	return ok && id != 0 && slices.Contains(d.Tags, id)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := r.URL.Query()
	tagFilter := atoi(q.Get("tags__id__all"))
	ids := make([]int, 0, len(s.documents))
	for id, d := range s.documents {
		if tagFilter != 0 && !slices.Contains(d.Tags, tagFilter) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)

	page := max(atoi(q.Get("page")), 1)
	size := atoi(q.Get("page_size"))
	if size <= 0 {
		size = 25
	}
	start := min((page-1)*size, len(ids))
	end := min(start+size, len(ids))
	results := []map[string]any{}
	for _, id := range ids[start:end] {
		d := s.documents[id]
		entry := map[string]any{"id": d.ID, "title": d.Title, "created": d.Created, "tags": nonNil(d.Tags)}
		if q.Get("query") != "" {
			entry["__search_hit__"] = map[string]any{"highlights": "<span>match</span> context"}
		}
		results = append(results, entry)
	}
	var next any
	if end < len(ids) {
		nq := r.URL.Query()
		nq.Set("page", strconv.Itoa(page+1))
		next = fmt.Sprintf("http://%s%s?%s", r.Host, r.URL.Path, nq.Encode())
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(ids), "next": next, "results": results})
}

func nonNil(ids []int) []int {
	if ids == nil {
		return []int{}
	}
	return ids
}

func (s *Server) getDocument(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.documents[atoi(r.PathValue("id"))]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.contentReads++
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) patchDocument(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.documents[atoi(r.PathValue("id"))]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var body struct {
		CustomFields []CustomFieldValue `json:"custom_fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	d.CustomFields = body.CustomFields
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) bulkEdit(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body struct {
		Documents  []int  `json:"documents"`
		Method     string `json:"method"`
		Parameters struct {
			AddTags    []int `json:"add_tags"`
			RemoveTags []int `json:"remove_tags"`
		} `json:"parameters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Method != "modify_tags" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	for _, id := range body.Documents {
		d, ok := s.documents[id]
		if !ok {
			http.Error(w, "not found", http.StatusBadRequest)
			return
		}
		tags := slices.DeleteFunc(d.Tags, func(t int) bool { return slices.Contains(body.Parameters.RemoveTags, t) })
		for _, t := range body.Parameters.AddTags {
			if !slices.Contains(tags, t) {
				tags = append(tags, t)
			}
		}
		d.Tags = tags
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "OK"})
}

func (s *Server) listCustomFields(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"count": len(s.customFields), "next": nil, "results": s.customFields})
}

func (s *Server) createCustomField(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body map[string]string
	json.NewDecoder(r.Body).Decode(&body)
	field := CustomField{ID: len(s.customFields) + 1, Name: body["name"], DataType: body["data_type"]}
	s.customFields = append(s.customFields, field)
	writeJSON(w, http.StatusCreated, field)
}

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := r.URL.Query().Get("name__iexact")
	results := []Tag{}
	for _, t := range s.tags {
		if name == "" || strings.EqualFold(t.Name, name) {
			results = append(results, t)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(results), "next": nil, "results": results})
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	id := s.addTagLocked(body.Name)
	writeJSON(w, http.StatusCreated, Tag{ID: id, Name: body.Name})
}
