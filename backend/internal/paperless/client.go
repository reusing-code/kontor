package paperless

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	requestTimeout  = 15 * time.Second
	apiVersion      = "9"
	CustomFieldName = "Kontor"
	pageSize        = 20
	listPageSize    = 100
)

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// Client talks to a single paperless-ngx instance with a user's token.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Accept", "application/json; version="+apiVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := c.newRequest(ctx, method, path, reader)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling paperless: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("paperless returned %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding paperless response: %w", err)
	}
	return nil
}

// Ping verifies the connection and token with a minimal document query.
func (c *Client) Ping(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodGet, "/api/documents/?page_size=1", nil, &struct{}{})
}

type searchHit struct {
	Highlights string `json:"highlights"`
}

type documentResult struct {
	ID        int        `json:"id"`
	Title     string     `json:"title"`
	Created   string     `json:"created"`
	SearchHit *searchHit `json:"__search_hit__"`
}

type documentPage struct {
	Count   int              `json:"count"`
	Results []documentResult `json:"results"`
}

// SearchDocuments runs a full-text query, or lists the most recent documents
// when the query is empty.
func (c *Client) SearchDocuments(ctx context.Context, query string, page int) (SearchPage, error) {
	if page < 1 {
		page = 1
	}
	params := url.Values{}
	params.Set("page", strconv.Itoa(page))
	params.Set("page_size", strconv.Itoa(pageSize))
	if strings.TrimSpace(query) != "" {
		params.Set("query", query)
	} else {
		params.Set("ordering", "-created")
	}
	var dp documentPage
	if err := c.doJSON(ctx, http.MethodGet, "/api/documents/?"+params.Encode(), nil, &dp); err != nil {
		return SearchPage{}, fmt.Errorf("searching documents: %w", err)
	}
	results := make([]SearchResult, 0, len(dp.Results))
	for _, doc := range dp.Results {
		res := SearchResult{ID: doc.ID, Title: doc.Title, Created: doc.Created}
		if doc.SearchHit != nil {
			res.Snippet = strings.TrimSpace(htmlTagPattern.ReplaceAllString(doc.SearchHit.Highlights, ""))
		}
		results = append(results, res)
	}
	return SearchPage{Count: dp.Count, Page: page, PageSize: pageSize, Results: results}, nil
}

// Thumbnail streams a document thumbnail. The caller must close the reader.
func (c *Client) Thumbnail(ctx context.Context, documentID int) (io.ReadCloser, string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/api/documents/%d/thumb/", documentID), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Del("Accept")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetching thumbnail: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, "", fmt.Errorf("paperless returned %s", resp.Status)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

type customField struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

type customFieldPage struct {
	Count   int           `json:"count"`
	Next    *string       `json:"next"`
	Results []customField `json:"results"`
}

// EnsureCustomField finds the Kontor URL custom field by name, creating it if
// missing, and returns its ID.
func (c *Client) EnsureCustomField(ctx context.Context) (int, error) {
	path := "/api/custom_fields/"
	for {
		var page customFieldPage
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &page); err != nil {
			return 0, fmt.Errorf("listing custom fields: %w", err)
		}
		for _, field := range page.Results {
			if field.Name == CustomFieldName {
				return field.ID, nil
			}
		}
		if page.Next == nil || *page.Next == "" {
			break
		}
		next, err := url.Parse(*page.Next)
		if err != nil {
			return 0, fmt.Errorf("parsing custom fields page url: %w", err)
		}
		path = next.RequestURI()
	}
	var created customField
	body := map[string]string{"name": CustomFieldName, "data_type": "url"}
	if err := c.doJSON(ctx, http.MethodPost, "/api/custom_fields/", body, &created); err != nil {
		return 0, fmt.Errorf("creating custom field: %w", err)
	}
	return created.ID, nil
}

type customFieldValue struct {
	Field int `json:"field"`
	Value any `json:"value"`
}

type documentFields struct {
	CustomFields []customFieldValue `json:"custom_fields"`
}

func (c *Client) getDocumentFields(ctx context.Context, documentID int) ([]customFieldValue, error) {
	var doc documentFields
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/documents/%d/", documentID), nil, &doc); err != nil {
		return nil, fmt.Errorf("fetching document %d: %w", documentID, err)
	}
	return doc.CustomFields, nil
}

func (c *Client) patchDocumentFields(ctx context.Context, documentID int, fields []customFieldValue) error {
	body := documentFields{CustomFields: fields}
	if err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/api/documents/%d/", documentID), body, nil); err != nil {
		return fmt.Errorf("updating document %d custom fields: %w", documentID, err)
	}
	return nil
}

// SetCustomField sets the back-link field on a document, preserving other
// custom field values.
func (c *Client) SetCustomField(ctx context.Context, documentID, fieldID int, value string) error {
	fields, err := c.getDocumentFields(ctx, documentID)
	if err != nil {
		return err
	}
	updated := false
	for i := range fields {
		if fields[i].Field == fieldID {
			fields[i].Value = value
			updated = true
		}
	}
	if !updated {
		fields = append(fields, customFieldValue{Field: fieldID, Value: value})
	}
	return c.patchDocumentFields(ctx, documentID, fields)
}

// ClearCustomFieldIfMatches removes the back-link field from a document, but
// only when its current value still points at the given URL — a document
// attached to another entity keeps that entity's back-link.
func (c *Client) ClearCustomFieldIfMatches(ctx context.Context, documentID, fieldID int, value string) error {
	fields, err := c.getDocumentFields(ctx, documentID)
	if err != nil {
		return err
	}
	kept := fields[:0]
	found := false
	for _, f := range fields {
		if f.Field == fieldID {
			current, _ := f.Value.(string)
			if current == value {
				found = true
				continue
			}
		}
		kept = append(kept, f)
	}
	if !found {
		return nil
	}
	return c.patchDocumentFields(ctx, documentID, kept)
}

// DocumentURL is the paperless web UI page of a document.
func DocumentURL(baseURL string, documentID int) string {
	return fmt.Sprintf("%s/documents/%d/details", strings.TrimRight(baseURL, "/"), documentID)
}

type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type tagPage struct {
	Results []Tag `json:"results"`
}

// FindTag looks up a tag by name, case-insensitively as paperless does.
// found is false when no such tag exists.
func (c *Client) FindTag(ctx context.Context, name string) (id int, found bool, err error) {
	params := url.Values{}
	params.Set("name__iexact", name)
	var page tagPage
	if err := c.doJSON(ctx, http.MethodGet, "/api/tags/?"+params.Encode(), nil, &page); err != nil {
		return 0, false, fmt.Errorf("looking up tag: %w", err)
	}
	for _, tag := range page.Results {
		if strings.EqualFold(tag.Name, name) {
			return tag.ID, true, nil
		}
	}
	return 0, false, nil
}

// EnsureTag returns the ID of the named tag, creating it without automatic
// matching when it does not exist.
func (c *Client) EnsureTag(ctx context.Context, name string) (int, error) {
	id, found, err := c.FindTag(ctx, name)
	if err != nil || found {
		return id, err
	}
	var created Tag
	body := map[string]any{"name": name, "matching_algorithm": 0}
	if err := c.doJSON(ctx, http.MethodPost, "/api/tags/", body, &created); err != nil {
		return 0, fmt.Errorf("creating tag: %w", err)
	}
	return created.ID, nil
}

type Document struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Created string `json:"created"`
	Content string `json:"content"`
	Tags    []int  `json:"tags"`
}

type documentListPage struct {
	Next    *string    `json:"next"`
	Results []Document `json:"results"`
}

// ListDocumentsWithTag returns the documents carrying the tag, oldest first,
// without their content and capped at limit.
func (c *Client) ListDocumentsWithTag(ctx context.Context, tagID, limit int) ([]Document, error) {
	params := url.Values{}
	params.Set("tags__id__all", strconv.Itoa(tagID))
	params.Set("ordering", "added")
	params.Set("page_size", strconv.Itoa(min(limit, listPageSize)))
	params.Set("fields", "id,title,created,tags")
	path := "/api/documents/?" + params.Encode()
	docs := []Document{}
	for len(docs) < limit {
		var page documentListPage
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, fmt.Errorf("listing tagged documents: %w", err)
		}
		docs = append(docs, page.Results...)
		if page.Next == nil || *page.Next == "" || len(page.Results) == 0 {
			break
		}
		next, err := url.Parse(*page.Next)
		if err != nil {
			return nil, fmt.Errorf("parsing documents page url: %w", err)
		}
		path = next.RequestURI()
	}
	return docs[:min(len(docs), limit)], nil
}

// GetDocument fetches a document including its OCR content.
func (c *Client) GetDocument(ctx context.Context, documentID int) (Document, error) {
	var doc Document
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/documents/%d/", documentID), nil, &doc); err != nil {
		return Document{}, fmt.Errorf("fetching document %d: %w", documentID, err)
	}
	return doc, nil
}

// ModifyTags adds and removes tags on documents in one bulk edit.
func (c *Client) ModifyTags(ctx context.Context, documentIDs, add, remove []int) error {
	if len(documentIDs) == 0 || (len(add) == 0 && len(remove) == 0) {
		return nil
	}
	body := map[string]any{
		"documents": documentIDs,
		"method":    "modify_tags",
		"parameters": map[string]any{
			"add_tags":    nonNil(add),
			"remove_tags": nonNil(remove),
		},
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/documents/bulk_edit/", body, nil); err != nil {
		return fmt.Errorf("modifying document tags: %w", err)
	}
	return nil
}

func nonNil(ids []int) []int {
	if ids == nil {
		return []int{}
	}
	return ids
}
