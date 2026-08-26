//go:build integration

package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// msgCapturePublisher keeps the whole published message, not just its id, so a
// test can assert what actually goes on the wire.
type msgCapturePublisher struct {
	mu   sync.Mutex
	msgs []common.Message
}

func (p *msgCapturePublisher) Identifier() string { return "msg-capture" }

func (p *msgCapturePublisher) Publish(_ context.Context, m common.Message) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, m)
	return m.ID, nil
}

func (p *msgCapturePublisher) PublishBatch(ctx context.Context, msgs []common.Message) ([]string, error) {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		id, _ := p.Publish(ctx, m)
		out = append(out, id)
	}
	return out, nil
}

func (p *msgCapturePublisher) captured() []common.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]common.Message(nil), p.msgs...)
}

// seedRoutableJob inserts a PENDING job carrying the routing inputs: its
// dispatch pool and owning client. Both are nullable, which is what makes the
// resolution chain have four branches.
func seedRoutableJob(t *testing.T, pool *pgxpool.Pool, id, mode string, poolID, clientID *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs
		    (id, code, target_url, status, mode, dispatch_pool_id, client_id)
		 VALUES ($1, 'app:evt', 'http://sub.example/hook', 'PENDING', $2, $3, $4)`,
		id, mode, poolID, clientID)
	require.NoError(t, err)
}

func pollAndCapture(t *testing.T, pool *pgxpool.Pool) []common.Message {
	t.Helper()
	pub := &msgCapturePublisher{}
	dispatcher := NewMessageGroupDispatcher(pool, pub,
		NewDispatchAuthService("test-secret"), "http://localhost:8080/api/dispatch/process")
	poller := NewPendingJobPoller(DefaultConfig(), pool, dispatcher,
		NewPausedConnectionCache(pool, time.Minute))

	require.NoError(t, poller.pollOnce(context.Background()))
	return pub.captured()
}

// TestPublishedMessageCarriesResolvedPoolCode drives the whole path a job takes:
// claim query → resolver → token → buildMessage. It is the end-to-end form of
// the ruling, and the one that would have caught the earlier attempt — which
// selected a dispatch_pool_code column that does not exist on
// msg_dispatch_jobs, so pollOnce failed every tick and nothing was published.
func TestPublishedMessageCarriesResolvedPoolCode(t *testing.T) {
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_e2e_acme", "acme-e2e")
	insertPool(t, pool, "dsp_e2e_fast", "FAST", ptr("clt_e2e_acme"), ptr("acme-e2e"))
	seedRoutableJob(t, pool, "dje2epool001", "BLOCK_ON_ERROR",
		ptr("dsp_e2e_fast"), ptr("clt_e2e_acme"))

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1)
	require.Equal(t, "acme-e2e-FAST", msgs[0].PoolCode,
		"the published code must be the client-namespaced pool code")
	require.Equal(t, common.DispatchBlockOnError, msgs[0].DispatchMode,
		"mode must travel with it — a partial fix leaves everything unordered")
}

// TestPublishedPoolCodeFallsBackToClientDefault: a job with no dispatch pool
// still gets its client's own fallback pool rather than the shared global one.
func TestPublishedPoolCodeFallsBackToClientDefault(t *testing.T) {
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_e2e_nopool", "nopool-e2e")
	seedRoutableJob(t, pool, "dje2epool002", "IMMEDIATE", nil, ptr("clt_e2e_nopool"))

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1)
	require.Equal(t, "nopool-e2e-DEFAULT-POOL", msgs[0].PoolCode)
}

// TestPublishedPoolCodeIsGlobalDefaultWithNeither preserves the pre-existing
// behaviour for a platform-level job: the bare global fallback.
func TestPublishedPoolCodeIsGlobalDefaultWithNeither(t *testing.T) {
	pool := testpg.Pool(t)

	seedRoutableJob(t, pool, "dje2epool003", "IMMEDIATE", nil, nil)

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1)
	require.Equal(t, "DEFAULT-POOL", msgs[0].PoolCode)
}

// TestPublishedPoolCodeForPlatformPoolHasNoPrefix: a pool with no owning client
// publishes its bare code — prefixing it would invent a client that isn't there.
func TestPublishedPoolCodeForPlatformPoolHasNoPrefix(t *testing.T) {
	pool := testpg.Pool(t)

	insertPool(t, pool, "dsp_e2e_platform", "PLATFORM-POOL", nil, nil)
	seedRoutableJob(t, pool, "dje2epool004", "IMMEDIATE", ptr("dsp_e2e_platform"), nil)

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1)
	require.Equal(t, "PLATFORM-POOL", msgs[0].PoolCode)
}

// TestUnknownModePublishesAsTheDefault matches the poller's own parse: an
// unrecognised mode string is a producer bug, and it must not silently become
// the one mode that abandons ordering. It takes the default, which orders.
func TestUnknownModePublishesAsTheDefault(t *testing.T) {
	pool := testpg.Pool(t)

	seedRoutableJob(t, pool, "dje2epool005", "NOT_A_REAL_MODE", nil, nil)

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1)
	require.Equal(t, common.DefaultDispatchMode, msgs[0].DispatchMode)
	require.True(t, msgs[0].DispatchMode.RequiresOrdering())
}
