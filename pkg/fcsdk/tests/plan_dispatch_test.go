package tests

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outbox"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxpgx"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tests/internal/pgtest"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// ── fixtures ────────────────────────────────────────────────────────

type thing struct{ ID, Name string }

func (t thing) IDStr() string { return t.ID }

// thingRepo stores things; failPersist makes Persist fail, persisted counts
// calls.
type thingRepo struct {
	failPersist bool
	persisted   *int
}

func (r thingRepo) Persist(ctx context.Context, t *thing, tx *usecasepgx.DbTx) error {
	if r.persisted != nil {
		*r.persisted++
	}
	if r.failPersist {
		return errors.New("persist refused")
	}
	_, err := tx.Inner().Exec(ctx, `INSERT INTO things (id, name) VALUES ($1, $2)`, t.ID, t.Name)
	return err
}

func (r thingRepo) Delete(ctx context.Context, t *thing, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM things WHERE id = $1`, t.ID)
	return err
}

type thingSaved struct {
	Meta usecase.EventMetadata
	ID   string
}

func (e thingSaved) EventID() string             { return e.Meta.EventID }
func (e thingSaved) EventType() string           { return e.Meta.Type }
func (e thingSaved) SpecVersion() string         { return e.Meta.SpecVersion }
func (e thingSaved) Source() string              { return e.Meta.Source }
func (e thingSaved) Subject() string             { return e.Meta.Subject }
func (e thingSaved) Time() time.Time             { return e.Meta.OccurredAt }
func (e thingSaved) PrincipalID() string         { return e.Meta.PrincipalID }
func (e thingSaved) CorrelationID() string       { return e.Meta.CorrelationID }
func (e thingSaved) CausationID() string         { return e.Meta.CausationID }
func (e thingSaved) ExecutionID() string         { return e.Meta.ExecutionID }
func (e thingSaved) MessageGroup() string        { return e.Meta.MessageGroup }
func (e thingSaved) ToDataJSON() ([]byte, error) { return json.Marshal(map[string]string{"id": e.ID}) }

func savedEvent(ec usecase.ExecutionContext, id string) thingSaved {
	m := usecase.NewEventMetadata(ec, "shop:catalog:thing:saved", "shop:catalog", usecase.BuildSubject("catalog", "thing", id))
	m.MessageGroup = usecase.BuildMessageGroup("catalog", "thing", id)
	return thingSaved{Meta: m, ID: id}
}

type saveThing struct{ Name string }

func (saveThing) AuditOperation() string { return "save-thing" }

// saveOp is an operation whose plan saves a thing and carries jobs.
func saveOp(repo thingRepo, jobs ...outbox.DispatchJob) usecaseop.Operation[saveThing, thingSaved] {
	return usecaseop.Operation[saveThing, thingSaved]{
		Name:      "SaveThing",
		Authorize: usecaseop.Public[saveThing],
		Execute: func(_ context.Context, cmd saveThing, ec usecase.ExecutionContext) (usecaseop.Plan[thingSaved], error) {
			t := &thing{ID: tsid.GenerateWithPrefix("thg"), Name: cmd.Name}
			return usecaseop.Save(t, repo, savedEvent(ec, t.ID)).Dispatch(jobs...), nil
		},
	}
}

// failingAuditSink writes events and dispatch jobs like the outbox sink, then
// fails the audit row: everything before it must roll back.
type failingAuditSink struct{ *outboxpgx.Sink }

func (failingAuditSink) WriteAudit(context.Context, *usecasepgx.DbTx, usecase.DomainEvent, any) error {
	return errors.New("audit refused")
}

// eventOnlySink cannot write dispatch jobs (like the platform's own sink).
type eventOnlySink struct{ inner *outboxpgx.Sink }

func (s eventOnlySink) WriteEvent(ctx context.Context, tx *usecasepgx.DbTx, e usecase.DomainEvent) error {
	return s.inner.WriteEvent(ctx, tx, e)
}

func (s eventOnlySink) WriteAudit(ctx context.Context, tx *usecasepgx.DbTx, e usecase.DomainEvent, c any) error {
	return s.inner.WriteAudit(ctx, tx, e, c)
}

func setup(t *testing.T) (*pgxpool.Pool, *outboxpgx.Sink) {
	t.Helper()
	pool := pgtest.Schema(t)
	_, err := pool.Exec(context.Background(), `CREATE TABLE things (id VARCHAR(17) PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)
	return pool, outboxpgx.NewSink(outboxpgx.Config{ClientCode: "acme", ApplicationCode: "shop", AuditEnabled: true})
}

func countThings(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM things`).Scan(&n))
	return n
}

func types(rows []pgtest.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Type
	}
	return out
}

func payloadOf(t *testing.T, r pgtest.Row) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Payload), &m), r.Payload)
	return m
}

var tsidPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{13}$`)

// ── the plan carries dispatch jobs ──────────────────────────────────

// A committed plan writes the aggregate, the event, the job and the audit
// row, in that order (write-path 7a: after the events, before the audit row),
// in one transaction. The job takes the plan's correlation id.
func TestPlanDispatchCommitWritesTheJobRow(t *testing.T) {
	pool, sink := setup(t)
	ctx := context.Background()
	ec := usecase.NewExecutionContext("prn_1")
	ec.CausationID = "evt_cause"

	job := outbox.NewDispatchJob("shop:billing:export", "https://billing.example.com/x").
		WithID("job-commit").WithMessageGroup("billing:export").WithClientCode("acme")
	ev, err := usecaseop.Run(ctx, usecasepgx.New(pool, sink), saveOp(thingRepo{}, job), saveThing{Name: "a"}, ec)
	require.NoError(t, err)

	assert.Equal(t, 1, countThings(t, pool))
	rows := pgtest.OutboxRows(t, pool)
	require.Equal(t, []string{"EVENT", "DISPATCH_JOB", "AUDIT_LOG"}, types(rows))
	assert.Equal(t, ev.EventID(), rows[0].ID)

	j := rows[1]
	assert.Equal(t, "job-commit", j.ID)
	require.NotNil(t, j.MessageGroup)
	assert.Equal(t, "billing:export", *j.MessageGroup)
	assert.Equal(t, 0, j.Status)
	require.NotNil(t, j.PayloadSize)
	assert.Equal(t, len(j.Payload), *j.PayloadSize)
	assert.Nil(t, j.Headers)
	assert.JSONEq(t, `{
		"id": "job-commit", "kind": "TASK", "code": "shop:billing:export", "source": "shop",
		"targetUrl": "https://billing.example.com/x", "payload": "{}", "payloadContentType": "application/json",
		"dataOnly": false, "correlationId": "`+ec.CorrelationID+`", "clientCode": "acme", "messageGroup": "billing:export"
	}`, j.Payload, "correlation from the plan's event; causation never written")
}

// A failure after the job row was written (the audit row) rolls back the
// aggregate, the event and the job: nothing is left.
func TestPlanDispatchRollbackWritesNothing(t *testing.T) {
	pool, sink := setup(t)
	ctx := context.Background()
	job := outbox.NewDispatchJob("shop:billing:export", "https://billing.example.com/x")

	_, err := usecaseop.Run(ctx, usecasepgx.New(pool, failingAuditSink{sink}), saveOp(thingRepo{}, job),
		saveThing{Name: "a"}, usecase.NewExecutionContext("prn_1"))
	require.Error(t, err)
	assert.Equal(t, "AUDIT_WRITE", usecase.AsError(err).Code)
	assert.Equal(t, 0, countThings(t, pool))
	assert.Empty(t, pgtest.OutboxRows(t, pool))

	// And a repository failure, before the event, writes nothing either.
	_, err = usecaseop.Run(ctx, usecasepgx.New(pool, sink), saveOp(thingRepo{failPersist: true}, job),
		saveThing{Name: "a"}, usecase.NewExecutionContext("prn_1"))
	require.Error(t, err)
	assert.Equal(t, 0, countThings(t, pool))
	assert.Empty(t, pgtest.OutboxRows(t, pool))
}

// An invalid job fails the plan before anything is written (the repository
// is not even called), as INTERNAL DISPATCH_JOB_INVALID with the 7a text.
func TestPlanDispatchInvalidJobRefusedBeforeAnyWrite(t *testing.T) {
	pool, sink := setup(t)
	ctx := context.Background()
	for _, c := range []struct {
		job  outbox.DispatchJob
		text string
	}{
		{
			outbox.NewDispatchJob("shop:x", "ftp://a.example.com"),
			"dispatch job targetUrl must be an absolute http(s) URL with a host and no user info",
		},
		{
			outbox.NewDispatchJob("Shop:x", "https://a.example.com"),
			"dispatch job code must be {application}:{rest} (application [a-z][a-z0-9-]*), at most 255 characters: Shop:x",
		},
		{
			outbox.NewDispatchJob("shop:x", "https://a.example.com").WithSequence(0),
			"sequence must be at least 1 (the platform turns 0 into its default)",
		},
		{
			outbox.NewDispatchJob("shop:x", "https://a.example.com").WithPlatformClientID("0CLIENT").WithClientCode("acme"),
			"set platformClientId or clientCode, not both (the platform ignores the code when an id is given)",
		},
	} {
		persisted := 0
		good := outbox.NewDispatchJob("shop:ok", "https://a.example.com")
		_, err := usecaseop.Run(ctx, usecasepgx.New(pool, sink), saveOp(thingRepo{persisted: &persisted}, good, c.job),
			saveThing{Name: "a"}, usecase.NewExecutionContext("prn_1"))
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%v", err)
		assert.Equal(t, usecase.KindInternal, ue.Kind)
		assert.Equal(t, "DISPATCH_JOB_INVALID", ue.Code)
		assert.Equal(t, c.text, ue.Message, "the 7a error text, unchanged")
		assert.Equal(t, c.job.Validate().Error(), ue.Message)
		assert.Equal(t, 1, ue.Details["index"])
		assert.Equal(t, 0, persisted, "refused before the repository runs")
	}
	assert.Equal(t, 0, countThings(t, pool))
	assert.Empty(t, pgtest.OutboxRows(t, pool))
}

// Two jobs: written in the order the plan lists them, after the event and
// before the audit row; a job without an id gets a TSID; an explicit
// correlation id is kept.
func TestPlanDispatchTwoJobsAndEventOrdering(t *testing.T) {
	pool, sink := setup(t)
	ctx := context.Background()
	ec := usecase.NewExecutionContext("prn_1")
	first := outbox.NewDispatchJob("shop:a", "https://a.example.com/1").WithCorrelationID("corr-own")
	second, err := outbox.NewDispatchJob("shop:b", "https://b.example.com/2").WithID("job-b").
		WithJSONPayload(map[string]int{"n": 2})
	require.NoError(t, err)

	_, err = usecaseop.Run(ctx, usecasepgx.New(pool, sink), saveOp(thingRepo{}, first, second), saveThing{Name: "a"}, ec)
	require.NoError(t, err)
	rows := pgtest.OutboxRows(t, pool)
	require.Equal(t, []string{"EVENT", "DISPATCH_JOB", "DISPATCH_JOB", "AUDIT_LOG"}, types(rows))
	a, b := payloadOf(t, rows[1]), payloadOf(t, rows[2])
	assert.Equal(t, "shop:a", a["code"])
	assert.Regexp(t, tsidPattern, rows[1].ID)
	assert.Equal(t, rows[1].ID, a["id"], "the row id is the job id")
	assert.Equal(t, "corr-own", a["correlationId"], "an explicit correlation id is kept")
	assert.Equal(t, "shop:b", b["code"])
	assert.Equal(t, "job-b", rows[2].ID)
	assert.Equal(t, ec.CorrelationID, b["correlationId"])
	assert.Equal(t, `{"n":2}`, b["payload"])
	assert.False(t, rows[0].CreatedAt.After(rows[1].CreatedAt))
	assert.False(t, rows[2].CreatedAt.After(rows[3].CreatedAt))
}

// Sync: the per-row events (each with its audit row), the rollup event, the
// jobs, the rollup's audit row.
func TestPlanDispatchSyncOrdering(t *testing.T) {
	pool, sink := setup(t)
	ctx := context.Background()
	ec := usecase.NewExecutionContext("prn_1")
	a, b := &thing{ID: "thg_a", Name: "a"}, &thing{ID: "thg_b", Name: "b"}
	op := usecaseop.Operation[saveThing, thingSaved]{
		Name:      "SyncThings",
		Authorize: usecaseop.Public[saveThing],
		Execute: func(_ context.Context, _ saveThing, ec usecase.ExecutionContext) (usecaseop.Plan[thingSaved], error) {
			return usecaseop.Sync(thingRepo{}, []usecasepgx.SyncSaveItem[thing]{
				{Aggregate: a, Event: savedEvent(ec, a.ID)}, {Aggregate: b, Event: savedEvent(ec, b.ID)},
			}, nil, savedEvent(ec, "all")).Dispatch(outbox.NewDispatchJob("shop:sync", "https://a.example.com/s").WithID("job-sync")), nil
		},
	}
	rollup, err := usecaseop.Run(ctx, usecasepgx.New(pool, sink), op, saveThing{}, ec)
	require.NoError(t, err)
	rows := pgtest.OutboxRows(t, pool)
	require.Equal(t, []string{"EVENT", "AUDIT_LOG", "EVENT", "AUDIT_LOG", "EVENT", "DISPATCH_JOB", "AUDIT_LOG"}, types(rows))
	assert.Equal(t, rollup.EventID(), rows[4].ID)
	assert.Equal(t, "job-sync", rows[5].ID)
	assert.Equal(t, 2, countThings(t, pool))
}

// Every plan constructor carries jobs; Dispatch calls accumulate.
func TestPlanDispatchEveryConstructor(t *testing.T) {
	pool, sink := setup(t)
	ctx := context.Background()
	uow := usecasepgx.New(pool, sink)
	ec := usecase.NewExecutionContext("prn_1")
	job := func(id string) outbox.DispatchJob {
		return outbox.NewDispatchJob("shop:x", "https://a.example.com").WithID(id)
	}

	saved := &thing{ID: "thg_del", Name: "d"}
	plans := []usecaseop.Plan[thingSaved]{
		usecaseop.Emit(savedEvent(ec, "e0")),
		usecaseop.Emit(savedEvent(ec, "e1")).Dispatch(job("job-emit-1")),
		usecaseop.Emit(savedEvent(ec, "e2")).Dispatch(job("job-emit-2a")).Dispatch(job("job-emit-2b")),
		usecaseop.Save(saved, thingRepo{}, savedEvent(ec, saved.ID)).Dispatch(job("job-save")),
		usecaseop.SaveAll([]thing{{ID: "thg_all", Name: "x"}}, thingRepo{}, savedEvent(ec, "all")).Dispatch(job("job-saveall")),
		usecaseop.Delete(saved, thingRepo{}, savedEvent(ec, saved.ID)).Dispatch(job("job-delete")),
	}
	for i, p := range plans {
		op := usecaseop.Operation[saveThing, thingSaved]{
			Name: "plan", Authorize: usecaseop.Public[saveThing],
			Execute: func(context.Context, saveThing, usecase.ExecutionContext) (usecaseop.Plan[thingSaved], error) {
				return p, nil
			},
		}
		_, err := usecaseop.Run(ctx, uow, op, saveThing{}, ec)
		require.NoError(t, err, "plan %d", i)
	}
	var jobIDs []string
	for _, r := range pgtest.OutboxRows(t, pool) {
		if r.Type == "DISPATCH_JOB" {
			jobIDs = append(jobIDs, r.ID)
		}
	}
	assert.Equal(t, []string{"job-emit-1", "job-emit-2a", "job-emit-2b", "job-save", "job-saveall", "job-delete"}, jobIDs)
	assert.Equal(t, 1, countThings(t, pool), "save, then delete; save-all's row stays")
}

// A sink that cannot write dispatch jobs fails a plan that carries one,
// before anything is written.
func TestPlanDispatchUnsupportedSinkWritesNothing(t *testing.T) {
	pool, sink := setup(t)
	persisted := 0
	_, err := usecaseop.Run(context.Background(), usecasepgx.New(pool, eventOnlySink{sink}),
		saveOp(thingRepo{persisted: &persisted}, outbox.NewDispatchJob("shop:x", "https://a.example.com")),
		saveThing{Name: "a"}, usecase.NewExecutionContext("prn_1"))
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%v", err)
	assert.Equal(t, "DISPATCH_JOB_UNSUPPORTED", ue.Code)
	assert.Equal(t, 0, persisted)
	assert.Empty(t, pgtest.OutboxRows(t, pool))

	// Without jobs the same sink is fine.
	_, err = usecaseop.Run(context.Background(), usecasepgx.New(pool, eventOnlySink{sink}),
		saveOp(thingRepo{}), saveThing{Name: "a"}, usecase.NewExecutionContext("prn_1"))
	require.NoError(t, err)
}

// The lower-level paths: a scoped commit inside an orchestrated transaction
// takes the jobs as an option and writes them in the same order.
func TestScopedCommitWithDispatchJobs(t *testing.T) {
	pool, sink := setup(t)
	ctx := context.Background()
	ec := usecase.NewExecutionContext("prn_1")
	err := usecasepgx.RunErr(ctx, usecasepgx.New(pool, sink), func(s *usecasepgx.TxScopedUnitOfWork) error {
		t1 := &thing{ID: "thg_s1", Name: "s"}
		_, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, t1, thingRepo{}, savedEvent(ec, t1.ID), saveThing{},
			usecasepgx.WithDispatchJobs(outbox.NewDispatchJob("shop:x", "https://a.example.com").WithID("job-scoped"))))
		return err
	})
	require.NoError(t, err)
	rows := pgtest.OutboxRows(t, pool)
	require.Equal(t, []string{"EVENT", "DISPATCH_JOB", "AUDIT_LOG"}, types(rows))
	assert.Equal(t, ec.CorrelationID, payloadOf(t, rows[1])["correlationId"])
}
