package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID             string  `json:"id"`
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	IconURL        *string `json:"iconUrl,omitempty"`
	Website        *string `json:"website,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
}

// UpdateApplication mutates mutable fields and emits [ApplicationUpdated].
func UpdateApplication(
	ctx context.Context,
	repo *application.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ApplicationUpdated], error) {
	var zero commit.Committed[ApplicationUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}

	a, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return zero, httperror.NotFound("Application", cmd.ID)
	}
	if cmd.Name != nil {
		a.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		a.Description = cmd.Description
	}
	if cmd.IconURL != nil {
		a.IconURL = cmd.IconURL
	}
	if cmd.Website != nil {
		a.Website = cmd.Website
	}
	if cmd.DefaultBaseURL != nil {
		a.DefaultBaseURL = cmd.DefaultBaseURL
	}

	event := ApplicationUpdated{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationUpdatedType, Source, subjectFor(a.ID)),
		ApplicationID: a.ID,
		Name:          a.Name,
	}
	return commit.Save(ctx, uow, a, repo, event, cmd)
}
