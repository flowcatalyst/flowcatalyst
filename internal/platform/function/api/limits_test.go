package api

import (
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
)

// The artifact upload takes a raw body of up to 64 MiB. huma gives a body
// operation a 1 MiB limit and a 5 second read deadline unless the operation
// sets its own, so both are pinned here: an upload must not start failing
// because a default changed.
func TestPutFunctionArtifact_BodyLimitsAreItsOwn(t *testing.T) {
	api := humachi.New(chi.NewRouter(), apiroute.PlatformAPIConfig("fn-limits", "test"))
	Register(api, &State{})

	item := api.OpenAPI().Paths["/api/functions/{id}/artifacts/{digest}"]
	if item == nil || item.Put == nil {
		t.Fatal("PUT /api/functions/{id}/artifacts/{digest} is not registered")
	}
	if got := item.Put.MaxBodyBytes; got != maxArtifactBytes {
		t.Errorf("MaxBodyBytes = %d, want %d", got, maxArtifactBytes)
	}
	if got := item.Put.BodyReadTimeout; got != artifactReadTimeout {
		t.Errorf("BodyReadTimeout = %s, want %s", got, artifactReadTimeout)
	}
}
