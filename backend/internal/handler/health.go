package handler

import (
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/store"
)

func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Readyz(w http.ResponseWriter, _ *http.Request) {
	if bs, ok := h.store.(*store.BadgerStore); ok {
		if err := bs.Healthy(); err != nil {
			h.errorResponse(w, http.StatusServiceUnavailable, "db unhealthy")
			return
		}
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
