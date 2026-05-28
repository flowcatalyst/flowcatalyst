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
	Status      *string `json:"status,omitempty"`
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
	if cmd.Status != nil {
		c.Status = connection.ParseStatus(strings.TrimSpace(*cmd.Status))
	}

	event := ConnectionUpdated{
		Metadata:     usecase.NewEventMetadata(ec, ConnectionUpdatedType, Source, subjectFor(c.ID)),
		ConnectionID: c.ID,
		Name:         c.Name,
	}
	return commit.Save(ctx, uow, c, repo, event, cmd)
}

// statusCommand is the input DTO for the pause/activate status-flip ops.
type statusCommand struct {
	ID string `json:"id"`
}

// PauseCommand flips a connection to PAUSED.
type PauseCommand = statusCommand

// ActivateCommand flips a connection to ACTIVE.
type ActivateCommand = statusCommand

// flipStatus loads the connection, applies the supplied mutator, and emits
// a [ConnectionUpdated] event. Shared body for PauseConnection/ActivateConnection.
func flipStatus(
	ctx context.Context,
	repo *connection.Repository,
	uow *usecasepgx.UnitOfWork,
	id string,
	ec usecase.ExecutionContext,
	apply func(*connection.Connection),
) (commit.Committed[ConnectionUpdated], error) {
	var zero commit.Committed[ConnectionUpdated]
	if strings.TrimSpace(id) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	c, err := repo.FindByID(ctx, id)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Connection", id)
	}
	apply(c)
	event := ConnectionUpdated{
		Metadata:     usecase.NewEventMetadata(ec, ConnectionUpdatedType, Source, subjectFor(c.ID)),
		ConnectionID: c.ID,
		Name:         c.Name,
	}
	return commit.Save(ctx, uow, c, repo, event, statusCommand{ID: id})
}

// PauseConnection flips the connection's status to PAUSED.
func PauseConnection(
	ctx context.Context,
	repo *connection.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd PauseCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ConnectionUpdated], error) {
	return flipStatus(ctx, repo, uow, cmd.ID, ec, func(c *connection.Connection) { c.Pause() })
}

// ActivateConnection flips the connection's status back to ACTIVE.
func ActivateConnection(
	ctx context.Context,
	repo *connection.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ActivateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ConnectionUpdated], error) {
	return flipStatus(ctx, repo, uow, cmd.ID, ec, func(c *connection.Connection) { c.Activate() })
}
