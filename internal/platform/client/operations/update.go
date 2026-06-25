package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

// UpdateClient mutates the client name and emits [ClientUpdated].
func UpdateClient(repo *client.Repository) usecaseop.Operation[UpdateCommand, ClientUpdated] {
	return usecaseop.Operation[UpdateCommand, ClientUpdated]{
		Name: "UpdateClient",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientUpdated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ID)
			}
			if cmd.Name != nil {
				c.Name = strings.TrimSpace(*cmd.Name)
			}

			event := ClientUpdated{
				Metadata: usecase.NewEventMetadata(ec, ClientUpdatedType, Source, subjectFor(c.ID)),
				ClientID: c.ID,
				Name:     c.Name,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}
