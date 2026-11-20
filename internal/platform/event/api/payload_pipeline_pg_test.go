//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/stream"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The whole path of a payload with the payload table written
// (docs/payload-offload-design.md, sections 3, 6, 10 and 11): the batch
// route, the events projection, the fan-out to two subscriptions and the
// delivery callback, against the subscriber that receives it. And the same
// path with the switch off, and a backlog of the legacy shape met by an
// instance with the switch on.

// The test subscribers are on loopback.
func init() { netguard.Default.AllowLoopback = true }

const pipelineSecret = "payload-pipeline-secret"

type pipeline struct {
	pool   *pgxpool.Pool
	store  *payloadtest.CountingStore
	events *State
	jobs   *dispatchjob.Repository
	base   string
	auth   *scheduler.DispatchAuthService
}

// newPipeline wires the event routes, the dispatch-job repository and the
// delivery callback over svc, as the server does (one payload service).
func newPipeline(t *testing.T, store *payloadtest.CountingStore, svc *payloadstore.Service) *pipeline {
	t.Helper()
	pool := testpg.Pool(t)
	jobs := dispatchjob.NewRepository(pool).UsePayloads(svc)
	auth := scheduler.NewDispatchAuthService(pipelineSecret)
	r := chi.NewRouter()
	processing.New(jobs, auth).Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return &pipeline{
		pool: pool, store: store, jobs: jobs, base: ts.URL, auth: auth,
		events: &State{Repo: event.NewRepository(pool).UsePayloads(svc)},
	}
}

// drain runs the events projection and the fan-out until neither has
// anything left (other tests' events included).
func (p *pipeline) drain(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	proj := stream.NewEventProjection(p.pool).Projector(stream.DefaultProjectorConfig())
	fan := stream.NewFanOut(p.pool).Projector(stream.DefaultProjectorConfig())
	for range 100 {
		a, err := proj.Step(ctx, 500)
		require.NoError(t, err)
		b, err := fan.Step(ctx, 500)
		require.NoError(t, err)
		if a == 0 && b == 0 {
			return
		}
	}
	t.Fatal("the projection and the fan-out did not drain")
}

// deliver runs the delivery callback for a job, as the router would.
func (p *pipeline) deliver(t *testing.T, jobID string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"messageId": jobID})
	req, err := http.NewRequest(http.MethodPost, p.base+"/api/dispatch/process", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.auth.Sign(jobID))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, true, out["ack"], string(raw))
}

// subscriber records the bodies it receives.
type subscriber struct {
	url string
	mu  sync.Mutex
	got []string
}

func newSubscriber(t *testing.T) *subscriber {
	t.Helper()
	s := &subscriber{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.got = append(s.got, string(b))
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

func (s *subscriber) last(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.NotEmpty(t, s.got, "the subscriber received nothing")
	return s.got[len(s.got)-1]
}

// subscribe makes an ACTIVE subscription of every client to typ, delivering
// to target, data-only or in the envelope.
func subscribe(t *testing.T, pool *pgxpool.Pool, typ, target string, dataOnly bool) string {
	t.Helper()
	ctx := context.Background()
	id := "sub_" + tsid.GenerateUntyped()
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscriptions (id, code, name, target, status, mode, source, data_only)
		 VALUES ($1, $1, $1, $2, 'ACTIVE', 'IMMEDIATE', 'UI', $3)`, id, target, dataOnly)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO msg_subscription_event_types (subscription_id, event_type_code) VALUES ($1, $2)`, id, typ)
	require.NoError(t, err)
	return id
}

// raisedJob is a job the fan-out raised, as stored.
type raisedJob struct {
	id        string
	payload   *string
	payloadID *string
	createdAt time.Time
}

// jobsOf returns the jobs an event raised, by subscription.
func jobsOf(t *testing.T, pool *pgxpool.Pool, eventID string) map[string]raisedJob {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id, subscription_id, payload, payload_id, created_at FROM msg_dispatch_jobs WHERE event_id = $1`, eventID)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]raisedJob{}
	for rows.Next() {
		var j raisedJob
		var sub string
		require.NoError(t, rows.Scan(&j.id, &sub, &j.payload, &j.payloadID, &j.createdAt))
		out[sub] = j
	}
	require.NoError(t, rows.Err())
	return out
}

// storedEvent is an event's msg_events and msg_events_read columns.
type storedEvent struct {
	data, payloadID, readData, readPayloadID *string
	createdAt                                time.Time
}

func readStored(t *testing.T, pool *pgxpool.Pool, id string) storedEvent {
	t.Helper()
	var s storedEvent
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT e.data::text, e.payload_id, e.created_at, r.data, r.payload_id
		   FROM msg_events e LEFT JOIN msg_events_read r ON r.id = e.id AND r.created_at = e.created_at
		  WHERE e.id = $1`, id).Scan(&s.data, &s.payloadID, &s.createdAt, &s.readData, &s.readPayloadID))
	return s
}

func payloadRowData(t *testing.T, pool *pgxpool.Pool, id string, createdAt time.Time) (data, ref *string, size int32) {
	t.Helper()
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT data, ref, size FROM msg_payloads WHERE id = $1 AND created_at = $2`, id, createdAt).Scan(&data, &ref, &size))
	return data, ref, size
}

// pipelineOdd is data JSONB would change: spacing, key order, a repeated key.
const pipelineOdd = `{"z": 1,"a":  [3, 1] , "z":2}`

// pipelineLarge is pipelineOdd's shape, large enough to be offloaded.
func pipelineLarge() string {
	return `{"z": 1, "pad":"` + strings.Repeat("w", 3*payloadtest.OffloadMinBytes) + `" , "z":2}`
}

func pipelineType() string {
	return "pipeline:" + strings.ToLower(tsid.GenerateUntyped()) + ":event:made"
}

// envelopeData is the data member of an envelope delivery, re-marshalled
// compactly, and the envelope's id.
func envelopeData(t *testing.T, body string) (data any, id string) {
	t.Helper()
	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	id, _ = env["id"].(string)
	return env["data"], id
}

func decoded(t *testing.T, s string) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(s), &v))
	return v
}

func TestPayloadPipeline_NewShapeEndToEnd(t *testing.T) {
	store := payloadtest.NewCountingStore()
	p := newPipeline(t, store, payloadtest.WritingService(t, store, nil))
	typ := pipelineType()
	dataOnly, envelope := newSubscriber(t), newSubscriber(t)
	subDataOnly := subscribe(t, p.pool, typ, dataOnly.url, true)
	subEnvelope := subscribe(t, p.pool, typ, envelope.url, false)

	small := pipelineOdd // spacing, key order and a repeated key JSONB would change
	large := pipelineLarge()
	out, err := p.events.batchIngest(anchorCtx(), &apicommon.In[BatchRequest]{Body: BatchRequest{Items: []BatchEventItem{
		{Type: typ, Source: "test://pipeline", Data: json.RawMessage(small)},
		{Type: typ, Source: "test://pipeline", Data: json.RawMessage(large)},
	}}})
	require.NoError(t, err)
	require.Len(t, out.Body.Results, 2)
	for _, r := range out.Body.Results {
		require.Equal(t, "SUCCESS", r.Status, r.Error)
	}
	assert.EqualValues(t, 1, store.Puts(), "one segment for the batch's one client")

	sent := map[string]string{out.Body.Results[0].ID: small, out.Body.Results[1].ID: large}
	for id, bytesSent := range sent {
		ev := readStored(t, p.pool, id)
		assert.Nil(t, ev.data, "msg_events.data is NULL")
		require.NotNil(t, ev.payloadID)
		data, ref, size := payloadRowData(t, p.pool, *ev.payloadID, ev.createdAt)
		assert.EqualValues(t, len(bytesSent), size)
		if bytesSent == small {
			require.NotNil(t, data)
			assert.Equal(t, small, *data, "exactly the bytes sent")
		} else {
			assert.Nil(t, data)
			assert.NotNil(t, ref, "offloaded")
		}
	}

	p.drain(t)
	for id, bytesSent := range sent {
		ev := readStored(t, p.pool, id)
		require.NotNil(t, ev.readPayloadID, "the projection copies payload_id")
		assert.Equal(t, *ev.payloadID, *ev.readPayloadID)
		assert.Nil(t, ev.readData, "and not the data")

		// The read API returns the bytes sent.
		got, err := p.events.getByID(anchorCtx(), &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		assert.Equal(t, bytesSent, string(got.Body.Data))

		jobs := jobsOf(t, p.pool, id)
		require.Contains(t, jobs, subDataOnly)
		require.Contains(t, jobs, subEnvelope)
		for _, sub := range []string{subDataOnly, subEnvelope} {
			j := jobs[sub]
			require.NotNil(t, j.payloadID, "the job points at the event's payload row")
			assert.Equal(t, *ev.payloadID, *j.payloadID)
			assert.Nil(t, j.payload, "no job holds a payload of its own")
			assert.Equal(t, ev.createdAt, j.createdAt)
		}

		p.deliver(t, jobs[subDataOnly].id)
		assert.Equal(t, bytesSent, dataOnly.last(t), "a data-only delivery is the producer's bytes")
		p.deliver(t, jobs[subEnvelope].id)
		data, envID := envelopeData(t, envelope.last(t))
		assert.Equal(t, jobs[subEnvelope].id, envID)
		assert.Equal(t, decoded(t, bytesSent), data)
	}
}

// What a subscriber receives for an event with no data is unchanged: an event
// ingested with none is stored as "{}" in either shape and delivers "{}"; a
// row with neither data nor a payload_id (as the legacy shape can hold) is
// fanned out the legacy way, as the payload "null".
func TestPayloadPipeline_EventWithNoData(t *testing.T) {
	ctx := context.Background()
	store := payloadtest.NewCountingStore()
	on := newPipeline(t, store, payloadtest.WritingService(t, store, nil))
	off := event.NewRepository(on.pool) // the switch off
	typ := pipelineType()
	dataOnly, envelope := newSubscriber(t), newSubscriber(t)
	subDataOnly := subscribe(t, on.pool, typ, dataOnly.url, true)
	subEnvelope := subscribe(t, on.pool, typ, envelope.url, false)

	newShape := event.New(typ, "test://pipeline", "", nil)
	legacy := event.New(typ, "test://pipeline", "", nil)
	_, err := on.events.Repo.Ingest(ctx, []event.Event{*newShape})
	require.NoError(t, err)
	_, err = off.Ingest(ctx, []event.Event{*legacy})
	require.NoError(t, err)
	bare := tsid.GenerateUntyped()
	_, err = on.pool.Exec(ctx, `INSERT INTO msg_events (id, type, source, time) VALUES ($1, $2, 'test://pipeline', NOW())`, bare, typ)
	require.NoError(t, err)
	on.drain(t)

	received := func(eventID string) (dataOnlyBody string, envData any) {
		t.Helper()
		jobs := jobsOf(t, on.pool, eventID)
		on.deliver(t, jobs[subDataOnly].id)
		on.deliver(t, jobs[subEnvelope].id)
		d, _ := envelopeData(t, envelope.last(t))
		return dataOnly.last(t), d
	}
	gotNew, envNew := received(newShape.ID)
	gotLegacy, envLegacy := received(legacy.ID)
	assert.Equal(t, gotLegacy, gotNew, "the same data-only body in either shape")
	assert.Equal(t, `{}`, gotNew)
	assert.Equal(t, envLegacy, envNew)
	assert.Equal(t, map[string]any{}, envNew)
	require.NotNil(t, readStored(t, on.pool, newShape.ID).payloadID, "the new shape, with its payload row")

	jobs := jobsOf(t, on.pool, bare)
	require.NotNil(t, jobs[subDataOnly].payload)
	assert.Equal(t, "null", *jobs[subDataOnly].payload, "as the fan-out has always written it")
	assert.Nil(t, jobs[subDataOnly].payloadID)
	gotBare, envBare := received(bare)
	assert.Equal(t, "null", gotBare)
	assert.Nil(t, envBare)
}

// A backlog of the legacy shape when the switch is turned on: an event stored
// with its data in msg_events.data, not yet projected or fanned out, is
// projected and fanned out the legacy way by an instance writing the new
// shape, and delivers its data.
func TestPayloadPipeline_LegacyBacklogUnderTheSwitch(t *testing.T) {
	ctx := context.Background()
	store := payloadtest.NewCountingStore()
	p := newPipeline(t, store, payloadtest.WritingService(t, store, nil))
	typ := pipelineType()
	dataOnly := newSubscriber(t)
	sub := subscribe(t, p.pool, typ, dataOnly.url, true)

	old := event.New(typ, "test://pipeline", "", json.RawMessage(`{"b":1, "a":2}`))
	_, err := event.NewRepository(p.pool).Ingest(ctx, []event.Event{*old}) // before the switch
	require.NoError(t, err)
	fresh := event.New(typ, "test://pipeline", "", json.RawMessage(`{"b":3, "a":4}`))
	_, err = p.events.Repo.Ingest(ctx, []event.Event{*fresh})
	require.NoError(t, err)
	p.drain(t)

	ev := readStored(t, p.pool, old.ID)
	assert.Nil(t, ev.payloadID)
	require.NotNil(t, ev.readData, "projected with its data")
	assert.JSONEq(t, `{"a":2,"b":1}`, *ev.readData)
	assert.Nil(t, ev.readPayloadID)
	j := jobsOf(t, p.pool, old.ID)[sub]
	require.NotNil(t, j.payload, "fanned out with its data")
	assert.Nil(t, j.payloadID)
	p.deliver(t, j.id)
	assert.Equal(t, *ev.data, dataOnly.last(t), "the stored JSONB text, as before")

	nj := jobsOf(t, p.pool, fresh.ID)[sub]
	require.NotNil(t, nj.payloadID, "the same backlog's new event is the new shape")
	p.deliver(t, nj.id)
	assert.Equal(t, `{"b":3, "a":4}`, dataOnly.last(t))
}

// The switch off: ingest, the projection and the fan-out write the legacy
// shape and no payload row at all.
func TestPayloadPipeline_SwitchOffWritesTheLegacyShape(t *testing.T) {
	store := payloadtest.NewCountingStore()
	p := newPipeline(t, store, payloadtest.Service(t, store))
	typ := pipelineType()
	dataOnly := newSubscriber(t)
	sub := subscribe(t, p.pool, typ, dataOnly.url, true)

	large := pipelineLarge()
	out, err := p.events.batchIngest(anchorCtx(), &apicommon.In[BatchRequest]{Body: BatchRequest{Items: []BatchEventItem{
		{Type: typ, Source: "test://pipeline", Data: json.RawMessage(large)},
	}}})
	require.NoError(t, err)
	require.Equal(t, "SUCCESS", out.Body.Results[0].Status)
	id := out.Body.Results[0].ID
	p.drain(t)

	ev := readStored(t, p.pool, id)
	assert.Nil(t, ev.payloadID)
	require.NotNil(t, ev.data)
	assert.JSONEq(t, large, *ev.data)
	assert.Nil(t, ev.readPayloadID)
	require.NotNil(t, ev.readData)
	j := jobsOf(t, p.pool, id)[sub]
	assert.Nil(t, j.payloadID)
	require.NotNil(t, j.payload)
	var n int
	require.NoError(t, p.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM msg_payloads WHERE created_at = $1`, ev.createdAt).Scan(&n))
	assert.Zero(t, n, "no payload row at all")
	assert.Zero(t, store.Puts(), "the store is not written")
	p.deliver(t, j.id)
	assert.Equal(t, *ev.data, dataOnly.last(t))
}

// The batch route's per-item results with the payload table written and the
// store failing (sections 7 and 8): positional, each item's own outcome, and
// the items that can be stored are.
func TestBatchIngest_PayloadRefusalsArePositional(t *testing.T) {
	store := payloadtest.NewCountingStore()
	store.FailWrites(errors.New("bucket unreachable"))
	svc := payloadtest.WritingService(t, store, func(c *payloadstore.Config) { c.InlineMaxBytes = 8192; c.MaxBytes = 16384 })
	s := &State{Repo: event.NewRepository(testpg.Pool(t)).UsePayloads(svc)}
	typ := pipelineType()
	str := func(n int) json.RawMessage { return json.RawMessage(`"` + strings.Repeat("q", n-2) + `"`) }

	out, err := s.batchIngest(anchorCtx(), &apicommon.In[BatchRequest]{Body: BatchRequest{Items: []BatchEventItem{
		{Type: typ, Source: "s", Data: json.RawMessage(`{"ok":1}`)},
		{Source: "s", Data: json.RawMessage(`{"no":"type"}`)},
		{Type: typ, Source: "s", Data: str(6000)},
		{Type: typ, Source: "s", Data: str(10000)},
		{Type: typ, Source: "s", Data: str(20000)},
		{Type: typ, Source: "s", Data: json.RawMessage("\"a\xffb\"")},
		{Type: typ, Source: "s", Data: json.RawMessage(`{"ok":2}`)},
	}}})
	require.NoError(t, err)
	r := out.Body.Results
	require.Len(t, r, 7)
	assert.Equal(t, "SUCCESS", r[0].Status)
	assert.Equal(t, "BAD_REQUEST", r[1].Status)
	assert.Equal(t, "type is required", r[1].Error)
	assert.Equal(t, "SUCCESS", r[2].Status, "kept in the database while the store is down")
	assert.Equal(t, "INTERNAL_ERROR", r[3].Status, "retried by the outbox processor")
	assert.True(t, strings.HasPrefix(r[3].Error, "PAYLOAD_STORE_UNAVAILABLE"), r[3].Error)
	assert.Equal(t, "BAD_REQUEST", r[4].Status, "terminal")
	assert.True(t, strings.HasPrefix(r[4].Error, "PAYLOAD_TOO_LARGE"), r[4].Error)
	assert.Equal(t, "BAD_REQUEST", r[5].Status)
	assert.True(t, strings.HasPrefix(r[5].Error, "PAYLOAD_NOT_TEXT"), r[5].Error)
	assert.Equal(t, "SUCCESS", r[6].Status)

	for i, stored := range map[int]bool{0: true, 2: true, 3: false, 4: false, 5: false, 6: true} {
		got, err := s.Repo.FindRawByID(context.Background(), r[i].ID)
		require.NoError(t, err)
		assert.Equal(t, stored, got != nil, "item %d stored", i)
	}
}

// The single route answers the same conditions as HTTP errors: 400
// PAYLOAD_TOO_LARGE, and 503 PAYLOAD_STORE_UNAVAILABLE (try again).
func TestCreateEvent_PayloadRefusals(t *testing.T) {
	store := payloadtest.NewCountingStore()
	svc := payloadtest.WritingService(t, store, func(c *payloadstore.Config) { c.InlineMaxBytes = 8192; c.MaxBytes = 16384 })
	s := &State{Repo: event.NewRepository(testpg.Pool(t)).UsePayloads(svc)}
	typ := pipelineType()
	create := func(data json.RawMessage) error {
		_, err := s.create(anchorCtx(), &apicommon.In[CreateEventRequest]{Body: CreateEventRequest{EventType: typ, Source: "s", Data: data}})
		return err
	}
	str := func(n int) json.RawMessage { return json.RawMessage(`"` + strings.Repeat("c", n-2) + `"`) }

	require.NoError(t, create(str(12000)), "offloaded")
	testpg.RequireUsecaseError(t, create(str(20000)), usecase.KindValidation, "PAYLOAD_TOO_LARGE")

	store.FailWrites(errors.New("bucket unreachable"))
	err := create(str(12000))
	var status interface{ GetStatus() int }
	require.ErrorAs(t, err, &status)
	assert.Equal(t, http.StatusServiceUnavailable, status.GetStatus())
	assert.Contains(t, err.Error(), "send it again later")
	require.NoError(t, create(str(6000)), "small enough to keep in the database")
}
