package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// SuspendCommand is the input DTO.
type SuspendCommand struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// SuspendClient marks a client suspended and emits [ClientSuspended].
func SuspendClient(
	ctx context.Context,
	repo *client.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd SuspendCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ClientSuspended], error) {
	var zero commit.Committed[ClientSuspended]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		return zero, usecase.Validation("REASON_REQUIRED", "reason is required")
	}

	c, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Client", cmd.ID)
	}
	c.Suspend(cmd.Reason)
	event := ClientSuspended{
		Metadata: usecase.NewEventMetadata(ec, ClientSuspendedType, Source, subjectFor(c.ID)),
		ClientID: c.ID,
		Reason:   cmd.Reason,
	}
	return commit.Save(ctx, uow, c, repo, event, cmd)
}
