package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetCapabilities(t *testing.T) {
	for _, llm := range []bool{false, true} {
		h, _ := newTestHandler(t)
		h.SetCapabilities(Capabilities{LLM: llm})

		rec := httptest.NewRecorder()
		h.GetCapabilities(rec, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		got := decodeJSON[map[string]any](t, rec)
		if len(got) != 1 || got["llm"] != llm {
			t.Errorf("body = %v, want {llm: %v}", got, llm)
		}
	}
}
