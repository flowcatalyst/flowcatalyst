package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	ServiceAccountID string  `json:"serviceAccountId"`
	ExternalID       *string `json:"externalId,omitempty"`
	ClientID         *string `json:"clientId,omitempty"`
}

// CreateConnection validates cmd, enforces anchor-only authorization and
// (code, clientID) uniqueness, persists the connection, and emits
// [ConnectionCreated].
//
// TODO(wave-3c): validate that ServiceAccountID exists once
// service_account is ported.
func CreateConnection(repo *connection.Repository) usecaseop.Operation[CreateCommand, ConnectionCreated] {
	return usecaseop.Operation[CreateCommand, ConnectionCreated]{
		Name: "CreateConnection",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			if code == "" {
				return usecase.Validation("CODE_REQUIRED", "Connection code is required")
			}
			if !validate.CodePattern.MatchString(code) {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"Code must start with lowercase letter, contain only lowercase alphanumeric and hyphens")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "Connection name is required")
			}
			if strings.TrimSpace(cmd.ServiceAccountID) == "" {
				return usecase.Validation("SERVICE_ACCOUNT_REQUIRED", "serviceAccountId is required")
			}
			return nil
		},
		// Resource-level authorization (the coarse "may create connections"
		// permission is enforced at the controller). A connection bound to a
		// client may only be created by a principal with access to that client;
		// a platform-wide connection (nil ClientID) requires anchor. This is
		// exactly auth.CheckScopeAccess on the target client.
		Authorize: func(ctx context.Context, cmd CreateCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ConnectionCreated], error) {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))

			existing, err := repo.FindByCodeAndClient(ctx, code, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code_and_client failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("CODE_EXISTS",
					"Connection with code '"+code+"' already exists")
			}

			c := connection.New(code, strings.TrimSpace(cmd.Name), cmd.ServiceAccountID)
			c.Description = cmd.Description
			c.ExternalID = cmd.ExternalID
			c.ClientID = cmd.ClientID

			event := ConnectionCreated{
				Metadata:     usecase.NewEventMetadata(ec, ConnectionCreatedType, Source, subjectFor(c.ID)),
				ConnectionID: c.ID,
				Code:         c.Code,
				Name:         c.Name,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}
