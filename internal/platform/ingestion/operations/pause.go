package operations

import (
	"context"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// PauseCommand is the input DTO of the pause and resume operations.
type PauseCommand struct {
	ID string `json:"id"`
}

// ResumeCommand is the same input.
type ResumeCommand = PauseCommand

// PauseIngestion stops runtimes running the ingestion without changing its
// definition, and emits [IngestionPaused]. Pausing is not editing, so an
// ingestion a set owns can still be paused.
func PauseIngestion(repo *ingestion.Repository) usecaseop.Operation[PauseCommand, IngestionPaused] {
	return usecaseop.Operation[PauseCommand, IngestionPaused]{
		Name: "PauseIngestion",
		Validate: func(_ context.Context, cmd PauseCommand) error {
			return requireID(cmd.ID)
		},
		// Per-resource authz runs post-load in Execute; the coarse "may update
		// ingestions" permission is the controller's.
		Authorize: usecaseop.Public[PauseCommand],
		Execute: func(ctx context.Context, cmd PauseCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IngestionPaused], error) {
			i, err := load(ctx, repo, cmd.ID)
			if err != nil {
				return nil, err
			}
			i.Pause()
			event := IngestionPaused{
				ingestionEvent: newEvent(ec, IngestionPausedType, i.ID),
				Code:           i.Code,
			}
			return usecaseop.Save(i, repo, event), nil
		},
	}
}

// ResumeIngestion undoes PauseIngestion and emits [IngestionResumed]. It
// refuses an ingestion that could not run yet (requireReady).
func ResumeIngestion(repo *ingestion.Repository) usecaseop.Operation[ResumeCommand, IngestionResumed] {
	return usecaseop.Operation[ResumeCommand, IngestionResumed]{
		Name: "ResumeIngestion",
		Validate: func(_ context.Context, cmd ResumeCommand) error {
			return requireID(cmd.ID)
		},
		Authorize: usecaseop.Public[ResumeCommand],
		Execute: func(ctx context.Context, cmd ResumeCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IngestionResumed], error) {
			i, err := load(ctx, repo, cmd.ID)
			if err != nil {
				return nil, err
			}
			if err := requireReady(ctx, repo, i); err != nil {
				return nil, err
			}
			i.Resume()
			event := IngestionResumed{
				ingestionEvent: newEvent(ec, IngestionResumedType, i.ID),
				Code:           i.Code,
			}
			return usecaseop.Save(i, repo, event), nil
		},
	}
}

// requireReady refuses to enable an ingestion whose environment is not set up
// (owner ruling 2026-10-29): a source that names a config entry needs the
// entry set, and the secrets its effective definition binds need values at the
// client. Enabling is when this is checked, not when the ingestion is saved,
// because the settings and secrets are written after the definition is. Only
// names are reported, never values. An ingestion with inline settings is not
// checked: it was saved with everything it needs, and a secret that is missing
// fails at the runtime as it always has.
func requireReady(ctx context.Context, repo *ingestion.Repository, i *ingestion.Ingestion) error {
	res, err := repo.ResolveOf(ctx, i, i.Definition)
	if err != nil {
		return usecase.Internal("REPO", "config entry lookup failed", err)
	}
	if res.Status == ingestion.ConfigMissing || res.Status == ingestion.ConfigInvalid {
		why := "is not set"
		if res.Status == ingestion.ConfigInvalid {
			why = "is not valid"
		}
		return usecase.Conflict("INGESTION_CONFIG_NOT_SET",
			fmt.Sprintf("the config entry %s %s for client %s; set it before resuming", res.Key, why, i.ClientID))
	}
	if res.Status == ingestion.ConfigInline {
		return nil
	}
	// The secrets the kind needs, not every text setting of an extension's.
	names, err := repo.SecretNamesOf(ctx, res.Definition.Source)
	if err != nil {
		return usecase.Internal("REPO", "live kinds failed", err)
	}
	if len(names) == 0 {
		return nil
	}
	app, err := repo.ApplicationCode(ctx, i.ApplicationID)
	if err != nil {
		return usecase.Internal("REPO", "application lookup failed", err)
	}
	missing, err := repo.MissingSecrets(ctx, app, i.ClientID, names)
	if err != nil {
		return usecase.Internal("REPO", "secret lookup failed", err)
	}
	if len(missing) > 0 {
		return usecase.Conflict("INGESTION_SECRET_MISSING",
			fmt.Sprintf("the secrets the ingestion binds are not set for client %s: %s; set each on the platform config page (application %s, section %s, client %s, type SECRET) before resuming",
				i.ClientID, strings.Join(missing, ", "), app, ingestion.SecretsSection, i.ClientID))
	}
	return nil
}
