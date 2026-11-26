//go:build integration

package fieldidentity_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// The purger removes expired unlock challenges and keys revoked more than the
// retention ago; live keys, recently revoked keys and live challenges stay.
func TestUnlockKeyPurge(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	repo := fieldidentity.NewRepos(pool).UnlockKeys
	key := func(revokedAgo string) string {
		id := "fwk_" + tsid.GenerateUntyped()[:13]
		q := `INSERT INTO field_unlock_keys (id, client_id, identity_id, device_id, jkt, public_key_jwk, pin_verified_at, created_at`
		if revokedAgo != "" {
			q += `, revoked_at, revoked_reason) VALUES ($1, 'clt_p', 'fwu_p', $2, $3, '{}', NOW(), NOW(), NOW() - $4::interval, 'ADMIN')`
			_, err := pool.Exec(ctx, q, id, "fwd_"+tsid.GenerateUntyped()[:13], tsid.GenerateUntyped(), revokedAgo)
			require.NoError(t, err)
		} else {
			q += `) VALUES ($1, 'clt_p', 'fwu_p', $2, $3, '{}', NOW(), NOW())`
			_, err := pool.Exec(ctx, q, id, "fwd_"+tsid.GenerateUntyped()[:13], tsid.GenerateUntyped())
			require.NoError(t, err)
		}
		return id
	}
	live, recent, old := key(""), key("1 day"), key("31 days")
	require.NoError(t, repo.StoreChallenge(ctx, "fresh-"+tsid.GenerateUntyped(), live, "fwd_x", time.Now().Add(time.Minute)))
	require.NoError(t, repo.StoreChallenge(ctx, "stale-"+tsid.GenerateUntyped(), live, "fwd_x", time.Now().Add(-time.Minute)))

	now := time.Now().UTC()
	challenges, keys, err := repo.Purge(ctx, now, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, challenges, int64(1))
	assert.GreaterOrEqual(t, keys, int64(1))
	for id, want := range map[string]bool{live: true, recent: true, old: false} {
		k, err := repo.FindByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, k != nil, id)
	}
	var left int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM field_unlock_challenges WHERE unlock_key_id = $1`, live).Scan(&left))
	assert.Equal(t, 1, left, "the live challenge stays")
}
