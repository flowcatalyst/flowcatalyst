package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO for CreateEventType.
type CreateCommand struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	// An event type has no client: event types are platform-wide
	// (msg_event_types has no client column). The wire bodies still accept a
	// clientId for compatibility and ignore it; ClientScoped is what marks
	// events of a type as carried per client.
	//
	// ClientScoped: events of this type are carried per client.
	ClientScoped bool            `json:"clientScoped,omitempty"`
	Schema       json.RawMessage `json:"schema,omitempty"`
}

// CreateEventType validates cmd, enforces per-resource client scope,
// enforces uniqueness on code, persists the entity, and atomically emits
// an [EventTypeCreated] event.
func CreateEventType(repo *eventtype.Repository) usecaseop.Operation[CreateCommand, EventTypeCreated] {
	return usecaseop.Operation[CreateCommand, EventTypeCreated]{
		Name: "CreateEventType",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			if strings.TrimSpace(cmd.Code) == "" {
				return usecase.Validation("CODE_REQUIRED", "Event type code is required")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "Event type name is required")
			}
			parts := strings.Split(cmd.Code, ":")
			if len(parts) != 4 {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"Event type code must follow format: application:subdomain:aggregate:event")
			}
			partNames := [...]string{"application", "subdomain", "aggregate", "event"}
			for i, p := range parts {
				if strings.TrimSpace(p) == "" {
					return usecase.Validation("INVALID_CODE_FORMAT",
						fmt.Sprintf("Event type code part '%s' cannot be empty", partNames[i]))
				}
			}
			return nil
		},
		// Resource-level authorization (the coarse "may create event types"
		// permission is enforced at the controller). Every event type is
		// platform-wide, so creating one is a platform-level act: anchor (or
		// super-admin) reach, auth.CheckScopeAccess on no client. A clientId
		// sent on the wire used to pass this check for the caller's own client
		// and then be dropped, creating a platform-wide type.
		Authorize: func(ctx context.Context, _ CreateCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), nil)
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EventTypeCreated], error) {
			existing, err := repo.FindByCode(ctx, cmd.Code)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("CODE_EXISTS",
					"Event type with code '"+cmd.Code+"' already exists")
			}

			et, err := eventtype.New(cmd.Code, cmd.Name)
			if err != nil {
				return nil, usecase.Validation("INVALID_CODE_FORMAT", err.Error())
			}
			et.Description = cmd.Description
			et.ClientScoped = cmd.ClientScoped
			et.CreatedBy = &ec.PrincipalID
			if len(cmd.Schema) > 0 {
				et.AddSchemaVersion(eventtype.NewSpecVersion(et.ID, "1.0", cmd.Schema))
			}

			event := EventTypeCreated{
				Metadata:    usecase.NewEventMetadata(ec, EventTypeCreatedType, EventTypeSourceConst, subjectFor(et.ID)),
				EventTypeID: et.ID,
				Code:        et.Code,
				Name:        et.Name,
				Application: et.Application,
				Subdomain:   et.Subdomain,
				Aggregate:   et.Aggregate,
				EventName:   et.EventName,
				Description: et.Description,
				ClientID:    ids.StringPtr(et.ClientID),
			}
			return usecaseop.Save(et, repo, event), nil
		},
	}
}
