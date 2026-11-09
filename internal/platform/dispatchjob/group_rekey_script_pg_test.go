//go:build integration

package dispatchjob_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// scripts/ops/109-dispatch-group-per-subscription.sql re-keys event fan-out
// jobs raised before groups carried their subscription — PENDING and FAILED
// only — to the same key the fan-out now writes (the digest for an over-long
// group), and is re-runnable.
func TestGroupRekeyOpsScript(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	script, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts/ops/109-dispatch-group-per-subscription.sql"))
	require.NoError(t, err)
	// psql runs the SET and the DO block as separate statements, outside a
	// transaction (the block commits per batch); do the same.
	src := string(script)
	at := strings.Index(src, "DO $$")
	require.Positive(t, at)
	stmts := []string{src[:at], src[at:]}

	long := strings.Repeat("L", 190)
	seed := func(id, status, sub, group string, fanOut bool) {
		key := "direct-" + id
		if fanOut {
			key = "ev" + id + ":" + sub
		}
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, subscription_id, message_group, event_id,
			                                idempotency_key, created_at, updated_at)
			 VALUES ($1::text, 'rekey:evt', 'http://x', $2, $3, $4, 'ev' || $1::text, $5, $6, $6)`,
			id, status, sub, group, key, time.Now().UTC().Add(-time.Hour))
		require.NoError(t, err)
	}
	seed("rkpend0001", "PENDING", "sub_rekey_a", "order-1", true)
	seed("rkfail0001", "FAILED", "sub_rekey_a", "order-1", true)
	seed("rkqued0001", "QUEUED", "sub_rekey_a", "order-1", true)
	seed("rkdone0001", "COMPLETED", "sub_rekey_a", "order-1", true)
	seed("rkdirct001", "PENDING", "sub_rekey_a", "order-1", false)
	seed("rklong0001", "PENDING", "sub_rekey_a", long, true)
	seed("rkdone0002", "PENDING", "sub_rekey_a", "sub_rekey_a:order-2", true)

	for range 2 { // re-runnable
		for _, s := range stmts {
			_, err := pool.Exec(ctx, s)
			require.NoError(t, err)
		}
	}
	group := func(id string) string {
		var g string
		require.NoError(t, pool.QueryRow(ctx, `SELECT message_group FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&g))
		return g
	}
	assert.Equal(t, "sub_rekey_a:order-1", group("rkpend0001"))
	assert.Equal(t, "sub_rekey_a:order-1", group("rkfail0001"))
	assert.Equal(t, "order-1", group("rkqued0001"), "in flight: finishes under its old key")
	assert.Equal(t, "order-1", group("rkdone0001"), "settled: untouched")
	assert.Equal(t, "order-1", group("rkdirct001"), "a directly created job keeps its creator's group")
	assert.Equal(t, *dispatchjob.SubscriptionMessageGroup("sub_rekey_a", &long), group("rklong0001"),
		"the same digest the fan-out writes")
	assert.Equal(t, "sub_rekey_a:order-2", group("rkdone0002"), "already keyed: not prefixed twice")
}
