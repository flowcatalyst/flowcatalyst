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

// checkAuthor refuses a caller who may not save next over the ingestion i (nil
// for a create): the kind's permission, the runtime identity and the secrets
// it points (ingestion.Repository.CheckAuthor).
func checkAuthor(ctx context.Context, repo *ingestion.Repository, i *ingestion.Ingestion, next ingestion.Definition) error {
	return repo.CheckAuthor(ctx, auth.FromContext(ctx), changeOf(i, next))
}

// changeOf is the change saving next over i makes (i nil: a create).
func changeOf(i *ingestion.Ingestion, next ingestion.Definition) ingestion.Change {
	c := ingestion.Change{Next: next}
	if i != nil {
		c.Current = &i.Definition
		if i.Version > 0 {
			c.Live = &i.Definition
		}
	}
	return c
}

// requirePromote refuses a save that would make a version live for a caller
// without platform:ingestion:ingestion:promote (owner ruling R3). message
// says which save it is and how to make it without the permission.
func requirePromote(ctx context.Context, message string) error {
	if auth.CanPromoteIngestions(auth.FromContext(ctx)) == nil {
		return nil
	}
	return usecase.Authorization("INGESTION_PROMOTE_REQUIRED", message)
}

// The refusals requirePromote gives each save.
const (
	promoteCreate = "creating a live ingestion needs platform:ingestion:ingestion:promote (live is true unless the request says false); " +
		"send live: false to save it as a draft that someone who holds the permission makes live"
	promoteUpdate = "an update saves the definition as the live version, which needs platform:ingestion:ingestion:promote; " +
		"save it with POST /api/ingestions/{id}/versions (live: false) for someone who holds the permission to make live"
	promoteVersion = "saving a version as live needs platform:ingestion:ingestion:promote; save it with live: false"
)

// validateDefinition is the shape check every operation that takes a
// definition makes before anything is read.
func validateDefinition(def ingestion.Definition) error {
	return def.Validate().Err()
}
