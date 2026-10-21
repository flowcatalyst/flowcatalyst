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
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Time is the canonical timestamp type for API responses. Always emits
// fixed-precision microsecond ISO-8601. Re-exported so api packages
// don't need a separate import.
type Time = jsontime.Time

// ErrorModel is the wire shape for every error response: { error, message },
// matching what [httperror.Write]
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
	huma.NewErrorWithContext = newErrorWithContext
	// The wire contract serializes arrays as non-nullable (`{"type":"array"}`).
	// huma defaults arrays to nullable (`{"type":["array","null"]}`), which would
	// diverge both the OpenAPI spec and the generated frontend client. All
	// our list handlers return non-nil slices (`make(...)`), so keep them
	// non-nullable.
	huma.DefaultArrayNullable = false
}

// StripBFFPaths removes /bff/* paths from the API's OpenAPI document —
// the published spec deliberately excludes BFF endpoints.
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
// silently ignore unknown fields — the leniency the API has always had.
//
// huma generates request-body schemas with `additionalProperties: false`, so
// ANY field a client sends that the Go DTO doesn't declare makes huma reject
// the whole request with a 400 before the handler runs. The SPA — generated
// against the lenient historical API — routinely sends supersets of what the Go DTO
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

// RelaxRequestBodiesDeep is RelaxRequestBodies for the named operations, all the
// way down: every object schema reachable from the request body (nested
// objects, list items) accepts and ignores unknown fields. For bodies a newer
// peer may extend at any depth, such as the function runners' trace batches: a
// runner that adds a field to a record must not have its whole batch refused
// by an older platform (the runner drops a refused batch).
//
// Only for types used by these operations' request bodies and nowhere else: it
// flips the shared component schemas. Call at the same place as
// RelaxRequestBodies, in the server and the spec dumper.
func RelaxRequestBodiesDeep(api huma.API, operationIDs ...string) {
	doc := api.OpenAPI()
	if doc == nil || doc.Paths == nil || doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	want := map[string]bool{}
	for _, id := range operationIDs {
		want[id] = true
	}
	reg := doc.Components.Schemas
	for _, item := range doc.Paths {
		if item == nil {
			continue
		}
		for _, op := range []*huma.Operation{item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch, item.Trace} {
			if op == nil || op.RequestBody == nil || !want[op.OperationID] {
				continue
			}
			seen := map[*huma.Schema]bool{}
			for _, mt := range op.RequestBody.Content {
				if mt != nil {
					relaxSchemaDeep(reg, mt.Schema, seen)
				}
			}
		}
	}
}

func relaxSchemaDeep(reg huma.Registry, s *huma.Schema, seen map[*huma.Schema]bool) {
	for s != nil && s.Ref != "" {
		s = reg.SchemaFromRef(s.Ref)
	}
	if s == nil || seen[s] {
		return
	}
	seen[s] = true
	if s.Type == huma.TypeObject {
		switch v := s.AdditionalProperties.(type) {
		case bool:
			if !v {
				s.AdditionalProperties = true
			}
		case nil:
			s.AdditionalProperties = true
		case *huma.Schema:
			relaxSchemaDeep(reg, v, seen)
		}
	}
	for _, p := range s.Properties {
		relaxSchemaDeep(reg, p, seen)
	}
	relaxSchemaDeep(reg, s.Items, seen)
	for _, group := range [][]*huma.Schema{s.AllOf, s.OneOf, s.AnyOf} {
		for _, sub := range group {
			relaxSchemaDeep(reg, sub, seen)
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
		if ue, ok := errors.AsType[*usecase.Error](e); ok {
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
		if d, ok := errors.AsType[*huma.ErrorDetail](e); ok {
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
	// 500 (the wire contract's catch-all — its status
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

// newErrorWithContext is huma's context-aware error constructor. It builds the
// response exactly as [newError] does, and additionally logs every 5xx ONCE at
// Error level: the wire envelope deliberately hides the cause, so without this
// line a handler's `usecase.Internal(code, msg, cause)` leaves no trace at all
// (huma's handler-error path does not go through httperror.Write).
func newErrorWithContext(ctx huma.Context, status int, message string, errs ...error) huma.StatusError {
	se := newError(status, message, errs...)
	if se.GetStatus() >= http.StatusInternalServerError {
		logInternal(ctx, se, errs)
	}
	return se
}

// logAttrser is implemented by errors that carry structured diagnostic fields
// (e.g. artifact.StoreError). Only non-secret fields may be exposed.
type logAttrser interface{ LogAttrs() []slog.Attr }

// urlQuery matches a URL's query string; presigned URLs carry their
// credentials/signature there, so anything after '?' in a URL is dropped.
var urlQuery = regexp.MustCompile(`(https?://[^\s"'?]+)\?[^\s"']*`)

func redact(s string) string { return urlQuery.ReplaceAllString(s, "$1?REDACTED") }

func logInternal(ctx huma.Context, se huma.StatusError, errs []error) {
	var code, msg string
	if em, ok := se.(*ErrorModel); ok {
		code, msg = em.Code, em.Message
	}
	attrs := []any{
		"code", code,
		"message", msg,
		"status", se.GetStatus(),
	}
	var cause error
	for _, e := range errs {
		if e == nil {
			continue
		}
		if cause == nil {
			cause = e
		}
		var la logAttrser
		if errors.As(e, &la) {
			for _, a := range la.LogAttrs() {
				attrs = append(attrs, a)
			}
		}
	}
	if cause != nil {
		attrs = append(attrs, "err", redact(cause.Error()))
	}
	l := slog.Default()
	if ctx != nil {
		attrs = append(attrs, "method", ctx.Method(), "path", ctx.URL().Path)
		l = logging.FromContext(ctx.Context())
		if id := chimw.GetReqID(ctx.Context()); id != "" {
			attrs = append(attrs, "request_id", id)
		}
	}
	l.Error("internal error response", attrs...)
}
