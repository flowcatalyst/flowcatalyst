//go:build integration

package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A real embedded cluster created with a custom password: the URL built by
// embeddedDBURL connects, the well-known password does not, the server listens
// on loopback only, and starting the same data directory with a different
// password fails before the server is started.
func TestEmbeddedPG_CustomPasswordAndLoopback(t *testing.T) {
	isolatedPGHome(t)
	ctx := context.Background()
	port := freeTCPPort(t)
	dataDir := filepath.Join(t.TempDir(), "pg")
	const pw = "custom-Passw0rd"

	pg, err := newEmbeddedPG(dataDir, port, pw)
	require.NoError(t, err)
	require.NoError(t, pg.Start())
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = pg.Stop()
		}
	})

	conn, err := pgx.Connect(ctx, embeddedDBURL(pw, port))
	require.NoError(t, err)
	var listen string
	require.NoError(t, conn.QueryRow(ctx, "SHOW listen_addresses").Scan(&listen))
	assert.Equal(t, "localhost", listen)
	_ = conn.Close(ctx)

	_, err = pgx.Connect(ctx, embeddedDBURL(defaultEmbeddedDBPassword, port))
	assert.Error(t, err, "the well-known password must not work on a cluster created with another")

	require.NoError(t, pg.Stop())
	stopped = true

	_, err = newEmbeddedPG(dataDir, port, defaultEmbeddedDBPassword)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different password")

	// The same password restarts it.
	pg2, err := newEmbeddedPG(dataDir, port, pw)
	require.NoError(t, err)
	require.NoError(t, pg2.Start())
	require.NoError(t, pg2.Stop())
}

func anchorUsers(t *testing.T, ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
},
) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM iam_principals WHERE type = 'USER' AND scope = 'ANCHOR'`).Scan(&n))
	return n
}

// On an external database fcdev creates no administrator by default; the
// built-in one appears only when asked for.
func TestDevAdminNotSeededOnExternalDatabaseByDefault(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		embedded bool
		seedDev  bool
		want     int
	}{
		{"external, default", false, decideSeedDevAdmin(false, false, false, ""), 0},
		{"external, asked for", false, decideSeedDevAdmin(false, true, true, ""), 1},
		{"embedded, default", true, decideSeedDevAdmin(true, false, false, ""), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearBootstrapEnv(t)
			pool := testpg.ScratchDB(t, "devadmin_seed_test")
			applyDevAdminDefaults(tc.seedDev)
			require.NoError(t, seed.NewSeeder(pool).Run(ctx))
			assert.Equal(t, tc.want, anchorUsers(t, ctx, pool))
			if tc.want == 1 {
				p, err := principal.NewRepository(pool).FindByEmail(ctx, devAdminEmail)
				require.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}
