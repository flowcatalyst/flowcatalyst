//go:build integration

package operations_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
)

// A function's subscription, written by the real promote wiring, still reaches
// the runner under the split delivery policies: the job fanned out for it is
// recognised from the stored subscription and dialled through the internal
// policy (which exempts the runner), while the same job delivered with no
// function recognition goes through the customer policy and is refused.
func TestPromote_FunctionSubscriptionJobStillReachesTheRunner(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var paths []string
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(runner.Close)

	f := newTestFunctionFixture(t, "app_optdlvpol0001", "optdlvpolapp", "dlvpol-fn")
	f.wiring.RunnerURLTemplate = runner.URL
	f.seedActiveServiceAccount("optdlvpolapp-sa")
	res := f.publish(putArtifactBytes(t, f.artifacts, jsArtifact(jsDescribeWithSub("optdlvpolapp:orders:order:created"))), runtimes.JS)
	f.markReady(res.Version.Number)
	require.NoError(t, setConfig(f, "GREETING", "hi"))
	_, err := f.putAlias("live", res.Version.Number)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = f.pool.Exec(ctx, `DELETE FROM msg_dispatch_pools WHERE code = $1`, "fn-optdlvpolapp-dlvpol-fn")
	})

	subRepo := subscription.NewRepository(f.pool)
	subs, err := subRepo.FindByFunctionID(ctx, f.fn.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	sub := subs[0]
	u, err := url.Parse(sub.Endpoint)
	require.NoError(t, err)
	require.Equal(t, runner.Listener.Addr().String(), u.Host, "the wiring targets the runner")

	// What the fan-out writes for this subscription.
	seed := func(id string) {
		_, err := f.pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, subscription_id, status, data_only, payload, max_retries, attempt_count)
			 VALUES ($1, 'optdlvpolapp:orders:order:created', $2, $3, 'QUEUED', FALSE, '{}', 3, 0)`, id, sub.Endpoint, sub.ID)
		require.NoError(t, err)
	}
	status := func(id string) string {
		var s string
		require.NoError(t, f.pool.QueryRow(ctx, `SELECT status FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&s))
		return s
	}

	customer, internal := &netguard.Policy{}, &netguard.Policy{}
	internal.AllowURL(runner.URL)
	auth := scheduler.NewDispatchAuthService("wiring-policy-secret")
	mount := func(h *processing.Handler) string {
		r := chi.NewRouter()
		h.Mount(r)
		ts := httptest.NewServer(r)
		t.Cleanup(ts.Close)
		return ts.URL
	}
	post := func(base, id string) {
		req, err := http.NewRequest(http.MethodPost, base+"/api/dispatch/process", jsonBody(id))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+auth.Sign(id))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	// Real resolver, over the real subscription repository.
	withFn := mount(processing.New(dispatchjob.NewRepository(f.pool), auth).
		WithDeliveryPolicies(customer, internal).
		WithFunctionTargetResolver(processing.NewFunctionTargetResolver(subRepo.FindByID)))
	seed("djwpolfn00001")
	post(withFn, "djwpolfn00001")
	assert.Equal(t, "COMPLETED", status("djwpolfn00001"))
	mu.Lock()
	assert.Equal(t, []string{u.Path}, paths, "the runner received the delivery at the wired path")
	mu.Unlock()

	// No function recognition: the customer policy refuses the runner address.
	without := mount(processing.New(dispatchjob.NewRepository(f.pool), auth).WithDeliveryPolicies(customer, internal))
	seed("djwpolcu00001")
	post(without, "djwpolcu00001")
	assert.NotEqual(t, "COMPLETED", status("djwpolcu00001"))
	mu.Lock()
	assert.Len(t, paths, 1, "the refused delivery never reached the runner")
	mu.Unlock()
}

func jsonBody(id string) *strings.Reader { return strings.NewReader(`{"messageId":"` + id + `"}`) }
