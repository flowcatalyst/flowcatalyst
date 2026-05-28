package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code               string                             `json:"code"`
	Name               string                             `json:"name"`
	Description        *string                            `json:"description,omitempty"`
	Scope              *string                            `json:"scope,omitempty"`
	ClientIDs          []string                           `json:"clientIds,omitempty"`
	ApplicationID      *string                            `json:"applicationId,omitempty"`
	WebhookCredentials *serviceaccount.WebhookCredentials `json:"webhookCredentials,omitempty"`
}

// CreateServiceAccount validates cmd, enforces code uniqueness, persists
// the service account, and emits [ServiceAccountCreated].
func CreateServiceAccount(
	ctx context.Context,
	repo *serviceaccount.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd CreateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ServiceAccountCreated], error) {
	var zero commit.Committed[ServiceAccountCreated]

	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return zero, usecase.Validation("CODE_REQUIRED", "code is required")
	}
	if !codePattern.MatchString(code) {
		return zero, usecase.Validation("INVALID_CODE_FORMAT",
			"code must start with a lowercase letter and contain only lowercase alphanumeric and hyphens")
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
			"CODE_EXISTS", "Service account with code '"+code+"' already exists")
	}

	sa := serviceaccount.New(code, strings.TrimSpace(cmd.Name))
	sa.Description = cmd.Description
	sa.Scope = cmd.Scope
	sa.ApplicationID = cmd.ApplicationID
	if cmd.ClientIDs != nil {
		sa.ClientIDs = cmd.ClientIDs
	}
	if cmd.WebhookCredentials != nil {
		sa.WebhookCredentials = *cmd.WebhookCredentials
	}

	event := ServiceAccountCreated{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountCreatedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
		Name:             sa.Name,
	}
	return commit.Save(ctx, uow, sa, repo, event, cmd)
}
