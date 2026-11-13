package core

import (
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/httputil"
)

// Capabilities are server-wide features the frontend can toggle on.
type Capabilities struct {
	LLM bool `json:"llm"`
	// LLMVision is true when a vision model is configured, so images can be sent.
	LLMVision  bool `json:"llmVision"`
	BankImport bool `json:"bankImport"`
}

func (h *Handler) SetCapabilities(c Capabilities) {
	h.capabilities = c
}

func (h *Handler) GetCapabilities(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(h.logger, w, http.StatusOK, h.capabilities)
}
