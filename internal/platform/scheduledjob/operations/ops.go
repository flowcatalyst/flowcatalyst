// Package operations holds all 7 scheduled_job admin use cases plus
// fire_now. Sync (bulk SDK upsert) is deferred to a focused follow-up
// alongside the other subdomain sync ops.
//
// All ops follow the same pattern; they're kept in one file to keep the
// pattern visible.
package operations

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ── Create ────────────────────────────────────────────────────────────────

type CreateCommand struct {
	Code                string          `json:"code"`
	Name                string          `json:"name"`
	Crons               []string        `json:"crons"`
	Timezone            string          `json:"timezone,omitempty"`
	ClientID            *string         `json:"clientId,omitempty"`
	Description         *string         `json:"description,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	Concurrent          bool            `json:"concurrent"`
	TracksCompletion    bool            `json:"tracksCompletion"`
	TimeoutSeconds      *int32          `json:"timeoutSeconds,omitempty"`
	DeliveryMaxAttempts *int32          `json:"deliveryMaxAttempts,omitempty"`
	TargetURL           *string         `json:"targetUrl,omitempty"`
}

type CreateUseCase struct {
	repo *scheduledjob.Repository
	uow  *usecasepgx.UnitOfWork
}

func NewCreateUseCase(repo *scheduledjob.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return usecase.Validation("CODE_REQUIRED", "code is required")
	}
	if !codePattern.MatchString(code) {
		return usecase.Validation("INVALID_CODE_FORMAT",
			"code must start with a lowercase letter and contain only lowercase alphanumeric and hyphens")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name is required")
	}
	if len(cmd.Crons) == 0 {
		return usecase.Validation("CRONS_REQUIRED", "at least one cron expression is required")
	}
	for _, c := range cmd.Crons {
		if strings.TrimSpace(c) == "" {
			return usecase.Validation("INVALID_CRON", "cron expressions cannot be empty")
		}
		// TODO(wave-3g): full cron validation via robfig/cron parser.
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[ScheduledJobCreated] {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	existing, err := uc.repo.FindByCode(ctx, code, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ScheduledJobCreated](usecase.Internal("REPO", "find_by_code failed", err))
	}
	if existing != nil {
		return usecase.Failure[ScheduledJobCreated](usecase.Conflict(
			"CODE_EXISTS", "Scheduled job with code '"+code+"' already exists"))
	}
	j := scheduledjob.New(code, strings.TrimSpace(cmd.Name), cmd.Crons)
	j.ClientID = cmd.ClientID
	j.Description = cmd.Description
	if cmd.Timezone != "" {
		j.Timezone = cmd.Timezone
	}
	j.Payload = cmd.Payload
	j.Concurrent = cmd.Concurrent
	j.TracksCompletion = cmd.TracksCompletion
	j.TimeoutSeconds = cmd.TimeoutSeconds
	if cmd.DeliveryMaxAttempts != nil {
		j.DeliveryMaxAttempts = *cmd.DeliveryMaxAttempts
	}
	j.TargetURL = cmd.TargetURL
	j.CreatedBy = &ec.PrincipalID

	event := ScheduledJobCreated{commonEvent: commonEvent{
		Metadata:       usecase.NewEventMetadata(ec, ScheduledJobCreatedType, Source, subjectFor(j.ID)),
		ScheduledJobID: j.ID,
		Code:           j.Code,
	}}
	return usecasepgx.Commit[scheduledjob.ScheduledJob, ScheduledJobCreated, CreateCommand](
		ctx, uc.uow, j, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, ScheduledJobCreated] = (*CreateUseCase)(nil)

// ── Update ────────────────────────────────────────────────────────────────

type UpdateCommand struct {
	ID                  string          `json:"id"`
	Name                *string         `json:"name,omitempty"`
	Description         *string         `json:"description,omitempty"`
	Crons               []string        `json:"crons,omitempty"`
	Timezone            *string         `json:"timezone,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	Concurrent          *bool           `json:"concurrent,omitempty"`
	TracksCompletion    *bool           `json:"tracksCompletion,omitempty"`
	TimeoutSeconds      *int32          `json:"timeoutSeconds,omitempty"`
	DeliveryMaxAttempts *int32          `json:"deliveryMaxAttempts,omitempty"`
	TargetURL           *string         `json:"targetUrl,omitempty"`
}

type UpdateUseCase struct {
	repo *scheduledjob.Repository
	uow  *usecasepgx.UnitOfWork
}

func NewUpdateUseCase(repo *scheduledjob.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[ScheduledJobUpdated] {
	j, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ScheduledJobUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if j == nil {
		return usecase.Failure[ScheduledJobUpdated](httperror.NotFound("ScheduledJob", cmd.ID))
	}
	if cmd.Name != nil {
		j.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		j.Description = cmd.Description
	}
	if cmd.Crons != nil {
		j.Crons = cmd.Crons
	}
	if cmd.Timezone != nil {
		j.Timezone = *cmd.Timezone
	}
	if cmd.Payload != nil {
		j.Payload = cmd.Payload
	}
	if cmd.Concurrent != nil {
		j.Concurrent = *cmd.Concurrent
	}
	if cmd.TracksCompletion != nil {
		j.TracksCompletion = *cmd.TracksCompletion
	}
	if cmd.TimeoutSeconds != nil {
		j.TimeoutSeconds = cmd.TimeoutSeconds
	}
	if cmd.DeliveryMaxAttempts != nil {
		j.DeliveryMaxAttempts = *cmd.DeliveryMaxAttempts
	}
	if cmd.TargetURL != nil {
		j.TargetURL = cmd.TargetURL
	}
	j.UpdatedBy = &ec.PrincipalID
	j.Version++

	event := ScheduledJobUpdated{commonEvent: commonEvent{
		Metadata:       usecase.NewEventMetadata(ec, ScheduledJobUpdatedType, Source, subjectFor(j.ID)),
		ScheduledJobID: j.ID,
		Code:           j.Code,
	}}
	return usecasepgx.Commit[scheduledjob.ScheduledJob, ScheduledJobUpdated, UpdateCommand](
		ctx, uc.uow, j, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, ScheduledJobUpdated] = (*UpdateUseCase)(nil)

// ── Pause / Resume / Archive ──────────────────────────────────────────────

// transition is the shared body for the three status-flip ops. Returns
// the updated job (caller wraps the typed event).
func (s sharedRepoUOW) transition(ctx context.Context, id, eventType string, ec usecase.ExecutionContext, apply func(*scheduledjob.ScheduledJob)) (*scheduledjob.ScheduledJob, error) {
	j, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("ScheduledJob", id)
	}
	apply(j)
	j.UpdatedBy = &ec.PrincipalID
	return j, nil
}

type sharedRepoUOW struct {
	repo *scheduledjob.Repository
	uow  *usecasepgx.UnitOfWork
}

// PauseCommand is the input DTO.
type PauseCommand struct{ ID string `json:"id"` }

type PauseUseCase struct{ sharedRepoUOW }

func NewPauseUseCase(repo *scheduledjob.Repository, uow *usecasepgx.UnitOfWork) *PauseUseCase {
	return &PauseUseCase{sharedRepoUOW{repo: repo, uow: uow}}
}
func (uc *PauseUseCase) Validate(_ context.Context, cmd PauseCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *PauseUseCase) Authorize(_ context.Context, _ PauseCommand, _ usecase.ExecutionContext) error { return nil }
func (uc *PauseUseCase) Execute(ctx context.Context, cmd PauseCommand, ec usecase.ExecutionContext) usecase.Result[ScheduledJobPaused] {
	j, err := uc.transition(ctx, cmd.ID, ScheduledJobPausedType, ec, func(j *scheduledjob.ScheduledJob) { j.Pause() })
	if err != nil {
		return usecase.Failure[ScheduledJobPaused](err)
	}
	event := ScheduledJobPaused{commonEvent: commonEvent{
		Metadata:       usecase.NewEventMetadata(ec, ScheduledJobPausedType, Source, subjectFor(j.ID)),
		ScheduledJobID: j.ID, Code: j.Code,
	}}
	return usecasepgx.Commit[scheduledjob.ScheduledJob, ScheduledJobPaused, PauseCommand](
		ctx, uc.uow, j, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[PauseCommand, ScheduledJobPaused] = (*PauseUseCase)(nil)

// ResumeCommand is the input DTO.
type ResumeCommand struct{ ID string `json:"id"` }

type ResumeUseCase struct{ sharedRepoUOW }

func NewResumeUseCase(repo *scheduledjob.Repository, uow *usecasepgx.UnitOfWork) *ResumeUseCase {
	return &ResumeUseCase{sharedRepoUOW{repo: repo, uow: uow}}
}
func (uc *ResumeUseCase) Validate(_ context.Context, cmd ResumeCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *ResumeUseCase) Authorize(_ context.Context, _ ResumeCommand, _ usecase.ExecutionContext) error { return nil }
func (uc *ResumeUseCase) Execute(ctx context.Context, cmd ResumeCommand, ec usecase.ExecutionContext) usecase.Result[ScheduledJobResumed] {
	j, err := uc.transition(ctx, cmd.ID, ScheduledJobResumedType, ec, func(j *scheduledjob.ScheduledJob) { j.Resume() })
	if err != nil {
		return usecase.Failure[ScheduledJobResumed](err)
	}
	event := ScheduledJobResumed{commonEvent: commonEvent{
		Metadata:       usecase.NewEventMetadata(ec, ScheduledJobResumedType, Source, subjectFor(j.ID)),
		ScheduledJobID: j.ID, Code: j.Code,
	}}
	return usecasepgx.Commit[scheduledjob.ScheduledJob, ScheduledJobResumed, ResumeCommand](
		ctx, uc.uow, j, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[ResumeCommand, ScheduledJobResumed] = (*ResumeUseCase)(nil)

// ArchiveCommand is the input DTO.
type ArchiveCommand struct{ ID string `json:"id"` }

type ArchiveUseCase struct{ sharedRepoUOW }

func NewArchiveUseCase(repo *scheduledjob.Repository, uow *usecasepgx.UnitOfWork) *ArchiveUseCase {
	return &ArchiveUseCase{sharedRepoUOW{repo: repo, uow: uow}}
}
func (uc *ArchiveUseCase) Validate(_ context.Context, cmd ArchiveCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *ArchiveUseCase) Authorize(_ context.Context, _ ArchiveCommand, _ usecase.ExecutionContext) error { return nil }
func (uc *ArchiveUseCase) Execute(ctx context.Context, cmd ArchiveCommand, ec usecase.ExecutionContext) usecase.Result[ScheduledJobArchived] {
	j, err := uc.transition(ctx, cmd.ID, ScheduledJobArchivedType, ec, func(j *scheduledjob.ScheduledJob) { j.Archive() })
	if err != nil {
		return usecase.Failure[ScheduledJobArchived](err)
	}
	event := ScheduledJobArchived{commonEvent: commonEvent{
		Metadata:       usecase.NewEventMetadata(ec, ScheduledJobArchivedType, Source, subjectFor(j.ID)),
		ScheduledJobID: j.ID, Code: j.Code,
	}}
	return usecasepgx.Commit[scheduledjob.ScheduledJob, ScheduledJobArchived, ArchiveCommand](
		ctx, uc.uow, j, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[ArchiveCommand, ScheduledJobArchived] = (*ArchiveUseCase)(nil)

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteCommand struct{ ID string `json:"id"` }

type DeleteUseCase struct {
	repo *scheduledjob.Repository
	uow  *usecasepgx.UnitOfWork
}

func NewDeleteUseCase(repo *scheduledjob.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
	return &DeleteUseCase{repo: repo, uow: uow}
}

func (uc *DeleteUseCase) Validate(_ context.Context, cmd DeleteCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *DeleteUseCase) Authorize(_ context.Context, _ DeleteCommand, _ usecase.ExecutionContext) error { return nil }
func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[ScheduledJobDeleted] {
	j, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ScheduledJobDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if j == nil {
		return usecase.Failure[ScheduledJobDeleted](httperror.NotFound("ScheduledJob", cmd.ID))
	}
	event := ScheduledJobDeleted{commonEvent: commonEvent{
		Metadata:       usecase.NewEventMetadata(ec, ScheduledJobDeletedType, Source, subjectFor(j.ID)),
		ScheduledJobID: j.ID, Code: j.Code,
	}}
	return usecasepgx.CommitDelete[scheduledjob.ScheduledJob, ScheduledJobDeleted, DeleteCommand](
		ctx, uc.uow, j, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, ScheduledJobDeleted] = (*DeleteUseCase)(nil)

// ── FireNow ───────────────────────────────────────────────────────────────

// FireNowCommand triggers a manual fire. The scheduler's dispatcher
// (Wave 3g) actually writes the msg_scheduled_job_instances row; this
// use case emits the audit event recording the human action.
type FireNowCommand struct{ ID string `json:"id"` }

type FireNowUseCase struct {
	repo *scheduledjob.Repository
	uow  *usecasepgx.UnitOfWork
}

func NewFireNowUseCase(repo *scheduledjob.Repository, uow *usecasepgx.UnitOfWork) *FireNowUseCase {
	return &FireNowUseCase{repo: repo, uow: uow}
}

func (uc *FireNowUseCase) Validate(_ context.Context, cmd FireNowCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}
func (uc *FireNowUseCase) Authorize(_ context.Context, _ FireNowCommand, _ usecase.ExecutionContext) error { return nil }
func (uc *FireNowUseCase) Execute(ctx context.Context, cmd FireNowCommand, ec usecase.ExecutionContext) usecase.Result[ScheduledJobFiredManually] {
	j, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ScheduledJobFiredManually](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if j == nil {
		return usecase.Failure[ScheduledJobFiredManually](httperror.NotFound("ScheduledJob", cmd.ID))
	}
	if j.Status != scheduledjob.StatusActive {
		return usecase.Failure[ScheduledJobFiredManually](usecase.Conflict(
			"NOT_ACTIVE", "Only ACTIVE jobs can be fired manually"))
	}
	// The instance ID is generated here; the dispatcher (Wave 3g) reads
	// the FiredManually event and inserts the corresponding instance row.
	instanceID := tsid.Generate(tsid.ScheduledJobInstance)
	event := ScheduledJobFiredManually{
		commonEvent: commonEvent{
			Metadata:       usecase.NewEventMetadata(ec, ScheduledJobFiredManuallyType, Source, subjectFor(j.ID)),
			ScheduledJobID: j.ID, Code: j.Code,
		},
		InstanceID: instanceID,
	}
	// EmitEvent (not Commit) because we're not modifying the aggregate.
	return usecasepgx.EmitEvent[ScheduledJobFiredManually, FireNowCommand](
		ctx, uc.uow, event, cmd,
	)
}

var _ usecase.UseCase[FireNowCommand, ScheduledJobFiredManually] = (*FireNowUseCase)(nil)
