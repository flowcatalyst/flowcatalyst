//go:build integration

package processing_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// F31: attempt_count is the number of delivery attempts made, so a job
// delivered on its first attempt reports 1, not 0. It was bumped only when a
// retry was scheduled, so a completed (or failed) job always read one lower
// than the attempt rows it had, and 0 for the common case.
func TestProcess_AttemptCountCountsTheTerminalAttempt(t *testing.T) {
	pool := testpg.Pool(t)
	base, auth := harness(t, pool)

	var calls atomic.Int32
	failFirst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(failFirst.Close)
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(ok.Close)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	t.Cleanup(bad.Close)

	t.Run("delivered on the first attempt", func(t *testing.T) {
		seedJob(t, pool, "djf31_first", ok.URL, 3, 0)
		callProcess(t, base, "djf31_first", auth.Sign("djf31_first"))
		status, attempts, _ := jobRow(t, pool, "djf31_first")
		assert.Equal(t, "COMPLETED", status)
		assert.EqualValues(t, 1, attempts)
		assert.Equal(t, attemptCount(t, pool, "djf31_first"), int(attempts), "the count equals the attempt rows")
	})

	t.Run("one retry then delivered", func(t *testing.T) {
		seedJob(t, pool, "djf31_retry", failFirst.URL, 3, 0)
		callProcess(t, base, "djf31_retry", auth.Sign("djf31_retry"))
		status, attempts, _ := jobRow(t, pool, "djf31_retry")
		assert.Equal(t, "PENDING", status)
		assert.EqualValues(t, 1, attempts, "a scheduled retry counts its failed attempt")

		// The poller would pick it up once due; make it deliverable now.
		_, err := pool.Exec(t.Context(), `UPDATE msg_dispatch_jobs SET status = 'QUEUED', scheduled_for = NULL WHERE id = 'djf31_retry'`)
		require.NoError(t, err)
		callProcess(t, base, "djf31_retry", auth.Sign("djf31_retry"))
		status, attempts, _ = jobRow(t, pool, "djf31_retry")
		assert.Equal(t, "COMPLETED", status)
		assert.EqualValues(t, 2, attempts)
		assert.Equal(t, 2, attemptCount(t, pool, "djf31_retry"))
	})

	t.Run("retries exhausted", func(t *testing.T) {
		seedJob(t, pool, "djf31_fail", bad.URL, 2, 1)
		callProcess(t, base, "djf31_fail", auth.Sign("djf31_fail"))
		status, attempts, _ := jobRow(t, pool, "djf31_fail")
		assert.Equal(t, "FAILED", status)
		assert.EqualValues(t, 2, attempts, "one earlier attempt plus the terminal one")
	})
}
