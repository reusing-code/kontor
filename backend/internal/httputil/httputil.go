package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/storage"
)

func WriteJSON(logger *slog.Logger, w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Error("encoding response", "error", err)
	}
}

func ReadJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func Error(logger *slog.Logger, w http.ResponseWriter, status int, msg string) {
	WriteJSON(logger, w, status, map[string]string{"error": msg})
}

// StoreError maps storage sentinel errors to HTTP responses.
func StoreError(logger *slog.Logger, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		Error(logger, w, http.StatusNotFound, "not found")
	case errors.Is(err, storage.ErrConflict):
		Error(logger, w, http.StatusConflict, err.Error())
	default:
		logger.Error("store error", "error", err)
		Error(logger, w, http.StatusInternalServerError, "internal error")
	}
}
