package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetCapabilities(t *testing.T) {
	for _, on := range []bool{false, true} {
		h, _ := newTestHandler(t)
		h.SetCapabilities(Capabilities{LLM: on, BankImport: !on})

		rec := httptest.NewRecorder()
		h.GetCapabilities(rec, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		got := decodeJSON[map[string]any](t, rec)
		if len(got) != 2 || got["llm"] != on || got["bankImport"] != !on {
			t.Errorf("body = %v, want {llm: %v, bankImport: %v}", got, on, !on)
		}
	}
}
