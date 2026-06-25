package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ActivateCommand is the input DTO.
type ActivateCommand struct {
	ID string `json:"id"`
}

// ActivateClient marks a client active and emits [ClientActivated].
func ActivateClient(repo *client.Repository) usecaseop.Operation[ActivateCommand, ClientActivated] {
	return usecaseop.Operation[ActivateCommand, ClientActivated]{
		Name: "ActivateClient",
		Validate: func(_ context.Context, cmd ActivateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[ActivateCommand],
		Execute: func(ctx context.Context, cmd ActivateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientActivated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ID)
			}
			c.Activate()
			event := ClientActivated{
				Metadata: usecase.NewEventMetadata(ec, ClientActivatedType, Source, subjectFor(c.ID)),
				ClientID: c.ID,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}
