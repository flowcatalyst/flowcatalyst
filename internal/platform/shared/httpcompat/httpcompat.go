// Package httpcompat is the integration layer between
// danielgtaylor/huma/v2 and FlowCatalyst's existing wire conventions.
//
// What it owns:
//
//   - The error envelope: huma errors marshal as the same
//     `{error, message}` envelope that [httperror.Write]
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
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Time is the canonical timestamp type for API responses. Always emits
// fixed-precision microsecond ISO-8601. Re-exported so api packages
// don't need a separate import.
type Time = jsontime.Time

// ErrorModel is the wire shape for every error response. Matches Rust's
// PlatformError → ErrorResponse { error, message } and what [httperror.Write]
// emits, which is what the consumer SDKs parse. Code is serialized as the
// wire field "error".
//
// The unexported `status` field is set at construct time so huma can
// honor whatever HTTP status the source [*usecase.Error] mapped to.
type ErrorModel struct {
	Code    string         `json:"error"`
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
	// Rust serializes arrays as non-nullable (`{"type":"array"}`). huma
	// defaults arrays to nullable (`{"type":["array","null"]}`), which would
	// diverge both the OpenAPI spec and the generated frontend client. All
	// our list handlers return non-nil slices (`make(...)`), so match Rust.
	huma.DefaultArrayNullable = false
}

// StripBFFPaths removes /bff/* paths from the API's OpenAPI document so the
// published spec matches Rust (which excludes BFF endpoints from its spec).
// The handlers stay mounted and keep serving; only the spec omits them. Call
// once after all routes are registered, before the spec is served/dumped.
func StripBFFPaths(api huma.API) {
	doc := api.OpenAPI()
	if doc == nil || doc.Paths == nil {
		return
	}
	for p := range doc.Paths {
		if strings.HasPrefix(p, "/bff/") {
			delete(doc.Paths, p)
		}
	}
}

// RelaxRequestBodies makes every operation's JSON request body accept and
// silently ignore unknown fields, matching Rust/serde's default leniency.
//
// huma generates request-body schemas with `additionalProperties: false`, so
// ANY field a client sends that the Go DTO doesn't declare makes huma reject
// the whole request with a 400 before the handler runs. The SPA — generated
// against the lenient Rust API — routinely sends supersets of what the Go DTO
// models, which silently breaks flows. This is the #1 recurring parity bug
// class (see the spa-go-compat audit). Flipping the top-level request-body
// schema to `additionalProperties: true` accepts those extra fields and drops
// the ones the DTO doesn't bind.
//
// Scope is deliberately narrow:
//   - Only the directly-referenced request-body schema is relaxed. Response
//     schemas stay strict so the Hey-API-generated frontend client keeps tight
//     response types, and nested request objects (which may be shared with
//     responses) are left alone — every known break is a top-level field.
//   - It only flips an explicit/absent `additionalProperties:false`; a
//     schema-valued additionalProperties (a typed map body) is never clobbered.
//
// This stops the 400 — it does NOT make a dropped field round-trip. Fields the
// SPA expects PERSISTED (e.g. subscription connectionId, application logo)
// still have to be modeled on the DTO and written through; leniency is just the
// safety net so future SPA-superset fields stop 400-ing.
//
// Call once after all routes are registered, before the spec is served. The
// runtime path honors this too: [huma.Validate] re-resolves the body $ref
// against the live registry on every request, so mutating the component schema
// here changes validation, not just the emitted document. Must be applied in
// both the server (WirePlatform) and the spec dumper so the lockfile matches.
func RelaxRequestBodies(api huma.API) {
	doc := api.OpenAPI()
	if doc == nil || doc.Paths == nil || doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	reg := doc.Components.Schemas
	for _, item := range doc.Paths {
		if item == nil {
			continue
		}
		for _, op := range []*huma.Operation{
			item.Get, item.Put, item.Post, item.Delete,
			item.Options, item.Head, item.Patch, item.Trace,
		} {
			relaxOperationBody(reg, op)
		}
	}
}

func relaxOperationBody(reg huma.Registry, op *huma.Operation) {
	if op == nil || op.RequestBody == nil {
		return
	}
	for _, mt := range op.RequestBody.Content {
		if mt == nil || mt.Schema == nil {
			continue
		}
		// Struct bodies are stored as a $ref into the registry — resolve it
		// to the live component schema that validation also resolves to.
		s := mt.Schema
		for s != nil && s.Ref != "" {
			s = reg.SchemaFromRef(s.Ref)
		}
		if s == nil || s.Type != huma.TypeObject {
			continue
		}
		switch v := s.AdditionalProperties.(type) {
		case bool:
			if !v {
				s.AdditionalProperties = true
			}
		case nil:
			s.AdditionalProperties = true
		}
	}
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
	// Huma synthesises errors for its own request validation (bad/missing/
	// unknown fields). Surface the per-field details under `details.errors`
	// so the caller (and the SPA) can see WHICH field failed and why, instead
	// of an opaque "validation failed". Without this every body-validation
	// failure looks identical and is undebuggable from the response.
	if message == "" {
		message = "Internal server error"
	}
	var details map[string]any
	if fields := validationDetails(errs); len(fields) > 0 {
		details = map[string]any{"errors": fields}
	}
	return &ErrorModel{Code: "VALIDATION", Message: message, Details: details, status: http.StatusBadRequest}
}

// validationDetails flattens huma's request-validation errors into a
// JSON-friendly list of {message, location, value}. huma passes each field
// failure as a *huma.ErrorDetail; anything else is rendered by its Error()
// string.
func validationDetails(errs []error) []map[string]any {
	out := make([]map[string]any, 0, len(errs))
	for _, e := range errs {
		if e == nil {
			continue
		}
		var d *huma.ErrorDetail
		if errors.As(e, &d) {
			m := map[string]any{"message": d.Message}
			if d.Location != "" {
				m["location"] = d.Location
			}
			if d.Value != nil {
				m["value"] = d.Value
			}
			out = append(out, m)
			continue
		}
		out = append(out, map[string]any{"message": e.Error()})
	}
	return out
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
	// `*_NOT_FOUND` → 404; `*_EXISTS` → 409; unknown codes fall back to
	// 500, matching Rust's PlatformError catch-all (the Rust platform
	// mapping has no 422). The live path always carries a Kind via
	// *usecase.Error, so this fallback only fires for bare code strings.
	if len(code) > 10 && code[len(code)-10:] == "_NOT_FOUND" {
		return http.StatusNotFound
	}
	if len(code) > 7 && code[len(code)-7:] == "_EXISTS" {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
