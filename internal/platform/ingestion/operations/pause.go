package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
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

// ResumeIngestion undoes PauseIngestion and emits [IngestionResumed].
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
			i.Resume()
			event := IngestionResumed{
				ingestionEvent: newEvent(ec, IngestionResumedType, i.ID),
				Code:           i.Code,
			}
			return usecaseop.Save(i, repo, event), nil
		},
	}
}
