package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Underscores are allowed: the Rust reference enforced no pattern at all
// (only non-empty + unique), and real application codes use them
// (logistics_portal, transport_order, master_data). Matches the dispatch-pool
// sync pattern. Keep the lowercase-letter start as a light convention.
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

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
func CreateApplication(
	ctx context.Context,
	repo *application.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd CreateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ApplicationCreated], error) {
	var zero commit.Committed[ApplicationCreated]

	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return zero, usecase.Validation("CODE_REQUIRED", "code is required")
	}
	if !codePattern.MatchString(code) {
		return zero, usecase.Validation("INVALID_CODE_FORMAT",
			"code must start with a lowercase letter and contain only lowercase alphanumerics, hyphens, and underscores")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name is required")
	}

	existing, err := repo.FindByCode(ctx, code)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict(
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
	return commit.Save(ctx, uow, a, repo, event, cmd)
}
