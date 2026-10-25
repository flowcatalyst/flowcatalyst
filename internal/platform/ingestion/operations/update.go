package operations

import (
	"context"
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID string `json:"id"`
	// ExpectedVersion is the live version the caller read (0 while none is
	// live). If the ingestion has moved on, the update is a conflict rather
	// than a lost update.
	ExpectedVersion int                  `json:"expectedVersion"`
	Definition      ingestion.Definition `json:"definition"`
	Note            *string              `json:"note,omitempty"`
}

// UpdateResult is what UpdateIngestion did (SaveResult). Outcome UNCHANGED
// wrote nothing: no version, no event, no audit row.
type UpdateResult = SaveResult

// UpdateIngestion saves the definition as the ingestion's next version,
// deduplicated by digest (owner ruling 2026-10-25):
//
//   - the live version already says it: nothing is written (UNCHANGED);
//   - for a caller who holds promote: a saved version that is not live and
//     says it is made live instead of saving a duplicate (MADE_LIVE);
//     otherwise it is saved and made live, emitting [IngestionUpdated] (SAVED);
//   - for a caller without promote: a draft that already says it is answered
//     and nothing is saved (DRAFT_EXISTS); otherwise it is saved as a new
//     version that is not live (DRAFT), which someone who holds promote makes
//     live. An update without promote is not refused.
func UpdateIngestion(repo *ingestion.Repository, resolver ingestion.Resolver) usecaseop.TxOperation[UpdateCommand, UpdateResult] {
	return usecaseop.TxOperation[UpdateCommand, UpdateResult]{
		Name: "UpdateIngestion",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if err := requireID(cmd.ID); err != nil {
				return err
			}
			return validateDefinition(cmd.Definition)
		},
		// Per-resource authz needs the loaded row, so it runs post-load in
		// Execute (reach, the set that owns it, the authoring checks); the
		// coarse "may update ingestions" permission is the controller's.
		// Promote decides only whether the save is live, never whether it is
		// allowed.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd UpdateCommand, ec usecase.ExecutionContext) (UpdateResult, error) {
			var zero UpdateResult
			i, err := loadEditable(ctx, repo, cmd.ID)
			if err != nil {
				return zero, err
			}
			if cmd.ExpectedVersion != i.Version {
				return zero, usecase.Conflict("INGESTION_VERSION_CONFLICT",
					fmt.Sprintf("expected live version %d but it is %d; read the ingestion again and retry", cmd.ExpectedVersion, i.Version))
			}
			if i.Version > 0 && cmd.Definition.Equal(i.Definition) {
				return UpdateResult{Ingestion: i, Outcome: OutcomeUnchanged, Version: i.Version}, nil
			}
			if err := checkAuthor(ctx, repo, i, cmd.Definition); err != nil {
				return zero, err
			}
			if err := checkTarget(ctx, resolver, i, cmd.Definition); err != nil {
				return zero, err
			}
			live := LiveIfPermitted(ctx, true)
			if res, done, err := reuseOrDraft(ctx, s, repo, i, cmd.Definition, live, cmd.Note, ec); done || err != nil {
				return res, err
			}

			i.SaveVersion(cmd.Definition, true, cmd.Note, new(ec.PrincipalID))
			event := IngestionUpdated{
				ingestionEvent: newEvent(ec, IngestionUpdatedType, i.ID),
				Code:           i.Code,
				Version:        i.Version,
			}
			if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event, cmd)); err != nil {
				return zero, err
			}
			return UpdateResult{Ingestion: i, Outcome: OutcomeSaved, Version: i.Version}, nil
		},
	}
}

// checkTarget resolves the target of a definition for an existing ingestion,
// which needs its application's code.
func checkTarget(ctx context.Context, resolver ingestion.Resolver, i *ingestion.Ingestion, def ingestion.Definition) error {
	app, err := resolver.ApplicationByID(ctx, string(i.ApplicationID))
	if err != nil {
		return err
	}
	if app == nil {
		return usecase.Internal("INGESTION_APPLICATION_MISSING", "the ingestion's application no longer exists", nil)
	}
	_, err = resolver.Target(ctx, app.Code, i.ClientID, def.Target)
	return err
}
