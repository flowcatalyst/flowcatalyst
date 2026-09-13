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

func (p *msgCapturePublisher) Publish(_ context.Context, items []PublishItem) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		p.msgs = append(p.msgs, item.Message)
	}
	return nil, nil
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
	insertPool(t, pool, "dsp_e2e_fast", "FAST", new("clt_e2e_acme"), new("acme-e2e"))
	seedRoutableJob(t, pool, "dje2epool001", "BLOCK_ON_ERROR",
		new("dsp_e2e_fast"), new("clt_e2e_acme"))

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
	seedRoutableJob(t, pool, "dje2epool002", "IMMEDIATE", nil, new("clt_e2e_nopool"))

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1)
	require.Equal(t, "nopool-e2e-DEFAULT-POOL", msgs[0].PoolCode)
}

// TestPublishedPoolCodeIsPlatformDefaultWithNeither: a job with neither a pool
// nor a resolvable client publishes the PLATFORM tenant's default pool, not the
// bare global one. That code self-synthesises through the router's
// -DEFAULT-POOL suffix rule exactly like any client's fallback, so the platform
// tenant needs no row in the served document.
func TestPublishedPoolCodeIsPlatformDefaultWithNeither(t *testing.T) {
	pool := testpg.Pool(t)

	seedRoutableJob(t, pool, "dje2epool003", "IMMEDIATE", nil, nil)

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1)
	require.Equal(t, "platform-DEFAULT-POOL", msgs[0].PoolCode)
}

// TestPublishedPoolCodeForPlatformPoolIsPrefixed: a pool with no owning client
// publishes platform-{code}. Publishing it bare was the earlier rule, and it is
// unsafe once the router merges this platform's document with other config
// sources: pools merge by code, first definition winning, so a bare WEBHOOKS
// would silently inherit another tenant's pool of that name — differing
// concurrency and rate limit, with only a merge-conflict log line to show it.
func TestPublishedPoolCodeForPlatformPoolIsPrefixed(t *testing.T) {
	pool := testpg.Pool(t)

	insertPool(t, pool, "dsp_e2e_platform", "PLATFORM-POOL", nil, nil)
	seedRoutableJob(t, pool, "dje2epool004", "IMMEDIATE", new("dsp_e2e_platform"), nil)

	msgs := pollAndCapture(t, pool)

	require.Len(t, msgs, 1)
	require.Equal(t, "platform-PLATFORM-POOL", msgs[0].PoolCode)
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
