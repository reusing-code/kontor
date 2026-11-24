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

// MaxJSONBytes caps a JSON request body read by ReadJSON.
const MaxJSONBytes = 1 << 20

// ReadJSON decodes a JSON body of at most MaxJSONBytes, refusing unknown fields.
func ReadJSON(r *http.Request, v any) error {
	return ReadJSONLimit(r, v, MaxJSONBytes)
}

// ReadJSONLimit is ReadJSON with its own size limit. A body over the limit
// fails with an error IsTooLarge reports.
func ReadJSONLimit(r *http.Request, v any, limit int64) error {
	body := http.MaxBytesReader(nil, r.Body, limit)
	defer body.Close()
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// LimitBody caps what handlers can read from r.Body. Wrap the body before
// ParseMultipartForm, which otherwise only caps memory and spills the rest
// to disk.
func LimitBody(w http.ResponseWriter, r *http.Request, limit int64) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
}

// IsTooLarge reports whether err comes from a body over its size limit.
func IsTooLarge(err error) bool {
	_, ok := errors.AsType[*http.MaxBytesError](err)
	return ok
}

// TooLarge answers 413 with the code body_too_large.
func TooLarge(logger *slog.Logger, w http.ResponseWriter) {
	WriteJSON(logger, w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large", "code": "body_too_large"})
}

// BodyError answers a failed ReadJSON: 413 for a body over the limit,
// otherwise 400 with msg.
func BodyError(logger *slog.Logger, w http.ResponseWriter, err error, msg string) {
	if IsTooLarge(err) {
		TooLarge(logger, w)
		return
	}
	Error(logger, w, http.StatusBadRequest, msg)
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
