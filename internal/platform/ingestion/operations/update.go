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

// UpdateResult is what UpdateIngestion did. Event is nil when the definition
// was already the live one and nothing was written.
type UpdateResult struct {
	Ingestion *ingestion.Ingestion
	Event     *IngestionUpdated
}

// UpdateIngestion saves the definition as the next version and makes it live,
// emitting [IngestionUpdated]. A definition equal to the live one changes
// nothing: no version, no event, no audit row.
func UpdateIngestion(repo *ingestion.Repository, resolver ingestion.Resolver) usecaseop.TxOperation[UpdateCommand, UpdateResult] {
	return usecaseop.TxOperation[UpdateCommand, UpdateResult]{
		Name: "UpdateIngestion",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if err := requireID(cmd.ID); err != nil {
				return err
			}
			return validateDefinition(cmd.Definition)
		},
		// An update makes the new version live, so it needs promote too (owner
		// ruling R3: refused without it, never quietly saved as a draft; the
		// draft is POST .../versions with live false). Per-resource authz needs
		// the loaded row, so it runs post-load in Execute; the coarse "may
		// update ingestions" permission is the controller's.
		Authorize: func(ctx context.Context, _ UpdateCommand) error {
			return requirePromote(ctx, promoteUpdate)
		},
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
				return UpdateResult{Ingestion: i}, nil
			}
			if err := checkAuthor(ctx, repo, i, cmd.Definition); err != nil {
				return zero, err
			}
			if err := checkTarget(ctx, resolver, i, cmd.Definition); err != nil {
				return zero, err
			}

			i.SaveVersion(cmd.Definition, true, cmd.Note, new(ec.PrincipalID))
			event := IngestionUpdated{
				ingestionEvent: newEvent(ec, IngestionUpdatedType, i.ID),
				Code:           i.Code,
				Version:        i.Version,
			}
			event, err = usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event, cmd))
			if err != nil {
				return zero, err
			}
			return UpdateResult{Ingestion: i, Event: &event}, nil
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
