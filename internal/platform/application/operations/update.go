package operations

import (
	"context"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
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
	// Type is APPLICATION or INTEGRATION; nil leaves the type unchanged.
	Type *string `json:"type,omitempty"`
}

// IngestionCounter counts the ingestions an application owns. The ingestion
// package imports application, so UpdateApplication takes the count as a
// dependency instead of importing it.
type IngestionCounter interface {
	CountByApplication(ctx context.Context, applicationID string) (int, error)
}

// UpdateApplication mutates mutable fields and emits [ApplicationUpdated].
//
// Changing the type: APPLICATION to INTEGRATION is always allowed;
// INTEGRATION to APPLICATION is refused (409 APPLICATION_TYPE_IN_USE) while
// the application owns any ingestion, since an ingestion may only belong to
// an integration. The count is read before the unit of work opens, like the
// reference counts in DeleteApplication. [ApplicationUpdated] records the new
// type and, when it changed, the previous one.
//
// An application is platform-level (no tenant ClientID), so there is no
// resource-level access check; the "may update applications" permission
// (auth.CanUpdateApplications) is enforced at the controller.
func UpdateApplication(repo *application.Repository, ingestions IngestionCounter) usecaseop.Operation[UpdateCommand, ApplicationUpdated] {
	return usecaseop.Operation[UpdateCommand, ApplicationUpdated]{
		Name: "UpdateApplication",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			if cmd.Type != nil {
				if _, ok := application.ParseType(*cmd.Type); !ok {
					return usecase.Validation("APPLICATION_TYPE_INVALID", "type must be APPLICATION or INTEGRATION")
				}
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
			var previousType string
			if cmd.Type != nil {
				newType, _ := application.ParseType(*cmd.Type)
				if newType != a.Type {
					if newType == application.TypeApplication {
						n, err := ingestions.CountByApplication(ctx, string(a.ID))
						if err != nil {
							return nil, usecase.Internal("REPO", "count ingestions failed", err)
						}
						if n > 0 {
							return nil, usecase.Conflict("APPLICATION_TYPE_IN_USE",
								fmt.Sprintf("application %s owns %d ingestion(s); delete them or move them to another application first", a.Code, n))
						}
					}
					previousType = string(a.Type)
					a.Type = newType
				}
			}

			event := ApplicationUpdated{
				Metadata:      usecase.NewEventMetadata(ec, ApplicationUpdatedType, Source, subjectFor(string(a.ID))),
				ApplicationID: string(a.ID),
				Name:          a.Name,
				Type:          string(a.Type),
				PreviousType:  previousType,
			}
			return usecaseop.Save(a, repo, event), nil
		},
	}
}
