package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// CreateCommand is the input DTO for the CreateEventType use case.
type CreateCommand struct {
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	ClientID    *string         `json:"clientId,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// CreateUseCase implements UseCase[CreateCommand, EventTypeCreated].
type CreateUseCase struct {
	repo *eventtype.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *eventtype.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

// Validate checks the command shape.
func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
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
}

// Authorize — handler enforces tenant-level access; nothing extra here.
func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

// Execute applies business rules (uniqueness) and commits via UoW.
// The seal on Result[EventTypeCreated] means this is the ONLY path to
// a successful create event.
func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[EventTypeCreated] {
	existing, err := uc.repo.FindByCode(ctx, cmd.Code)
	if err != nil {
		return usecase.Failure[EventTypeCreated](usecase.Internal("REPO", "find_by_code failed", err))
	}
	if existing != nil {
		return usecase.Failure[EventTypeCreated](usecase.Conflict(
			"CODE_EXISTS",
			"Event type with code '"+cmd.Code+"' already exists",
		))
	}

	et, err := eventtype.New(cmd.Code, cmd.Name)
	if err != nil {
		return usecase.Failure[EventTypeCreated](
			usecase.Validation("INVALID_CODE_FORMAT", err.Error()))
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

	return usecasepgx.Commit[eventtype.EventType, EventTypeCreated, CreateCommand](
		ctx, uc.uow, et, uc.repo, event, cmd,
	)
}

// Compile-time check: CreateUseCase implements UseCase.
var _ usecase.UseCase[CreateCommand, EventTypeCreated] = (*CreateUseCase)(nil)
