package enablebanking

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
)

var (
	errSigning = errors.New("signing request token failed")
	errorCode  = regexp.MustCompile(`^[A-Z_]{1,64}$`)
)

// APIError is a non-2xx answer. Only the machine-readable upstream code is
// kept; upstream message texts are dropped.
type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("enable banking: status %d", e.Status)
	}
	return fmt.Sprintf("enable banking: status %d (%s)", e.Status, e.Code)
}

// TransportError is a failure before a response arrived. It carries only the
// underlying error, never the request URL.
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return "enable banking: request failed: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

func apiError(err error) (*APIError, bool) {
	var apiErr *APIError
	return apiErr, errors.As(err, &apiErr)
}

func hasCode(err error, codes ...string) bool {
	apiErr, ok := apiError(err)
	if !ok {
		return false
	}
	for _, c := range codes {
		if apiErr.Code == c {
			return true
		}
	}
	return false
}

func IsRateLimited(err error) bool {
	apiErr, ok := apiError(err)
	return ok && (apiErr.Status == http.StatusTooManyRequests || apiErr.Code == "ASPSP_RATE_LIMIT_EXCEEDED")
}

// IsSessionUnusable reports that the consent is expired, revoked, closed or unknown.
func IsSessionUnusable(err error) bool {
	return hasCode(err, "EXPIRED_SESSION", "REVOKED_SESSION", "CLOSED_SESSION", "SESSION_DOES_NOT_EXIST")
}

func IsAccountNotAccessible(err error) bool {
	return hasCode(err, "ASPSP_ACCOUNT_NOT_ACCESSIBLE", "ACCOUNT_DOES_NOT_EXIST", "ACCESS_DENIED")
}

// IsAppAuth reports a problem with the application's own credentials.
func IsAppAuth(err error) bool {
	return hasCode(err, "UNAUTHORIZED_ACCESS", "AUTHORIZATION_NOT_PROVIDED", "UNAUTHORIZED_IP", "INVALID_HOST")
}

func IsWrongTransactionsPeriod(err error) bool {
	return hasCode(err, "WRONG_TRANSACTIONS_PERIOD")
}

// IsUnavailable reports a 5xx answer, a bank-side error or a transport failure.
func IsUnavailable(err error) bool {
	var transport *TransportError
	if errors.As(err, &transport) {
		return true
	}
	apiErr, ok := apiError(err)
	return ok && (apiErr.Status >= 500 || hasCode(err, "ASPSP_ERROR", "ASPSP_TIMEOUT"))
}
