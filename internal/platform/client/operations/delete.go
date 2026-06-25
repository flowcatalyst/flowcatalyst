package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteClient removes a client and emits [ClientDeleted]. The handler also
// uses this op for the deactivate (soft-delete) alias — both guarded by the
// coarse CanDeleteClients (anchor-only) check at the controller; tenant
// management has no per-resource dimension, so the use case is Public.
func DeleteClient(repo *client.Repository) usecaseop.Operation[DeleteCommand, ClientDeleted] {
	return usecaseop.Operation[DeleteCommand, ClientDeleted]{
		Name: "DeleteClient",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientDeleted], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ID)
			}
			event := ClientDeleted{
				Metadata:   usecase.NewEventMetadata(ec, ClientDeletedType, Source, subjectFor(c.ID)),
				ClientID:   c.ID,
				Identifier: c.Identifier,
			}
			return usecaseop.Delete(c, repo, event), nil
		},
	}
}
