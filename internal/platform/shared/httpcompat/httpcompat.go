// Package httpcompat is the integration layer between
// danielgtaylor/huma/v2 and FlowCatalyst's existing wire conventions.
//
// What it owns:
//
//   - The error envelope: huma errors marshal as the same
//     `{code, message, details}` envelope that [httperror.Write]
//     produces, so the wire format is identical whether a request flows
//     through a huma-registered handler or a legacy chi handler.
//   - The status-code mapping: [*usecase.Error.Kind] → HTTP status,
//     same table as [httperror.Status].
//   - The microsecond timestamp type: re-exported from
//     [jsontime.Time]. Use this on every API response struct that
//     carries a timestamp.
//
// The huma migration replaces the existing chi handlers per-aggregate.
// Use [Init] to wire the error transformer into a huma API on startup;
// thereafter `return nil, err` from a huma handler produces the
// canonical envelope.
package httpcompat

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Time is the canonical timestamp type for API responses. Always emits
// fixed-precision microsecond ISO-8601. Re-exported so api packages
// don't need a separate import.
type Time = jsontime.Time

// ErrorModel is the wire shape for every error response. Matches what
// the existing [httperror.Write] emits, which matches what the Rust
// platform emits, which is what the consumer SDKs parse.
//
// The unexported `status` field is set at construct time so huma can
// honor whatever HTTP status the source [*usecase.Error] mapped to.
type ErrorModel struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	status  int
}

// Error implements the standard error interface so ErrorModel can flow
// through huma as a regular error and round-trip through middleware
// chains.
func (e *ErrorModel) Error() string { return e.Message }

// GetStatus reports the HTTP status code for this error. Huma calls
// this to decide the response status when an handler returns an error.
func (e *ErrorModel) GetStatus() int {
	if e.status != 0 {
		return e.status
	}
	return statusFor(e.Code)
}

// Init wires the error transformer into huma's package-level
// constructor. Call once at startup, before mounting the huma API.
//
// After Init, any handler that returns an error has it run through
// [transform]: *usecase.Error values become *ErrorModel with the right
// code/message/details; other errors fall back to a generic 500.
func Init() {
	huma.NewError = newError
}

// newError is huma's pluggable constructor for error responses. We
// intentionally ignore the supplied status — the status is derived
// from the [*usecase.Error.Kind] so handlers don't have to thread it.
func newError(_ int, message string, errs ...error) huma.StatusError {
	for _, e := range errs {
		var ue *usecase.Error
		if errors.As(e, &ue) {
			return &ErrorModel{
				Code:    ue.Code,
				Message: ue.Message,
				Details: ue.Details,
				status:  ue.HTTPStatus(),
			}
		}
	}
	// Huma synthesises errors for its own validation failures (the
	// message arg). Preserve them as VALIDATION-shaped envelopes so the
	// shape on the wire stays consistent.
	if message == "" {
		message = "Internal server error"
	}
	return &ErrorModel{Code: "VALIDATION", Message: message, status: http.StatusBadRequest}
}

// statusFor returns the HTTP status code for an envelope code.
func statusFor(code string) int {
	switch code {
	case "VALIDATION", "INVALID_JSON", "BAD_REQUEST":
		return http.StatusBadRequest
	case "FORBIDDEN":
		return http.StatusForbidden
	case "UNAUTHORIZED":
		return http.StatusUnauthorized
	case "":
		return http.StatusInternalServerError
	}
	// Default: derive from suffix conventions used in the codebase.
	// `*_NOT_FOUND` → 404; `*_EXISTS` → 409; everything else falls back
	// to 422 (BusinessRule) per the Rust mapping table.
	if len(code) > 10 && code[len(code)-10:] == "_NOT_FOUND" {
		return http.StatusNotFound
	}
	if len(code) > 7 && code[len(code)-7:] == "_EXISTS" {
		return http.StatusConflict
	}
	return http.StatusUnprocessableEntity
}

