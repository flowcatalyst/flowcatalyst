//go:build integration

package fieldidentity_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Console sign-in codes carry no foreign key (migration 114,
// docs/foreign-keys.md), so deleting a worker removes its codes itself: an
// unused code of a deleted worker must not survive to be redeemed. Another
// worker's codes are untouched, and badges still go by cascade.
func TestIdentityDelete_RemovesTheSignInCodes(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repos := fieldidentity.NewRepos(pool)
	client := "clt_" + strings.ToLower(tsid.GenerateUntyped())

	seed := func() string {
		t.Helper()
		suffix := strings.ToLower(tsid.GenerateUntyped())
		id := "fwu_" + suffix
		for _, q := range []string{
			`INSERT INTO field_identities (id, client_id, worker_code, name) VALUES ($1, $2, upper($3), 'FK cleanup')`,
			`INSERT INTO field_badges (id, client_id, identity_id, token_hash) VALUES ('fwb_' || $3, $2, $1, 'badge-' || $3)`,
			`INSERT INTO field_sign_in_codes (id, client_id, identity_id, application_id, code_hash, created_by, expires_at)
			 VALUES ('fwi_' || $3, $2, $1, 'app_fkcleanup0001', 'code-' || $3, 'prn_fkcleanup0001', NOW() + interval '10 minutes')`,
		} {
			_, err := pool.Exec(ctx, q, id, client, suffix)
			require.NoError(t, err)
		}
		return id
	}
	gone, kept := seed(), seed()

	i, err := repos.Identities.FindByID(ctx, gone)
	require.NoError(t, err)
	require.NotNil(t, i)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, repos.Identities.Delete(ctx, i, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(ctx))

	count := func(table, id string) int {
		t.Helper()
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE identity_id = $1`, id).Scan(&n))
		return n
	}
	for _, table := range []string{"field_sign_in_codes", "field_badges"} {
		assert.Equal(t, 0, count(table, gone), "%s of the deleted worker", table)
		assert.Equal(t, 1, count(table, kept), "%s of the other worker", table)
	}
}
