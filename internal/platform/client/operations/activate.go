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

// ActivateCommand is the input DTO.
type ActivateCommand struct {
	ID string `json:"id"`
}

// ActivateClient marks a client active and emits [ClientActivated].
func ActivateClient(
	ctx context.Context,
	repo *client.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ActivateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ClientActivated], error) {
	var zero commit.Committed[ClientActivated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	c, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Client", cmd.ID)
	}
	c.Activate()
	event := ClientActivated{
		Metadata: usecase.NewEventMetadata(ec, ClientActivatedType, Source, subjectFor(c.ID)),
		ClientID: c.ID,
	}
	return commit.Save(ctx, uow, c, repo, event, cmd)
}
