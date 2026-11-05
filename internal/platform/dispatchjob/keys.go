package dispatchjob

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Idempotency keys of SDK-created jobs (msg_dispatch_job_keys, migration 099).
//
// A job created through POST /api/dispatch-jobs(/batch) may carry an
// idempotencyKey. The first job to name a key under its scope wins it, in the
// transaction that creates the job; a later request naming the same key under
// the same scope answers that job's id and creates nothing. The scope is the
// job's application (the first segment of its code, the application facet
// used for a job everywhere else) and its client ("" when it has none), so two
// tenants of one application using the same key never answer each other's
// jobs. msg_dispatch_jobs cannot hold this constraint itself: it is
// partitioned by created_at, and a unique index there would have to include
// created_at.
//
// The SQL is hand-rolled, as the Lifecycle's is: a batch claims all its keys
// in one statement and reads the holders back in a second, so a 1000-item
// keyed batch costs two round trips, not two thousand.

// JobKeysRetention is how long an idempotency key keeps answering the job it
// made. A caller's retry comes within minutes or hours, not weeks.
const JobKeysRetention = 7 * 24 * time.Hour

// MaxIdempotencyKeyLen is the longest idempotency key a job may carry: the
// width of msg_dispatch_jobs.idempotency_key and msg_dispatch_job_keys'.
const MaxIdempotencyKeyLen = 100

// keyPruneChunk bounds one retention DELETE.
const keyPruneChunk = 10000

// KeyScope is the scope a job's idempotency key is unique in: the job's
// application (the first segment of its code) and its client ("" for none).
func KeyScope(j *DispatchJob) (application, client string) {
	application, _, _ = strings.Cut(j.Code, ":")
	if j.ClientID != nil {
		client = *j.ClientID
	}
	return application, client
}

func idemKey(j *DispatchJob) string {
	if j.IdempotencyKey == nil {
		return ""
	}
	return *j.IdempotencyKey
}

// keyHolder is a claimed key read back: the item's position among the keyed
// items, and the job that holds its key.
type keyHolder struct {
	n   int32
	job string
}

// InsertBatchKeyed creates jobs as PENDING, as InsertBatch does, honouring
// each job's IdempotencyKey. It returns, in the order of jobs, the id each one
// resolved to: its own when it was created, or the id of the job that already
// holds its key (an earlier request's, or an earlier item's of this batch),
// in which case it was not created. A batch without keys is a plain
// InsertBatch.
//
// Keys and jobs are written in one transaction: if creating the jobs fails no
// key is kept, so a retry is never told about a job that was never made. Keys
// are claimed in scope order in one statement, so two batches sharing keys
// wait on each other rather than deadlock; of two items of one batch with the
// same key, the earlier one wins.
func (r *Repository) InsertBatchKeyed(ctx context.Context, jobs []DispatchJob) ([]string, error) {
	out := make([]string, len(jobs))
	var keyed []int
	for i := range jobs {
		out[i] = jobs[i].ID
		if idemKey(&jobs[i]) != "" {
			keyed = append(keyed, i)
		}
	}
	if len(keyed) == 0 {
		return out, r.InsertBatch(ctx, jobs)
	}
	n := len(keyed)
	apps, clients, keys, jobIDs, ords := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]int32, n)
	for p, i := range keyed {
		apps[p], clients[p] = KeyScope(&jobs[i])
		keys[p], jobIDs[p], ords[p] = idemKey(&jobs[i]), jobs[i].ID, int32(p)
	}
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// ORDER BY the key (then the item's position) so the rows are
		// inserted, and their locks taken, in one global order, and the
		// earlier of two items sharing a key is the one that lands.
		if _, err := tx.Exec(ctx, `
			INSERT INTO msg_dispatch_job_keys (application_code, client_id, idempotency_key, job_id)
			SELECT t.a, t.c, t.k, t.j
			  FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::int[]) AS t(a, c, k, j, n)
			 ORDER BY t.a, t.c, t.k, t.n
			ON CONFLICT (application_code, client_id, idempotency_key) DO NOTHING`,
			apps, clients, keys, jobIDs, ords); err != nil {
			return fmt.Errorf("claim idempotency keys: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT t.n, k.job_id
			  FROM unnest($1::text[], $2::text[], $3::text[], $4::int[]) AS t(a, c, k, n)
			  JOIN msg_dispatch_job_keys k
			    ON k.application_code = t.a AND k.client_id = t.c AND k.idempotency_key = t.k`,
			apps, clients, keys, ords)
		if err != nil {
			return fmt.Errorf("find idempotency keys: %w", err)
		}
		holders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (keyHolder, error) {
			var h keyHolder
			err := row.Scan(&h.n, &h.job)
			return h, err
		})
		if err != nil {
			return fmt.Errorf("find idempotency keys: %w", err)
		}
		if len(holders) != n {
			return fmt.Errorf("find idempotency keys: %d holders for %d keys", len(holders), n)
		}
		for _, h := range holders {
			out[keyed[h.n]] = h.job
		}
		create := make([]DispatchJob, 0, len(jobs))
		for i := range jobs {
			if out[i] == jobs[i].ID {
				create = append(create, jobs[i])
			}
		}
		_, err = r.lc.In(tx).CreateBatch(ctx, create)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PruneJobKeys deletes up to one chunk of idempotency keys created before
// cutoff, returning how many went.
func (r *Repository) PruneJobKeys(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM msg_dispatch_job_keys WHERE ctid = ANY(ARRAY(
			SELECT ctid FROM msg_dispatch_job_keys WHERE created_at < $1 LIMIT $2))`,
		cutoff.UTC(), keyPruneChunk)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RunJobKeyRetention prunes idempotency keys older than JobKeysRetention on a
// ticker until ctx is cancelled, draining a backlog chunk by chunk. Not
// leader-gated: bounded, idempotent deletes, as the reaper (RunReaper).
func RunJobKeyRetention(ctx context.Context, repo *Repository, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			cutoff := time.Now().UTC().Add(-JobKeysRetention)
			var total int64
			for range 100 { // at most 100 chunks per tick
				n, err := repo.PruneJobKeys(ctx, cutoff)
				if err != nil {
					slog.Warn("dispatch-job idempotency key prune failed", "err", err)
					break
				}
				total += n
				if n < keyPruneChunk {
					break
				}
			}
			if total > 0 {
				slog.Debug("dispatch-job idempotency keys pruned", "removed", total)
			}
		}
	}
}
