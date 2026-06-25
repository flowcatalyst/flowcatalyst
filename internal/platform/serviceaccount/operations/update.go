package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID                 string                             `json:"id"`
	Name               *string                            `json:"name,omitempty"`
	Description        *string                            `json:"description,omitempty"`
	Scope              *string                            `json:"scope,omitempty"`
	ClientIDs          []string                           `json:"clientIds,omitempty"`
	WebhookCredentials *serviceaccount.WebhookCredentials `json:"webhookCredentials,omitempty"`
}

// UpdateServiceAccount mutates mutable fields and emits [ServiceAccountUpdated].
func UpdateServiceAccount(repo *serviceaccount.Repository) usecaseop.Operation[UpdateCommand, ServiceAccountUpdated] {
	return usecaseop.Operation[UpdateCommand, ServiceAccountUpdated]{
		Name: "UpdateServiceAccount",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			return nil
		},
		// The coarse "may write service accounts" permission is enforced at the
		// controller; this admin-managed update has no per-client resource
		// check, so the operation is intentionally open.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ServiceAccountUpdated], error) {
			sa, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if sa == nil {
				return nil, httperror.NotFound("ServiceAccount", cmd.ID)
			}
			if cmd.Name != nil {
				sa.Name = strings.TrimSpace(*cmd.Name)
			}
			if cmd.Description != nil {
				sa.Description = cmd.Description
			}
			if cmd.Scope != nil {
				sa.Scope = cmd.Scope
			}
			if cmd.ClientIDs != nil {
				sa.ClientIDs = cmd.ClientIDs
			}
			if cmd.WebhookCredentials != nil {
				sa.WebhookCredentials = *cmd.WebhookCredentials
			}

			event := ServiceAccountUpdated{
				Metadata:         usecase.NewEventMetadata(ec, ServiceAccountUpdatedType, Source, subjectFor(sa.ID)),
				ServiceAccountID: sa.ID,
				Name:             sa.Name,
			}
			return usecaseop.Save(sa, repo, event), nil
		},
	}
}
