//go:build integration

package processing_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Delivery of a job of the new shape (docs/payload-offload-design.md,
// sections 3.2 and 7): the payload comes from the job's payload row, held in
// the database or resolved from the payload store, and a payload that cannot
// be produced is a platform fault (store unavailable) or a terminal failure
// (missing), never a delivery.

// deliveryOddPayload is a payload JSONB would have changed: a data-only
// delivery of the new shape sends exactly these bytes.
const deliveryOddPayload = `{"z": 1,"a":  [3, 1] , "z":2}`

type payloadHarness struct {
	pool    *pgxpool.Pool
	store   *payloadtest.CountingStore
	svc     *payloadstore.Service
	repo    *dispatchjob.Repository
	base    string
	auth    *scheduler.DispatchAuthService
	handler *processing.Handler
}

func newPayloadHarness(t *testing.T) *payloadHarness {
	t.Helper()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.Service(t, store)
	repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
	auth := scheduler.NewDispatchAuthService(testSecret)
	h := processing.New(repo, auth)
	r := chi.NewRouter()
	h.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return &payloadHarness{pool: pool, store: store, svc: svc, repo: repo, base: ts.URL, auth: auth, handler: h}
}

// job writes a QUEUED job of the new shape pointing at a payload row holding
// payload (offloaded when it is larger than the test threshold).
func (h *payloadHarness) job(t *testing.T, target string, dataOnly bool, payload string) (id string, row payloadstore.Row) {
	t.Helper()
	at := time.Now().UTC().Truncate(time.Microsecond)
	row = payloadtest.Store(t, h.svc, h.pool, "", at, []byte(payload))
	id = tsid.GenerateUntyped()
	_, err := h.pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs
		     (id, code, target_url, status, data_only, payload, payload_id, max_retries, attempt_count, created_at, updated_at)
		 VALUES ($1, 'proc:payload:evt', $2, 'QUEUED', $3, NULL, $4, 3, 0, $5, $5)`,
		id, target, dataOnly, row.ID, at)
	require.NoError(t, err)
	return id, row
}

// payloadSubscriber records each body it receives.
type payloadSubscriber struct {
	calls atomic.Int32
	body  atomic.Value
	url   string
}

func newPayloadSubscriber(t *testing.T) *payloadSubscriber {
	s := &payloadSubscriber{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.body.Store(string(b))
		s.calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

func (s *payloadSubscriber) last() string {
	v, _ := s.body.Load().(string)
	return v
}

func TestProcess_NewShapeDataOnlySendsThePayloadBytes(t *testing.T) {
	h := newPayloadHarness(t)
	sub := newPayloadSubscriber(t)
	id, row := h.job(t, sub.url, true, deliveryOddPayload)
	require.False(t, row.Offloaded())

	code, out := callProcess(t, h.base, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])
	assert.Equal(t, deliveryOddPayload, sub.last(), "byte for byte the payload row's bytes")
	status, _, _ := jobRow(t, h.pool, id)
	assert.Equal(t, "COMPLETED", status)
	assert.Zero(t, h.store.Gets(), "a payload held in the database needs no store")
}

func TestProcess_NewShapeEnvelopeCarriesTheData(t *testing.T) {
	h := newPayloadHarness(t)
	sub := newPayloadSubscriber(t)
	id, _ := h.job(t, sub.url, false, `{"order":"SO-1","lines":[1,2]}`)

	code, _ := callProcess(t, h.base, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(sub.last()), &env))
	assert.Equal(t, id, env["id"])
	assert.Equal(t, map[string]any{"order": "SO-1", "lines": []any{float64(1), float64(2)}}, env["data"])
}

func TestProcess_NewShapeOffloadedIsResolved(t *testing.T) {
	h := newPayloadHarness(t)
	sub := newPayloadSubscriber(t)
	payload := `{"z": 1, "pad":"` + strings.Repeat("d", 3*payloadtest.OffloadMinBytes) + `" ,"a":2}`
	id, row := h.job(t, sub.url, true, payload)
	require.True(t, row.Offloaded())

	code, out := callProcess(t, h.base, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])
	assert.Equal(t, payload, sub.last())
	assert.EqualValues(t, 1, h.store.Gets())
	status, attempts, _ := jobRow(t, h.pool, id)
	assert.Equal(t, "COMPLETED", status)
	assert.EqualValues(t, 1, attempts)
}

// The store cannot be read: a platform fault. No attempt is recorded or
// counted, the job is left as it was, nothing is sent, and the answer is the
// one a failed load gets (503, ack=false) so the queue redelivers.
func TestProcess_StoreUnavailableIsRedelivered(t *testing.T) {
	h := newPayloadHarness(t)
	sub := newPayloadSubscriber(t)
	payload := `{"pad":"` + strings.Repeat("u", 3*payloadtest.OffloadMinBytes) + `"}`
	id, _ := h.job(t, sub.url, true, payload)
	h.store.FailReads(errors.New("connection refused"))

	code, out := callProcess(t, h.base, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, false, out["ack"])
	assert.Equal(t, "payload store unavailable", out["message"])
	status, attempts, scheduled := jobRow(t, h.pool, id)
	assert.Equal(t, "QUEUED", status, "not claimed, not failed")
	assert.EqualValues(t, 0, attempts, "no retry budget spent")
	assert.Nil(t, scheduled)
	assert.Zero(t, attemptCount(t, h.pool, id), "no attempt recorded")
	assert.Zero(t, sub.calls.Load(), "nothing sent")

	// The store is back: the redelivery delivers.
	h.store.FailReads(nil)
	code, _ = callProcess(t, h.base, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, payload, sub.last())
	status, _, _ = jobRow(t, h.pool, id)
	assert.Equal(t, "COMPLETED", status)
}

// No store configured but the payload is offloaded: also a platform fault,
// said plainly.
func TestProcess_StoreNotConfiguredIsRedelivered(t *testing.T) {
	h := newPayloadHarness(t)
	sub := newPayloadSubscriber(t)
	payload := `{"pad":"` + strings.Repeat("n", 3*payloadtest.OffloadMinBytes) + `"}`
	id, _ := h.job(t, sub.url, true, payload)

	bare := processing.New(dispatchjob.NewRepository(h.pool), h.auth)
	r := chi.NewRouter()
	bare.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	code, out := callProcess(t, ts.URL, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, false, out["ack"])
	assert.Equal(t, "payload store not configured", out["message"])
	status, attempts, _ := jobRow(t, h.pool, id)
	assert.Equal(t, "QUEUED", status)
	assert.EqualValues(t, 0, attempts)
	assert.Zero(t, attemptCount(t, h.pool, id))
	assert.Zero(t, sub.calls.Load())
}

// The payload is gone: the job fails, terminally, with PAYLOAD_MISSING, and is
// not retried; nothing is sent and no attempt is made.
func TestProcess_PayloadMissingFailsTheJob(t *testing.T) {
	h := newPayloadHarness(t)
	sub := newPayloadSubscriber(t)
	payload := `{"pad":"` + strings.Repeat("m", 3*payloadtest.OffloadMinBytes) + `"}`
	id, row := h.job(t, sub.url, true, payload)
	h.store.Delete(row.Ref.Key)

	code, out := callProcess(t, h.base, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"], "acked: a redelivery cannot bring the payload back")
	assert.Equal(t, "payload missing", out["message"])

	var status string
	var attempts int32
	var scheduled *time.Time
	var lastError *string
	var completed *time.Time
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT status, attempt_count, scheduled_for, last_error, completed_at FROM msg_dispatch_jobs WHERE id = $1`, id).
		Scan(&status, &attempts, &scheduled, &lastError, &completed))
	assert.Equal(t, "FAILED", status)
	require.NotNil(t, lastError)
	assert.True(t, strings.HasPrefix(*lastError, processing.PayloadMissingPrefix), *lastError)
	assert.NotContains(t, *lastError, "mmmm", "no payload bytes in the last error")
	assert.Nil(t, scheduled, "no retry scheduled")
	assert.NotNil(t, completed)
	assert.EqualValues(t, 0, attempts, "no attempt counted: nothing was sent")
	assert.Zero(t, attemptCount(t, h.pool, id))
	assert.Zero(t, sub.calls.Load(), "no HTTP call to the subscriber")

	// A redelivery of the same message finds the job settled.
	code, out = callProcess(t, h.base, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])
	assert.Zero(t, sub.calls.Load())
}

// The delivery plan (the sign action) shows the resolved body; a missing
// payload is a typed error, not a plan with an empty body.
func TestPlan_NewShapeResolvesThePayload(t *testing.T) {
	h := newPayloadHarness(t)
	payload := `{"z": 1, "pad":"` + strings.Repeat("p", 3*payloadtest.OffloadMinBytes) + `"}`
	id, row := h.job(t, "http://subscriber.invalid/hook", true, payload)

	j, err := h.repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	plan, err := h.handler.Plan(context.Background(), j)
	require.NoError(t, err)
	assert.Equal(t, payload, plan.Body)

	h.store.Delete(row.Ref.Key)
	j, err = h.repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	_, err = h.handler.Plan(context.Background(), j)
	ue, ok := errors.AsType[*usecase.Error](err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, processing.PayloadMissingPrefix, ue.Code)
}
