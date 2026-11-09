//go:build integration

package processing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Owner rulings 2026-11-09, enforced by the delivery callback: expiry,
// not-before, held pools, the event's own group in the envelope, the retry
// strategy, and a subscriber's own notBefore / HTTP-date Retry-After.

// countingSubscriber answers every delivery with fn and counts the calls.
type countingSubscriber struct {
	calls atomic.Int32
	mu    sync.Mutex
	last  []byte
	srv   *httptest.Server
}

func newSubscriber(t *testing.T, fn func(w http.ResponseWriter)) *countingSubscriber {
	t.Helper()
	s := &countingSubscriber{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.last = b
		s.mu.Unlock()
		s.calls.Add(1)
		fn(w)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func ok200(w http.ResponseWriter) { w.WriteHeader(http.StatusOK) }

// seedRulingJob inserts a QUEUED job; set overrides columns by name.
func seedRulingJob(t *testing.T, pool *pgxpool.Pool, target string, set map[string]any) string {
	t.Helper()
	id := tsid.GenerateUntyped()
	cols := []string{"id", "code", "target_url", "status", "data_only", "payload", "max_retries", "updated_at"}
	vals := []any{id, "proc:rulings:evt", target, "QUEUED", false, `{"hello":"world"}`, 3, time.Now().UTC().Add(-time.Hour)}
	for k, v := range set {
		replaced := false
		for i, c := range cols {
			if c == k {
				vals[i], replaced = v, true
			}
		}
		if !replaced {
			cols, vals = append(cols, k), append(vals, v)
		}
	}
	ph := make([]string, len(cols))
	for i := range cols {
		ph[i] = fmt.Sprintf("$%d", i+1)
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(ph, ", ")+`)`, vals...)
	require.NoError(t, err)
	return id
}

type jobState struct {
	status       string
	attempts     int32
	scheduledFor *time.Time
	updatedAt    time.Time
	lastError    *string
}

func readJob(t *testing.T, pool *pgxpool.Pool, id string) jobState {
	t.Helper()
	var s jobState
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, attempt_count, scheduled_for, updated_at, last_error FROM msg_dispatch_jobs WHERE id = $1`, id).
		Scan(&s.status, &s.attempts, &s.scheduledFor, &s.updatedAt, &s.lastError))
	return s
}

// harnessWithPools is harness plus the held-pool resolver production wires.
func harnessWithPools(t *testing.T, pool *pgxpool.Pool) (string, *scheduler.DispatchAuthService) {
	t.Helper()
	auth := scheduler.NewDispatchAuthService(testSecret)
	h := processing.New(dispatchjob.NewRepository(pool), auth).
		WithPoolHeldResolver(dispatchpool.NewHeldPoolCache(dispatchpool.NewRepository(pool), 0).IsHeld)
	r := chi.NewRouter()
	h.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts.URL, auth
}

// A job reached after its expires_at (the subscription's maxAgeSeconds, made
// absolute at fan-out) is settled EXPIRED, acked, and never delivered.
func TestProcess_ExpiredJobIsSettledExpiredNotDelivered(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)
	sub := newSubscriber(t, ok200)

	id := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"expires_at": time.Now().UTC().Add(-time.Second)})
	code, out := callProcess(t, base, id, auth.Sign(id))

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"], "the message is dropped")
	assert.Equal(t, int32(0), sub.calls.Load(), "an expired job is not delivered")
	s := readJob(t, pool, id)
	assert.Equal(t, "EXPIRED", s.status)
	assert.Equal(t, int32(0), s.attempts, "nothing was sent: no attempt counted")
	require.NotNil(t, s.lastError)
	assert.Contains(t, *s.lastError, "expired")
	assert.Equal(t, 0, attemptCount(t, pool, id))

	// Not yet expired: delivered.
	live := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"expires_at": time.Now().UTC().Add(time.Hour)})
	code, out = callProcess(t, base, live, auth.Sign(live))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])
	assert.Equal(t, int32(1), sub.calls.Load())
	assert.Equal(t, "COMPLETED", readJob(t, pool, live).status)
}

// A job reached before its not_before is handed back to the router with WHEN
// to come back (notBefore + delaySeconds), stays QUEUED, and has its
// updated_at stamped so stale recovery does not republish it. A long delay is
// served at most notBeforeRecheck (5 min) per answer.
func TestProcess_NotBeforeAnswersTheRouterWithTheTime(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)
	sub := newSubscriber(t, ok200)

	notBefore := time.Now().UTC().Add(90 * time.Second).Truncate(time.Millisecond)
	id := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"not_before": notBefore})
	before := readJob(t, pool, id)
	code, out := callProcess(t, base, id, auth.Sign(id))

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, out["ack"], "the router keeps the message")
	at, err := time.Parse(time.RFC3339Nano, fmt.Sprint(out["notBefore"]))
	require.NoError(t, err, "notBefore is RFC 3339")
	assert.True(t, at.Equal(notBefore), "come back exactly at not_before: got %s want %s", at, notBefore)
	secs, _ := out["delaySeconds"].(float64)
	assert.InDelta(t, 90, secs, 1, "delaySeconds is the same instant for an older router")
	assert.Equal(t, int32(0), sub.calls.Load(), "not delivered before its time")
	after := readJob(t, pool, id)
	assert.Equal(t, "QUEUED", after.status)
	assert.True(t, after.updatedAt.After(before.updatedAt), "stamped, so stale recovery leaves it alone")
	assert.Equal(t, int32(0), after.attempts)

	// A delay far ahead is answered in holds of at most 5 minutes.
	far := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"not_before": time.Now().UTC().Add(2 * time.Hour)})
	_, out = callProcess(t, base, far, auth.Sign(far))
	secs, _ = out["delaySeconds"].(float64)
	assert.InDelta(t, 300, secs, 1)
	at, err = time.Parse(time.RFC3339Nano, fmt.Sprint(out["notBefore"]))
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(5*time.Minute), at, 5*time.Second)

	// Once due, it is delivered.
	due := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"not_before": time.Now().UTC().Add(-time.Second)})
	_, out = callProcess(t, base, due, auth.Sign(due))
	assert.Equal(t, true, out["ack"])
	assert.Equal(t, int32(1), sub.calls.Load())
	assert.Equal(t, "COMPLETED", readJob(t, pool, due).status)
}

// A job of a SUSPENDED (or ARCHIVED) pool already on the queue goes back to
// PENDING undelivered, its message acked; an ACTIVE pool's job delivers.
func TestProcess_HeldPoolReturnsTheJobToPending(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harnessWithPools(t, pool)
	sub := newSubscriber(t, ok200)

	seed := func(status string) string {
		id := tsid.Generate(tsid.DispatchPool)
		_, err := pool.Exec(context.Background(),
			`INSERT INTO msg_dispatch_pools (id, code, name, status) VALUES ($1, $2, $2, $3)`, id, "proc-"+strings.ToLower(id), status)
		require.NoError(t, err)
		return id
	}
	for _, status := range []string{"SUSPENDED", "ARCHIVED"} {
		id := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"dispatch_pool_id": seed(status)})
		code, out := callProcess(t, base, id, auth.Sign(id))
		assert.Equal(t, http.StatusOK, code, status)
		assert.Equal(t, true, out["ack"], status)
		s := readJob(t, pool, id)
		assert.Equal(t, "PENDING", s.status, "%s: it waits", status)
		assert.Equal(t, int32(0), s.attempts, status)
	}
	assert.Equal(t, int32(0), sub.calls.Load(), "a held pool delivers nothing")

	active := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"dispatch_pool_id": seed("ACTIVE")})
	_, out := callProcess(t, base, active, auth.Sign(active))
	assert.Equal(t, true, out["ack"])
	assert.Equal(t, int32(1), sub.calls.Load())
}

// The envelope's messageGroup is the event's own group, not the
// per-subscription key the job is ordered by — including the digest form,
// whose event group is read from the event.
func TestProcess_EnvelopeCarriesTheEventsOwnGroup(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)
	sub := newSubscriber(t, ok200)
	subID := tsid.Generate(tsid.Subscription)

	envelopeGroup := func(id string) any {
		_, out := callProcess(t, base, id, auth.Sign(id))
		require.Equal(t, true, out["ack"])
		sub.mu.Lock()
		defer sub.mu.Unlock()
		var env map[string]any
		require.NoError(t, json.Unmarshal(sub.last, &env))
		return env["messageGroup"]
	}

	g := dispatchjob.SubscriptionMessageGroup(subID, new("order-42"))
	id := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"subscription_id": subID, "message_group": *g})
	assert.Equal(t, "order-42", envelopeGroup(id))

	// An event group too long for the column: the job holds the digest.
	long := strings.Repeat("x", 190)
	evID := tsid.GenerateUntyped()
	created := time.Now().UTC().Truncate(time.Microsecond)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_events (id, type, source, time, data, message_group, created_at)
		 VALUES ($1, 'proc:rulings:evt', 'proc:rulings', $2, '{}', $3, $2)`, evID, created, long)
	require.NoError(t, err)
	dg := dispatchjob.SubscriptionMessageGroup(subID, &long)
	require.LessOrEqual(t, len(*dg), dispatchjob.MaxMessageGroupLen)
	id = seedRulingJob(t, pool, sub.srv.URL, map[string]any{
		"subscription_id": subID, "message_group": *dg, "event_id": evID, "created_at": created,
	})
	assert.Equal(t, long, envelopeGroup(id))
}

// retryStrategy decides the wait after a retryable failure: fixed 30s,
// immediate none, exponential (default) 5s after the first attempt.
func TestProcess_RetryStrategyDecidesTheBackoff(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)
	sub := newSubscriber(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) })

	for _, tc := range []struct {
		strategy string
		wait     time.Duration
	}{{"fixed", 30 * time.Second}, {"immediate", 0}, {"exponential", 5 * time.Second}} {
		id := seedRulingJob(t, pool, sub.srv.URL, map[string]any{"retry_strategy": tc.strategy})
		start := time.Now()
		_, out := callProcess(t, base, id, auth.Sign(id))
		require.Equal(t, true, out["ack"])
		s := readJob(t, pool, id)
		assert.Equal(t, "PENDING", s.status, tc.strategy)
		assert.Equal(t, int32(1), s.attempts, tc.strategy)
		require.NotNil(t, s.scheduledFor, tc.strategy)
		assert.WithinDuration(t, start.Add(tc.wait), *s.scheduledFor, 3*time.Second, tc.strategy)
	}
}

// A subscriber's own deferral may say WHEN: {"ack":false,"notBefore":t}
// reschedules the job to t; a 429 with an HTTP-date Retry-After to that date.
// Neither spends the retry budget.
func TestProcess_SubscriberDeferralNotBeforeAndRetryAfterDate(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	when := time.Now().UTC().Add(40 * time.Second).Truncate(time.Second)
	defers := newSubscriber(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"ack":false,"notBefore":%q,"delaySeconds":5}`, when.Format(time.RFC3339))
	})
	id := seedRulingJob(t, pool, defers.srv.URL, nil)
	_, out := callProcess(t, base, id, auth.Sign(id))
	require.Equal(t, true, out["ack"])
	s := readJob(t, pool, id)
	assert.Equal(t, "PENDING", s.status)
	assert.Equal(t, int32(0), s.attempts)
	require.NotNil(t, s.scheduledFor)
	assert.WithinDuration(t, when, *s.scheduledFor, 2*time.Second, "notBefore wins over delaySeconds")

	limited := newSubscriber(t, func(w http.ResponseWriter) {
		w.Header().Set("Retry-After", when.Add(20*time.Second).Format(http.TimeFormat))
		w.WriteHeader(http.StatusTooManyRequests)
	})
	id = seedRulingJob(t, pool, limited.srv.URL, nil)
	_, out = callProcess(t, base, id, auth.Sign(id))
	require.Equal(t, true, out["ack"])
	s = readJob(t, pool, id)
	assert.Equal(t, int32(0), s.attempts)
	require.NotNil(t, s.scheduledFor)
	assert.WithinDuration(t, when.Add(20*time.Second), *s.scheduledFor, 2*time.Second, "an HTTP-date Retry-After is honoured")
}
