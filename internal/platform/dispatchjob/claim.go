package dispatchjob

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// claim.go is the scheduler's READ of the job table: which PENDING jobs are due.
// It writes nothing. A claimed job stays PENDING in the table and is kept out of the
// next claim by the caller's in-memory in-flight set (passed in as $4).

// ClaimedJob is one PENDING job a claim returned: everything the scheduler needs to
// publish it without reading the job again.
type ClaimedJob struct {
	ID             string
	CreatedAt      time.Time
	MessageGroup   *string
	Sequence       int32
	ScheduledFor   *time.Time
	SubscriptionID *string
	DispatchPoolID *string
	ClientID       *string
	Mode           string
	Queue          *string
	// UpdatedAt is the job's version at the claim; mark-QUEUED is optimistic on it.
	UpdatedAt time.Time
}

// ClaimPendingSQL is the claim ($1 = limit, $2 = paused subscription ids, $3 =
// groups remembered as held, $4 = the caller's in-flight ids). One plain SELECT: no
// lock, no transaction, no write. It walks idx_dispatch_jobs_status_group in order
// (the status equality prefix, then the index's own order — a Merge Append across
// the partitions) and stops at the limit: no sort, whatever the statistics say. The
// status is a literal; under force_custom_plan a bind gives the same plan. No array
// may be NULL (`<> ALL(NULL)` is NULL).
const ClaimPendingSQL = `SELECT id, created_at, message_group, sequence, scheduled_for, subscription_id,
       dispatch_pool_id, client_id, mode, queue, updated_at
  FROM msg_dispatch_jobs
 WHERE status = 'PENDING'
   AND (scheduled_for IS NULL OR scheduled_for <= NOW())
   AND (subscription_id IS NULL OR subscription_id <> ALL($2::text[]))
   AND (message_group IS NULL OR message_group <> ALL($3::text[]))
   AND id <> ALL($4::text[])
 ORDER BY message_group NULLS LAST, sequence, created_at, id
 LIMIT $1`

// ClaimPendingStatement is the claim's SQL and arguments, for plan tests.
func ClaimPendingStatement(limit int, paused, held, inFlight []string) Statement {
	return Statement{"claim", ClaimPendingSQL, []any{limit, nn(paused), nn(held), nn(inFlight)}}
}

func nn(a []string) []string {
	if a == nil {
		return []string{}
	}
	return a
}

// ClaimPending returns up to limit due PENDING jobs in delivery order — message_group
// (NULLs last), sequence, created_at, id — outside the paused subscriptions, the held
// groups and the in-flight ids. RETURNING-free: nothing is written.
func (l *Lifecycle) ClaimPending(ctx context.Context, limit int, paused, held, inFlight []string) ([]ClaimedJob, error) {
	rows, err := l.ex.Query(ctx, ClaimPendingSQL, limit, nn(paused), nn(held), nn(inFlight))
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ClaimedJob, error) {
		var c ClaimedJob
		err := r.Scan(&c.ID, &c.CreatedAt, &c.MessageGroup, &c.Sequence, &c.ScheduledFor,
			&c.SubscriptionID, &c.DispatchPoolID, &c.ClientID, &c.Mode, &c.Queue, &c.UpdatedAt)
		return c, err
	})
	if err != nil {
		return nil, err
	}
	// The database orders by its collation; within a group the (sequence, created_at,
	// id) order is what matters and is collation-independent for TSIDs. Re-sorting
	// makes the order exact for the caller.
	slices.SortStableFunc(out, compareClaimed)
	return out, nil
}

func compareClaimed(a, b ClaimedJob) int {
	switch {
	case a.MessageGroup == nil && b.MessageGroup != nil:
		return 1
	case a.MessageGroup != nil && b.MessageGroup == nil:
		return -1
	case a.MessageGroup != nil && b.MessageGroup != nil:
		if c := strings.Compare(*a.MessageGroup, *b.MessageGroup); c != 0 {
			return c
		}
	}
	if a.Sequence != b.Sequence {
		return cmp.Compare(a.Sequence, b.Sequence)
	}
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

// PendingBacklogCap is where the backlog count saturates: "100,000+".
const PendingBacklogCap = 100_000

// PendingBacklogSQL counts PENDING jobs up to a cap, so a huge backlog is never
// scanned in full (an index range of at most cap+1 entries of the status prefix).
const PendingBacklogSQL = `SELECT count(*) FROM (SELECT 1 FROM msg_dispatch_jobs WHERE status = 'PENDING' LIMIT 100001) s`

// OldestWaitingSQL is the first DUE PENDING job in claim order: its created_at is the
// "oldest waiting" the gauge reports.
const OldestWaitingSQL = `SELECT created_at FROM msg_dispatch_jobs
 WHERE status = 'PENDING' AND (scheduled_for IS NULL OR scheduled_for <= NOW())
 ORDER BY message_group NULLS LAST, sequence, created_at, id
 LIMIT 1`

// PendingBacklog returns the number of PENDING jobs (saturating at PendingBacklogCap+1)
// and the age of the first due one in claim order (zero when there is none).
func (l *Lifecycle) PendingBacklog(ctx context.Context) (count int64, oldest time.Duration, err error) {
	rows, err := l.ex.Query(ctx, PendingBacklogSQL)
	if err != nil {
		return 0, 0, err
	}
	if count, err = pgx.CollectOneRow(rows, pgx.RowTo[int64]); err != nil {
		return 0, 0, err
	}
	rows, err = l.ex.Query(ctx, OldestWaitingSQL)
	if err != nil {
		return 0, 0, err
	}
	created, err := pgx.CollectRows(rows, pgx.RowTo[time.Time])
	if err != nil {
		return 0, 0, err
	}
	if len(created) > 0 {
		oldest = time.Since(created[0])
	}
	return count, oldest, nil
}
