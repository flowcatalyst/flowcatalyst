package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
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

// identity is what names an ingestion's config entry: its application's code,
// its client and its own code (`config: true` is the code).
type identity struct {
	Application string
	Client      ids.ClientID
	Code        string
}

// identityOf is the identity of a saved ingestion.
func identityOf(ctx context.Context, repo *ingestion.Repository, i *ingestion.Ingestion) (identity, error) {
	app, err := repo.ApplicationCode(ctx, i.ApplicationID)
	if err != nil {
		return identity{}, usecase.Internal("REPO", "application lookup failed", err)
	}
	return identity{Application: app, Client: i.ClientID, Code: i.Code}, nil
}

// checkAuthor refuses a caller who may not save next over the ingestion i (nil
// for a create): the config entry it names must make a valid definition when
// there is one, and the kind's permission, the runtime identity and the secrets
// it points are judged on the effective definitions (ingestion.Repository.CheckAuthor):
// next with its entry's settings against the live version with its entry's
// settings. An entry that is not there does not stop the save; the answer's
// Warnings say so (CONFIG_MISSING). The answer's Definition is the effective
// one, whose target is the one to resolve.
func checkAuthor(ctx context.Context, repo *ingestion.Repository, i *ingestion.Ingestion, who identity, next ingestion.Definition) (ingestion.Resolved, error) {
	return judge(ctx, repo, i, who, next, true)
}

// judge is checkAuthor, with validate false when the effective definition
// is not to be validated (making a saved version live: its shape was judged
// when it was saved).
func judge(ctx context.Context, repo *ingestion.Repository, i *ingestion.Ingestion, who identity, next ingestion.Definition, validate bool) (ingestion.Resolved, error) {
	nextRes, err := repo.Resolve(ctx, who.Application, who.Client, who.Code, next)
	if err != nil {
		return ingestion.Resolved{}, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	if validate && nextRes.Status == ingestion.ConfigInvalid {
		return ingestion.Resolved{}, EntryProblems(nextRes).Err()
	}
	c := ingestion.Change{Next: nextRes.Definition}
	if i != nil {
		c.Current = &i.Definition
		if i.Version > 0 {
			liveRes, err := repo.Resolve(ctx, who.Application, who.Client, who.Code, i.Definition)
			if err != nil {
				return ingestion.Resolved{}, usecase.Internal("REPO", "config entry lookup failed", err)
			}
			// A live version whose entry is not usable runs nowhere: a save is
			// judged as a first one.
			if liveRes.Runnable() {
				c.Live = &liveRes.Definition
			}
		}
	}
	if err := repo.CheckAuthor(ctx, auth.FromContext(ctx), c); err != nil {
		return ingestion.Resolved{}, err
	}
	return nextRes, nil
}

// EntryProblems are the problems of an invalid entry, each saying whose they
// are.
func EntryProblems(r ingestion.Resolved) ingestion.Problems {
	out := make(ingestion.Problems, len(r.Problems))
	for n, p := range r.Problems {
		p.Message = "(config entry " + r.Key + ") " + p.Message
		out[n] = p
	}
	return out
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

// The refusals requirePromote gives each save that names live explicitly.
// Create and update do not refuse a caller without promote: their callers
// ask for live through LiveIfPermitted, and without it the save is a draft.
const (
	promoteCreate = "creating a live ingestion needs platform:ingestion:ingestion:promote; " +
		"without it the ingestion is created as a draft that someone who holds the permission makes live"
	promoteVersion = "saving a version as live needs platform:ingestion:ingestion:promote; save it with live: false"
)

// validateDefinition is the shape check every operation that takes a
// definition makes before anything is read.
func validateDefinition(def ingestion.Definition) error {
	return def.Validate().Err()
}
