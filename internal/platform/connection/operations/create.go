package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
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

// CreateConnection validates cmd, enforces (code, clientID) uniqueness,
// persists the connection, and emits [ConnectionCreated].
//
// TODO(wave-3c): validate that ServiceAccountID exists once
// service_account is ported.
func CreateConnection(
	ctx context.Context,
	repo *connection.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd CreateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ConnectionCreated], error) {
	var zero commit.Committed[ConnectionCreated]

	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return zero, usecase.Validation("CODE_REQUIRED", "Connection code is required")
	}
	if !validate.CodePattern.MatchString(code) {
		return zero, usecase.Validation("INVALID_CODE_FORMAT",
			"Code must start with lowercase letter, contain only lowercase alphanumeric and hyphens")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "Connection name is required")
	}
	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return zero, usecase.Validation("SERVICE_ACCOUNT_REQUIRED", "serviceAccountId is required")
	}

	existing, err := repo.FindByCodeAndClient(ctx, code, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_code_and_client failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict(
			"CODE_EXISTS",
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
	return commit.Save(ctx, uow, c, repo, event, cmd)
}
