// Package operations holds all 7 scheduled_job admin use cases plus
// fire_now, and the bulk SDK sync (sync.go).
//
// All CRUD ops follow the same use-case envelope pattern; they're kept in one
// file to keep the pattern visible.
package operations

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateCommand struct {
	Code                string          `json:"code"`
	Name                string          `json:"name"`
	Crons               []string        `json:"crons"`
	Timezone            string          `json:"timezone,omitempty"`
	ClientID            *string         `json:"clientId,omitempty"`
	ApplicationID       *string         `json:"applicationId,omitempty"`
	Description         *string         `json:"description,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	Concurrent          bool            `json:"concurrent"`
	TracksCompletion    bool            `json:"tracksCompletion"`
	TimeoutSeconds      *int32          `json:"timeoutSeconds,omitempty"`
	DeliveryMaxAttempts *int32          `json:"deliveryMaxAttempts,omitempty"`
	TargetURL           *string         `json:"targetUrl,omitempty"`
}

// CreateScheduledJob validates the command, persists a new job, and emits
// [ScheduledJobCreated].
//
// Authorization (the coarse "may write scheduled jobs" permission is the
// controller's): a client-scoped job (ClientID set) requires the caller can
// access that client; a platform-scoped job (ClientID nil) requires anchor.
func CreateScheduledJob(repo *scheduledjob.Repository) usecaseop.Operation[CreateCommand, ScheduledJobCreated] {
	return usecaseop.Operation[CreateCommand, ScheduledJobCreated]{
		Name: "CreateScheduledJob",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			if code == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			if !validate.CodePattern.MatchString(code) {
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
				if err := scheduledjob.ValidateCronShape(c); err != nil {
					return usecase.Validation("CRON_INVALID_SHAPE", err.Error())
				}
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd CreateCommand) error {
			ac := auth.FromContext(ctx)
			if cmd.ClientID != nil {
				if !ac.CanAccessClient(*cmd.ClientID) {
					return httperror.Forbidden("No access to client: " + *cmd.ClientID)
				}
				return nil
			}
			if !ac.IsAnchor() {
				return httperror.Forbidden("Only anchor users can create platform-scoped jobs")
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ScheduledJobCreated], error) {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))

			existing, err := repo.FindByCode(ctx, code, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("CODE_EXISTS", "Scheduled job with code '"+code+"' already exists")
			}
			j := scheduledjob.New(code, strings.TrimSpace(cmd.Name), cmd.Crons)
			j.ClientID = cmd.ClientID
			j.ApplicationID = cmd.ApplicationID
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
			return usecaseop.Save(j, repo, event), nil
		},
	}
}

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

// UpdateScheduledJob mutates mutable fields and emits [ScheduledJobUpdated].
// Per-resource scope is enforced post-load in Execute; the coarse "may write
// scheduled jobs" permission is on the controller.
func UpdateScheduledJob(repo *scheduledjob.Repository) usecaseop.Operation[UpdateCommand, ScheduledJobUpdated] {
	return usecaseop.Operation[UpdateCommand, ScheduledJobUpdated]{
		Name: "UpdateScheduledJob",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			if cmd.Crons != nil {
				if len(cmd.Crons) == 0 {
					return usecase.Validation("CRONS_REQUIRED", "at least one cron expression is required")
				}
				for _, c := range cmd.Crons {
					if strings.TrimSpace(c) == "" {
						return usecase.Validation("INVALID_CRON", "cron expressions cannot be empty")
					}
					if err := scheduledjob.ValidateCronShape(c); err != nil {
						return usecase.Validation("CRON_INVALID_SHAPE", err.Error())
					}
				}
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ScheduledJobUpdated], error) {
			j, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if j == nil {
				return nil, httperror.NotFound("ScheduledJob", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), j.ClientID); err != nil {
				return nil, err
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
			return usecaseop.Save(j, repo, event), nil
		},
	}
}

// ── Pause / Resume / Archive ──────────────────────────────────────────────

// statusFlip builds a status-flip Operation: load the job, enforce per-resource
// scope, apply the supplied mutator, emit the typed event. Shared body for
// PauseScheduledJob / ResumeScheduledJob / ArchiveScheduledJob.
func statusFlip[E usecase.DomainEvent](
	name string,
	repo *scheduledjob.Repository,
	apply func(*scheduledjob.ScheduledJob),
	event func(*scheduledjob.ScheduledJob, usecase.ExecutionContext) E,
) usecaseop.Operation[transitionCommand, E] {
	return usecaseop.Operation[transitionCommand, E]{
		Name: name,
		Validate: func(_ context.Context, cmd transitionCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[transitionCommand],
		Execute: func(ctx context.Context, cmd transitionCommand, ec usecase.ExecutionContext) (usecaseop.Plan[E], error) {
			j, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if j == nil {
				return nil, httperror.NotFound("ScheduledJob", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), j.ClientID); err != nil {
				return nil, err
			}
			apply(j)
			j.UpdatedBy = &ec.PrincipalID
			return usecaseop.Save(j, repo, event(j, ec)), nil
		},
	}
}

// transitionCommand is the shared id-only command for the status-flip ops.
type transitionCommand struct {
	ID string `json:"id"`
}

type (
	PauseCommand   = transitionCommand
	ResumeCommand  = transitionCommand
	ArchiveCommand = transitionCommand
)

// PauseScheduledJob flips a job to PAUSED.
func PauseScheduledJob(repo *scheduledjob.Repository) usecaseop.Operation[PauseCommand, ScheduledJobPaused] {
	return statusFlip("PauseScheduledJob", repo,
		func(j *scheduledjob.ScheduledJob) { j.Pause() },
		func(j *scheduledjob.ScheduledJob, ec usecase.ExecutionContext) ScheduledJobPaused {
			return ScheduledJobPaused{commonEvent: commonEvent{
				Metadata:       usecase.NewEventMetadata(ec, ScheduledJobPausedType, Source, subjectFor(j.ID)),
				ScheduledJobID: j.ID, Code: j.Code,
			}}
		})
}

// ResumeScheduledJob flips a job back to ACTIVE.
func ResumeScheduledJob(repo *scheduledjob.Repository) usecaseop.Operation[ResumeCommand, ScheduledJobResumed] {
	return statusFlip("ResumeScheduledJob", repo,
		func(j *scheduledjob.ScheduledJob) { j.Resume() },
		func(j *scheduledjob.ScheduledJob, ec usecase.ExecutionContext) ScheduledJobResumed {
			return ScheduledJobResumed{commonEvent: commonEvent{
				Metadata:       usecase.NewEventMetadata(ec, ScheduledJobResumedType, Source, subjectFor(j.ID)),
				ScheduledJobID: j.ID, Code: j.Code,
			}}
		})
}

// ArchiveScheduledJob soft-archives a job.
func ArchiveScheduledJob(repo *scheduledjob.Repository) usecaseop.Operation[ArchiveCommand, ScheduledJobArchived] {
	return statusFlip("ArchiveScheduledJob", repo,
		func(j *scheduledjob.ScheduledJob) { j.Archive() },
		func(j *scheduledjob.ScheduledJob, ec usecase.ExecutionContext) ScheduledJobArchived {
			return ScheduledJobArchived{commonEvent: commonEvent{
				Metadata:       usecase.NewEventMetadata(ec, ScheduledJobArchivedType, Source, subjectFor(j.ID)),
				ScheduledJobID: j.ID, Code: j.Code,
			}}
		})
}

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteScheduledJob hard-deletes a job and emits [ScheduledJobDeleted].
// Per-resource scope is enforced post-load in Execute.
func DeleteScheduledJob(repo *scheduledjob.Repository) usecaseop.Operation[DeleteCommand, ScheduledJobDeleted] {
	return usecaseop.Operation[DeleteCommand, ScheduledJobDeleted]{
		Name: "DeleteScheduledJob",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ScheduledJobDeleted], error) {
			j, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if j == nil {
				return nil, httperror.NotFound("ScheduledJob", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), j.ClientID); err != nil {
				return nil, err
			}
			event := ScheduledJobDeleted{commonEvent: commonEvent{
				Metadata:       usecase.NewEventMetadata(ec, ScheduledJobDeletedType, Source, subjectFor(j.ID)),
				ScheduledJobID: j.ID, Code: j.Code,
			}}
			return usecaseop.Delete(j, repo, event), nil
		},
	}
}

// ── FireNow ───────────────────────────────────────────────────────────────

// FireNowCommand triggers a manual fire. An optional CorrelationID is
// stamped on the instance + carried in the firing webhook (mirrors the Rust
// FireRequest.correlation_id).
type FireNowCommand struct {
	ID            string  `json:"id"`
	CorrelationID *string `json:"correlationId,omitempty"`
}

// FireNow inserts a MANUAL instance row (QUEUED, picked up by the dispatcher
// on its next tick) and emits the ScheduledJobFiredManually audit event.
// Two-phase, mirroring Rust fire_now: the infrastructure insert happens first
// (instances are a projection, written directly), then the event is emitted via
// the envelope's [usecaseop.Emit]; a failed insert yields no event.
//
// Per-resource scope is enforced post-load in Execute; the coarse "may fire
// scheduled jobs" permission is on the controller.
//
// PAUSED jobs ARE firable manually — that's the point of a manual trigger
// (the poller skips PAUSED; a human can override). Only ARCHIVED is rejected.
func FireNow(repo *scheduledjob.Repository, instances *scheduledjob.InstanceRepository) usecaseop.Operation[FireNowCommand, ScheduledJobFiredManually] {
	return usecaseop.Operation[FireNowCommand, ScheduledJobFiredManually]{
		Name: "FireNow",
		Validate: func(_ context.Context, cmd FireNowCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[FireNowCommand],
		Execute: func(ctx context.Context, cmd FireNowCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ScheduledJobFiredManually], error) {
			j, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if j == nil {
				return nil, httperror.NotFound("ScheduledJob", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), j.ClientID); err != nil {
				return nil, err
			}
			if j.Status == scheduledjob.StatusArchived {
				return nil, usecase.Conflict("ARCHIVED", "Archived jobs cannot be fired")
			}

			now := time.Now().UTC()
			instanceID := tsid.Generate(tsid.ScheduledJobInstance)
			inst := &scheduledjob.ScheduledJobInstance{
				ID:               instanceID,
				ScheduledJobID:   j.ID,
				ClientID:         j.ClientID,
				JobCode:          j.Code,
				TriggerKind:      scheduledjob.TriggerManual,
				FiredAt:          now,
				Status:           scheduledjob.InstanceStatusQueued,
				DeliveryAttempts: 0,
				CorrelationID:    cmd.CorrelationID,
				CreatedAt:        now,
			}
			if err := instances.Insert(ctx, inst); err != nil {
				return nil, usecase.Internal("REPO", "insert instance failed", err)
			}

			event := ScheduledJobFiredManually{
				commonEvent: commonEvent{
					Metadata:       usecase.NewEventMetadata(ec, ScheduledJobFiredManuallyType, Source, subjectFor(j.ID)),
					ScheduledJobID: j.ID, Code: j.Code,
				},
				InstanceID: instanceID,
			}
			return usecaseop.Emit(event), nil
		},
	}
}
