package ingestion

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// WithTraceAccess runs insert in one transaction, telling it which of the
// ingestions a trace batch names the runtime may not post records for, each
// with the code the jobs call would refuse it with (owner's decision D2,
// 2026-11-22: posting trace records for a polled ingestion needs the lease, as
// the jobs call does). The checks are the jobs call's, in its order: the
// ingestion exists (INGESTION_NOT_FOUND) and is live (INGESTION_NOT_LIVE), the
// runtime carries its kind (RUNTIME_KIND_NOT_CARRIED), for a polled source the
// runtime holds its lease (LEASE_NOT_HELD), with the lease row held FOR SHARE
// until insert's transaction commits, and its client is active
// (CLIENT_NOT_ACTIVE). An ingestion absent from refused may be posted for. The
// lease is the one held now, so records queued through an outage are taken
// while the runtime still holds it. A zero now is the database's clock.
func (r *Repository) WithTraceAccess(
	ctx context.Context, runtimeID string, ingestionIDs []string, now time.Time,
	insert func(ctx context.Context, tx pgx.Tx, refused map[string]string) error,
) error {
	// In id order, so two batches naming the same polled ingestions hold their
	// lease rows in the same order.
	named := slices.Clone(ingestionIDs)
	slices.Sort(named)
	named = slices.Compact(named)
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		now, err := clock(ctx, q, now)
		if err != nil {
			return err
		}
		rows, err := q.IngestionFindByIDs(ctx, named)
		if err != nil {
			return fmt.Errorf("ingestion repo: find ingestions: %w", err)
		}
		found := make(map[string]*Ingestion, len(rows))
		for _, row := range rows {
			i, err := rowToIngestion(row)
			if err != nil {
				continue // logged by rowToIngestion; its records are refused as not found
			}
			found[i.ID] = i
		}
		carried, err := runtimeCarries(ctx, q, runtimeID)
		if err != nil {
			return err
		}
		refused := map[string]string{}
		clients := map[ids.ClientID]error{}
		for _, id := range named {
			code, err := traceRefusal(ctx, q, found[id], runtimeID, carried, now, clients)
			if err != nil {
				return err
			}
			if code != "" {
				refused[id] = code
			}
		}
		return insert(ctx, tx, refused)
	})
}

// traceRefusal is why the runtime may not post trace records for the
// ingestion i (nil: not found), or "" when it may; err is a fault, not a
// refusal. clients caches each client's status check within one batch.
func traceRefusal(
	ctx context.Context, q *dbq.Queries, i *Ingestion, runtimeID string, carried []string, now time.Time,
	clients map[ids.ClientID]error,
) (string, error) {
	if i == nil {
		return "INGESTION_NOT_FOUND", nil
	}
	if !i.Live() {
		return "INGESTION_NOT_LIVE", nil
	}
	if err := checkKindCarried(carried, i, runtimeID); err != nil {
		return refusalCode(err)
	}
	if err := holdLease(ctx, q, i, runtimeID, now); err != nil {
		return refusalCode(err)
	}
	active, checked := clients[i.ClientID]
	if !checked {
		active = clientActiveIn(ctx, q, i.ClientID)
		clients[i.ClientID] = active
	}
	if active != nil {
		return refusalCode(active)
	}
	return "", nil
}

// refusalCode is the code of a refusal, or err itself when it is a fault.
func refusalCode(err error) (string, error) {
	if e := usecase.AsError(err); e != nil {
		return e.Code, nil
	}
	return "", err
}
