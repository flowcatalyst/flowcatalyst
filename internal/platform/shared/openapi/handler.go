package openapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts:
//
//	GET /api/openapi.json — the assembled spec as JSON
//
// Idempotent on the chi router but mounting twice would clobber the
// first registration. Call once per Doc.
func RegisterRoutes(r chi.Router, doc *Doc) {
	r.Get("/api/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Cache-busting matters: the spec only changes on deploy, but a
		// developer mid-iteration may want to see updates without a
		// browser cache hit.
		w.Header().Set("Cache-Control", "no-cache")
		_ = json.NewEncoder(w).Encode(doc.Spec())
	})
}
