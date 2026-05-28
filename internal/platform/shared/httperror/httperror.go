// Package httperror maps internal errors to HTTP responses with the
// JSON envelope shape used by the Rust platform:
//
//	{ "error": "ERR_CODE", "message": "human readable" }
//
// The `error` field carries the error CODE string, matching Rust's
// PlatformError → ErrorResponse { error, message } (shared/error.rs).
// Status code mapping must match the Rust PlatformError → response
// mapping byte-for-byte during the cutover. See docs/api-parity.md.
package httperror

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Envelope is the JSON response shape for all error responses.
// Matches Rust's PlatformError → ErrorResponse { error, message }.
// Code is serialized as the wire field "error"; Details is emitted only
// by the middleware ApiError path and omitted otherwise.
type Envelope struct {
	Code    string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Status returns the HTTP status code for a use case error.
func Status(err error) int {
	uc := usecase.AsError(err)
	if uc == nil {
		return http.StatusInternalServerError
	}
	switch uc.Kind {
	case usecase.KindValidation:
		return http.StatusBadRequest
	case usecase.KindAuthorization:
		return http.StatusForbidden
	case usecase.KindNotFound:
		return http.StatusNotFound
	case usecase.KindConflict:
		return http.StatusConflict
	case usecase.KindBusinessRule:
		// Rust maps business-rule violations to 409 (typical: uniqueness)
		// or 422 (state transitions). Default to 409 since uniqueness
		// is by far the dominant case; callers can override with a
		// specific Kind for state errors.
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// Write renders an error as the canonical JSON envelope + status code.
func Write(w http.ResponseWriter, err error) {
	uc := usecase.AsError(err)
	status := Status(err)
	env := Envelope{
		Code:    "INTERNAL",
		Message: "Internal server error",
	}
	if uc != nil {
		env.Code = uc.Code
		env.Message = uc.Message
		env.Details = uc.Details
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}

// Forbidden is a convenience for handler-layer permission rejections.
func Forbidden(msg string) error {
	return usecase.Authorization("FORBIDDEN", msg)
}

// BadRequest is a convenience for handler-layer input validation failures.
func BadRequest(code, msg string) error {
	return usecase.Validation(code, msg)
}

// NotFound builds a not-found error matching the Rust OrNotFound helper.
func NotFound(resource, id string) error {
	return usecase.NotFound(
		resource+"_NOT_FOUND",
		resource+" not found: "+id,
	)
}

// IsNotFound reports whether err is a not-found.
func IsNotFound(err error) bool {
	uc := usecase.AsError(err)
	return uc != nil && uc.Kind == usecase.KindNotFound
}

// IsValidation reports whether err is a validation error.
func IsValidation(err error) bool {
	uc := usecase.AsError(err)
	return uc != nil && uc.Kind == usecase.KindValidation
}

// As extracts a *usecase.Error from any error in the chain.
func As(err error) (*usecase.Error, bool) {
	var ue *usecase.Error
	if errors.As(err, &ue) {
		return ue, true
	}
	return nil, false
}
