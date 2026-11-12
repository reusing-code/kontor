package mcptool

import "fmt"

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
	CodeInternal               = "internal_error"
)

// ToolError is a failure that is safe to show to the agent: a fixed code and
// a message that contains no stored user content.
type ToolError struct{ Code, Message string }

func (e *ToolError) Error() string { return e.Code + ": " + e.Message }

func Errorf(code, format string, args ...any) *ToolError {
	return &ToolError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func internalError() *ToolError { return &ToolError{Code: CodeInternal, Message: "internal error"} }
