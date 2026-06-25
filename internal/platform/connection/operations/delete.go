package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteConnection removes a connection and emits [ConnectionDeleted].
func DeleteConnection(repo *connection.Repository) usecaseop.Operation[DeleteCommand, ConnectionDeleted] {
	return usecaseop.Operation[DeleteCommand, ConnectionDeleted]{
		Name: "DeleteConnection",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may delete
		// connections" permission is on the controller.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ConnectionDeleted], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Connection", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), c.ClientID); err != nil {
				return nil, err
			}
			event := ConnectionDeleted{
				Metadata:     usecase.NewEventMetadata(ec, ConnectionDeletedType, Source, subjectFor(c.ID)),
				ConnectionID: c.ID,
				Code:         c.Code,
			}
			return usecaseop.Delete(c, repo, event), nil
		},
	}
}
