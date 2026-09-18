//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func insertSubscriptionWithQueue(t *testing.T, pool *pgxpool.Pool, id, code string, clientIdentifier, queue *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_subscriptions (id, code, name, target, client_identifier, status, queue, mode, source)
		 VALUES ($1, $2, $2, 'https://pub.example.test/hook', $3, 'ACTIVE', $4, 'NEXT_ON_ERROR', 'UI')
		 ON CONFLICT (id) DO NOTHING`,
		id, code, clientIdentifier, queue)
	require.NoError(t, err)
}

// queueNameOf reads back which row-queue a published job landed on.
func queueNameOf(t *testing.T, pool *pgxpool.Pool, jobID string) string {
	t.Helper()
	var name string
	err := pool.QueryRow(context.Background(),
		`SELECT queue_name FROM queue_messages WHERE id = $1`, jobID).Scan(&name)
	require.NoError(t, err, "job %s was never published", jobID)
	return name
}

// The dev publisher must route per (tenant, priority) exactly as the SQS one
// does. Publishing every job to one fixed queue named after the database was
// the old behaviour, and the moment the router consumes the platform's served
// document — which advertises composed names — that puts every dev dispatch
// job where nothing is listening.
func TestPostgresDispatchPublisher_RoutesPerTenantAndPriority(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_pub_acme", "pubacme")
	hi := "HIGH_PRIORITY"
	insertSubscriptionWithQueue(t, pool, "sub_pub_hi", "pub-hi", strPtrPub("pubacme"), &hi)
	insertSubscriptionWithQueue(t, pool, "sub_pub_lo", "pub-lo", strPtrPub("pubacme"), nil)
	// A legacy value nothing can write any more still has to route.
	legacy := "workers-high"
	insertSubscriptionWithQueue(t, pool, "sub_pub_legacy", "pub-legacy", strPtrPub("pubacme"), &legacy)

	settings, err := dispatch.ResolveSettings("postgres", "", "", "FC-dev", "postgresql://x@localhost/fc")
	require.NoError(t, err)
	destinations := NewDestinationResolver(
		NewPoolCodeResolver(pool, time.Minute),
		NewSubscriptionPriorityCache(pool, time.Minute),
		settings,
	)
	pub, err := NewPostgresDispatchPublisher(ctx, pool, destinations)
	require.NoError(t, err)

	items := []PublishItem{
		{JobID: "djpub001", ClientID: "clt_pub_acme", SubscriptionID: "sub_pub_hi",
			Message: common.Message{ID: "djpub001"}},
		{JobID: "djpub002", ClientID: "clt_pub_acme", SubscriptionID: "sub_pub_lo",
			Message: common.Message{ID: "djpub002"}},
		{JobID: "djpub003", ClientID: "clt_pub_acme", SubscriptionID: "sub_pub_legacy",
			Message: common.Message{ID: "djpub003"}},
		// Client-less: the platform tenant's lane.
		{JobID: "djpub004", Message: common.Message{ID: "djpub004"}},
		// A client that does not resolve falls back the same way.
		{JobID: "djpub005", ClientID: "clt_pub_missing", Message: common.Message{ID: "djpub005"}},
	}
	unpublished, err := pub.Publish(ctx, items)
	require.NoError(t, err)
	require.Empty(t, unpublished)

	assert.Equal(t, "FC-dev-pubacme-HIGH_PRIORITY", queueNameOf(t, pool, "djpub001"))
	assert.Equal(t, "FC-dev-pubacme-DEFAULT", queueNameOf(t, pool, "djpub002"))
	assert.Equal(t, "FC-dev-pubacme-DEFAULT", queueNameOf(t, pool, "djpub003"),
		"an unusable legacy value publishes as DEFAULT, never an error")
	assert.Equal(t, "FC-dev-platform-DEFAULT", queueNameOf(t, pool, "djpub004"))
	assert.Equal(t, "FC-dev-platform-DEFAULT", queueNameOf(t, pool, "djpub005"))
}

// The rows this publisher writes are the rows the router's own Postgres
// consumer claims: same table, same column mapping, so a message published
// here is consumable there.
func TestPostgresDispatchPublisher_WritesConsumableRows(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	settings, err := dispatch.ResolveSettings("postgres", "", "", "", "postgresql://x@localhost/fc")
	require.NoError(t, err)
	destinations := NewDestinationResolver(
		NewPoolCodeResolver(pool, time.Minute),
		NewSubscriptionPriorityCache(pool, time.Minute),
		settings,
	)
	pub, err := NewPostgresDispatchPublisher(ctx, pool, destinations)
	require.NoError(t, err)

	group := "grp-pub"
	_, err = pub.Publish(ctx, []PublishItem{{
		JobID: "djpub010",
		Message: common.Message{
			ID:              "djpub010",
			MediationType:   common.MediationTypeHTTP,
			MediationTarget: "http://localhost:8080/api/dispatch/process",
			PoolCode:        "platform-DEFAULT-POOL",
			MessageGroupID:  &group,
		},
	}})
	require.NoError(t, err)

	var payload string
	var storedGroup *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT payload, message_group_id FROM queue_messages WHERE id = $1`, "djpub010").
		Scan(&payload, &storedGroup))
	require.NotNil(t, storedGroup)
	assert.Equal(t, group, *storedGroup, "the group is stored in its own column for FIFO claiming")
	assert.Contains(t, payload, "platform-DEFAULT-POOL")

	// Re-publishing the same job is a no-op rather than a duplicate row.
	_, err = pub.Publish(ctx, []PublishItem{{JobID: "djpub010", Message: common.Message{ID: "djpub010"}}})
	require.NoError(t, err)
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM queue_messages WHERE id = $1`, "djpub010").Scan(&count))
	assert.Equal(t, 1, count)
}

// TestDestinationResolver_JobOwnQueueWinsOverSubscription pins R4/T6: a
// job's own recognised queue value takes precedence over its subscription's
// disagreeing one. Mutant: consult the subscription first — this must fail
// under it (see docs/spec/dispatch-job-priority.md).
func TestDestinationResolver_JobOwnQueueWinsOverSubscription(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_pri_acme", "priacme")
	lo := "DEFAULT"
	insertSubscriptionWithQueue(t, pool, "sub_pri_lo", "pri-lo", strPtrPub("priacme"), &lo)

	settings, err := dispatch.ResolveSettings("postgres", "", "", "FC-dev", "postgresql://x@localhost/fc")
	require.NoError(t, err)
	destinations := NewDestinationResolver(
		NewPoolCodeResolver(pool, time.Minute),
		NewSubscriptionPriorityCache(pool, time.Minute),
		settings,
	)

	// The job's own queue says HIGH_PRIORITY; its subscription says DEFAULT.
	// The job must win.
	name, err := destinations.Destination(ctx, PublishItem{
		ClientID: "clt_pri_acme", SubscriptionID: "sub_pri_lo", Queue: "HIGH_PRIORITY",
	})
	require.NoError(t, err)
	assert.Equal(t, "FC-dev-priacme-HIGH_PRIORITY", name,
		"the job's own queue must win over a disagreeing subscription")
}

// TestDestinationResolver_LegacyJobFallsBackToSubscription pins R4/T7: a job
// with no queue of its own (the pre-existing state) still resolves through
// its subscription, exactly as it did before this column existed. Mutant:
// drop the subscription fallback — this must fail under it.
func TestDestinationResolver_LegacyJobFallsBackToSubscription(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_pri_globex", "priglobex")
	hi := "HIGH_PRIORITY"
	insertSubscriptionWithQueue(t, pool, "sub_pri_hi2", "pri-hi2", strPtrPub("priglobex"), &hi)

	settings, err := dispatch.ResolveSettings("postgres", "", "", "FC-dev", "postgresql://x@localhost/fc")
	require.NoError(t, err)
	destinations := NewDestinationResolver(
		NewPoolCodeResolver(pool, time.Minute),
		NewSubscriptionPriorityCache(pool, time.Minute),
		settings,
	)

	name, err := destinations.Destination(ctx, PublishItem{
		ClientID: "clt_pri_globex", SubscriptionID: "sub_pri_hi2", Queue: "",
	})
	require.NoError(t, err)
	assert.Equal(t, "FC-dev-priglobex-HIGH_PRIORITY", name,
		"a legacy job (no queue of its own) must still fall back to its subscription")
}

// TestDestinationResolver_JobWithLegacyTextFallsBackToSubscription pins the
// other half of R4's fallback: a job whose OWN queue holds unrecognised
// legacy text has not named a priority, so it defers to its subscription
// rather than reading as DEFAULT. Distinct from T7, where the job's column
// is NULL. Mutant: make ForJob treat unrecognised text as DEFAULT+ok — this
// must fail under it (T7 and T8 both survive that mutant).
func TestDestinationResolver_JobWithLegacyTextFallsBackToSubscription(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_pri_legacy", "prilegacy")
	hi := "HIGH_PRIORITY"
	insertSubscriptionWithQueue(t, pool, "sub_pri_legacy", "pri-legacy", strPtrPub("prilegacy"), &hi)

	settings, err := dispatch.ResolveSettings("postgres", "", "", "FC-dev", "postgresql://x@localhost/fc")
	require.NoError(t, err)
	destinations := NewDestinationResolver(
		NewPoolCodeResolver(pool, time.Minute),
		NewSubscriptionPriorityCache(pool, time.Minute),
		settings,
	)

	name, err := destinations.Destination(ctx, PublishItem{
		ClientID: "clt_pri_legacy", SubscriptionID: "sub_pri_legacy", Queue: "workers-high",
	})
	require.NoError(t, err)
	assert.Equal(t, "FC-dev-prilegacy-HIGH_PRIORITY", name,
		"legacy text on the job names no priority, so the subscription still decides")
}

// TestDestinationResolver_UnrecognisedEverywhereReadsAsDefault pins R4/T8:
// unrecognised text on both the job's own queue and its subscription's
// reads as DEFAULT, never an error. Mutant: throw on unrecognised text —
// this must fail (as an error) under it.
func TestDestinationResolver_UnrecognisedEverywhereReadsAsDefault(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_pri_legacy", "prilegacy")
	legacy := "workers-high"
	insertSubscriptionWithQueue(t, pool, "sub_pri_legacy2", "pri-legacy2", strPtrPub("prilegacy"), &legacy)

	settings, err := dispatch.ResolveSettings("postgres", "", "", "FC-dev", "postgresql://x@localhost/fc")
	require.NoError(t, err)
	destinations := NewDestinationResolver(
		NewPoolCodeResolver(pool, time.Minute),
		NewSubscriptionPriorityCache(pool, time.Minute),
		settings,
	)

	name, err := destinations.Destination(ctx, PublishItem{
		ClientID: "clt_pri_legacy", SubscriptionID: "sub_pri_legacy2", Queue: "workers-high",
	})
	require.NoError(t, err)
	assert.Equal(t, "FC-dev-prilegacy-DEFAULT", name,
		"unrecognised text anywhere in the chain must read as DEFAULT, never error")
}

// TestDestinationResolver_DirectJobWithNoSubscriptionUsesOwnQueue pins the
// publish-routing half of T1/T2: a directly-created job (no
// subscription_id — the SDK ingest surface, docs/spec/dispatch-job-priority.md
// R3) publishes per its own queue when set, and to DEFAULT when it isn't.
func TestDestinationResolver_DirectJobWithNoSubscriptionUsesOwnQueue(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_pri_direct", "pridirect")

	settings, err := dispatch.ResolveSettings("postgres", "", "", "FC-dev", "postgresql://x@localhost/fc")
	require.NoError(t, err)
	destinations := NewDestinationResolver(
		NewPoolCodeResolver(pool, time.Minute),
		NewSubscriptionPriorityCache(pool, time.Minute),
		settings,
	)

	// T1: own queue set, no subscription at all.
	name, err := destinations.Destination(ctx, PublishItem{ClientID: "clt_pri_direct", Queue: "HIGH_PRIORITY"})
	require.NoError(t, err)
	assert.Equal(t, "FC-dev-pridirect-HIGH_PRIORITY", name)

	// T2: queue absent, no subscription at all → DEFAULT.
	name, err = destinations.Destination(ctx, PublishItem{ClientID: "clt_pri_direct", Queue: ""})
	require.NoError(t, err)
	assert.Equal(t, "FC-dev-pridirect-DEFAULT", name)
}

func strPtrPub(s string) *string { return &s }
