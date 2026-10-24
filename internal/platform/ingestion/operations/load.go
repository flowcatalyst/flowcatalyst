package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// NotFound is the 404 for an ingestion id that names nothing.
func NotFound(id string) error {
	return usecase.NotFound("INGESTION_NOT_FOUND", "Ingestion not found: "+id)
}

// load reads the ingestion a by-id operation acts on and checks the caller
// reaches it. The coarse permission is the controller's; reach needs the row.
func load(ctx context.Context, repo *ingestion.Repository, id string) (*ingestion.Ingestion, error) {
	i, err := repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if i == nil {
		return nil, NotFound(id)
	}
	if err := ingestion.CheckReach(auth.FromContext(ctx), i); err != nil {
		return nil, err
	}
	return i, nil
}

// loadEditable is load for an operation that changes the definition: a set
// owns its ingestions until it is released, and they are read-only meanwhile.
func loadEditable(ctx context.Context, repo *ingestion.Repository, id string) (*ingestion.Ingestion, error) {
	i, err := load(ctx, repo, id)
	if err != nil {
		return nil, err
	}
	if i.ManagedBy != nil {
		return nil, usecase.Conflict("INGESTION_SET_OWNED",
			"the ingestion belongs to set "+*i.ManagedBy+" and is changed by applying the set, or after releasing it")
	}
	return i, nil
}

func requireID(id string) error {
	if strings.TrimSpace(id) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

// requireKindPermission refuses a caller who lacks the permission a custom
// source kind declares (camel-route runs a route on the runtime, so creating
// or changing one needs platform:ingestion:ingestion:route). Built-in kinds
// declare none.
func requireKindPermission(ctx context.Context, repo *ingestion.Repository, sources ...ingestion.Source) error {
	ac := auth.FromContext(ctx)
	for _, src := range sources {
		custom, ok := src.(ingestion.Custom)
		if !ok {
			continue
		}
		perm, err := repo.RequiredPermission(ctx, custom.Type)
		if err != nil {
			return usecase.Internal("REPO", "required permission lookup failed", err)
		}
		if perm != "" && !ac.HasPermission(perm) {
			return usecase.Authorization("INGESTION_KIND_PERMISSION_REQUIRED",
				"permission required for source kind "+custom.Type+": "+perm)
		}
	}
	return nil
}

// validateDefinition is the shape check every operation that takes a
// definition makes before anything is read.
func validateDefinition(def ingestion.Definition) error {
	return def.Validate().Err()
}
