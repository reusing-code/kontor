package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type testInput struct {
	Name string `json:"name"`
}

func (in *testInput) Validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

func TestDecode(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	tests := []struct {
		name, body string
		ok         bool
		status     int
		errText    string
	}{
		{"valid, validated in place", `{"name":"  A "}`, true, http.StatusOK, ""},
		{"not json", `{`, false, http.StatusBadRequest, "invalid request body"},
		{"unknown field", `{"name":"A","x":1}`, false, http.StatusBadRequest, "invalid request body"},
		{"invalid", `{"name":" "}`, false, http.StatusBadRequest, "name is required"},
		{"too large", `{"name":"` + strings.Repeat("a", MaxJSONBytes) + `"}`, false, http.StatusRequestEntityTooLarge, "request body too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			input, ok := Decode[testInput](logger, rec, jsonRequest(tt.body))
			if ok != tt.ok || rec.Code != tt.status {
				t.Fatalf("ok = %v, status = %d; want %v, %d", ok, rec.Code, tt.ok, tt.status)
			}
			if ok {
				if input.Name != "A" {
					t.Errorf("Name = %q, want the validated A", input.Name)
				}
				return
			}
			var resp map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil || resp["error"] != tt.errText {
				t.Errorf("response = %v (%v), want error %q", resp, err, tt.errText)
			}
		})
	}
}

func TestPathUUID(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	id := uuid.New()
	for _, tt := range []struct {
		value string
		ok    bool
	}{{id.String(), true}, {"nope", false}, {"", false}} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.SetPathValue("id", tt.value)
		rec := httptest.NewRecorder()
		got, ok := PathUUID(logger, rec, r, "id", "invalid vehicle id")
		if ok != tt.ok {
			t.Errorf("PathUUID(%q) ok = %v", tt.value, ok)
			continue
		}
		if ok && got != id {
			t.Errorf("PathUUID = %s, want %s", got, id)
		}
		if !ok && (rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid vehicle id")) {
			t.Errorf("PathUUID(%q) answered %d %s", tt.value, rec.Code, rec.Body.String())
		}
	}
}
