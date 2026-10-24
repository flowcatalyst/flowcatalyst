package operations

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// ReleaseSetCommand is the input DTO.
type ReleaseSetCommand struct {
	Name string `json:"name"`
}

// ReleaseSetResult is how many ingestions the set let go of.
type ReleaseSetResult struct {
	Name     string
	Released int
}

// ReleaseSet ends a set's ownership: its ingestions stay exactly as they are,
// version and all, and are edited one at a time again. It emits
// [IngestionManaged] for each and [IngestionSetReleased].
func ReleaseSet(repo *ingestion.Repository) usecaseop.TxOperation[ReleaseSetCommand, ReleaseSetResult] {
	return usecaseop.TxOperation[ReleaseSetCommand, ReleaseSetResult]{
		Name: "ReleaseIngestionSet",
		Validate: func(_ context.Context, cmd ReleaseSetCommand) error {
			if !ingestion.ValidSetName(cmd.Name) {
				return usecase.Validation("INGESTION_SET_NAME_INVALID", "name must be lowercase letters, digits and '-', at most 64")
			}
			return nil
		},
		// Reach is checked on each ingestion in Execute; the coarse "may apply
		// sets" permission is the controller's.
		Authorize: usecaseop.Public[ReleaseSetCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd ReleaseSetCommand, ec usecase.ExecutionContext) (ReleaseSetResult, error) {
			var zero ReleaseSetResult
			if err := s.WithTx(ctx, func(tx pgx.Tx) error { return repo.LockSets(ctx, tx) }); err != nil {
				return zero, usecase.Internal("REPO", "lock sets failed", err)
			}
			set, err := repo.FindSetByName(ctx, cmd.Name)
			if err != nil {
				return zero, usecase.Internal("REPO", "find set failed", err)
			}
			if set == nil {
				return zero, usecase.NotFound("INGESTION_SET_NOT_FOUND", "Ingestion set not found: "+cmd.Name)
			}
			owned, err := repo.ListManagedBy(ctx, set.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "list managed failed", err)
			}
			ac := auth.FromContext(ctx)
			for _, i := range owned {
				if err := ingestion.CheckReach(ac, i); err != nil {
					return zero, err
				}
			}
			for _, i := range owned {
				i.Release()
				event := IngestionManaged{ingestionEvent: newEvent(ec, IngestionManagedType, i.ID), Code: i.Code}
				if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event, cmd)); err != nil {
					return zero, err
				}
			}
			event := IngestionSetReleased{setEvent: newSetEvent(ec, IngestionSetReleasedType, set.ID), Name: cmd.Name, Released: len(owned)}
			if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, event, cmd)); err != nil {
				return zero, err
			}
			return ReleaseSetResult{Name: cmd.Name, Released: len(owned)}, nil
		},
	}
}
