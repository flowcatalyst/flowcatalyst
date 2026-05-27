package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ExternalID  *string `json:"externalId,omitempty"`
}

// UpdateConnection mutates mutable fields and emits [ConnectionUpdated].
func UpdateConnection(
	ctx context.Context,
	repo *connection.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ConnectionUpdated], error) {
	var zero commit.Committed[ConnectionUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "Connection name is required")
	}

	c, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Connection", cmd.ID)
	}
	c.Name = strings.TrimSpace(cmd.Name)
	c.Description = cmd.Description
	c.ExternalID = cmd.ExternalID

	event := ConnectionUpdated{
		Metadata:     usecase.NewEventMetadata(ec, ConnectionUpdatedType, Source, subjectFor(c.ID)),
		ConnectionID: c.ID,
		Name:         c.Name,
	}
	return commit.Save(ctx, uow, c, repo, event, cmd)
}
