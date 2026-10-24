package ingestion

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// JobKeysRetention is how long a record's external id keeps answering the job
// it made: a runtime redelivers within minutes or hours, not weeks.
const JobKeysRetention = 7 * 24 * time.Hour

// CheckRuntime refuses a runtime that may not act for the ingestion: it must
// have polled (so the platform knows what it carries), carry the ingestion's
// kind, and for a polled source hold its lease. The runtime API's job and
// secret calls are authorised by this on top of the runtime permission.
func (r *Repository) CheckRuntime(ctx context.Context, i *Ingestion, runtimeID string, now time.Time) error {
	raw, err := r.q.IngestionRuntimeKinds(ctx, runtimeID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ingestion repo: runtime kinds: %w", err)
	}
	var reported []RuntimeKind
	if err == nil {
		if jsonErr := json.Unmarshal(raw, &reported); jsonErr != nil {
			return fmt.Errorf("ingestion repo: runtime kinds: %w", jsonErr)
		}
	}
	src := i.Definition.Source
	if err != nil || !Carries(reported, RunKind(src)) {
		return usecase.Conflict("RUNTIME_KIND_NOT_CARRIED",
			fmt.Sprintf("runtime %s has not reported that it carries kind %s; it must poll first", runtimeID, RunKind(src)))
	}
	if !src.SingleOwner() {
		return nil
	}
	held, err := r.q.IngestionLeaseHeld(ctx, dbq.IngestionLeaseHeldParams{IngestionID: i.ID, RuntimeID: runtimeID, Now: now})
	if err != nil {
		return fmt.Errorf("ingestion repo: lease held: %w", err)
	}
	if !held {
		return leaseNotHeld(runtimeID, i.ID)
	}
	return nil
}

// JobKey is the job a record is about to become, under the external id the
// runtime gave the record.
type JobKey struct {
	ExternalID string
	JobID      string
}

// WithJobKeys claims the keys for the ingestion and calls create, in the same
// transaction, with the jobs whose key another request (or an earlier record
// of this one) already holds: each such JobID mapped to the job that made the
// key. create makes the jobs of the keys that are new; if it fails, no key is kept, so the runtime's retry
// is not told a job exists that was never made.
func (r *Repository) WithJobKeys(
	ctx context.Context, ingestionID string, keys []JobKey, now time.Time,
	create func(ctx context.Context, tx pgx.Tx, existing map[string]string) error,
) error {
	// In external id order, so two requests sharing keys take them in the same
	// order and wait on each other rather than deadlock.
	ordered := slices.SortedFunc(slices.Values(keys), func(a, b JobKey) int {
		return cmp.Compare(a.ExternalID, b.ExternalID)
	})
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		names := make([]string, 0, len(ordered))
		for _, k := range ordered {
			if _, err := q.IngestionJobKeyClaim(ctx, dbq.IngestionJobKeyClaimParams{
				IngestionID: ingestionID, ExternalID: k.ExternalID, JobID: k.JobID, CreatedAt: now,
			}); err != nil {
				return fmt.Errorf("claim job key: %w", err)
			}
			names = append(names, k.ExternalID)
		}
		rows, err := q.IngestionJobKeysFind(ctx, dbq.IngestionJobKeysFindParams{IngestionID: ingestionID, ExternalIds: names})
		if err != nil {
			return fmt.Errorf("find job keys: %w", err)
		}
		held := make(map[string]string, len(rows))
		for _, row := range rows {
			held[row.ExternalID] = row.JobID
		}
		existing := map[string]string{}
		for _, k := range keys {
			if held[k.ExternalID] != k.JobID {
				existing[k.JobID] = held[k.ExternalID]
			}
		}
		return create(ctx, tx, existing)
	})
}
