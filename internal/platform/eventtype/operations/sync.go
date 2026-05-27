package operations

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
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

// SyncEventTypesCommand is the input DTO. RemoveUnlisted=true removes
// API-sourced event types that aren't in EventTypes (UI/CODE sources
// are never touched by sync).
type SyncEventTypesCommand struct {
	ApplicationCode string               `json:"applicationCode"`
	EventTypes      []SyncEventTypeInput `json:"eventTypes"`
	RemoveUnlisted  bool                 `json:"removeUnlisted,omitempty"`
}

// SyncEventTypes bulk-upserts an application's event-type catalog within
// a single multi-row transaction, then emits a single EventTypesSynced
// rollup event with create/update/delete counts.
//
// NB the rollup event is emitted in a separate transaction from the
// bulk writes — same as the previous implementation. Per-row events
// (Created/Updated/Deleted) match the Rust source but aren't ported yet.
func SyncEventTypes(
	ctx context.Context,
	repo *eventtype.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd SyncEventTypesCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EventTypesSynced], error) {
	var zero commit.Committed[EventTypesSynced]

	if strings.TrimSpace(cmd.ApplicationCode) == "" {
		return zero, usecase.Validation("APPLICATION_CODE_REQUIRED", "Application code is required")
	}

	existing, err := repo.FindByApplication(ctx, cmd.ApplicationCode)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_application failed", err)
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

	tx, err := repo.Pool().Begin(ctx)
	if err != nil {
		return zero, usecase.Internal("TX_BEGIN", "begin tx failed", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, in := range cmd.EventTypes {
		syncedCodes = append(syncedCodes, in.Code)
		if cur, ok := existingByCode[in.Code]; ok {
			cur.Name = in.Name
			cur.Description = in.Description
			cur.UpdatedAt = time.Now().UTC()
			if err := repo.PersistTx(ctx, cur, tx); err != nil {
				return zero, usecase.Internal("REPO", "update failed", err)
			}
			updated++
			continue
		}
		et, err := eventtype.New(in.Code, in.Name)
		if err != nil {
			return zero, usecase.Validation("INVALID_CODE", err.Error())
		}
		et.Description = in.Description
		et.Source = eventtype.SourceAPI
		if err := repo.PersistTx(ctx, et, tx); err != nil {
			return zero, usecase.Internal("REPO", "create failed", err)
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
			if err := repo.DeleteTx(ctx, cur, tx); err != nil {
				return zero, usecase.Internal("REPO", "delete failed", err)
			}
			deleted++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return zero, usecase.Internal("TX_COMMIT", "commit failed", err)
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
	return commit.Emit(ctx, uow, event, cmd)
}
