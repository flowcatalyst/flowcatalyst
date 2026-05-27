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

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

// UpdateClient mutates the client name and emits [ClientUpdated].
func UpdateClient(
	ctx context.Context,
	repo *client.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ClientUpdated], error) {
	var zero commit.Committed[ClientUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}

	c, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Client", cmd.ID)
	}
	if cmd.Name != nil {
		c.Name = strings.TrimSpace(*cmd.Name)
	}

	event := ClientUpdated{
		Metadata: usecase.NewEventMetadata(ec, ClientUpdatedType, Source, subjectFor(c.ID)),
		ClientID: c.ID,
		Name:     c.Name,
	}
	return commit.Save(ctx, uow, c, repo, event, cmd)
}
