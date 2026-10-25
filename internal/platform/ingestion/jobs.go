package ingestion

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// JobKeysRetention is how long a record's external id keeps answering the job
// it made: a runtime redelivers within minutes or hours, not weeks.
const JobKeysRetention = 7 * 24 * time.Hour

// CheckRuntimeKind refuses a runtime that has not polled (so the platform
// does not know what it carries) or does not carry the ingestion's kind.
func (r *Repository) CheckRuntimeKind(ctx context.Context, i *Ingestion, runtimeID string) error {
	row, err := r.q.IngestionRuntimeKinds(ctx, runtimeID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ingestion repo: runtime kinds: %w", err)
	}
	var carried []string
	if err == nil {
		if carried, err = decodeCarried(row.Kinds, row.BuiltinKinds); err != nil {
			return fmt.Errorf("ingestion repo: runtime kinds: %w", err)
		}
	}
	kind := RunKind(i.Definition.Source)
	if !slices.Contains(carried, kind) {
		return usecase.Conflict("RUNTIME_KIND_NOT_CARRIED",
			fmt.Sprintf("runtime %s has not reported that it carries kind %s; it must poll first", runtimeID, kind))
	}
	return nil
}

// CheckRuntime refuses a runtime that may not act for the ingestion: it must
// carry the ingestion's kind (CheckRuntimeKind) and for a polled source hold
// its lease. The runtime API's secret call is authorised by this on top of the
// runtime permission; the jobs call checks the lease again inside the
// transaction that creates the jobs (WithJobKeys). A zero now is the
// database's clock.
func (r *Repository) CheckRuntime(ctx context.Context, i *Ingestion, runtimeID string, now time.Time) error {
	if err := r.CheckRuntimeKind(ctx, i, runtimeID); err != nil {
		return err
	}
	if !i.Definition.Source.SingleOwner() {
		return nil
	}
	now, err := clock(ctx, r.q, now)
	if err != nil {
		return fmt.Errorf("ingestion repo: %w", err)
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

// ClientActive refuses an ingestion whose client is suspended or deactivated:
// its ingestions take no jobs and are listed to no runtime.
func ClientActive(status string, clientID ids.ClientID) error {
	if status == "ACTIVE" {
		return nil
	}
	return usecase.Conflict("CLIENT_NOT_ACTIVE",
		fmt.Sprintf("client %s is %s: its ingestions take no jobs", clientID, status))
}

// JobKey is the job a record is about to become, under the external id the
// runtime gave the record.
type JobKey struct {
	ExternalID string
	JobID      string
}

// WithJobKeys checks, claims and creates in one transaction. It holds the
// ingestion's lease row (a polled source) so the lease cannot pass to another
// runtime while the jobs are made, refuses a client that is not active, claims
// the keys, and calls create with the jobs whose key another request (or an
// earlier record of this one) already holds: each such JobID mapped to the job
// that made the key. create makes the jobs of the keys that are new; if it
// fails, no key is kept, so the runtime's retry is not told a job exists that
// was never made. A zero now is the database's clock.
func (r *Repository) WithJobKeys(
	ctx context.Context, i *Ingestion, runtimeID string, keys []JobKey, now time.Time,
	create func(ctx context.Context, tx pgx.Tx, existing map[string]string) error,
) error {
	// In external id order, so two requests sharing keys take them in the same
	// order and wait on each other rather than deadlock. Stable, so of two
	// records with the same key the earlier one claims it and the later one
	// answers the earlier one's job, every time.
	ordered := slices.Clone(keys)
	slices.SortStableFunc(ordered, func(a, b JobKey) int { return cmp.Compare(a.ExternalID, b.ExternalID) })
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		now, err := clock(ctx, q, now)
		if err != nil {
			return err
		}
		if i.Definition.Source.SingleOwner() {
			_, err := q.IngestionLeaseShareHeld(ctx, dbq.IngestionLeaseShareHeldParams{IngestionID: i.ID, RuntimeID: runtimeID, Now: now})
			if errors.Is(err, pgx.ErrNoRows) {
				return leaseNotHeld(runtimeID, i.ID)
			}
			if err != nil {
				return fmt.Errorf("lock lease: %w", err)
			}
		}
		status, err := q.IngestionClientStatus(ctx, i.ClientID)
		if err != nil {
			return fmt.Errorf("client status: %w", err)
		}
		if err := ClientActive(status, i.ClientID); err != nil {
			return err
		}
		names := make([]string, 0, len(ordered))
		for _, k := range ordered {
			if _, err := q.IngestionJobKeyClaim(ctx, dbq.IngestionJobKeyClaimParams{
				IngestionID: i.ID, ExternalID: k.ExternalID, JobID: k.JobID, CreatedAt: now,
			}); err != nil {
				return fmt.Errorf("claim job key: %w", err)
			}
			names = append(names, k.ExternalID)
		}
		rows, err := q.IngestionJobKeysFind(ctx, dbq.IngestionJobKeysFindParams{IngestionID: i.ID, ExternalIds: names})
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
