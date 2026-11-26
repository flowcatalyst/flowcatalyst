// Package apiroute collapses the standard huma route registration
// ceremony into one line per route. Every platform CRUD route sets
// exactly six Operation fields — {OperationID, Method, Path, Summary,
// Tags, DefaultStatus} — so the helpers here take just those and build
// the Operation verbatim; the generated OpenAPI spec is byte-identical
// to a hand-rolled huma.Register call. A route that needs its own request
// body cap passes MaxBodyBytes (an Option); a route needing anything else
// beyond the six fields keeps using huma.Register directly.
package apiroute

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Group carries the per-module registration context. Modules with more
// than one tag (e.g. auth) create one Group per tag.
type Group struct {
	API huma.API
	Tag string
}

// New returns a Group that registers routes under tag.
func New(api huma.API, tag string) Group {
	return Group{API: api, Tag: tag}
}

// Option sets something on a route beyond the six fields. It changes how
// the route serves, not the OpenAPI document.
type Option func(*huma.Operation)

// MaxBodyBytes caps the route's request body at n bytes, in place of huma's
// default of 1 MiB. huma refuses a larger body before the handler runs,
// answered 413 BODY_TOO_LARGE (httpcompat). n <= 0 leaves the default.
func MaxBodyBytes(n int64) Option {
	return func(op *huma.Operation) {
		if n > 0 {
			op.MaxBodyBytes = n
		}
	}
}

// Get registers a GET route. Every platform GET responds 200, so
// DefaultStatus is fixed.
func Get[I, O any](g Group, id, path, summary string, h func(context.Context, *I) (*O, error), opts ...Option) {
	register(g, http.MethodGet, http.StatusOK, id, path, summary, h, opts)
}

// Post registers a POST route with the supplied DefaultStatus.
func Post[I, O any](g Group, id, path, summary string, status int, h func(context.Context, *I) (*O, error), opts ...Option) {
	register(g, http.MethodPost, status, id, path, summary, h, opts)
}

// Put registers a PUT route with the supplied DefaultStatus.
func Put[I, O any](g Group, id, path, summary string, status int, h func(context.Context, *I) (*O, error), opts ...Option) {
	register(g, http.MethodPut, status, id, path, summary, h, opts)
}

// Patch registers a PATCH route with the supplied DefaultStatus.
func Patch[I, O any](g Group, id, path, summary string, status int, h func(context.Context, *I) (*O, error), opts ...Option) {
	register(g, http.MethodPatch, status, id, path, summary, h, opts)
}

// Delete registers a DELETE route with the supplied DefaultStatus.
func Delete[I, O any](g Group, id, path, summary string, status int, h func(context.Context, *I) (*O, error), opts ...Option) {
	register(g, http.MethodDelete, status, id, path, summary, h, opts)
}

func register[I, O any](g Group, method string, status int, id, path, summary string, h func(context.Context, *I) (*O, error), opts []Option) {
	op := huma.Operation{
		OperationID:   id,
		Method:        method,
		Path:          path,
		Summary:       summary,
		Tags:          []string{g.Tag},
		DefaultStatus: status,
	}
	for _, o := range opts {
		o(&op)
	}
	huma.Register(g.API, op, h)
}
