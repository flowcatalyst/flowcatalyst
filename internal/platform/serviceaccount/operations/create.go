package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

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
func CreateServiceAccount(repo *serviceaccount.Repository) usecaseop.Operation[CreateCommand, ServiceAccountCreated] {
	return usecaseop.Operation[CreateCommand, ServiceAccountCreated]{
		Name: "CreateServiceAccount",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			if code == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			if !validate.CodePattern.MatchString(code) {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"code must start with a lowercase letter and contain only lowercase alphanumeric and hyphens")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			return nil
		},
		// The coarse "may write service accounts" permission is enforced at the
		// controller; this admin-managed create has no per-client resource
		// check, so the operation is intentionally open.
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ServiceAccountCreated], error) {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))

			existing, err := repo.FindByCode(ctx, code)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict(
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
			return usecaseop.Save(sa, repo, event), nil
		},
	}
}
