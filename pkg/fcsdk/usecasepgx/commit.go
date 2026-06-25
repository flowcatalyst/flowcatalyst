package usecasepgx

import (
	"context"
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/internal/sealed"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Commit upserts the aggregate via its repository, writes the domain
// event and audit log via the configured Sink — all in one transaction.
//
// This is one of the few public paths to a Success-valued
// usecase.Result outside the SDK. The seal on usecase.Success is
// satisfied here by importing internal/sealed.
func Commit[A usecase.HasID, E usecase.DomainEvent, C any](
	ctx context.Context,
	uow *UnitOfWork,
	aggregate *A,
	repo Persist[A],
	event E,
	command C,
) usecase.Result[E] {
	tx, err := uow.pool.Begin(ctx)
	if err != nil {
		return usecase.Failure[E](usecase.Internal("TX_BEGIN", "could not open transaction", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dbTx := newDbTx(tx)
	if err := repo.Persist(ctx, aggregate, dbTx); err != nil {
		return usecase.Failure[E](usecase.Internal("PERSIST", "repository persist failed", err))
	}
	if err := uow.sink.WriteEvent(ctx, dbTx, event); err != nil {
		return usecase.Failure[E](usecase.Internal("EVENT_WRITE", "could not write domain event", err))
	}
	if err := uow.sink.WriteAudit(ctx, dbTx, event, command); err != nil {
		return usecase.Failure[E](usecase.Internal("AUDIT_WRITE", "could not write audit log", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return usecase.Failure[E](usecase.Internal("TX_COMMIT", "could not commit transaction", err))
	}

	return usecase.Success[E](sealed.New(), event)
}

// CommitDelete deletes the aggregate via its repository and emits the
// deletion event + audit log atomically.
func CommitDelete[A usecase.HasID, E usecase.DomainEvent, C any](
	ctx context.Context,
	uow *UnitOfWork,
	aggregate *A,
	repo Persist[A],
	event E,
	command C,
) usecase.Result[E] {
	tx, err := uow.pool.Begin(ctx)
	if err != nil {
		return usecase.Failure[E](usecase.Internal("TX_BEGIN", "could not open transaction", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dbTx := newDbTx(tx)
	if err := repo.Delete(ctx, aggregate, dbTx); err != nil {
		return usecase.Failure[E](usecase.Internal("DELETE", "repository delete failed", err))
	}
	if err := uow.sink.WriteEvent(ctx, dbTx, event); err != nil {
		return usecase.Failure[E](usecase.Internal("EVENT_WRITE", "could not write domain event", err))
	}
	if err := uow.sink.WriteAudit(ctx, dbTx, event, command); err != nil {
		return usecase.Failure[E](usecase.Internal("AUDIT_WRITE", "could not write audit log", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return usecase.Failure[E](usecase.Internal("TX_COMMIT", "could not commit transaction", err))
	}

	return usecase.Success[E](sealed.New(), event)
}

// EmitEvent writes a domain event + audit log without an entity change.
// Used for events that don't modify an entity directly (e.g.
// UserLoggedIn).
func EmitEvent[E usecase.DomainEvent, C any](
	ctx context.Context,
	uow *UnitOfWork,
	event E,
	command C,
) usecase.Result[E] {
	tx, err := uow.pool.Begin(ctx)
	if err != nil {
		return usecase.Failure[E](usecase.Internal("TX_BEGIN", "could not open transaction", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dbTx := newDbTx(tx)
	if err := uow.sink.WriteEvent(ctx, dbTx, event); err != nil {
		return usecase.Failure[E](usecase.Internal("EVENT_WRITE", "could not write domain event", err))
	}
	if err := uow.sink.WriteAudit(ctx, dbTx, event, command); err != nil {
		return usecase.Failure[E](usecase.Internal("AUDIT_WRITE", "could not write audit log", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return usecase.Failure[E](usecase.Internal("TX_COMMIT", "could not commit transaction", err))
	}

	return usecase.Success[E](sealed.New(), event)
}

// CommitAll upserts a batch of aggregates of the same type via one
// repository and emits a single summary event + audit log. Use when one
// logical operation touches many rows (e.g. toggling client → application
// enablement).
func CommitAll[A usecase.HasID, E usecase.DomainEvent, C any](
	ctx context.Context,
	uow *UnitOfWork,
	aggregates []A,
	repo Persist[A],
	event E,
	command C,
) usecase.Result[E] {
	tx, err := uow.pool.Begin(ctx)
	if err != nil {
		return usecase.Failure[E](usecase.Internal("TX_BEGIN", "could not open transaction", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dbTx := newDbTx(tx)
	for i := range aggregates {
		if err := repo.Persist(ctx, &aggregates[i], dbTx); err != nil {
			return usecase.Failure[E](usecase.Internal("PERSIST_BATCH", fmt.Sprintf("persist failed at index %d", i), err))
		}
	}
	if err := uow.sink.WriteEvent(ctx, dbTx, event); err != nil {
		return usecase.Failure[E](usecase.Internal("EVENT_WRITE", "could not write domain event", err))
	}
	if err := uow.sink.WriteAudit(ctx, dbTx, event, command); err != nil {
		return usecase.Failure[E](usecase.Internal("AUDIT_WRITE", "could not write audit log", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return usecase.Failure[E](usecase.Internal("TX_COMMIT", "could not commit transaction", err))
	}

	return usecase.Success[E](sealed.New(), event)
}

// SyncSaveItem pairs an aggregate to upsert with the per-row domain event
// to emit for it (typically a Created or Updated event). See CommitSync.
type SyncSaveItem[A usecase.HasID] struct {
	Aggregate *A
	Event     usecase.DomainEvent
}

// SyncDeleteItem pairs an aggregate to delete with the per-row domain
// event to emit for it. See CommitSync.
type SyncDeleteItem[A usecase.HasID] struct {
	Aggregate *A
	Event     usecase.DomainEvent
}

// CommitSync persists a batch of aggregate saves and deletes, writes one
// domain event + audit row per touched aggregate, and writes a rollup
// event + audit row — ALL within a single transaction. The rollup's
// value is returned as the sealed Success.
//
// Use for sync / bulk-upsert endpoints (e.g. the SDK's sync-platform
// catalog refresh) whose consumers project the per-row events. Compared
// to CommitAll (one summary event only), this gives subscribers
// incremental visibility into what changed while preserving the
// transactional atomicity guarantee.
//
// Write order: saves (in given order) → deletes (in given order) →
// rollup. Each event row hits msg_events / outbox_messages in commit
// order, so downstream readers see per-row events before the rollup.
//
// All audit rows record the outer `command` (typically the bulk sync
// command). Per-row context lives in the event payloads themselves.
func CommitSync[A usecase.HasID, RE usecase.DomainEvent, C any](
	ctx context.Context,
	uow *UnitOfWork,
	repo Persist[A],
	saves []SyncSaveItem[A],
	deletes []SyncDeleteItem[A],
	rollup RE,
	command C,
) usecase.Result[RE] {
	tx, err := uow.pool.Begin(ctx)
	if err != nil {
		return usecase.Failure[RE](usecase.Internal("TX_BEGIN", "could not open transaction", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dbTx := newDbTx(tx)

	for i := range saves {
		if err := repo.Persist(ctx, saves[i].Aggregate, dbTx); err != nil {
			return usecase.Failure[RE](usecase.Internal("PERSIST_BATCH", fmt.Sprintf("sync save failed at index %d", i), err))
		}
		if err := uow.sink.WriteEvent(ctx, dbTx, saves[i].Event); err != nil {
			return usecase.Failure[RE](usecase.Internal("EVENT_WRITE", fmt.Sprintf("per-row save event write failed at index %d", i), err))
		}
		if err := uow.sink.WriteAudit(ctx, dbTx, saves[i].Event, command); err != nil {
			return usecase.Failure[RE](usecase.Internal("AUDIT_WRITE", fmt.Sprintf("per-row save audit write failed at index %d", i), err))
		}
	}

	for i := range deletes {
		if err := repo.Delete(ctx, deletes[i].Aggregate, dbTx); err != nil {
			return usecase.Failure[RE](usecase.Internal("DELETE_BATCH", fmt.Sprintf("sync delete failed at index %d", i), err))
		}
		if err := uow.sink.WriteEvent(ctx, dbTx, deletes[i].Event); err != nil {
			return usecase.Failure[RE](usecase.Internal("EVENT_WRITE", fmt.Sprintf("per-row delete event write failed at index %d", i), err))
		}
		if err := uow.sink.WriteAudit(ctx, dbTx, deletes[i].Event, command); err != nil {
			return usecase.Failure[RE](usecase.Internal("AUDIT_WRITE", fmt.Sprintf("per-row delete audit write failed at index %d", i), err))
		}
	}

	if err := uow.sink.WriteEvent(ctx, dbTx, rollup); err != nil {
		return usecase.Failure[RE](usecase.Internal("EVENT_WRITE", "rollup event write failed", err))
	}
	if err := uow.sink.WriteAudit(ctx, dbTx, rollup, command); err != nil {
		return usecase.Failure[RE](usecase.Internal("AUDIT_WRITE", "rollup audit write failed", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return usecase.Failure[RE](usecase.Internal("TX_COMMIT", "could not commit transaction", err))
	}

	return usecase.Success[RE](sealed.New(), rollup)
}
