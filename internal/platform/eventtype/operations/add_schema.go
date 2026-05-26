package operations

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// AddSchemaCommand is the input DTO for the AddSchema use case.
type AddSchemaCommand struct {
	EventTypeID string          `json:"eventTypeId"`
	Version     string          `json:"version"`
	Schema      json.RawMessage `json:"schema"`
}

// AddSchemaUseCase implements UseCase[AddSchemaCommand, EventTypeSchemaAdded].
type AddSchemaUseCase struct {
	repo *eventtype.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewAddSchemaUseCase wires the use case.
func NewAddSchemaUseCase(repo *eventtype.Repository, uow *usecasepgx.UnitOfWork) *AddSchemaUseCase {
	return &AddSchemaUseCase{repo: repo, uow: uow}
}

func (uc *AddSchemaUseCase) Validate(_ context.Context, cmd AddSchemaCommand) error {
	if strings.TrimSpace(cmd.EventTypeID) == "" {
		return usecase.Validation("ID_REQUIRED", "eventTypeId is required")
	}
	if strings.TrimSpace(cmd.Version) == "" {
		return usecase.Validation("VERSION_REQUIRED", "version is required")
	}
	if len(cmd.Schema) == 0 {
		return usecase.Validation("SCHEMA_REQUIRED", "schema payload is required")
	}
	return nil
}

func (uc *AddSchemaUseCase) Authorize(_ context.Context, _ AddSchemaCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *AddSchemaUseCase) Execute(ctx context.Context, cmd AddSchemaCommand, ec usecase.ExecutionContext) usecase.Result[EventTypeSchemaAdded] {
	et, err := uc.repo.FindByID(ctx, cmd.EventTypeID)
	if err != nil {
		return usecase.Failure[EventTypeSchemaAdded](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if et == nil {
		return usecase.Failure[EventTypeSchemaAdded](httperror.NotFound("EventType", cmd.EventTypeID))
	}

	// Uniqueness: a (eventTypeId, version) pair should be unique. Repo
	// loads spec_versions on hydrate; check in memory.
	for _, sv := range et.SpecVersions {
		if sv.Version == cmd.Version {
			return usecase.Failure[EventTypeSchemaAdded](usecase.Conflict(
				"VERSION_EXISTS",
				"Schema version '"+cmd.Version+"' already exists for this event type",
			))
		}
	}

	sv := eventtype.NewSpecVersion(et.ID, cmd.Version, cmd.Schema)
	et.AddSchemaVersion(sv)

	event := EventTypeSchemaAdded{
		Metadata:    usecase.NewEventMetadata(ec, EventTypeSchemaAddedType, EventTypeSourceConst, subjectFor(et.ID)),
		EventTypeID: et.ID,
		Version:     sv.Version,
	}
	return usecasepgx.Commit[eventtype.EventType, EventTypeSchemaAdded, AddSchemaCommand](
		ctx, uc.uow, et, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[AddSchemaCommand, EventTypeSchemaAdded] = (*AddSchemaUseCase)(nil)
