package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID                 string                             `json:"id"`
	Name               *string                            `json:"name,omitempty"`
	Description        *string                            `json:"description,omitempty"`
	WebhookCredentials *serviceaccount.WebhookCredentials `json:"webhookCredentials,omitempty"`
}

// UpdateServiceAccount mutates mutable fields and emits [ServiceAccountUpdated].
func UpdateServiceAccount(
	ctx context.Context,
	repo *serviceaccount.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ServiceAccountUpdated], error) {
	var zero commit.Committed[ServiceAccountUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}

	sa, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return zero, httperror.NotFound("ServiceAccount", cmd.ID)
	}
	if cmd.Name != nil {
		sa.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		sa.Description = cmd.Description
	}
	if cmd.WebhookCredentials != nil {
		sa.WebhookCredentials = *cmd.WebhookCredentials
	}

	event := ServiceAccountUpdated{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountUpdatedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		Name:             sa.Name,
	}
	return commit.Save(ctx, uow, sa, repo, event, cmd)
}
