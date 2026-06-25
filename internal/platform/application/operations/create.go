package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Type           string  `json:"type,omitempty"`
	Description    *string `json:"description,omitempty"`
	IconURL        *string `json:"iconUrl,omitempty"`
	Website        *string `json:"website,omitempty"`
	Logo           *string `json:"logo,omitempty"`
	LogoMimeType   *string `json:"logoMimeType,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
}

// CreateApplication validates cmd, enforces uniqueness on code, persists
// the application, and atomically emits an [ApplicationCreated] event.
//
// An application is platform-level (no tenant ClientID), so the use case has
// no resource-level access check — the coarse "may create applications"
// permission (auth.CanWriteApplications) is enforced at the controller.
func CreateApplication(repo *application.Repository) usecaseop.Operation[CreateCommand, ApplicationCreated] {
	return usecaseop.Operation[CreateCommand, ApplicationCreated]{
		Name: "CreateApplication",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			if code == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			// Underscores are allowed: the Rust reference enforced no pattern at
			// all (only non-empty + unique), and real application codes use them
			// (logistics_portal, transport_order, master_data). Matches the
			// dispatch-pool sync pattern.
			if !validate.CodeUnderscorePattern.MatchString(code) {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"code must start with a lowercase letter and contain only lowercase alphanumerics, hyphens, and underscores")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ApplicationCreated], error) {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))

			existing, err := repo.FindByCode(ctx, code)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict(
					"CODE_EXISTS", "Application with code '"+code+"' already exists")
			}

			var a *application.Application
			if cmd.Type == string(application.TypeIntegration) {
				a = application.NewIntegration(code, strings.TrimSpace(cmd.Name))
			} else {
				a = application.New(code, strings.TrimSpace(cmd.Name))
			}
			a.Description = cmd.Description
			a.IconURL = cmd.IconURL
			a.Website = cmd.Website
			a.Logo = cmd.Logo
			a.LogoMimeType = cmd.LogoMimeType
			a.DefaultBaseURL = cmd.DefaultBaseURL

			event := ApplicationCreated{
				Metadata:      usecase.NewEventMetadata(ec, ApplicationCreatedType, Source, subjectFor(a.ID)),
				ApplicationID: a.ID,
				Code:          a.Code,
				Name:          a.Name,
			}
			return usecaseop.Save(a, repo, event), nil
		},
	}
}
