// Package openapi builds the FlowCatalyst Go server's OpenAPI 3.0
// spec at runtime by reflecting on the Command/Response Go types each
// api package already exposes. The spec is served at
// `/api/openapi.json` and consumed by:
//
//   - The parity strategy's Layer 1 spec-diff job (docs/api-parity.md).
//     `oasdiff breaking <rust.json> <go.json>` fails on any breaking
//     change between the two binaries.
//   - The frontend's Hey-API codegen (`pnpm api:generate`). Until the
//     Go spec lands the frontend reads Rust's spec; switch the
//     `openapi-ts.config.ts` source once we trust Go's emitter.
//   - Swagger UI at `/api/swagger` (separate follow-up — see HANDOFF).
//
// # Usage pattern
//
// Each `internal/platform/<aggregate>/api` package owns a small
// `OpenAPI(doc *openapi.Doc)` function alongside its `RegisterRoutes`.
// `WirePlatform` builds the Doc once, threads it through every
// aggregate's registrar, and mounts `/api/openapi.json`:
//
//	doc := openapi.NewDoc("FlowCatalyst Platform API", Version)
//	eventtypeapi.OpenAPI(doc)
//	principalapi.OpenAPI(doc)
//	subscriptionapi.OpenAPI(doc)
//	openapi.RegisterRoutes(r, doc)
//
// New aggregates: implement `OpenAPI(doc *openapi.Doc)` alongside
// `RegisterRoutes` and add a single line in WirePlatform. Eventually
// the two functions should fuse into a single `Mount(r, doc, state)`
// per package so route + spec can't drift — separate refactor.
//
// # Why programmatic vs comment-based (swag, etc.)
//
// Programmatic specs live next to the code that owns the route, so
// they participate in normal Go review and refactor flows. Comments
// drift silently when handlers change. Slightly more verbose to write
// per route, but the verbosity is offset by the Op() builder below.
package openapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
)

// Doc is a thread-safe builder for an OpenAPI 3.0 spec. Hand it to
// each aggregate's OpenAPI() registrar during WirePlatform.
type Doc struct {
	mu   sync.Mutex
	spec *openapi3.T
	// gen reflects on Go types to produce schemas. Shared so repeated
	// reflection of the same type returns the same $ref.
	gen *openapi3gen.Generator
}

// NewDoc seeds an empty spec with the supplied title + version.
func NewDoc(title, version string) *Doc {
	return &Doc{
		spec: &openapi3.T{
			OpenAPI: "3.0.3",
			Info: &openapi3.Info{
				Title:   title,
				Version: version,
			},
			Paths: openapi3.NewPaths(),
			Components: &openapi3.Components{
				Schemas: openapi3.Schemas{},
			},
		},
		gen: openapi3gen.NewGenerator(openapi3gen.UseAllExportedFields()),
	}
}

// Spec returns the assembled spec. Safe to call after every registrar
// has run.
func (d *Doc) Spec() *openapi3.T {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.spec
}

// Schema generates (or returns the cached) `#/components/schemas/<name>`
// $ref for the given Go value. Pass a non-nil pointer to a zero-value
// struct or a slice literal; reflect-based generators don't work with
// nil interface values.
//
// The schema is registered under `name` in the components/schemas
// table so the produced $ref is stable across calls.
func (d *Doc) Schema(name string, sample any) *openapi3.SchemaRef {
	d.mu.Lock()
	defer d.mu.Unlock()
	if existing, ok := d.spec.Components.Schemas[name]; ok {
		return &openapi3.SchemaRef{Ref: "#/components/schemas/" + name, Value: existing.Value}
	}
	ref, err := d.gen.NewSchemaRefForValue(sample, d.spec.Components.Schemas)
	if err != nil {
		// Fall back to a free-form object schema rather than panicking;
		// the consumer will see an under-specified schema, not a
		// missing one.
		ref = &openapi3.SchemaRef{Value: openapi3.NewObjectSchema()}
	}
	d.spec.Components.Schemas[name] = ref
	return &openapi3.SchemaRef{Ref: "#/components/schemas/" + name, Value: ref.Value}
}

// OperationOption configures a registered operation.
type OperationOption func(*openapi3.Operation, *Doc)

// Op registers a single operation on the spec. `method` must be one of
// the canonical HTTP method strings ("GET", "POST", ...); `path` uses
// OpenAPI's `{param}` syntax (`/api/event-types/{id}`); `opID` becomes
// the operationId the frontend's TS codegen turns into a method name
// (camelCase, no spaces, e.g. `listEventTypes`).
//
// Returns the operation so callers can attach more options after the
// fact — but the variadic options handle the common case.
func (d *Doc) Op(method, path, opID, summary string, opts ...OperationOption) *openapi3.Operation {
	op := &openapi3.Operation{
		OperationID: opID,
		Summary:     summary,
		Responses:   openapi3.NewResponses(),
	}
	for _, opt := range opts {
		opt(op, d)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	pi := d.spec.Paths.Find(path)
	if pi == nil {
		pi = &openapi3.PathItem{}
		d.spec.Paths.Set(path, pi)
	}
	switch method {
	case http.MethodGet:
		pi.Get = op
	case http.MethodPost:
		pi.Post = op
	case http.MethodPut:
		pi.Put = op
	case http.MethodDelete:
		pi.Delete = op
	case http.MethodPatch:
		pi.Patch = op
	}
	return op
}

// ── OperationOption builders ─────────────────────────────────────────────

// Tag attaches a tag (used by Swagger UI for grouping).
func Tag(name string) OperationOption {
	return func(op *openapi3.Operation, _ *Doc) {
		op.Tags = append(op.Tags, name)
	}
}

// PathParam declares a path parameter (e.g. `{id}` in
// `/api/event-types/{id}`).
func PathParam(name, description string) OperationOption {
	return func(op *openapi3.Operation, _ *Doc) {
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        name,
				In:          "path",
				Required:    true,
				Description: description,
				Schema:      &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
			},
		})
	}
}

// QueryParam declares an optional query-string parameter. Pass a
// zero-value sample for the parameter's Go type (e.g. `(*string)(nil)`
// or `""`) so the schema reflects the expected shape.
func QueryParam(name, description string, sample any) OperationOption {
	return func(op *openapi3.Operation, d *Doc) {
		s := openapi3.NewStringSchema()
		// Best-effort type detection — defaults to string if reflect
		// can't resolve.
		if sample != nil {
			t := reflect.TypeOf(sample)
			if t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			switch t.Kind() {
			case reflect.Int, reflect.Int32, reflect.Int64:
				s = openapi3.NewIntegerSchema()
			case reflect.Bool:
				s = openapi3.NewBoolSchema()
			}
		}
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        name,
				In:          "query",
				Required:    false,
				Description: description,
				Schema:      &openapi3.SchemaRef{Value: s},
			},
		})
	}
}

// RequestBody declares an application/json request body, with the
// schema generated from `sample` (a zero-value pointer to the struct).
// `name` is the components/schemas key — should be the struct's type
// name for clean cross-references.
func RequestBody(name, description string, sample any) OperationOption {
	return func(op *openapi3.Operation, d *Doc) {
		ref := d.Schema(name, sample)
		op.RequestBody = &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Description: description,
				Required:    true,
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{Schema: ref},
				},
			},
		}
	}
}

// Response declares one possible response. Most operations have at
// least a 2xx + an error response. `sample` may be nil for empty
// bodies (e.g. 204 No Content).
func Response(status int, description, schemaName string, sample any) OperationOption {
	return func(op *openapi3.Operation, d *Doc) {
		resp := &openapi3.Response{Description: &description}
		if sample != nil && schemaName != "" {
			ref := d.Schema(schemaName, sample)
			resp.Content = openapi3.Content{
				"application/json": &openapi3.MediaType{Schema: ref},
			}
		}
		op.Responses.Set(strconv.Itoa(status), &openapi3.ResponseRef{Value: resp})
	}
}

// MarshalJSON renders the spec for the /api/openapi.json handler.
func (d *Doc) MarshalJSON() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return json.Marshal(d.spec)
}
