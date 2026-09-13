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

func strPtrPub(s string) *string { return &s }
