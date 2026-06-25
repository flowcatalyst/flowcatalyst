package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID             string  `json:"id"`
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	IconURL        *string `json:"iconUrl,omitempty"`
	Website        *string `json:"website,omitempty"`
	Logo           *string `json:"logo,omitempty"`
	LogoMimeType   *string `json:"logoMimeType,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
}

// UpdateApplication mutates mutable fields and emits [ApplicationUpdated].
//
// An application is platform-level (no tenant ClientID), so there is no
// resource-level access check; the coarse "may write applications" permission
// (auth.CanWriteApplications) is enforced at the controller.
func UpdateApplication(repo *application.Repository) usecaseop.Operation[UpdateCommand, ApplicationUpdated] {
	return usecaseop.Operation[UpdateCommand, ApplicationUpdated]{
		Name: "UpdateApplication",
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
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ApplicationUpdated], error) {
			a, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if a == nil {
				return nil, httperror.NotFound("Application", cmd.ID)
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
			if cmd.Logo != nil {
				a.Logo = cmd.Logo
			}
			if cmd.LogoMimeType != nil {
				a.LogoMimeType = cmd.LogoMimeType
			}
			if cmd.DefaultBaseURL != nil {
				a.DefaultBaseURL = cmd.DefaultBaseURL
			}

			event := ApplicationUpdated{
				Metadata:      usecase.NewEventMetadata(ec, ApplicationUpdatedType, Source, subjectFor(a.ID)),
				ApplicationID: a.ID,
				Name:          a.Name,
			}
			return usecaseop.Save(a, repo, event), nil
		},
	}
}
