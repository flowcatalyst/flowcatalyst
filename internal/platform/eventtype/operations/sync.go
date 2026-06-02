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
// a single transaction. Emits a per-row [EventTypeCreated], [EventTypeUpdated],
// or [EventTypeDeleted] event for every row touched, alongside one
// [EventTypesSynced] rollup carrying the create/update/delete counts.
// All event writes are atomic with the row writes via [commit.Sync].
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

	var (
		saves       []commit.SyncSave[eventtype.EventType]
		deletes     []commit.SyncDelete[eventtype.EventType]
		syncedCodes = make([]string, 0, len(cmd.EventTypes))
		created     int
		updated     int
		deleted     int
	)

	for _, in := range cmd.EventTypes {
		syncedCodes = append(syncedCodes, in.Code)
		if cur, ok := existingByCode[in.Code]; ok {
			cur.Name = in.Name
			cur.Description = in.Description
			saves = append(saves, commit.SyncSave[eventtype.EventType]{
				Aggregate: cur,
				Event: EventTypeUpdated{
					Metadata:    usecase.NewEventMetadata(ec, EventTypeUpdatedType, EventTypeSourceConst, subjectFor(cur.ID)),
					EventTypeID: cur.ID,
					Name:        cur.Name,
					Description: cur.Description,
				},
			})
			updated++
			continue
		}
		et, err := eventtype.New(in.Code, in.Name)
		if err != nil {
			// Name the offending code: sync aborts on the first bad row, and a
			// bare format message gives no clue which of N codes failed.
			return zero, usecase.Validation("INVALID_CODE",
				fmt.Sprintf("%s (offending code: %q)", err.Error(), in.Code))
		}
		et.Description = in.Description
		et.Source = eventtype.SourceAPI
		saves = append(saves, commit.SyncSave[eventtype.EventType]{
			Aggregate: et,
			Event: EventTypeCreated{
				Metadata:    usecase.NewEventMetadata(ec, EventTypeCreatedType, EventTypeSourceConst, subjectFor(et.ID)),
				EventTypeID: et.ID,
				Code:        et.Code,
				Name:        et.Name,
				Description: et.Description,
				Application: et.Application,
				Subdomain:   et.Subdomain,
				Aggregate:   et.Aggregate,
				EventName:   et.EventName,
				ClientID:    et.ClientID,
			},
		})
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
			deletes = append(deletes, commit.SyncDelete[eventtype.EventType]{
				Aggregate: cur,
				Event: EventTypeDeleted{
					Metadata:    usecase.NewEventMetadata(ec, EventTypeDeletedType, EventTypeSourceConst, subjectFor(cur.ID)),
					EventTypeID: cur.ID,
					Code:        cur.Code,
				},
			})
			deleted++
		}
	}

	rollup := EventTypesSynced{
		Metadata: usecase.NewEventMetadata(ec, EventTypesSyncedType, EventTypeSourceConst,
			"platform.eventtypes."+cmd.ApplicationCode),
		ApplicationCode: cmd.ApplicationCode,
		Created:         uint32(created),
		Updated:         uint32(updated),
		Deleted:         uint32(deleted),
		SyncedCodes:     syncedCodes,
	}
	return commit.Sync(ctx, uow, repo, saves, deletes, rollup, cmd)
}
