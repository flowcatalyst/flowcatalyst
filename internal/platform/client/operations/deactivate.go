package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeactivateCommand is the input DTO. Reason is optional.
type DeactivateCommand struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

// DeactivateClient marks a client INACTIVE (a soft delete: the row and its
// configuration stay, and ActivateClient reactivates it) and emits
// [ClientDeactivated]. DeleteClient is the irreversible removal.
func DeactivateClient(repo *client.Repository) usecaseop.Operation[DeactivateCommand, ClientDeactivated] {
	return usecaseop.Operation[DeactivateCommand, ClientDeactivated]{
		Name: "DeactivateClient",
		Validate: func(_ context.Context, cmd DeactivateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Tenant management is anchor-only at the controller
		// (CanDeactivateClients); no per-resource dimension.
		Authorize: usecaseop.Public[DeactivateCommand],
		Execute: func(ctx context.Context, cmd DeactivateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientDeactivated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ID)
			}
			var reason *string
			if r := strings.TrimSpace(cmd.Reason); r != "" {
				reason = &r
			}
			c.SetStatus(client.StatusInactive, reason)
			event := ClientDeactivated{
				Metadata: usecase.NewEventMetadata(ec, ClientDeactivatedType, Source, subjectFor(string(c.ID))),
				ClientID: string(c.ID),
				Reason:   strings.TrimSpace(cmd.Reason),
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}
