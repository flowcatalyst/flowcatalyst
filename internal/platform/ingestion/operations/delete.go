package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteIngestion removes an ingestion with its versions, leases and state,
// and emits [IngestionDeleted]. Removing one needs only the delete permission,
// whatever its source kind.
func DeleteIngestion(repo *ingestion.Repository) usecaseop.Operation[DeleteCommand, IngestionDeleted] {
	return usecaseop.Operation[DeleteCommand, IngestionDeleted]{
		Name: "DeleteIngestion",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			return requireID(cmd.ID)
		},
		// Per-resource authz runs post-load in Execute; the coarse "may delete
		// ingestions" permission is the controller's.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IngestionDeleted], error) {
			i, err := loadEditable(ctx, repo, cmd.ID)
			if err != nil {
				return nil, err
			}
			event := IngestionDeleted{
				ingestionEvent: newEvent(ec, IngestionDeletedType, i.ID),
				Code:           i.Code,
			}
			return usecaseop.Delete(i, repo, event), nil
		},
	}
}
