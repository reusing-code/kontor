package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func jsonRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
}

func TestReadJSON_RefusesBodyOverLimit(t *testing.T) {
	big := `{"name":"` + strings.Repeat("a", MaxJSONBytes) + `"}`
	var v struct{ Name string }
	err := ReadJSON(jsonRequest(big), &v)
	if !IsTooLarge(err) {
		t.Fatalf("err = %v, want a too-large error", err)
	}

	rec := httptest.NewRecorder()
	BodyError(slog.New(slog.DiscardHandler), rec, err, "invalid request body")
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if rec.Code != http.StatusRequestEntityTooLarge || resp["code"] != "body_too_large" {
		t.Errorf("response = %d %v", rec.Code, resp)
	}
}

func TestReadJSON_AcceptsBodyWithinLimit(t *testing.T) {
	var v struct{ Name string }
	if err := ReadJSON(jsonRequest(`{"name":"`+strings.Repeat("a", MaxJSONBytes-100)+`"}`), &v); err != nil {
		t.Fatalf("err = %v", err)
	}
	if err := ReadJSONLimit(jsonRequest(`{"name":"abcdef"}`), &v, 8); !IsTooLarge(err) {
		t.Errorf("custom limit: err = %v", err)
	}
}

func TestBodyError_OtherErrorsAreBadRequest(t *testing.T) {
	var v struct{ Name string }
	err := ReadJSON(jsonRequest(`{"other":1}`), &v)
	rec := httptest.NewRecorder()
	BodyError(slog.New(slog.DiscardHandler), rec, err, "invalid request body")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid request body") {
		t.Errorf("response = %d %s", rec.Code, rec.Body.String())
	}
}
