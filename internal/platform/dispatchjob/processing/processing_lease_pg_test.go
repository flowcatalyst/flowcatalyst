//go:build integration

package processing_test

import (
	"context"
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
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// markProcessing puts a seeded job into PROCESSING, claimed claimedAgo ago —
// the state a platform killed mid-delivery leaves behind.
func markProcessing(t *testing.T, pool *pgxpool.Pool, id string, claimedAgo time.Duration) {
	t.Helper()
	at := time.Now().Add(-claimedAgo).UTC()
	_, err := pool.Exec(context.Background(),
		`UPDATE msg_dispatch_jobs SET status = 'PROCESSING', last_attempt_at = $2, updated_at = $2 WHERE id = $1`,
		id, at)
	require.NoError(t, err)
	testpg.SyncDispatchQueue(t, pool, id)
}

// A copy of a job whose delivery is still inside its lease must not be
// acked away: the router is asked to come back when the lease ends, and the
// subscriber is not called a second time.
func TestProcess_LostClaimInsideTheLeaseIsDeferred(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)
	var hits atomic.Int32
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)

	const id = "djlsalive001"
	seedJob(t, pool, id, sub.URL, 3, 0)
	markProcessing(t, pool, id, 5*time.Second)

	code, out := callProcess(t, base, id, auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, out["ack"], "the message must be kept while another attempt may be live")
	delay, ok := out["delaySeconds"].(float64)
	require.True(t, ok, "the deferral names when to come back")
	// Default timeout 30s + 30s margin, 5s already used.
	assert.InDelta(t, 55, delay, 2)
	assert.EqualValues(t, 0, hits.Load(), "no second delivery while the first may be live")
	status, _, _ := jobRow(t, pool, id)
	assert.Equal(t, "PROCESSING", status)
}

// A job left PROCESSING by an attempt that never finished (the platform was
// killed mid-delivery) used to stay PROCESSING for good: its next copy lost
// the claim and was acked away. Past the lease, that copy now takes the
// claim over and delivers.
func TestProcess_LostClaimPastTheLeaseTakesOver(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)
	var hits atomic.Int32
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)

	const id = "djlsdead0001"
	seedJob(t, pool, id, sub.URL, 3, 0)
	markProcessing(t, pool, id, 10*time.Minute)

	code, out := callProcess(t, base, id, auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])
	assert.EqualValues(t, 1, hits.Load(), "the dead attempt's job is delivered again")
	status, _, _ := jobRow(t, pool, id)
	assert.Equal(t, "COMPLETED", status)
}

// An internal error answers 503, never 500: every router ACKs a 500 away as
// the target's permanent answer (R-57), which stranded the job QUEUED.
func TestProcess_InternalErrorAnswers503(t *testing.T) {
	shared := testpg.Pool(t)
	cfg := shared.Config().Copy()
	closed, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	closed.Close() // every query now fails

	auth := scheduler.NewDispatchAuthService(testSecret)
	h := processing.New(dispatchjob.NewRepository(closed), auth)
	r := chi.NewRouter()
	h.Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	code, out := callProcess(t, ts.URL, "djlserr00001", auth.Sign("djlserr00001"))
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, false, out["ack"])
}

// A router that hangs up mid-call (it is restarting) no longer cancels the
// delivery half way: the attempt runs to the end and its outcome is written,
// so the job does not sit PROCESSING with the webhook already sent.
func TestProcess_RouterHangingUpDoesNotCutTheAttemptShort(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)

	const id = "djlshang0001"
	seedJob(t, pool, id, sub.URL, 3, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/dispatch/process",
		stringsReader(`{"messageId":"`+id+`"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+auth.Sign(id))
	_, err = http.DefaultClient.Do(req)
	require.Error(t, err, "the caller gave up before the delivery finished")

	assert.Eventually(t, func() bool {
		status, _, _ := jobRow(t, pool, id)
		return status == "COMPLETED"
	}, 3*time.Second, 20*time.Millisecond, "the attempt completes and is recorded")
	assert.Equal(t, 1, attemptCount(t, pool, id))
}

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }
