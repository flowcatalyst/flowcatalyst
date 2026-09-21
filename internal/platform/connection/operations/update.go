package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// ApplicationCode is set-if-provided: nil/omitted leaves the existing
	// link alone, so it cannot be cleared through this endpoint (mirrors
	// ConnectionID's set-if-provided semantics on subscription update).
	ApplicationCode *string `json:"applicationCode,omitempty"`
	Description     *string `json:"description,omitempty"`
	ExternalID      *string `json:"externalId,omitempty"`
	Status          *string `json:"status,omitempty"`
}

// UpdateConnection mutates mutable fields and emits [ConnectionUpdated].
func UpdateConnection(repo *connection.Repository, apps *application.Repository) usecaseop.Operation[UpdateCommand, ConnectionUpdated] {
	return usecaseop.Operation[UpdateCommand, ConnectionUpdated]{
		Name: "UpdateConnection",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "Connection name is required")
			}
			return nil
		},
		// Per-resource authz needs the loaded row, so it runs post-load in
		// Execute; the coarse "may update connections" permission is on the
		// controller.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ConnectionUpdated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Connection", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), c.ClientID); err != nil {
				return nil, err
			}
			c.Name = strings.TrimSpace(cmd.Name)
			c.Description = cmd.Description
			c.ExternalID = cmd.ExternalID
			if cmd.ApplicationCode != nil && !strPtrEqual(cmd.ApplicationCode, c.ApplicationCode) {
				app, err := apps.FindByCode(ctx, *cmd.ApplicationCode)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_application_by_code failed", err)
				}
				if app == nil {
					return nil, httperror.NotFound("Application", *cmd.ApplicationCode)
				}
				if err := requireApplicationAccess(ctx, app); err != nil {
					return nil, err
				}
				// Re-pointing to a different application changes the effective
				// uniqueness key (application_code, client_id, code); a collision
				// there must be reported the same way create does, not surface
				// as a raw database error out of the unique index.
				dup, err := repo.FindByCode(ctx, c.Code, cmd.ApplicationCode, c.ClientID)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_by_code failed", err)
				}
				if dup != nil && dup.ID != c.ID {
					return nil, usecase.Conflict("CODE_EXISTS",
						"Connection with code '"+c.Code+"' already exists")
				}
				c.ApplicationCode = cmd.ApplicationCode
			}
			if cmd.Status != nil {
				status, ok := connection.ParseStatus(strings.TrimSpace(*cmd.Status))
				if !ok {
					return nil, usecase.Validation("INVALID_STATUS", "status must be ACTIVE or PAUSED")
				}
				c.Status = status
			}
			event := ConnectionUpdated{
				Metadata:     usecase.NewEventMetadata(ec, ConnectionUpdatedType, Source, subjectFor(c.ID)),
				ConnectionID: c.ID,
				Name:         c.Name,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// strPtrEqual reports whether two optional strings hold the same value,
// treating nil and nil as equal.
func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// statusCommand is the input DTO for the pause/activate status-flip ops.
type statusCommand struct {
	ID string `json:"id"`
}

// PauseCommand flips a connection to PAUSED.
type PauseCommand = statusCommand

// ActivateCommand flips a connection to ACTIVE.
type ActivateCommand = statusCommand

// flipStatus builds a status-flip Operation: load the connection, apply the
// supplied mutator, emit a [ConnectionUpdated] event. Shared body for
// PauseConnection / ActivateConnection.
func flipStatus(
	name string,
	repo *connection.Repository,
	apply func(*connection.Connection),
) usecaseop.Operation[statusCommand, ConnectionUpdated] {
	return usecaseop.Operation[statusCommand, ConnectionUpdated]{
		Name: name,
		Validate: func(_ context.Context, cmd statusCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[statusCommand],
		Execute: func(ctx context.Context, cmd statusCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ConnectionUpdated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Connection", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), c.ClientID); err != nil {
				return nil, err
			}
			apply(c)
			event := ConnectionUpdated{
				Metadata:     usecase.NewEventMetadata(ec, ConnectionUpdatedType, Source, subjectFor(c.ID)),
				ConnectionID: c.ID,
				Name:         c.Name,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// PauseConnection flips the connection's status to PAUSED.
func PauseConnection(repo *connection.Repository) usecaseop.Operation[PauseCommand, ConnectionUpdated] {
	return flipStatus("PauseConnection", repo, func(c *connection.Connection) { c.Pause() })
}

// ActivateConnection flips the connection's status back to ACTIVE.
func ActivateConnection(repo *connection.Repository) usecaseop.Operation[ActivateCommand, ConnectionUpdated] {
	return flipStatus("ActivateConnection", repo, func(c *connection.Connection) { c.Activate() })
}
