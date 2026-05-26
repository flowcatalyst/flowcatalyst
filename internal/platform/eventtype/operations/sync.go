package operations

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// SyncEventTypeInput is one entry in a sync batch.
type SyncEventTypeInput struct {
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// SyncEventTypesCommand is the input DTO. RemoveUnlisted = true removes
// API-sourced event types that aren't in EventTypes (UI/CODE sources
// are never touched by sync).
type SyncEventTypesCommand struct {
	ApplicationCode string               `json:"applicationCode"`
	EventTypes      []SyncEventTypeInput `json:"eventTypes"`
	RemoveUnlisted  bool                 `json:"removeUnlisted,omitempty"`
}

// SyncEventTypesUseCase implements UseCase. Bulk-upserts an application's
// event-type catalog atomically (single pgx.Tx) and emits a single
// EventTypesSynced rollup event with the create/update/delete counts.
type SyncEventTypesUseCase struct {
	repo *eventtype.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewSyncEventTypesUseCase wires the use case.
func NewSyncEventTypesUseCase(repo *eventtype.Repository, uow *usecasepgx.UnitOfWork) *SyncEventTypesUseCase {
	return &SyncEventTypesUseCase{repo: repo, uow: uow}
}

func (uc *SyncEventTypesUseCase) Validate(_ context.Context, cmd SyncEventTypesCommand) error {
	if strings.TrimSpace(cmd.ApplicationCode) == "" {
		return usecase.Validation("APPLICATION_CODE_REQUIRED", "Application code is required")
	}
	return nil
}

func (uc *SyncEventTypesUseCase) Authorize(_ context.Context, _ SyncEventTypesCommand, _ usecase.ExecutionContext) error {
	return nil
}

// Execute computes the create/update/delete delta and applies it atomically.
// The Rust impl emits per-row Created/Updated/Deleted events alongside
// the rollup — the Go port today emits only the rollup; per-row events
// land in a focused follow-up (SDK consumers + the audit log key off the
// rollup, so this is the load-bearing event).
func (uc *SyncEventTypesUseCase) Execute(ctx context.Context, cmd SyncEventTypesCommand, ec usecase.ExecutionContext) usecase.Result[EventTypesSynced] {
	existing, err := uc.repo.FindByApplication(ctx, cmd.ApplicationCode)
	if err != nil {
		return usecase.Failure[EventTypesSynced](usecase.Internal("REPO", "find_by_application failed", err))
	}

	existingByCode := make(map[string]*eventtype.EventType, len(existing))
	for i := range existing {
		existingByCode[existing[i].Code] = &existing[i]
	}
	incomingCodes := make(map[string]struct{}, len(cmd.EventTypes))
	for _, in := range cmd.EventTypes {
		incomingCodes[in.Code] = struct{}{}
	}

	var created, updated, deleted int
	syncedCodes := make([]string, 0, len(cmd.EventTypes))

	tx, err := uc.repo.Pool().Begin(ctx)
	if err != nil {
		return usecase.Failure[EventTypesSynced](usecase.Internal("TX_BEGIN", "begin tx failed", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, in := range cmd.EventTypes {
		syncedCodes = append(syncedCodes, in.Code)
		if cur, ok := existingByCode[in.Code]; ok {
			cur.Name = in.Name
			cur.Description = in.Description
			cur.UpdatedAt = time.Now().UTC()
			if err := uc.repo.PersistTx(ctx, cur, tx); err != nil {
				return usecase.Failure[EventTypesSynced](usecase.Internal("REPO", "update failed", err))
			}
			updated++
			continue
		}
		et, err := eventtype.New(in.Code, in.Name)
		if err != nil {
			return usecase.Failure[EventTypesSynced](usecase.Validation("INVALID_CODE", err.Error()))
		}
		et.Description = in.Description
		et.Source = eventtype.SourceAPI
		if err := uc.repo.PersistTx(ctx, et, tx); err != nil {
			return usecase.Failure[EventTypesSynced](usecase.Internal("REPO", "create failed", err))
		}
		created++
	}

	if cmd.RemoveUnlisted {
		for code, cur := range existingByCode {
			if _, present := incomingCodes[code]; present {
				continue
			}
			if cur.Source != eventtype.SourceAPI {
				continue // never touch UI/CODE-managed rows
			}
			if err := uc.repo.DeleteTx(ctx, cur, tx); err != nil {
				return usecase.Failure[EventTypesSynced](usecase.Internal("REPO", "delete failed", err))
			}
			deleted++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return usecase.Failure[EventTypesSynced](usecase.Internal("TX_COMMIT", "commit failed", err))
	}

	event := EventTypesSynced{
		Metadata: usecase.NewEventMetadata(ec, EventTypesSyncedType, EventTypeSourceConst,
			"platform.eventtypes."+cmd.ApplicationCode),
		ApplicationCode: cmd.ApplicationCode,
		Created:         uint32(created),
		Updated:         uint32(updated),
		Deleted:         uint32(deleted),
		SyncedCodes:     syncedCodes,
	}
	return usecasepgx.EmitEvent[EventTypesSynced, SyncEventTypesCommand](
		ctx, uc.uow, event, cmd,
	)
}

var _ usecase.UseCase[SyncEventTypesCommand, EventTypesSynced] = (*SyncEventTypesUseCase)(nil)
