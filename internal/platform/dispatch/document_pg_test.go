//go:build integration

package dispatch_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// seedPool inserts a dispatch pool. client identifier nil = platform-level.
func seedPool(t *testing.T, pool *pgxpool.Pool, id, code string, clientIdentifier *string, concurrency int32, rateLimit *int32) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_pools (id, code, name, client_identifier, concurrency, rate_limit)
		 VALUES ($1, $2, $2, $3, $4, $5) ON CONFLICT (id) DO NOTHING`,
		id, code, clientIdentifier, concurrency, rateLimit)
	require.NoError(t, err)
}

// seedSubscription inserts a subscription with a stored queue value.
func seedSubscription(t *testing.T, pool *pgxpool.Pool, id, code string, clientIdentifier *string, status string, queue *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_subscriptions (id, code, name, target, client_identifier, status, queue, mode, source)
		 VALUES ($1, $2, $2, 'https://doc.example.test/hook', $3, $4, $5, 'NEXT_ON_ERROR', 'UI')
		 ON CONFLICT (id) DO NOTHING`,
		id, code, clientIdentifier, status, queue)
	require.NoError(t, err)
}

// queueNamed finds a queue by name. The fixture is shared with every other
// test in the suite, so the document legitimately contains other tenants'
// rows: assertions look up what this test seeded rather than asserting
// table-wide.
func queueNamed(doc common.RouterConfig, name string) *common.QueueConfig {
	for i := range doc.Queues {
		if doc.Queues[i].Name == name {
			return &doc.Queues[i]
		}
	}
	return nil
}

func poolCoded(doc common.RouterConfig, code string) *common.PoolConfig {
	for i := range doc.ProcessingPools {
		if doc.ProcessingPools[i].Code == code {
			return &doc.ProcessingPools[i]
		}
	}
	return nil
}

func TestDocument_PoolsAreNamespacedAndPrioritiesDriveQueues(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	acme := "docacme"
	globex := "docglobex"
	rate := int32(600)
	seedPool(t, pool, "dsp_doc_acme", "DOCFAST", &acme, 7, &rate)
	seedPool(t, pool, "dsp_doc_platform", "DOCWIDE", nil, 3, nil)
	// globex has only a subscription, no pool of its own — it still gets a
	// DEFAULT queue, because it has dispatch work.
	seedSubscription(t, pool, "sub_doc_globex", "doc-globex", &globex, "ACTIVE", nil)
	// acme asks for the expedited lane.
	seedSubscription(t, pool, "sub_doc_acme_hi", "doc-acme-hi", &acme, "ACTIVE", strPtr("HIGH_PRIORITY"))

	settings, err := dispatch.ResolveSettings("SQS", sqsURL, "", "FC-test", "")
	require.NoError(t, err)
	doc, err := dispatch.NewDocumentBuilder(pool, settings).Build(ctx)
	require.NoError(t, err)

	// Pools carry the code the scheduler stamps, platform-level included.
	acmePool := poolCoded(doc, "docacme-DOCFAST")
	require.NotNil(t, acmePool, "a client-owned pool is namespaced by its client")
	assert.Equal(t, uint32(7), acmePool.Concurrency)
	require.NotNil(t, acmePool.RateLimitPerMinute)
	assert.Equal(t, uint32(600), *acmePool.RateLimitPerMinute)

	platformPool := poolCoded(doc, "platform-DOCWIDE")
	require.NotNil(t, platformPool, "a platform-level pool takes the platform- prefix")
	assert.Nil(t, platformPool.RateLimitPerMinute, "no rate limit is an absent field, i.e. unlimited")

	// The document deliberately carries no *-DEFAULT-POOL rows: the router
	// synthesises those on demand.
	assert.Nil(t, poolCoded(doc, "platform-DEFAULT-POOL"))
	assert.Nil(t, poolCoded(doc, "docacme-DEFAULT-POOL"))

	// Queues: every tenant with work gets DEFAULT; only acme asked for the
	// expedited lane. The platform tenant always qualifies.
	require.NotNil(t, queueNamed(doc, "FC-test-platform-DEFAULT.fifo"))
	require.NotNil(t, queueNamed(doc, "FC-test-docacme-DEFAULT.fifo"))
	require.NotNil(t, queueNamed(doc, "FC-test-docglobex-DEFAULT.fifo"))
	require.NotNil(t, queueNamed(doc, "FC-test-docacme-HIGH_PRIORITY.fifo"))
	assert.Nil(t, queueNamed(doc, "FC-test-docglobex-HIGH_PRIORITY.fifo"),
		"a tenant with no HIGH_PRIORITY subscription gets no HIGH_PRIORITY queue")

	q := queueNamed(doc, "FC-test-docacme-DEFAULT.fifo")
	assert.Equal(t, "https://sqs.eu-west-1.amazonaws.com/123456789012/FC-test-docacme-DEFAULT.fifo", q.URI)
}

// Legacy and unusable stored values must not open the expedited lane: they
// read as DEFAULT on the publish path, so advertising a HIGH_PRIORITY queue
// for them would create a queue nothing ever publishes to.
func TestDocument_UnusableQueueValuesDoNotCreateHighPriorityQueues(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	tenant := "doclegacy"
	seedSubscription(t, pool, "sub_doc_legacy", "doc-legacy", &tenant, "ACTIVE", strPtr("workers-high"))
	seedSubscription(t, pool, "sub_doc_blank", "doc-blank", &tenant, "ACTIVE", strPtr("   "))

	settings, err := dispatch.ResolveSettings("SQS", sqsURL, "", "FC-test", "")
	require.NoError(t, err)
	doc, err := dispatch.NewDocumentBuilder(pool, settings).Build(ctx)
	require.NoError(t, err)

	require.NotNil(t, queueNamed(doc, "FC-test-doclegacy-DEFAULT.fifo"))
	assert.Nil(t, queueNamed(doc, "FC-test-doclegacy-HIGH_PRIORITY.fifo"))
}

// A paused subscription is not dispatch work: it must not be the only reason a
// tenant appears. (A tenant owning a pool still appears, whatever its status —
// pool status governs nothing downstream.)
func TestDocument_OnlyActiveSubscriptionsCountAsWork(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	tenant := "docpaused"
	seedSubscription(t, pool, "sub_doc_paused", "doc-paused", &tenant, "PAUSED", strPtr("HIGH_PRIORITY"))

	settings, err := dispatch.ResolveSettings("SQS", sqsURL, "", "FC-test", "")
	require.NoError(t, err)
	doc, err := dispatch.NewDocumentBuilder(pool, settings).Build(ctx)
	require.NoError(t, err)

	assert.Nil(t, queueNamed(doc, "FC-test-docpaused-DEFAULT.fifo"))
	assert.Nil(t, queueNamed(doc, "FC-test-docpaused-HIGH_PRIORITY.fifo"))
}

// In dev the same document names Postgres-backed queues: same names minus the
// .fifo suffix, every one addressed at the platform's own database. The queue
// TYPE is the only dev/prod difference.
func TestDocument_PostgresFlavour(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	settings, err := dispatch.ResolveSettings("postgres", "", "", "", "postgresql://u@localhost:5432/fc")
	require.NoError(t, err)
	doc, err := dispatch.NewDocumentBuilder(pool, settings).Build(ctx)
	require.NoError(t, err)

	q := queueNamed(doc, "platform-DEFAULT")
	require.NotNil(t, q, "an unprefixed Postgres name, no .fifo suffix")
	assert.Equal(t, "postgres://u@localhost:5432/fc", q.URI)
}

func strPtr(s string) *string { return &s }
