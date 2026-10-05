package dispatchjob

// lifecycle.go is the ONE owner of msg_dispatch_jobs.status.
//
// Every production INSERT into the table and every write of its status
// column lives in this file (and is policed by lifecycle_enforce_test.go).
// Fan-out, API ingest, the delivery callback, the scheduler, stale recovery,
// the reaper, the settled hook and the operator actions all call it.
//
// Inside it exactly two private primitives decide whether a job is PENDING:
//
//   - enterPending: the single place a job becomes (or is refreshed as)
//     PENDING — create, retry, deferral, hold, settled-return, reaper sweep,
//     stale recovery, operator requeue.
//   - leavePending: the single place a job stops being PENDING — the
//     scheduler's mark-QUEUED, the callback's claim-for-delivery, and the
//     callback's terminal outcomes (which may find the job still PENDING).
//
// Both are thin faces of one statement builder (transition): ONE UPDATE per
// call, ALWAYS naming the statuses it may move FROM in its WHERE, ALWAYS
// stamping updated_at, ALWAYS RETURNING the columns the caller needs. Transitions that don't touch PENDING on either side (the
// PROCESSING lease takeover, operator cancel/complete from FAILED) go through
// the same builder directly (passThrough) so the status column still has one
// owner.
//
// A transition that matches no row is not an error. It is counted
// (fc_dispatch_job_transition_refused_total{transition}) and logged at debug
// with the status the job was found in, so a late callback finding a settled
// job is visible rather than silent.
//
// Every statement touches msg_dispatch_jobs ONLY. (A queue table that mirrored the
// PENDING jobs was tried — migrations 066/067 — and retired by 068: a small table
// that swings between empty and very full is the planner's worst case, and every
// statement that joined to it was planned as a scan when its statistics said it
// was empty.) The scheduler claims PENDING jobs with a plain read and keeps its own
// claimed jobs out of the next claim with an in-memory set.
//
// The lifecycle accepts whatever executor the caller owns: the platform pool,
// the scheduler's own pool, or a transaction (fan-out and the operator use
// cases run inside one with other writes). It adds and removes no transaction.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Executor is what the lifecycle runs its statements on: satisfied by
// *pgxpool.Pool and by pgx.Tx.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// Lifecycle is the sole writer of msg_dispatch_jobs. Cheap to construct; bind
// it to another executor (typically a transaction) with In.
type Lifecycle struct {
	ex Executor
}

// NewLifecycle binds a lifecycle to an executor (pool or transaction).
func NewLifecycle(ex Executor) *Lifecycle { return &Lifecycle{ex: ex} }

// In returns the same lifecycle bound to another executor — the way a caller
// that already holds a transaction passes it in.
func (l *Lifecycle) In(ex Executor) *Lifecycle { return &Lifecycle{ex: ex} }

// JobRow is what every transition RETURNs: the columns a caller needs to
// identify and order the job. Status is the status the row now has.
type JobRow struct {
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
	UpdatedAt      time.Time
	Status         string
}

const returningCols = ` RETURNING j.id, j.created_at, j.message_group, j.sequence, j.scheduled_for,
	j.subscription_id, j.dispatch_pool_id, j.client_id, j.mode, j.queue, j.updated_at, j.status`

// insertReturningCols is returningCols for an INSERT, which has no alias.
const insertReturningCols = ` RETURNING id, created_at, message_group, sequence, scheduled_for,
	subscription_id, dispatch_pool_id, client_id, mode, queue, updated_at, status`

func scanJobRow(row pgx.CollectableRow) (JobRow, error) {
	var r JobRow
	err := row.Scan(&r.ID, &r.CreatedAt, &r.MessageGroup, &r.Sequence, &r.ScheduledFor,
		&r.SubscriptionID, &r.DispatchPoolID, &r.ClientID, &r.Mode, &r.Queue, &r.UpdatedAt, &r.Status)
	return r, err
}

// ── the transition table ────────────────────────────────────────────────

// Status groups. The status values are literals in the statement text (they are
// constants of the statement, not inputs): the planner then sees the value, and
// the plain (status, ...) index of migration 067 is chosen on its own merits
// whatever the table statistics say.
var (
	// LiveStatuses are the statuses a job is still in the machine in. IN_PROGRESS
	// is the legacy alias of PROCESSING (see common.ParseDispatchStatus).
	liveStatuses = []common.DispatchStatus{common.DispatchPending, common.DispatchQueued, common.DispatchProcessing, "IN_PROGRESS"}
	// SettledStatuses (documentation + tests): no callback may move these.
	// ERROR is the legacy alias of FAILED.
	SettledStatuses = []common.DispatchStatus{common.DispatchCompleted, common.DispatchFailed, common.DispatchCancelled, common.DispatchExpired, "ERROR"}
)

// Transition names a lifecycle transition: its metric label, the statuses it
// may move FROM (nil = any), and the status it moves TO. The wrappers below
// build their SQL from these, so this table is the single statement of the
// lifecycle (lifecycle_table_pg_test.go verifies it against a real database).
type Transition struct {
	Name string
	From []common.DispatchStatus
	To   common.DispatchStatus
}

// The transitions. A wrapper below exists for each.
var (
	TCreate            = Transition{"create", nil, common.DispatchPending}
	TRetry             = Transition{"retry", liveStatuses, common.DispatchPending}
	TDefer             = Transition{"defer", liveStatuses, common.DispatchPending}
	THold              = Transition{"hold", liveStatuses, common.DispatchPending}
	TSettleAcked       = Transition{"settle_acked", []common.DispatchStatus{common.DispatchQueued, common.DispatchProcessing}, common.DispatchPending}
	TSweepStranded     = Transition{"sweep_stranded", []common.DispatchStatus{common.DispatchQueued, common.DispatchProcessing}, common.DispatchPending}
	TStaleQueued       = Transition{"stale_queued", []common.DispatchStatus{common.DispatchQueued}, common.DispatchPending}
	TStaleProcessing   = Transition{"stale_processing", []common.DispatchStatus{common.DispatchProcessing}, common.DispatchPending}
	TRequeue           = Transition{"requeue", nil, common.DispatchPending}
	TMarkQueued        = Transition{"mark_queued", []common.DispatchStatus{common.DispatchPending}, common.DispatchQueued}
	TClaimForDelivery  = Transition{"claim_for_delivery", []common.DispatchStatus{common.DispatchPending, common.DispatchQueued}, common.DispatchProcessing}
	TReclaimStale      = Transition{"reclaim_stale_delivery", []common.DispatchStatus{common.DispatchProcessing}, common.DispatchProcessing}
	TComplete          = Transition{"complete", liveStatuses, common.DispatchCompleted}
	TFail              = Transition{"fail", liveStatuses, common.DispatchFailed}
	TOperatorCancel    = Transition{"operator_cancel", []common.DispatchStatus{common.DispatchFailed}, common.DispatchCancelled}
	TOperatorComplete  = Transition{"operator_complete", []common.DispatchStatus{common.DispatchFailed}, common.DispatchCompleted}
	allTransitionNames = []Transition{
		TCreate, TRetry, TDefer, THold, TSettleAcked, TSweepStranded, TStaleQueued,
		TStaleProcessing, TRequeue, TMarkQueued, TClaimForDelivery, TReclaimStale, TComplete, TFail,
		TOperatorCancel, TOperatorComplete,
	}
)

// Transitions lists every lifecycle transition (documentation and tests).
func Transitions() []Transition { return append([]Transition(nil), allTransitionNames...) }

// ── statement building ──────────────────────────────────────────────────

// changes accumulates the SET list beyond the status. Parameters are numbered
// from $1; the selector's parameters follow.
type changes struct {
	sets []string
	args []any
}

func (c *changes) set(col string, val any) *changes {
	c.args = append(c.args, val)
	c.sets = append(c.sets, fmt.Sprintf("%s = $%d", col, len(c.args)))
	return c
}

// raw sets a column from a parameter-free SQL expression.
func (c *changes) raw(col, expr string) *changes {
	c.sets = append(c.sets, col+" = "+expr)
	return c
}

func (c *changes) has(col string) bool {
	for _, s := range c.sets {
		if strings.HasPrefix(s, col+" = ") {
			return true
		}
	}
	return false
}

// selector says WHICH rows a transition applies to. Built once the change
// parameters are numbered (first = the next free $n).
type selector func(first int) selection

type selection struct {
	with  string // optional "WITH ... " prefix
	from  string // optional FROM clause of the UPDATE
	where string
	args  []any
	// expect is how many rows the caller expects the transition to move; 0 for
	// a sweep (matching nothing is its normal outcome). The shortfall is the
	// refusal count.
	expect int
	// opaqueStatus writes the status guard so the planner cannot use it as an index
	// condition (`j.status || '' = 'X'`). For a selector that already pins every row
	// by primary key: a sargable `status = 'PENDING'` lets the planner walk the
	// status index instead — and when the statistics say PENDING is absent (the
	// active partition analysed before a burst) it believes that costs one row, and
	// reads every PENDING job (6 s at 200,000).
	opaqueStatus bool
	// key, when the selector names one job, lets a refusal log the status the
	// job was found in.
	key *jobKey
}

type jobKey struct {
	id        string
	createdAt time.Time
}

// byKey selects one job by (id, created_at) — created_at lets the
// created_at-partitioned table prune to the row's own partition.
func byKey(id string, createdAt time.Time) selector {
	return func(n int) selection {
		return selection{
			where:        fmt.Sprintf("j.id = $%d AND j.created_at = $%d", n, n+1),
			args:         []any{id, createdAt},
			expect:       1,
			key:          &jobKey{id, createdAt},
			opaqueStatus: true,
		}
	}
}

// byIDs selects a list of jobs by id (no created_at is known: scans by id
// across partitions).
func byIDs(ids []string) selector {
	return func(n int) selection {
		return selection{
			where:        fmt.Sprintf("j.id = ANY($%d::text[])", n),
			args:         []any{ids},
			expect:       len(ids),
			opaqueStatus: true,
		}
	}
}

// byClaims selects the rows a scheduler claim read, optimistically: each (id,
// created_at) pair only while the job is still PENDING at the version (updated_at)
// the claim saw. The access path is the primary key for every pair whatever the
// statistics say: a LATERAL subquery (OFFSET 0 keeps the planner from flattening
// it into a join it might run as a scan) looks each pair up by (id, created_at) alone
// — a status test inside it lets the planner walk the status index instead — and the
// version and status are checked on what it returns; the UPDATE joins the few matches back by the same key.
func byClaims(ids []string, createdAts, updatedAts []time.Time) selector {
	return func(n int) selection {
		return selection{
			with: fmt.Sprintf(`WITH pk AS (
    SELECT p.id, p.created_at
      FROM unnest($%d::text[], $%d::timestamptz[], $%d::timestamptz[]) AS c(id, created_at, v)
     CROSS JOIN LATERAL (
          SELECT id, created_at, updated_at, status FROM msg_dispatch_jobs
           WHERE id = c.id AND created_at = c.created_at
           OFFSET 0) p
     WHERE p.updated_at = c.v AND p.status = 'PENDING'
) `, n, n+1, n+2),
			from:         "pk",
			where:        "j.id = pk.id AND j.created_at = pk.created_at",
			args:         []any{ids, createdAts, updatedAts},
			expect:       len(ids),
			opaqueStatus: true,
		}
	}
}

// staleBefore selects the jobs whose updated_at is older than cutoff (a
// predicate sweep; the status guard is the transition's own FROM).
func staleBefore(cutoff time.Time) selector {
	return func(n int) selection {
		return selection{where: fmt.Sprintf("j.updated_at < $%d", n), args: []any{cutoff}}
	}
}

// strandedSiblings selects QUEUED/PROCESSING members of a BLOCK_ON_ERROR
// message group whose head is terminally failed (the reaper's sweep). A
// PROCESSING row updated more recently than liveBefore is presumed to be a
// genuine in-flight delivery and left alone; QUEUED rows have no such window.
func strandedSiblings(liveBefore time.Time) selector {
	return func(n int) selection {
		return selection{
			with: fmt.Sprintf(`WITH stranded AS (
    SELECT s.id, s.created_at
      FROM msg_dispatch_jobs s
      JOIN msg_dispatch_jobs h
        ON h.message_group = s.message_group
       -- Both terminal-failure statuses, matching GroupHoldersSQL
       -- exactly. 'ERROR' is the legacy value that predates the current status
       -- set; the poller still treats it as holding its group, so a sweep that
       -- recognised only 'FAILED' would leave siblings behind an ERROR head held
       -- at claim time but never reset here.
       AND h.status IN ('FAILED', 'ERROR')
       AND (h.sequence, h.created_at, h.id) < (s.sequence, s.created_at, s.id)
     WHERE s.mode = 'BLOCK_ON_ERROR'
       AND s.message_group IS NOT NULL
       AND s.status IN ('QUEUED', 'PROCESSING')
       AND (s.status <> 'PROCESSING' OR s.updated_at < $%d::timestamptz)
) `, n),
			from:  "stranded st",
			where: "j.id = st.id AND j.created_at = st.created_at",
			args:  []any{liveBefore},
		}
	}
}

func statusList(ss []common.DispatchStatus) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = "'" + string(s) + "'"
	}
	return strings.Join(q, ", ")
}

// buildTransition renders the one UPDATE a transition issues.
func buildTransition(t Transition, sel selector, ch *changes) (string, []any, selection) {
	if ch == nil {
		ch = &changes{}
	}
	// Every transition stamps updated_at: the database clock unless the caller
	// supplied the value it already uses for the same instant.
	sets := append([]string{"status = '" + string(t.To) + "'"}, ch.sets...)
	if !ch.has("updated_at") {
		sets = append(sets, "updated_at = NOW()")
	}
	s := sel(len(ch.args) + 1)
	var b strings.Builder
	b.WriteString(s.with)
	b.WriteString("UPDATE msg_dispatch_jobs j SET ")
	b.WriteString(strings.Join(sets, ", "))
	if s.from != "" {
		b.WriteString(" FROM " + s.from)
	}
	b.WriteString(" WHERE " + s.where)
	switch len(t.From) {
	case 0:
	case 1:
		if s.opaqueStatus {
			b.WriteString(" AND j.status || '' = '" + string(t.From[0]) + "'")
		} else {
			b.WriteString(" AND j.status = '" + string(t.From[0]) + "'")
		}
	default:
		if s.opaqueStatus {
			b.WriteString(" AND j.status || '' IN (" + statusList(t.From) + ")")
		} else {
			b.WriteString(" AND j.status IN (" + statusList(t.From) + ")")
		}
	}
	b.WriteString(returningCols)
	return b.String(), append(append([]any(nil), ch.args...), s.args...), s
}

// Statement is one transition's rendered SQL, for plan tests.
type Statement struct {
	Name string
	SQL  string
	Args []any
}

// StaleRecoveryStatements renders the two stale-recovery statements for a
// cutoff, so the scheduler's plan test can EXPLAIN exactly what runs.
func StaleRecoveryStatements(cutoff time.Time) []Statement {
	q, qa, _ := buildTransition(TStaleQueued, staleBefore(cutoff), staleQueuedChanges())
	p, pa, _ := buildTransition(TStaleProcessing, staleBefore(cutoff), staleProcessingChanges(StaleProcessingReason))
	return []Statement{{TStaleQueued.Name, q, qa}, {TStaleProcessing.Name, p, pa}}
}

// StrandedStatement renders the reaper's sweep statement, for plan tests.
func StrandedStatement(liveBefore time.Time) Statement {
	q, qa, _ := buildTransition(TSweepStranded, strandedSiblings(liveBefore), new(changes).raw("scheduled_for", "NULL").set("last_error", "reaper"))
	return Statement{TSweepStranded.Name, q, qa}
}

// MarkQueuedStatement renders mark-QUEUED for the given claimed pairs, for plan tests.
func MarkQueuedStatement(ids []string, createdAts, updatedAts []time.Time) Statement {
	q, qa, _ := buildTransition(TMarkQueued, byClaims(ids, createdAts, updatedAts), new(changes).raw("queued_at", "NOW()"))
	return Statement{TMarkQueued.Name, q, qa}
}

// run executes a built transition and does the refusal accounting.
func (l *Lifecycle) run(ctx context.Context, t Transition, sel selector, ch *changes) ([]JobRow, error) {
	sql, args, s := buildTransition(t, sel, ch)
	rows, err := l.ex.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, scanJobRow)
	if err != nil {
		return nil, err
	}
	l.countRefused(ctx, t, s, len(out))
	return out, nil
}

func (l *Lifecycle) countRefused(ctx context.Context, t Transition, s selection, moved int) {
	if s.expect <= moved {
		return
	}
	refused := s.expect - moved
	transitionRefused.WithLabelValues(t.Name).Add(float64(refused))
	if s.key != nil && slog.Default().Enabled(ctx, slog.LevelDebug) {
		var found *string
		rows, err := l.ex.Query(ctx, `SELECT status FROM msg_dispatch_jobs WHERE id = $1 AND created_at = $2`, s.key.id, s.key.createdAt)
		if err == nil {
			if st, err := pgx.CollectOneRow(rows, pgx.RowTo[string]); err == nil {
				found = &st
			}
		}
		slog.Debug("dispatch job transition refused", "transition", t.Name, "id", s.key.id, "found_status", found)
	}
}

// enterPending is THE place a job becomes (or is refreshed as) PENDING.
func (l *Lifecycle) enterPending(ctx context.Context, t Transition, sel selector, ch *changes) ([]JobRow, error) {
	if t.To != common.DispatchPending {
		panic("dispatchjob: enterPending used for transition " + t.Name + " that does not end PENDING")
	}
	return l.run(ctx, t, sel, ch)
}

// leavePending is THE place a job stops being PENDING.
func (l *Lifecycle) leavePending(ctx context.Context, t Transition, sel selector, ch *changes) ([]JobRow, error) {
	leaves := false
	for _, s := range t.From {
		if s == common.DispatchPending {
			leaves = true
		}
	}
	if !leaves || t.To == common.DispatchPending {
		panic("dispatchjob: leavePending used for transition " + t.Name + " that does not leave PENDING")
	}
	return l.run(ctx, t, sel, ch)
}

// passThrough is for transitions that involve PENDING on neither side.
func (l *Lifecycle) passThrough(ctx context.Context, t Transition, sel selector, ch *changes) ([]JobRow, error) {
	for _, s := range t.From {
		if s == common.DispatchPending {
			panic("dispatchjob: passThrough used for transition " + t.Name + " that involves PENDING")
		}
	}
	if t.To == common.DispatchPending {
		panic("dispatchjob: passThrough used for transition " + t.Name + " that ends PENDING")
	}
	return l.run(ctx, t, sel, ch)
}

// ── creation (enters PENDING) ───────────────────────────────────────────

type insCol struct {
	name string
	// expr: "" = the next positional parameter; an expression containing %d =
	// a parameter-bearing expression (%d becomes the parameter number); any
	// other expression is a literal that consumes no parameter.
	expr string
}

// insertPending is THE place a job is inserted: always as PENDING, one
// statement per job queued on one batch, ON CONFLICT (id, created_at) DO
// NOTHING (the composite PK partitioning introduced), RETURNING the inserted
// rows (a conflicting id returns none).
func (l *Lifecycle) insertPending(ctx context.Context, t Transition, cols []insCol, rows [][]any) ([]JobRow, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	names := make([]string, len(cols))
	vals := make([]string, len(cols))
	p := 0
	for i, c := range cols {
		names[i] = c.name
		switch {
		case c.name == "status":
			vals[i] = "'PENDING'"
		case c.expr == "":
			p++
			vals[i] = fmt.Sprintf("$%d", p)
		case !strings.Contains(c.expr, "%d"):
			vals[i] = c.expr // a literal: consumes no parameter
		default:
			p++
			vals[i] = fmt.Sprintf(c.expr, p)
		}
	}
	sql := "INSERT INTO msg_dispatch_jobs (" + strings.Join(names, ", ") + ")\n VALUES (" +
		strings.Join(vals, ", ") + ")\n ON CONFLICT (id, created_at) DO NOTHING" + insertReturningCols
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(sql, r...)
	}
	br := l.ex.SendBatch(ctx, batch)
	defer br.Close()
	var out []JobRow
	for range rows {
		rs, err := br.Query()
		if err != nil {
			return nil, err
		}
		got, err := pgx.CollectRows(rs, scanJobRow)
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
	}
	l.countRefused(ctx, t, selection{expect: len(rows)}, len(out))
	return out, nil
}

// CreateBatch inserts full-envelope jobs as PENDING (API ingest). The status
// on the entities is ignored: a job is born PENDING.
func (l *Lifecycle) CreateBatch(ctx context.Context, jobs []DispatchJob) ([]JobRow, error) {
	now := time.Now().UTC()
	rows := make([][]any, 0, len(jobs))
	for _, j := range jobs {
		if j.CreatedAt.IsZero() {
			j.CreatedAt = now
		}
		rows = append(rows, []any{
			j.ID, j.ExternalID, j.Source, string(j.Kind), j.Code, j.Subject, j.EventID,
			j.CorrelationID, metadataJSON(j.Metadata), j.TargetURL, string(j.Protocol), j.Payload,
			j.PayloadContentType, j.DataOnly, j.ServiceAccountID, j.ClientID,
			j.SubscriptionID, string(j.Mode), j.DispatchPoolID, j.MessageGroup,
			j.Sequence, j.TimeoutSeconds, j.SchemaID, j.MaxRetries,
			string(j.RetryStrategy), j.ScheduledFor, j.ExpiresAt, j.AttemptCount,
			j.LastAttemptAt, j.CompletedAt, j.DurationMillis, j.LastError,
			j.IdempotencyKey, j.Queue, j.Descriptor, j.CreatedAt, now,
		})
	}
	return l.insertPending(ctx, TCreate, createBatchCols, rows)
}

var createBatchCols = []insCol{
	{"id", ""},
	{"external_id", ""},
	{"source", ""},
	{"kind", ""},
	{"code", ""},
	{"subject", ""},
	{"event_id", ""},
	{"correlation_id", ""},
	{"metadata", "$%d::jsonb"},
	{"target_url", ""},
	{"protocol", ""},
	{"payload", ""},
	{"payload_content_type", ""},
	{"data_only", ""},
	{"service_account_id", ""},
	{"client_id", ""},
	{"subscription_id", ""},
	{"mode", ""},
	{"dispatch_pool_id", ""},
	{"message_group", ""},
	{"sequence", ""},
	{"timeout_seconds", ""},
	{"schema_id", ""},
	{"status", ""},
	{"max_retries", ""},
	{"retry_strategy", ""},
	{"scheduled_for", ""},
	{"expires_at", ""},
	{"attempt_count", ""},
	{"last_attempt_at", ""},
	{"completed_at", ""},
	{"duration_millis", ""},
	{"last_error", ""},
	{"idempotency_key", ""},
	{"queue", ""},
	{"descriptor", ""},
	{"created_at", ""},
	{"updated_at", ""},
}

// FanOutJob is the subset of columns the stream fan-out sets; everything else
// takes the table default (kind='EVENT', retry_strategy='exponential', ...).
type FanOutJob struct {
	ID             string
	Code           string
	Source         string
	Subject        *string
	EventID        string
	CorrelationID  *string
	TargetURL      string
	Payload        string
	DataOnly       bool
	ServiceAcctID  *string
	ClientID       *string
	SubscriptionID string
	Mode           string
	DispatchPoolID *string
	MessageGroup   *string
	Sequence       int32
	TimeoutSeconds int32
	MaxRetries     int32
	IdempotencyKey string
	CreatedAt      time.Time
	// Descriptor is the raising subscription's name; Metadata the raising
	// event's context_data, verbatim. nil when absent.
	Descriptor *string
	Metadata   json.RawMessage
	// Queue is the raising subscription's queue value, copied verbatim — nil
	// when the subscription has none set.
	Queue *string
}

// CreateFanOut inserts the fan-out's jobs as PENDING. Run it on the
// transaction that stamps fanned_out_at (l.In(tx)).
func (l *Lifecycle) CreateFanOut(ctx context.Context, jobs []FanOutJob) ([]JobRow, error) {
	rows := make([][]any, 0, len(jobs))
	for _, j := range jobs {
		var meta *string
		if len(j.Metadata) > 0 {
			s := string(j.Metadata)
			meta = &s
		}
		rows = append(rows, []any{
			j.ID, j.Code, j.Source, j.Subject, j.EventID, j.CorrelationID,
			j.TargetURL, j.Payload, j.DataOnly, j.ServiceAcctID,
			j.ClientID, j.SubscriptionID, j.Mode, j.DispatchPoolID,
			j.MessageGroup, j.Sequence, j.TimeoutSeconds,
			j.MaxRetries, j.IdempotencyKey, j.Queue, j.Descriptor,
			meta, j.CreatedAt, j.CreatedAt,
		})
	}
	return l.insertPending(ctx, TCreate, fanOutCols, rows)
}

var fanOutCols = []insCol{
	{"id", ""},
	{"code", ""},
	{"source", ""},
	{"subject", ""},
	{"event_id", ""},
	{"correlation_id", ""},
	{"target_url", ""},
	{"protocol", "'HTTP_WEBHOOK'"},
	{"payload", ""},
	{"data_only", ""},
	{"service_account_id", ""},
	{"client_id", ""},
	{"subscription_id", ""},
	{"mode", ""},
	{"dispatch_pool_id", ""},
	{"message_group", ""},
	{"sequence", ""},
	{"timeout_seconds", ""},
	{"status", ""},
	{"max_retries", ""},
	{"idempotency_key", ""},
	{"queue", ""},
	{"descriptor", ""},
	{"metadata", "COALESCE($%d::jsonb, '[]'::jsonb)"},
	{"created_at", ""},
	{"updated_at", ""},
}

// ── the delivery callback's outcomes ────────────────────────────────────

// ClaimForDelivery atomically claims a job for ONE delivery attempt (PENDING
// or QUEUED -> PROCESSING). Reports whether this caller won: false means a
// delivery of the job is already in flight, or the job is settled.
func (l *Lifecycle) ClaimForDelivery(ctx context.Context, id string, createdAt time.Time) (bool, error) {
	now := time.Now().UTC()
	rows, err := l.leavePending(ctx, TClaimForDelivery, byKey(id, createdAt),
		new(changes).set("last_attempt_at", now).set("updated_at", now))
	return len(rows) == 1, err
}

// ReclaimStaleDelivery takes over a delivery whose attempt died with its
// process (PROCESSING -> PROCESSING with a fresh claim time), only when the
// current claim was made before claimedBefore.
func (l *Lifecycle) ReclaimStaleDelivery(ctx context.Context, id string, createdAt, claimedBefore time.Time) (bool, error) {
	now := time.Now().UTC()
	claimedBefore = claimedBefore.UTC()
	rows, err := l.passThrough(ctx, TReclaimStale, func(n int) selection {
		s := byKey(id, createdAt)(n)
		s.where += fmt.Sprintf(" AND COALESCE(j.last_attempt_at, j.updated_at) < $%d", n+2)
		s.args = append(s.args, claimedBefore)
		return s
	}, new(changes).set("last_attempt_at", now).set("updated_at", now))
	return len(rows) == 1, err
}

// Complete records a successful delivery: -> COMPLETED, stamping completed_at
// and the end-to-end duration. Only from a live status: a settled job is never
// overwritten by a late callback.
func (l *Lifecycle) Complete(ctx context.Context, id string, createdAt time.Time, durationMillis int64) (bool, error) {
	now := time.Now().UTC()
	rows, err := l.leavePending(ctx, TComplete, byKey(id, createdAt),
		new(changes).set("completed_at", now).set("duration_millis", durationMillis).set("updated_at", now))
	return len(rows) == 1, err
}

// Fail records a terminal failure: -> FAILED, stamping last_error,
// completed_at and the duration. Only from a live status.
func (l *Lifecycle) Fail(ctx context.Context, id string, createdAt time.Time, lastError *string, durationMillis int64) (bool, error) {
	now := time.Now().UTC()
	rows, err := l.leavePending(ctx, TFail, byKey(id, createdAt),
		new(changes).set("completed_at", now).set("duration_millis", durationMillis).
			set("last_error", lastError).set("updated_at", now))
	return len(rows) == 1, err
}

// Retry bumps attempt_count, stamps last_error and scheduled_for and puts the
// job back to PENDING for the poller to pick up once due. Only from a live
// status.
func (l *Lifecycle) Retry(ctx context.Context, id string, createdAt, scheduledFor time.Time, lastError *string) (bool, error) {
	rows, err := l.enterPending(ctx, TRetry, byKey(id, createdAt),
		new(changes).raw("attempt_count", "j.attempt_count + 1").set("scheduled_for", scheduledFor).
			set("last_error", lastError).raw("last_attempt_at", "NOW()"))
	return len(rows) == 1, err
}

// Defer puts a job back to PENDING with a future scheduled_for WITHOUT
// bumping attempt_count: cooperative back-pressure (ack=false, HTTP 429) is a
// "try again later", not a delivery failure. Only from a live status.
func (l *Lifecycle) Defer(ctx context.Context, id string, createdAt, scheduledFor time.Time) (bool, error) {
	rows, err := l.enterPending(ctx, TDefer, byKey(id, createdAt),
		new(changes).set("scheduled_for", scheduledFor.UTC()))
	return len(rows) == 1, err
}

// Hold puts a job back to PENDING because an earlier job of its
// BLOCK_ON_ERROR group is holding the group. Same effect as Defer, its own
// transition so a refusal is attributable.
func (l *Lifecycle) Hold(ctx context.Context, id string, createdAt, scheduledFor time.Time) (bool, error) {
	rows, err := l.enterPending(ctx, THold, byKey(id, createdAt),
		new(changes).set("scheduled_for", scheduledFor.UTC()))
	return len(rows) == 1, err
}

// ── the scheduler ───────────────────────────────────────────────────────

// MarkQueued moves the published jobs PENDING -> QUEUED, optimistically on the
// row version (updated_at) the claim read. Every transition stamps
// updated_at, so any move at all changes the version; a job that was
// delivered, processed and rescheduled before this ran is left alone rather
// than stranded QUEUED with no message on the queue. Returns rows moved.
func (l *Lifecycle) MarkQueued(ctx context.Context, ids []string, createdAts, updatedAts []time.Time) (int64, error) {
	rows, err := l.leavePending(ctx, TMarkQueued, byClaims(ids, createdAts, updatedAts),
		new(changes).raw("queued_at", "NOW()"))
	return int64(len(rows)), err
}

// ── recovery ────────────────────────────────────────────────────────────

// StaleProcessingReason is recorded in last_error on a PROCESSING job stale
// recovery returns to PENDING, so an operator can tell why it was
// re-dispatched.
const StaleProcessingReason = "stale recovery: PROCESSING with no outcome recorded; returned to PENDING"

func staleQueuedChanges() *changes { return new(changes) }

func staleProcessingChanges(reason string) *changes { return new(changes).set("last_error", reason) }

// RecoverStaleQueued returns QUEUED jobs not touched since cutoff to PENDING.
// Both sweeps read the status prefix of idx_dispatch_jobs_status_group (status,
// message_group, sequence, created_at, id) — migration 067 — and filter
// updated_at on the heap.
func (l *Lifecycle) RecoverStaleQueued(ctx context.Context, cutoff time.Time) (int64, error) {
	rows, err := l.enterPending(ctx, TStaleQueued, staleBefore(cutoff), staleQueuedChanges())
	return int64(len(rows)), err
}

// RecoverStaleProcessing returns PROCESSING jobs not touched since cutoff to
// PENDING, recording StaleProcessingReason.
func (l *Lifecycle) RecoverStaleProcessing(ctx context.Context, cutoff time.Time) (int64, error) {
	rows, err := l.enterPending(ctx, TStaleProcessing, staleBefore(cutoff), staleProcessingChanges(StaleProcessingReason))
	return int64(len(rows)), err
}

// SettleAcked is the router -> platform settled-message hook: resets the
// given ids to PENDING, recording reason in last_error, scoped to
// QUEUED/PROCESSING so a row a concurrent path already advanced is left alone
// (idempotent: a duplicate hook call is harmless, counted as refused). Returns
// the ids actually reset. No created_at is known (the router only has ids), so
// this scans by id across partitions.
func (l *Lifecycle) SettleAcked(ctx context.Context, ids []string, reason string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := l.enterPending(ctx, TSettleAcked, byIDs(ids),
		new(changes).raw("scheduled_for", "NULL").set("last_error", reason))
	return jobIDs(rows), err
}

// SweepStranded is the reaper backstop: resets to PENDING any QUEUED/PROCESSING
// job whose message group is headed by a terminally FAILED job under
// BLOCK_ON_ERROR (see strandedSiblings). Returns the ids reset.
func (l *Lifecycle) SweepStranded(ctx context.Context, liveBefore time.Time, reason string) ([]string, error) {
	rows, err := l.enterPending(ctx, TSweepStranded, strandedSiblings(liveBefore),
		new(changes).raw("scheduled_for", "NULL").set("last_error", reason))
	return jobIDs(rows), err
}

func jobIDs(rows []JobRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

// ── operator actions ────────────────────────────────────────────────────

// Requeue is the operator's reset: from ANY status back to PENDING for a fresh
// delivery cycle — clears scheduled_for (immediate eligibility), zeroes
// attempt_count (a full retry budget again), clears the terminal stamps and
// last_error. Reports whether a row was reset.
func (l *Lifecycle) Requeue(ctx context.Context, id string, createdAt time.Time) (bool, error) {
	rows, err := l.enterPending(ctx, TRequeue, byKey(id, createdAt),
		new(changes).raw("scheduled_for", "NULL").raw("attempt_count", "0").
			raw("completed_at", "NULL").raw("duration_millis", "NULL").raw("last_error", "NULL"))
	return len(rows) == 1, err
}

// OperatorCancel moves a FAILED job to CANCELLED. The FAILED check is in the
// SQL: reports false (and changes nothing) when the job is in any other status.
func (l *Lifecycle) OperatorCancel(ctx context.Context, id string, createdAt time.Time) (bool, error) {
	now := time.Now().UTC()
	rows, err := l.passThrough(ctx, TOperatorCancel, byKey(id, createdAt),
		new(changes).set("completed_at", now).set("updated_at", now))
	return len(rows) == 1, err
}

// OperatorComplete moves a FAILED job to COMPLETED (handled out of band). Same
// guard as OperatorCancel.
func (l *Lifecycle) OperatorComplete(ctx context.Context, id string, createdAt time.Time) (bool, error) {
	now := time.Now().UTC()
	rows, err := l.passThrough(ctx, TOperatorComplete, byKey(id, createdAt),
		new(changes).set("completed_at", now).set("updated_at", now))
	return len(rows) == 1, err
}
