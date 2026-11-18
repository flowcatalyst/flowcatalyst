package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// APIError is returned when the platform responds with a non-2xx
// status code. Inspect via errors.As(err, &client.APIError{}).
type APIError struct {
	// StatusCode is the HTTP status returned by the platform.
	StatusCode int
	// Body is the raw response body. May contain a structured error
	// envelope (see Code, Message, Details below) or a plain string.
	Body string
}

// Error renders a human-readable summary. Includes the status code and
// the structured Code if one is present in the body.
func (e *APIError) Error() string {
	if code, msg := e.codeAndMessage(); code != "" || msg != "" {
		return fmt.Sprintf("flowcatalyst api: %d %s: %s — %s", e.StatusCode, http.StatusText(e.StatusCode), code, msg)
	}
	if e.Body != "" {
		return fmt.Sprintf("flowcatalyst api: %d %s: %s", e.StatusCode, http.StatusText(e.StatusCode), e.Body)
	}
	return fmt.Sprintf("flowcatalyst api: %d %s", e.StatusCode, http.StatusText(e.StatusCode))
}

// Code returns the platform's error code if the body is a structured
// envelope (e.g. {"code": "EVENT_TYPE_NOT_FOUND", "message": "..."}),
// or "" if the body is unstructured.
func (e *APIError) Code() string {
	code, _ := e.codeAndMessage()
	return code
}

// Message returns the platform's error message if present.
func (e *APIError) Message() string {
	_, msg := e.codeAndMessage()
	return msg
}

// Retryable reports whether the status is a transient one (408, 425, 429,
// 500, 502, 503, 504). Whether sending the request again is safe also
// depends on the request: a 500, 502, 503 or 504 to a POST that creates or
// changes something may have been applied, and the client does not repeat
// it (see WithRetry and OutcomeUnknownError).
func (e *APIError) Retryable() bool {
	switch e.StatusCode {
	case http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// IsNotFound is a convenience for status 404.
func (e *APIError) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsUnauthorized is a convenience for status 401.
func (e *APIError) IsUnauthorized() bool { return e.StatusCode == http.StatusUnauthorized }

// IsForbidden is a convenience for status 403.
func (e *APIError) IsForbidden() bool { return e.StatusCode == http.StatusForbidden }

// IsConflict is a convenience for status 409.
func (e *APIError) IsConflict() bool { return e.StatusCode == http.StatusConflict }

// codeAndMessage attempts to parse the body as a structured error
// envelope. Falls back to ("","") on any parsing failure.
func (e *APIError) codeAndMessage() (string, string) {
	if e.Body == "" {
		return "", ""
	}
	var env struct {
		Error            string `json:"error"`             // platform code + OAuth error code
		Code             string `json:"code"`              // legacy envelope shape
		Message          string `json:"message"`           // platform message
		ErrorDescription string `json:"error_description"` // OAuth message
	}
	if err := json.Unmarshal([]byte(e.Body), &env); err != nil {
		return "", ""
	}
	code := env.Error
	if code == "" {
		code = env.Code
	}
	msg := env.Message
	if msg == "" {
		msg = env.ErrorDescription
	}
	return code, msg
}

// OutcomeUnknownError is returned when a request that is not repeatable (a
// POST that creates or changes something) may have been applied by the
// platform but its answer was lost: a gateway answered 502, 503 or 504, or
// the connection failed or timed out after the request may have been sent.
// The client did not send it again. Read the state (for example GET the
// record) before deciding to send it again.
//
// Unwrap yields the *APIError (gateway status) or the transport error, so
// errors.As(err, &apiErr) still sees a 502. Test for it with
// IsOutcomeUnknown or errors.As(err, &outcomeErr).
type OutcomeUnknownError struct {
	// Method and URL are the request's.
	Method string
	URL    string
	// StatusCode is the gateway's status (502, 503, 504), or 0 when the
	// request ended with a transport error.
	StatusCode int
	// Body is the gateway's response body, if any.
	Body string
	// Err is the *APIError for a gateway status, else the transport error.
	Err error
}

// Error says what was sent, that it may have been applied, and why.
func (e *OutcomeUnknownError) Error() string {
	return fmt.Sprintf("flowcatalyst api: %s %s: outcome unknown, the request may have been applied and was not repeated: %v",
		e.Method, e.URL, e.Err)
}

// Unwrap returns the *APIError or the transport error.
func (e *OutcomeUnknownError) Unwrap() error { return e.Err }

// IsOutcomeUnknown reports whether err is, or wraps, an *OutcomeUnknownError.
func IsOutcomeUnknown(err error) bool {
	var o *OutcomeUnknownError
	return errors.As(err, &o)
}
