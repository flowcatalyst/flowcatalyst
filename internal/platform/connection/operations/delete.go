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

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteConnection removes a connection and emits [ConnectionDeleted].
func DeleteConnection(
	ctx context.Context,
	repo *connection.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ConnectionDeleted], error) {
	var zero commit.Committed[ConnectionDeleted]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	c, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Connection", cmd.ID)
	}
	event := ConnectionDeleted{
		Metadata:     usecase.NewEventMetadata(ec, ConnectionDeletedType, Source, subjectFor(c.ID)),
		ConnectionID: c.ID,
		Code:         c.Code,
	}
	return commit.Delete(ctx, uow, c, repo, event, cmd)
}
