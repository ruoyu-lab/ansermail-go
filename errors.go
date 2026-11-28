package anser

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is an error the API answered with. Code is the API's error code ("not_found", "validation_error", …),
// Param the field it is about ("to[1]", "attachments[0].content") or "", and RequestID what to look up in the
// request logs.
//
//	var apiErr *anser.Error
//	if errors.As(err, &apiErr) && apiErr.Code == "not_found" { … }
type Error struct {
	StatusCode int
	Code       string
	Message    string
	Param      string
	RequestID  string
	Header     http.Header
}

func (e *Error) Error() string {
	if e.Param != "" {
		return fmt.Sprintf("anser: %d %s: %s: %s", e.StatusCode, e.Code, e.Param, e.Message)
	}
	return fmt.Sprintf("anser: %d %s: %s", e.StatusCode, e.Code, e.Message)
}

// retryable: turned away before it ran (a rate limit, the same key still running), or a server error on a
// safe call.
func (e *Error) retryable(safe bool) bool {
	switch e.StatusCode {
	case http.StatusTooManyRequests:
		return e.Code == "rate_limit_exceeded"
	case http.StatusConflict:
		return e.Code == "idempotency_key_in_use"
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return safe
	}
	return false
}

// ConnectionError means no answer: the connection failed or the request timed out.
type ConnectionError struct{ Err error }

func (e *ConnectionError) Error() string { return "anser: no answer: " + e.Err.Error() }
func (e *ConnectionError) Unwrap() error { return e.Err }

// ErrWebhookVerification is wrapped by every error VerifyWebhook returns.
var ErrWebhookVerification = errors.New("anser: webhook verification failed")
