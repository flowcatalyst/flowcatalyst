package usecase

import (
	"errors"
	"fmt"
)

// Kind classifies a use case error. The Kind drives the HTTP status
// code mapping in API handlers.
type Kind string

const (
	KindValidation    Kind = "validation"
	KindBusinessRule  Kind = "business_rule"
	KindAuthorization Kind = "authorization"
	KindNotFound      Kind = "not_found"
	KindConflict      Kind = "conflict"
	KindInternal      Kind = "internal"
)

// Error is the canonical use case error. Implements the standard error
// interface; inspect with errors.As(err, &usecase.Error{}).
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Details map[string]any
	// Cause is the wrapped error, if any.
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %s: %v", e.Kind, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s: %s", e.Kind, e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// HTTPStatus maps the Kind to an HTTP status code.
func (e *Error) HTTPStatus() int {
	switch e.Kind {
	case KindValidation:
		return 400
	case KindAuthorization:
		return 403
	case KindNotFound:
		return 404
	case KindBusinessRule, KindConflict:
		return 409
	default:
		return 500
	}
}

// Validation builds a validation error. Use for field-level checks.
func Validation(code, message string) *Error {
	return &Error{Kind: KindValidation, Code: code, Message: message}
}

// BusinessRule builds a business rule violation error.
func BusinessRule(code, message string) *Error {
	return &Error{Kind: KindBusinessRule, Code: code, Message: message}
}

// Authorization builds an authorization error.
func Authorization(code, message string) *Error {
	return &Error{Kind: KindAuthorization, Code: code, Message: message}
}

// NotFound builds a not-found error.
func NotFound(code, message string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Message: message}
}

// Conflict builds a conflict error (e.g., uniqueness violation).
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

// Internal builds an internal error wrapping a lower-level cause.
func Internal(code, message string, cause error) *Error {
	return &Error{Kind: KindInternal, Code: code, Message: message, Cause: cause}
}

// WithDetails attaches structured details (e.g., field name → constraint).
func (e *Error) WithDetails(details map[string]any) *Error {
	e.Details = details
	return e
}

// AsError extracts a *Error from any error in the chain. Returns nil if absent.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
