package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// MaxStateValue is the longest value a polled source keeps, in characters.
const MaxStateValue = 4000

// leaseNotHeld is the 409 for a runtime that does not hold the ingestion's
// lease: a polled source's state belongs to its holder alone.
func leaseNotHeld(runtimeID, ingestionID string) error {
	return usecase.Conflict("LEASE_NOT_HELD",
		fmt.Sprintf("runtime %s does not hold the lease of ingestion %s", runtimeID, ingestionID))
}

// GetState reads a value a polled source keeps, for the holder of its lease;
// found is false when nothing is stored under the key. A released lease still
// counts until it expires: its holder may be finishing its last batch.
func (r *Repository) GetState(ctx context.Context, ingestionID, runtimeID, key string, now time.Time) (value string, found bool, err error) {
	held, err := r.q.IngestionLeaseHeld(ctx, dbq.IngestionLeaseHeldParams{IngestionID: ingestionID, RuntimeID: runtimeID, Now: now})
	if err != nil {
		return "", false, fmt.Errorf("ingestion repo: lease held: %w", err)
	}
	if !held {
		return "", false, leaseNotHeld(runtimeID, ingestionID)
	}
	value, err = r.q.IngestionStateGet(ctx, dbq.IngestionStateGetParams{IngestionID: ingestionID, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("ingestion repo: get state: %w", err)
	}
	return value, true, nil
}

// PutState stores a value, if the runtime holds the ingestion's lease. The
// lease row is locked while the value is written, so the lease cannot run out
// and pass to another runtime in between.
func (r *Repository) PutState(ctx context.Context, ingestionID, runtimeID, key, value string, now time.Time) error {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		if _, err := q.IngestionLeaseLockHeld(ctx, dbq.IngestionLeaseLockHeldParams{IngestionID: ingestionID, RuntimeID: runtimeID, Now: now}); err != nil {
			return err
		}
		return q.IngestionStatePut(ctx, dbq.IngestionStatePutParams{IngestionID: ingestionID, Key: key, Value: value, UpdatedAt: now})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return leaseNotHeld(runtimeID, ingestionID)
	}
	if err != nil {
		return fmt.Errorf("ingestion repo: put state: %w", err)
	}
	return nil
}
