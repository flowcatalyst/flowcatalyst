package common

import (
	"fmt"
	"net/http"
)

// ErrorKind represents the category of use case error.
// Each kind maps to a specific HTTP status code.
type ErrorKind int

const (
	// ErrorKindValidation represents input validation failures.
	// Maps to HTTP 400 Bad Request.
	ErrorKindValidation ErrorKind = iota

	// ErrorKindBusinessRule represents business rule violations.
	// Maps to HTTP 409 Conflict.
	ErrorKindBusinessRule

	// ErrorKindNotFound represents entity not found errors.
	// Maps to HTTP 404 Not Found.
	ErrorKindNotFound

	// ErrorKindConcurrency represents optimistic locking conflicts.
	// Maps to HTTP 409 Conflict.
	ErrorKindConcurrency

	// ErrorKindUnauthorized represents authentication/authorization failures.
	// Maps to HTTP 401/403.
	ErrorKindUnauthorized

	// ErrorKindInternal represents unexpected internal errors.
	// Maps to HTTP 500 Internal Server Error.
	ErrorKindInternal
)

// String returns the string representation of the error kind.
func (k ErrorKind) String() string {
	switch k {
	case ErrorKindValidation:
		return "VALIDATION"
	case ErrorKindBusinessRule:
		return "BUSINESS_RULE"
	case ErrorKindNotFound:
		return "NOT_FOUND"
	case ErrorKindConcurrency:
		return "CONCURRENCY"
	case ErrorKindUnauthorized:
		return "UNAUTHORIZED"
	case ErrorKindInternal:
		return "INTERNAL"
	default:
		return "UNKNOWN"
	}
}

// HTTPStatus returns the HTTP status code for this error kind.
func (k ErrorKind) HTTPStatus() int {
	switch k {
	case ErrorKindValidation:
		return http.StatusBadRequest
	case ErrorKindBusinessRule:
		return http.StatusConflict
	case ErrorKindNotFound:
		return http.StatusNotFound
	case ErrorKindConcurrency:
		return http.StatusConflict
	case ErrorKindUnauthorized:
		return http.StatusForbidden
	case ErrorKindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// UseCaseError represents an error from a use case execution.
// It contains structured information about what went wrong,
// suitable for both logging and API responses.
type UseCaseError struct {
	Kind    ErrorKind      `json:"kind"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *UseCaseError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Kind.String(), e.Code, e.Message)
}

// HTTPStatus returns the appropriate HTTP status code for this error.
func (e *UseCaseError) HTTPStatus() int {
	return e.Kind.HTTPStatus()
}

// WithDetail adds a detail to the error and returns it for chaining.
func (e *UseCaseError) WithDetail(key string, value any) *UseCaseError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

// ValidationError creates a new validation error.
// Use for input validation failures (missing required fields, invalid format, etc.)
// Maps to HTTP 400 Bad Request.
func ValidationError(code, message string, details map[string]any) *UseCaseError {
	return &UseCaseError{
		Kind:    ErrorKindValidation,
		Code:    code,
		Message: message,
		Details: details,
	}
}

// BusinessRuleError creates a new business rule violation error.
// Use for business logic violations (entity in wrong state, constraint violated, etc.)
// Maps to HTTP 409 Conflict.
func BusinessRuleError(code, message string, details map[string]any) *UseCaseError {
	return &UseCaseError{
		Kind:    ErrorKindBusinessRule,
		Code:    code,
		Message: message,
		Details: details,
	}
}

// NotFoundError creates a new not found error.
// Use when an entity cannot be found by ID or other criteria.
// Maps to HTTP 404 Not Found.
func NotFoundError(code, message string, details map[string]any) *UseCaseError {
	return &UseCaseError{
		Kind:    ErrorKindNotFound,
		Code:    code,
		Message: message,
		Details: details,
	}
}

// ConcurrencyError creates a new concurrency/optimistic locking error.
// Use when an entity was modified by another transaction.
// Maps to HTTP 409 Conflict.
func ConcurrencyError(code, message string, details map[string]any) *UseCaseError {
	return &UseCaseError{
		Kind:    ErrorKindConcurrency,
		Code:    code,
		Message: message,
		Details: details,
	}
}

// UnauthorizedError creates a new authorization error.
// Use when the principal lacks permission to perform the operation.
// Maps to HTTP 403 Forbidden.
func UnauthorizedError(code, message string, details map[string]any) *UseCaseError {
	return &UseCaseError{
		Kind:    ErrorKindUnauthorized,
		Code:    code,
		Message: message,
		Details: details,
	}
}

// InternalError creates a new internal error.
// Use for unexpected errors that shouldn't happen in normal operation.
// Maps to HTTP 500 Internal Server Error.
func InternalError(code, message string, details map[string]any) *UseCaseError {
	return &UseCaseError{
		Kind:    ErrorKindInternal,
		Code:    code,
		Message: message,
		Details: details,
	}
}

// Common error codes for reuse across use cases
const (
	// Validation error codes
	ErrCodeRequired        = "REQUIRED"
	ErrCodeInvalidFormat   = "INVALID_FORMAT"
	ErrCodeTooLong         = "TOO_LONG"
	ErrCodeTooShort        = "TOO_SHORT"
	ErrCodeInvalidValue    = "INVALID_VALUE"
	ErrCodeInvalidEmail    = "INVALID_EMAIL"
	ErrCodeInvalidPassword = "INVALID_PASSWORD"

	// Business rule error codes
	ErrCodeAlreadyExists   = "ALREADY_EXISTS"
	ErrCodeDuplicateCode   = "DUPLICATE_CODE"
	ErrCodeDuplicateEmail  = "DUPLICATE_EMAIL"
	ErrCodeInvalidState    = "INVALID_STATE"
	ErrCodeOperationFailed = "OPERATION_FAILED"
	ErrCodeCommitFailed    = "COMMIT_FAILED"

	// Not found error codes
	ErrCodeEntityNotFound     = "ENTITY_NOT_FOUND"
	ErrCodeEventTypeNotFound  = "EVENT_TYPE_NOT_FOUND"
	ErrCodeSubscriptionNotFound = "SUBSCRIPTION_NOT_FOUND"
	ErrCodeDispatchPoolNotFound = "DISPATCH_POOL_NOT_FOUND"
	ErrCodePrincipalNotFound  = "PRINCIPAL_NOT_FOUND"
	ErrCodeClientNotFound     = "CLIENT_NOT_FOUND"
	ErrCodeRoleNotFound       = "ROLE_NOT_FOUND"

	// Authorization error codes
	ErrCodeAccessDenied   = "ACCESS_DENIED"
	ErrCodeInsufficientPermissions = "INSUFFICIENT_PERMISSIONS"
)
