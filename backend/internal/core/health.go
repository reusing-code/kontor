package core

import (
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/httputil"
)

func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(h.logger, w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Readyz(w http.ResponseWriter, _ *http.Request) {
	if err := h.store.Healthy(); err != nil {
		httputil.Error(h.logger, w, http.StatusServiceUnavailable, "db unhealthy")
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, map[string]string{"status": "ok"})
}
