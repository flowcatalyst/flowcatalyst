package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// SuspendCommand is the input DTO.
type SuspendCommand struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// SuspendClient marks a client suspended and emits [ClientSuspended].
func SuspendClient(repo *client.Repository) usecaseop.Operation[SuspendCommand, ClientSuspended] {
	return usecaseop.Operation[SuspendCommand, ClientSuspended]{
		Name: "SuspendClient",
		Validate: func(_ context.Context, cmd SuspendCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if strings.TrimSpace(cmd.Reason) == "" {
				return usecase.Validation("REASON_REQUIRED", "reason is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[SuspendCommand],
		Execute: func(ctx context.Context, cmd SuspendCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientSuspended], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ID)
			}
			c.Suspend(cmd.Reason)
			event := ClientSuspended{
				Metadata: usecase.NewEventMetadata(ec, ClientSuspendedType, Source, subjectFor(c.ID)),
				ClientID: c.ID,
				Reason:   cmd.Reason,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}
