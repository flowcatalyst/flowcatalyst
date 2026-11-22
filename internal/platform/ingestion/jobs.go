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
	"github.com/flowcatalyst/go-sdk/usecase"
)

// JobKeysRetention is how long a record's external id keeps answering the job
// it made: a runtime redelivers within minutes or hours, not weeks.
const JobKeysRetention = 7 * 24 * time.Hour

// CheckRuntimeKind refuses a runtime that has not polled (so the platform
// does not know what it carries) or does not carry the ingestion's kind.
func (r *Repository) CheckRuntimeKind(ctx context.Context, i *Ingestion, runtimeID string) error {
	carried, err := runtimeCarries(ctx, r.q, runtimeID)
	if err != nil {
		return err
	}
	return checkKindCarried(carried, i, runtimeID)
}

// runtimeCarries is the kinds the runtime reported at its last poll; none
// when it has not polled (or was pruned).
func runtimeCarries(ctx context.Context, q *dbq.Queries, runtimeID string) ([]string, error) {
	row, err := q.IngestionRuntimeKinds(ctx, runtimeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: runtime kinds: %w", err)
	}
	carried, err := decodeCarried(row.Kinds, row.BuiltinKinds)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: runtime kinds: %w", err)
	}
	return carried, nil
}

// checkKindCarried refuses (RUNTIME_KIND_NOT_CARRIED) an ingestion whose kind
// is not among those the runtime carries.
func checkKindCarried(carried []string, i *Ingestion, runtimeID string) error {
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
		if err := holdLease(ctx, q, i, runtimeID, now); err != nil {
			return err
		}
		if err := clientActiveIn(ctx, q, i.ClientID); err != nil {
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

// WithRuntimeLease runs fn in a transaction that first checks, as WithJobKeys
// does, that the runtime holds the ingestion's lease (a polled source;
// LEASE_NOT_HELD), holding the lease row until fn's work commits, and that the
// client is active (CLIENT_NOT_ACTIVE). The runtime API stores an EVENT
// target's events in it, so a request that lost its lease stores nothing. A
// zero now is the database's clock.
func (r *Repository) WithRuntimeLease(
	ctx context.Context, i *Ingestion, runtimeID string, now time.Time,
	fn func(ctx context.Context, tx pgx.Tx) error,
) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		now, err := clock(ctx, q, now)
		if err != nil {
			return err
		}
		if err := holdLease(ctx, q, i, runtimeID, now); err != nil {
			return err
		}
		if err := clientActiveIn(ctx, q, i.ClientID); err != nil {
			return err
		}
		return fn(ctx, tx)
	})
}

// holdLease checks, for a polled source, that the runtime holds the
// ingestion's lease (LEASE_NOT_HELD), and holds the lease row (FOR SHARE) until
// the caller's transaction ends: a claim by another runtime waits until then,
// while the holder's own concurrent requests do not wait on each other. A
// shared source has no lease and passes.
func holdLease(ctx context.Context, q *dbq.Queries, i *Ingestion, runtimeID string, now time.Time) error {
	if !i.Definition.Source.SingleOwner() {
		return nil
	}
	_, err := q.IngestionLeaseShareHeld(ctx, dbq.IngestionLeaseShareHeldParams{IngestionID: i.ID, RuntimeID: runtimeID, Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return leaseNotHeld(runtimeID, i.ID)
	}
	if err != nil {
		return fmt.Errorf("lock lease: %w", err)
	}
	return nil
}

// clientActiveIn refuses (CLIENT_NOT_ACTIVE) a client that is suspended or
// deactivated, read in the caller's transaction.
func clientActiveIn(ctx context.Context, q *dbq.Queries, clientID ids.ClientID) error {
	status, err := q.IngestionClientStatus(ctx, clientID)
	if err != nil {
		return fmt.Errorf("client status: %w", err)
	}
	return ClientActive(status, clientID)
}
