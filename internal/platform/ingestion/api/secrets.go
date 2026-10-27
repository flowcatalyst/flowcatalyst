package api

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// IngestionSecretFetchResponse is when one runtime last fetched a secret.
type IngestionSecretFetchResponse struct {
	RuntimeID     string          `json:"runtimeId"`
	Version       int             `json:"version" doc:"The live version the last fetch was made under"`
	PrincipalID   *string         `json:"principalId,omitempty" doc:"The runtime's service account that made it"`
	LastFetchedAt httpcompat.Time `json:"lastFetchedAt"`
	FetchCount    int64           `json:"fetchCount" doc:"How many times this runtime has fetched it"`
}

// IngestionSecretResponse is one secret name the ingestion binds, never its
// value.
type IngestionSecretResponse struct {
	Name string `json:"name"`
	// Bound is false for a name a runtime fetched under an earlier live
	// version that the live definition no longer names.
	Bound   bool                           `json:"bound" doc:"The live definition names it, so its runtime may fetch it; false for a name only fetched under an earlier live version"`
	Fetches []IngestionSecretFetchResponse `json:"fetches" doc:"Per runtime, newest first; empty when no runtime has fetched it"`
}

// IngestionSecretListResponse is the wire shape for GET /api/ingestions/{id}/secrets.
type IngestionSecretListResponse struct {
	Secrets []IngestionSecretResponse `json:"secrets"`
	// NamesHidden: the settings are in a config entry, whose secret names are
	// shown only to a caller who can update ingestions.
	NamesHidden bool `json:"namesHidden,omitempty" doc:"The source's settings are in a config entry and the secret names it binds are withheld: they are shown only to a caller who can update ingestions (ingestion:update)"`
}

// secrets lists the secret names the ingestion's live definition binds and,
// for each, which runtime last fetched it and when. A runtime's fetch is
// security telemetry, not an audited reveal (owner ruling 2026-10-25); this
// is where it shows. Names only: no value is ever read here.
func (s *State) secrets(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IngestionSecretListResponse], error) {
	if err := auth.CanReadIngestions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	// A caller who may not read the config entry is not told which secrets its
	// settings bind, nor which of them a runtime fetched: the secrets come only
	// from the entry for a source that names one.
	if _, byConfig := i.Definition.Source.(ingestion.Configured); byConfig && !canSeeEntries(ctx) {
		return &apicommon.Out[IngestionSecretListResponse]{Body: IngestionSecretListResponse{Secrets: []IngestionSecretResponse{}, NamesHidden: true}}, nil
	}
	fetches, err := s.Repo.ListSecretFetches(ctx, i.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_secret_fetches failed", err)
	}
	// The names are those of the effective live source: when the settings are in
	// a config entry, the entry's value binds the secrets. An entry that is not
	// usable binds none (nothing runs).
	named := *i
	if i.Version > 0 {
		app, err := s.Repo.ApplicationCode(ctx, i.ApplicationID)
		if err != nil {
			return nil, usecase.Internal("REPO", "application lookup failed", err)
		}
		res, err := s.Repo.Resolve(ctx, app, i.ClientID, i.Code, i.Definition)
		if err != nil {
			return nil, usecase.Internal("REPO", "config entry lookup failed", err)
		}
		if res.Runnable() {
			named.Definition = res.Definition
		} else {
			named.Version = 0
		}
	}
	bound := ingestion.BoundSecrets(&named, fetches)
	body := IngestionSecretListResponse{Secrets: make([]IngestionSecretResponse, 0, len(bound))}
	for _, b := range bound {
		r := IngestionSecretResponse{Name: b.Name, Bound: b.Bound, Fetches: make([]IngestionSecretFetchResponse, 0, len(b.Fetches))}
		for _, f := range b.Fetches {
			r.Fetches = append(r.Fetches, IngestionSecretFetchResponse{
				RuntimeID: f.RuntimeID, Version: f.Version, PrincipalID: f.PrincipalID,
				LastFetchedAt: jsontime.New(f.LastFetchedAt), FetchCount: f.FetchCount,
			})
		}
		body.Secrets = append(body.Secrets, r)
	}
	return &apicommon.Out[IngestionSecretListResponse]{Body: body}, nil
}
