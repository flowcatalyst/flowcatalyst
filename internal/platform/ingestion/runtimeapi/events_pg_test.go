//go:build integration

package runtimeapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/stream"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// eventType stores the event type an EVENT target names and returns its code.
func (e *env) eventType() string {
	e.t.Helper()
	code := e.app.Code + ":orders:order:received"
	et, err := eventtype.New(code, "Order received")
	require.NoError(e.t, err)
	e.persist(e.uow(), func(ctx context.Context, tx *usecasepgx.DbTx) error {
		return eventtype.NewRepository(e.pool).Persist(ctx, et, tx)
	})
	return code
}

func (e *env) eventTarget(code, extra string) string {
	return `{"kind":"EVENT","code":"` + code + `","batchSize":100` + extra + `}`
}

type eventRow struct {
	typ, source, subject, data                string
	client, group, correlation, deduplication *string
}

func (e *env) event(id string) eventRow {
	e.t.Helper()
	var r eventRow
	require.NoError(e.t, e.pool.QueryRow(context.Background(),
		`SELECT type, source, subject, data::text, client_id, message_group, correlation_id, deduplication_id FROM msg_events WHERE id = $1`, id).
		Scan(&r.typ, &r.source, &r.subject, &r.data, &r.client, &r.group, &r.correlation, &r.deduplication))
	return r
}

// A job of a target with dataOnly is made to be delivered raw; without it, in
// the envelope.
func TestJobs_DataOnlyIsTheTargets(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	raw := e.saveTarget("raw", sqsSource, tg.target(`,"dataOnly":true`), true)
	wrapped := e.saveTarget("wrapped", sqsSource, tg.target(``), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	dataOnly := func(id string) bool {
		var b bool
		require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT data_only FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&b))
		return b
	}
	r := e.do(http.MethodPost, jobsPath(raw.ID, rt), map[string]any{"records": []map[string]any{{"payload": "<a/>", "payloadContentType": "application/xml"}}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "DISPATCH_JOB", r.str("kind"))
	id := r.list("results")[0]["id"].(string)
	assert.True(t, dataOnly(id))
	assert.Equal(t, "application/xml", e.job(id).contentType)

	r = e.do(http.MethodPost, jobsPath(wrapped.ID, rt), map[string]any{"records": records(1, "w")})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "DISPATCH_JOB", r.str("kind"))
	assert.False(t, dataOnly(r.list("results")[0]["id"].(string)))
}

// An EVENT target stores one event per record through the event insert path,
// of the target's type, from the ingestion's source and client.
func TestJobs_EventTargetMakesEvents(t *testing.T) {
	e := newEnv(t)
	code := e.eventType()
	i := e.saveTarget("evt", sqsSource, e.eventTarget(code, ""), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	r := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{{
		"payload": `{"order":1}`, "payloadContentType": "application/json", "messageGroup": "order-1",
		"externalId": "ext-1", "correlationId": "corr-1", "subject": "orders/1",
	}, {"payload": `{"order":2}`, "payloadContentType": "application/vnd.acme+json; charset=utf-8"}, {"payload": `[1,2]`}}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "EVENT", r.str("kind"))
	assert.EqualValues(t, 3, r.json["created"])
	results := r.list("results")
	require.Len(t, results, 3)

	ev := e.event(results[0]["id"].(string))
	assert.Equal(t, code, ev.typ)
	assert.Equal(t, "ingestion:"+e.app.Code+":evt", ev.source)
	assert.Equal(t, "orders/1", ev.subject)
	assert.JSONEq(t, `{"order":1}`, ev.data)
	assert.Equal(t, string(e.client.ID), deref(ev.client))
	assert.Equal(t, "order-1", deref(ev.group))
	assert.Equal(t, "corr-1", deref(ev.correlation))
	assert.Equal(t, "ing:"+i.ID+":ext-1", deref(ev.deduplication))
	assert.NotContains(t, results[0], "existing")

	assert.JSONEq(t, `{"order":2}`, e.event(results[1]["id"].(string)).data, "+json is JSON")
	assert.JSONEq(t, `[1,2]`, e.event(results[2]["id"].(string)).data, "no content type is JSON")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE metadata::text LIKE $1`, "%"+i.ID+"%"), "an event target makes no job")
}

func TestJobs_EventTargetWrapsOrRefusesWhatIsNotJSON(t *testing.T) {
	e := newEnv(t)
	code := e.eventType()
	wrap := e.saveTarget("wrap", sqsSource, e.eventTarget(code, `,"wrapNonJson":true`), true)
	plain := e.saveTarget("plain", sqsSource, e.eventTarget(code, ``), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	recs := []map[string]any{
		{"payload": "<a>1</a>", "payloadContentType": "application/xml"},
		{"payload": "a,b\n1,2", "payloadContentType": "text/csv"},
		{"payload": "not json", "payloadContentType": "application/json"}, // says JSON, is not
		{"payload": `{"fine":true}`, "payloadContentType": "application/json"},
	}
	r := e.do(http.MethodPost, jobsPath(wrap.ID, rt), map[string]any{"records": recs})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, 4, r.json["created"])
	res := r.list("results")
	assert.JSONEq(t, `{"contentType":"application/xml","body":"<a>1</a>"}`, e.event(res[0]["id"].(string)).data)
	assert.JSONEq(t, `{"contentType":"text/csv","body":"a,b\n1,2"}`, e.event(res[1]["id"].(string)).data)
	assert.JSONEq(t, `{"contentType":"application/json","body":"not json"}`, e.event(res[2]["id"].(string)).data)
	assert.JSONEq(t, `{"fine":true}`, e.event(res[3]["id"].(string)).data, "JSON is not wrapped")

	r = e.do(http.MethodPost, jobsPath(wrap.ID, rt), map[string]any{"records": []map[string]any{{"payload": "plain text"}}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.JSONEq(t, `{"contentType":"text/plain","body":"plain text"}`, e.event(r.list("results")[0]["id"].(string)).data,
		"text/plain when the record has no type and is not JSON")

	// Without wrapNonJson each such record is its own error and the rest land.
	r = e.do(http.MethodPost, jobsPath(plain.ID, rt), map[string]any{"records": recs})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, 1, r.json["created"])
	res = r.list("results")
	for _, n := range []int{0, 1, 2} {
		assert.NotContains(t, res[n], "id", n)
		assert.Equal(t, "the record is not JSON and the target's wrapNonJson is off", res[n]["error"], n)
	}
	assert.NotContains(t, res[3], "error")
	assert.JSONEq(t, `{"fine":true}`, e.event(res[3]["id"].(string)).data)
}

// A record with an externalId makes its event once: a resend, or the same key
// twice in one request, answers the first event with existing: true.
func TestJobs_EventTargetIsIdempotent(t *testing.T) {
	e := newEnv(t)
	code := e.eventType()
	i := e.saveTarget("evt-idem", sqsSource, e.eventTarget(code, `,"wrapNonJson":true`), true)
	other := e.saveTarget("evt-idem-other", sqsSource, e.eventTarget(code, `,"wrapNonJson":true`), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	first := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{{"payload": `{"n":1}`, "externalId": "k1"}, {"payload": `{"n":2}`, "externalId": "k2"}}})
	require.Equal(t, http.StatusOK, first.code, first.raw)
	assert.EqualValues(t, 2, first.json["created"])
	ids := []any{first.list("results")[0]["id"], first.list("results")[1]["id"]}

	again := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{
		{"payload": `{"n":2}`, "externalId": "k2"}, {"payload": `{"n":3}`, "externalId": "k3"}, {"payload": `{"n":4}`}, {"payload": `{"n":"3 again"}`, "externalId": "k3"},
	}})
	require.Equal(t, http.StatusOK, again.code, again.raw)
	assert.Equal(t, "EVENT", again.str("kind"))
	assert.EqualValues(t, 2, again.json["created"], "k3 once and the record with no key")
	results := again.list("results")
	require.Len(t, results, 4)
	assert.Equal(t, ids[1], results[0]["id"], "the resend answers the event it made")
	assert.Equal(t, true, results[0]["existing"])
	assert.Equal(t, results[1]["id"], results[3]["id"], "the same key within one request is one event")
	assert.Equal(t, true, results[3]["existing"])
	assert.NotContains(t, results[1], "existing")
	assert.NotContains(t, results[2], "existing")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM msg_events WHERE deduplication_id = $1`, "ing:"+i.ID+":k2"))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM msg_events WHERE deduplication_id = $1`, "ing:"+i.ID+":k3"))
	assert.JSONEq(t, `{"n":3}`, e.event(results[1]["id"].(string)).data, "the first record of a key wins")

	r := e.do(http.MethodPost, jobsPath(other.ID, rt), map[string]any{"records": []map[string]any{{"payload": `{"n":1}`, "externalId": "k1"}}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, 1, r.json["created"], "keys are per ingestion")
	assert.NotEqual(t, ids[0], r.list("results")[0]["id"])
}

// The event target resolves as at save: an event type that has gone is a 409
// on the ingestion, and the usual record and ingestion checks still hold.
func TestJobs_EventTargetChecks(t *testing.T) {
	e := newEnv(t)
	gone := e.saveTarget("evt-gone", sqsSource, e.eventTarget(e.app.Code+":orders:order:missing", `,"wrapNonJson":true`), true)
	code := e.eventType()
	ok := e.saveTarget("evt-ok", sqsSource, e.eventTarget(code, `,"wrapNonJson":true`), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	r := e.do(http.MethodPost, jobsPath(gone.ID, rt), map[string]any{"records": records(1, "a")})
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM msg_events WHERE source = $1`, "ingestion:"+e.app.Code+":evt-gone"))

	r = e.do(http.MethodPost, jobsPath(ok.ID, rt), map[string]any{"records": []map[string]any{{"payload": ""}, {"payload": "x", "externalId": " "}, {"payload": "x"}}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, 1, r.json["created"])
	res := r.list("results")
	assert.Equal(t, "payload is required", res[0]["error"])
	assert.Equal(t, "externalId may not be blank", res[1]["error"])
	assert.NotContains(t, res[2], "error")

	e.pause(ok.ID)
	r = e.do(http.MethodPost, jobsPath(ok.ID, rt), map[string]any{"records": records(1, "p")})
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "INGESTION_NOT_LIVE", r.errCode())
}

// An event an ingestion made reaches a subscription of its type like any other
// event: the stream fans it out into a dispatch job for the subscription.
func TestJobs_EventTargetEventReachesASubscription(t *testing.T) {
	e := newEnv(t)
	code := e.eventType()
	i := e.saveTarget("evt-sub", sqsSource, e.eventTarget(code, `,"wrapNonJson":true`), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)

	sub := "sub_" + unique()
	ctx := context.Background()
	_, err := e.pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, status, mode, source, client_id)
		 VALUES ($1, $1, 'Orders listener', 'https://sub.ingest.test/hook', 'ACTIVE', 'IMMEDIATE', 'UI', $2)`, sub, string(e.client.ID))
	require.NoError(t, err)
	_, err = e.pool.Exec(ctx, `INSERT INTO msg_subscription_event_types (subscription_id, event_type_code) VALUES ($1, $2)`, sub, code)
	require.NoError(t, err)

	r := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{{"payload": `{"order":7}`, "externalId": "o7"}}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	eventID := r.list("results")[0]["id"].(string)

	fan := stream.NewFanOut(e.pool).Projector(stream.ProjectorConfig{})
	require.Eventually(t, func() bool {
		_, err := fan.Step(ctx, 1000)
		require.NoError(t, err)
		return e.count(`SELECT count(*) FROM msg_events WHERE id = $1 AND fanned_out_at IS NOT NULL`, eventID) == 1
	}, 30*time.Second, 50*time.Millisecond)

	var target, payload string
	var clientID *string
	require.NoError(t, e.pool.QueryRow(ctx,
		`SELECT target_url, payload, client_id FROM msg_dispatch_jobs WHERE event_id = $1 AND subscription_id = $2`, eventID, sub).
		Scan(&target, &payload, &clientID))
	assert.Equal(t, "https://sub.ingest.test/hook", target)
	assert.JSONEq(t, `{"order":7}`, payload)
	assert.Equal(t, string(e.client.ID), deref(clientID))
}
