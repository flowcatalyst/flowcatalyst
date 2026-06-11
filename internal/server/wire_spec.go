package server

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

// registerSpecRoutes mounts the spec + Swagger UI on the PARENT router
// (outside the Authenticator Group) so tooling — oasdiff, the Hey-API
// codegen in the Vue frontend, browser visitors to /api/docs — can fetch
// them without a bearer token. The huma API itself was created inside
// the Group (registerPlatformAPI) so every *route* it owns inherits auth.
func registerSpecRoutes(r chi.Router, humaAPI huma.API) {
	r.Get("/api/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		spec, err := humaAPI.OpenAPI().MarshalJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(spec)
	})
	r.Get("/api/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		spec, err := humaAPI.OpenAPI().YAML()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(spec)
	})

	// Rust serves the spec at /q/openapi and Swagger UI at /swagger-ui;
	// alias both for drop-in tooling parity. /api/openapi.json is kept for
	// the existing make/Hey-API codegen tooling.
	r.Get("/q/openapi", func(w http.ResponseWriter, _ *http.Request) {
		spec, err := humaAPI.OpenAPI().MarshalJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(spec)
	})
	r.Get("/swagger-ui", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerUIHTML))
	})
}
