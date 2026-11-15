package mcptool

import (
	"context"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"

	"github.com/reusing-code/kontor/backend/internal/storage"
)

// Error codes returned to agents. The code is the first word of the tool
// error text.
const (
	CodeInsufficientScope      = "insufficient_scope"
	CodeModuleDisabled         = "module_disabled"
	CodeNotFound               = "not_found"
	CodeInvalidInput           = "invalid_input"
	CodeAlreadyPending         = "already_pending"
	CodeTooManyPending         = "too_many_pending"
	CodePaperlessNotConfigured = "paperless_not_configured"
	CodePaperlessUnavailable   = "paperless_unavailable"
	CodePublicURLNotConfigured = "public_url_not_configured"
	// CodeConflict and CodeCancelled are transient: the same call can be repeated.
	CodeConflict  = "conflict"
	CodeCancelled = "cancelled"
	CodeInternal  = "internal_error"
)

// ToolError is a failure that is safe to show to the agent: a fixed code and
// a message that contains no stored user content.
type ToolError struct{ Code, Message string }

func (e *ToolError) Error() string { return e.Code + ": " + e.Message }

func Errorf(code, format string, args ...any) *ToolError {
	return &ToolError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// retryable maps the failures a repeated call can get past: a write that kept
// losing against concurrent transactions and a request that ended before the
// tool did.
func retryable(ctx context.Context, err error) *ToolError {
	switch {
	case errors.Is(err, storage.ErrRetriesExhausted), errors.Is(err, badger.ErrConflict):
		return &ToolError{Code: CodeConflict, Message: "concurrent update, try again"}
	case ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)):
		return &ToolError{Code: CodeCancelled, Message: "request ended before the tool finished, try again"}
	}
	return nil
}

func internalError() *ToolError { return &ToolError{Code: CodeInternal, Message: "internal error"} }
