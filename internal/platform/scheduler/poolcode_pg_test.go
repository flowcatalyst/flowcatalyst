//go:build integration

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func insertClient(t *testing.T, pool *pgxpool.Pool, id, identifier string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		id, identifier, identifier)
	require.NoError(t, err)
}

func insertPool(t *testing.T, pool *pgxpool.Pool, id, code string, clientID, clientIdentifier *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_pools (id, code, name, client_id, client_identifier)
		 VALUES ($1, $2, $2, $3, $4) ON CONFLICT (id) DO NOTHING`,
		id, code, clientID, clientIdentifier)
	require.NoError(t, err)
}

func ptr(s string) *string { return &s }

// TestPoolCodeResolutionChain walks every branch of the ruled chain. The
// namespacing exists because msg_dispatch_pools is unique on (code, client_id):
// two clients may each own a pool coded FAST with different concurrency, while
// the router keys pools by code alone and treats one code with differing
// settings as a conflict to reject rather than two pools. Publishing flat codes
// would merge two clients' traffic into whichever config won.
func TestPoolCodeResolutionChain(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_acme", "acme")
	insertClient(t, pool, "clt_globex", "globex")
	insertPool(t, pool, "dsp_acme_fast", "FAST", ptr("clt_acme"), ptr("acme"))
	insertPool(t, pool, "dsp_globex_fast", "FAST", ptr("clt_globex"), ptr("globex"))
	insertPool(t, pool, "dsp_platform", "PLATFORM-WIDE", nil, nil)

	r := NewPoolCodeResolver(pool, time.Minute)

	for _, tc := range []struct {
		name     string
		poolID   string
		clientID string
		want     string
	}{
		{"client-owned pool is namespaced", "dsp_acme_fast", "clt_acme", "acme-FAST"},
		{"same code, other client, distinct pool", "dsp_globex_fast", "clt_globex", "globex-FAST"},
		{"platform-level pool keeps its bare code", "dsp_platform", "", "PLATFORM-WIDE"},
		{"no pool but a resolvable client", "", "clt_acme", "acme-DEFAULT-POOL"},
		{"neither pool nor client", "", "", "DEFAULT-POOL"},
		{"unknown pool falls back to the client's default", "dsp_deleted", "clt_acme", "acme-DEFAULT-POOL"},
		{"unknown client falls back to the global default", "", "clt_missing", "DEFAULT-POOL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, r.Resolve(ctx, tc.poolID, tc.clientID))
		})
	}
}

// TestSameCodeDifferentClientsDoNotCollide is the collision the ruling exists to
// prevent, stated directly: two clients' pools sharing the code FAST must
// publish distinct codes so the router governs them as separate pools.
func TestSameCodeDifferentClientsDoNotCollide(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	insertClient(t, pool, "clt_a", "alpha")
	insertClient(t, pool, "clt_b", "beta")
	insertPool(t, pool, "dsp_a", "SHARED-CODE", ptr("clt_a"), ptr("alpha"))
	insertPool(t, pool, "dsp_b", "SHARED-CODE", ptr("clt_b"), ptr("beta"))

	r := NewPoolCodeResolver(pool, time.Minute)

	a := r.Resolve(ctx, "dsp_a", "clt_a")
	b := r.Resolve(ctx, "dsp_b", "clt_b")

	require.Equal(t, "alpha-SHARED-CODE", a)
	require.Equal(t, "beta-SHARED-CODE", b)
	require.NotEqual(t, a, b, "identically-coded pools of different clients must not merge")
}

// TestIsDefaultPoolCode pins the ONLY structural read permitted on a composed
// code. Both halves may contain hyphens, so the string can never be split back
// into its parts — a suffix test is unambiguous only because the literal is
// fixed.
func TestIsDefaultPoolCode(t *testing.T) {
	for code, want := range map[string]bool{
		"DEFAULT-POOL":                true,
		"acme-DEFAULT-POOL":           true,
		"multi-part-id-DEFAULT-POOL":  true, // hyphenated identifier still matches
		"FAST":                        false,
		"acme-FAST":                   false,
		"acme-DEFAULT-POOL-SOMETHING": false, // suffix only, not substring
		"DEFAULT-POOLX":               false,
		"":                            false,
	} {
		require.Equalf(t, want, IsDefaultPoolCode(code), "IsDefaultPoolCode(%q)", code)
	}
}
