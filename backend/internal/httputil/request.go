package httputil

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

// Validator is a request input that checks itself.
type Validator interface {
	Validate() error
}

// Decode reads a JSON body (ReadJSON: at most MaxJSONBytes, no unknown
// fields) into a T and validates it. On failure it has answered the request:
// 413 for a body over the limit, 400 "invalid request body" for one that
// does not decode, 400 with the validation error otherwise.
func Decode[T any, PT interface {
	*T
	Validator
}](logger *slog.Logger, w http.ResponseWriter, r *http.Request) (T, bool) {
	var input T
	if err := ReadJSON(r, &input); err != nil {
		BodyError(logger, w, err, "invalid request body")
		return input, false
	}
	if err := PT(&input).Validate(); err != nil {
		Error(logger, w, http.StatusBadRequest, err.Error())
		return input, false
	}
	return input, true
}

// PathUUID parses the path value name as a UUID. On failure it has answered
// the request with 400 and msg.
func PathUUID(logger *slog.Logger, w http.ResponseWriter, r *http.Request, name, msg string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		Error(logger, w, http.StatusBadRequest, msg)
		return uuid.UUID{}, false
	}
	return id, true
}
