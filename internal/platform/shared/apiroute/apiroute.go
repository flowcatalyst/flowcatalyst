// Package apiroute collapses the standard huma route registration
// ceremony into one line per route. Every platform CRUD route sets
// exactly six Operation fields — {OperationID, Method, Path, Summary,
// Tags, DefaultStatus} — so the helpers here take just those and build
// the Operation verbatim; the generated OpenAPI spec is byte-identical
// to a hand-rolled huma.Register call. Routes needing anything beyond
// the six fields keep using huma.Register directly.
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

// Get registers a GET route. Every platform GET responds 200, so
// DefaultStatus is fixed.
func Get[I, O any](g Group, id, path, summary string, h func(context.Context, *I) (*O, error)) {
	register(g, http.MethodGet, http.StatusOK, id, path, summary, h)
}

// Post registers a POST route with the supplied DefaultStatus.
func Post[I, O any](g Group, id, path, summary string, status int, h func(context.Context, *I) (*O, error)) {
	register(g, http.MethodPost, status, id, path, summary, h)
}

// Put registers a PUT route with the supplied DefaultStatus.
func Put[I, O any](g Group, id, path, summary string, status int, h func(context.Context, *I) (*O, error)) {
	register(g, http.MethodPut, status, id, path, summary, h)
}

// Delete registers a DELETE route with the supplied DefaultStatus.
func Delete[I, O any](g Group, id, path, summary string, status int, h func(context.Context, *I) (*O, error)) {
	register(g, http.MethodDelete, status, id, path, summary, h)
}

func register[I, O any](g Group, method string, status int, id, path, summary string, h func(context.Context, *I) (*O, error)) {
	huma.Register(g.API, huma.Operation{
		OperationID:   id,
		Method:        method,
		Path:          path,
		Summary:       summary,
		Tags:          []string{g.Tag},
		DefaultStatus: status,
	}, h)
}
