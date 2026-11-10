//go:build integration

package principal_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// A user's pending email PINs, remembered devices and reset-approval requests
// carry no foreign key (migration 114, docs/foreign-keys.md), so Delete
// removes them itself, in its transaction; another user's rows are untouched,
// and the role assignments still go by cascade.
func TestDelete_RemovesTheTransactionalSignInRows(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)

	seed := func(pid, tag string) {
		t.Helper()
		for _, q := range []string{
			`INSERT INTO iam_principals (id, type, scope, name, active, email)
			 VALUES ($1, 'USER', 'CLIENT', 'FK cleanup', TRUE, $2 || '@fk-cleanup.example.com')`,
			`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, 'fkcleanup:role', 'MANUAL')`,
			`INSERT INTO iam_mfa_email_pins (id, principal_id, pin_hash, expires_at) VALUES ('mep_' || $2, $1, 'h', NOW() + interval '10 minutes')`,
			`INSERT INTO iam_mfa_trusted_devices (id, principal_id, token_hash, expires_at) VALUES ('mtd_' || $2, $1, 'fkcleanup-' || $2, NOW() + interval '30 days')`,
			`INSERT INTO iam_reset_approval_requests (id, principal_id, expires_at) VALUES ('rar_' || $2, $1, NOW() + interval '1 day')`,
		} {
			args := []any{pid}
			if strings.Contains(q, "$2") {
				args = append(args, tag)
			}
			_, err := pool.Exec(ctx, q, args...)
			require.NoError(t, err)
		}
	}
	const gone, kept = "prn_fkcleanup0001", "prn_fkcleanup0002"
	seed(gone, "fkcleanup0001")
	seed(kept, "fkcleanup0002")

	p, err := repo.FindByID(ctx, gone)
	require.NoError(t, err)
	require.NotNil(t, p)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, p, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(ctx))

	count := func(table, pid string) int {
		t.Helper()
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE principal_id = $1`, pid).Scan(&n))
		return n
	}
	for _, table := range []string{"iam_principal_roles", "iam_mfa_email_pins", "iam_mfa_trusted_devices", "iam_reset_approval_requests"} {
		assert.Equal(t, 0, count(table, gone), "%s of the deleted user", table)
		assert.Equal(t, 1, count(table, kept), "%s of the other user", table)
	}
}
