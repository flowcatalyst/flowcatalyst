package dispatchjob

// A job HOLDS its message group when it has not got through and is not about to
// on its own:
//
//   - FAILED / ERROR — terminal failure, held until an operator resolves it.
//     ('ERROR' is a legacy value that predates the current status set; it is
//     matched so old rows keep blocking as they always did.)
//   - PENDING with a future scheduled_for — sitting out a retry backoff. This
//     one is easy to miss, because such a job looks idle from every angle: it
//     is excluded from the claim by its own scheduled_for and it is not
//     FAILED, so nothing used to treat it as holding anything. Its successors
//     were therefore claimed and delivered while it waited, which is exactly
//     the reordering BLOCK_ON_ERROR exists to prevent. A backed-off job still
//     owns its place at the front of the group.
//
// Deliberately NOT holding: QUEUED and PROCESSING. Those are the normal flow —
// the poller hands a group's whole eligible run to the router in one batch and
// the router's per-group FIFO delivers them in order. Treating them as holders
// would reduce every ordered group to one job per poll cycle.
//
// The two kinds live in different tables, so the lookup has two halves:
// FAILED/ERROR holders are read from msg_dispatch_jobs by (status, message_group)
// — idx_dispatch_jobs_status_group, migration 067 — and PENDING-with-future-
// scheduled_for holders from msg_dispatch_queue by message_group (the order
// index starts with it). A NULL message_group never matches `= ANY` / `=`, so a
// failed ungrouped job holds nothing.
//
// Used by the scheduler's claim-time hold-back (GroupHoldersSQL) and by the
// delivery-time check in the processing endpoint (GroupHeldBeforeSQL); both
// must agree, or a job held at one gate and waved through at the other would
// loop. Status values are a bind parameter (GroupHoldingStatuses) and nothing
// about either statement depends on the planner proving a literal.

// GroupHoldingStatuses are the terminal-failure statuses that hold a group.
var GroupHoldingStatuses = []string{"FAILED", "ERROR"}

// GroupHoldersSQL returns, per candidate group, the EARLIEST holder as
// (message_group, sequence, created_at, id). $1 = GroupHoldingStatuses, $2 =
// candidate groups; $3..$5 are, per group (same order as $2), the position
// (sequence, created_at, id) of the group's LAST candidate in the claim. DISTINCT ON keeps the earliest: anything behind it is held
// by it too. The queue half is one ordered index probe per candidate group (the
// first backed-off row in the group's order), so its cost does not grow with the
// depth of the queue, and bounded by the group's last candidate: only a holder
// positioned before a candidate can hold it, and the claim has just deleted the
// candidates' own rows, so the probe reads only the rows ahead of the claim (not
// all of a deep group's due rows looking for a future-scheduled one).
const GroupHoldersSQL = `SELECT DISTINCT ON (message_group) message_group, sequence, created_at, id FROM (
    SELECT message_group, sequence, created_at, id
      FROM msg_dispatch_jobs
     WHERE status = ANY($1::text[]) AND message_group = ANY($2::text[])
    UNION ALL
    SELECT h.message_group, h.sequence, h.job_created_at, h.job_id
      FROM unnest($2::text[], $3::int[], $4::timestamptz[], $5::text[]) AS g(grp, seq, ca, jid)
     CROSS JOIN LATERAL (
          SELECT message_group, sequence, job_created_at, job_id
            FROM msg_dispatch_queue
           WHERE message_group = g.grp AND scheduled_for > NOW()
             AND (sequence, job_created_at, job_id) < (g.seq, g.ca, g.jid)
           ORDER BY sequence, job_created_at, job_id
           LIMIT 1) h
) h2
ORDER BY message_group, sequence, created_at, id`

// GroupHeldBeforeSQL reports whether an EARLIER job of one group holds it up.
// $1 = GroupHoldingStatuses, $2 = group, $3 = sequence, $4 = created_at, $5 = id
// of the job asking. "Earlier" is positional — (sequence, created_at, id) — not
// mere membership: asking whether the group contains a held job would also catch
// the held job itself the moment it became deliverable again, and the group would
// never move.
const GroupHeldBeforeSQL = `SELECT EXISTS (
    SELECT 1 FROM msg_dispatch_jobs
     WHERE status = ANY($1::text[]) AND message_group = $2
       AND (sequence, created_at, id) < ($3::int, $4::timestamptz, $5::text))
 OR EXISTS (
    SELECT 1 FROM msg_dispatch_queue
     WHERE message_group = $2 AND scheduled_for > NOW()
       AND (sequence, job_created_at, job_id) < ($3::int, $4::timestamptz, $5::text))`
