package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// CreateCommand is the input DTO for CreateEventType.
type CreateCommand struct {
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	ClientID    *string         `json:"clientId,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// CreateEventType validates cmd, enforces uniqueness on code, persists
// the entity, and atomically emits an [EventTypeCreated] event.
func CreateEventType(
	ctx context.Context,
	repo *eventtype.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd CreateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EventTypeCreated], error) {
	var zero commit.Committed[EventTypeCreated]

	if strings.TrimSpace(cmd.Code) == "" {
		return zero, usecase.Validation("CODE_REQUIRED", "Event type code is required")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "Event type name is required")
	}
	parts := strings.Split(cmd.Code, ":")
	if len(parts) != 4 {
		return zero, usecase.Validation("INVALID_CODE_FORMAT",
			"Event type code must follow format: application:subdomain:aggregate:event")
	}
	partNames := [...]string{"application", "subdomain", "aggregate", "event"}
	for i, p := range parts {
		if strings.TrimSpace(p) == "" {
			return zero, usecase.Validation("INVALID_CODE_FORMAT",
				fmt.Sprintf("Event type code part '%s' cannot be empty", partNames[i]))
		}
	}

	existing, err := repo.FindByCode(ctx, cmd.Code)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict("CODE_EXISTS",
			"Event type with code '"+cmd.Code+"' already exists")
	}

	et, err := eventtype.New(cmd.Code, cmd.Name)
	if err != nil {
		return zero, usecase.Validation("INVALID_CODE_FORMAT", err.Error())
	}
	et.Description = cmd.Description
	et.ClientID = cmd.ClientID
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
		ClientID:    et.ClientID,
	}

	return commit.Save(ctx, uow, et, repo, event, cmd)
}
