//go:build integration

package resetapproval_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// A stored request that claims a decision nobody made must fail the read, not
// come back as an APPROVED request an admin never approved.
func TestFindByID_CorruptDecisionFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := resetapproval.NewRepository(pool)

	_, err := pool.Exec(ctx, `INSERT INTO iam_principals (id, type, scope, name, active, all_applications)
		VALUES ('prn_rar_corrupt', 'USER', 'CLIENT', 'Corrupt Row Test', true, false)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO iam_reset_approval_requests (id, principal_id, status, expires_at)
		VALUES ('rar_corrupt_1', 'prn_rar_corrupt', 'APPROVED', $1)`, time.Now().Add(time.Hour))
	require.NoError(t, err)

	got, err := repo.FindByID(ctx, "rar_corrupt_1")
	require.ErrorIs(t, err, resetapproval.ErrCorruptRow)
	require.Contains(t, err.Error(), "rar_corrupt_1")
	require.Nil(t, got)
}

func TestDecide_RejectsANonDecisionAndAnEmptyDecider(t *testing.T) {
	repo := resetapproval.NewRepository(testpg.Pool(t))
	_, err := repo.Decide(context.Background(), "rar_x", resetapproval.StatusPending, "prn_admin")
	require.Error(t, err)
	_, err = repo.Decide(context.Background(), "rar_x", resetapproval.StatusApproved, "")
	require.Error(t, err)
}

// PurgeExpired drops requests that ended more than 90 days ago (decided ones by
// decision time, pending ones by expiry) and keeps everything newer.
func TestPurgeExpired_KeepsTheAuditWindow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := resetapproval.NewRepository(pool)

	_, err := pool.Exec(ctx, `INSERT INTO iam_principals (id, type, scope, name, active, all_applications)
		VALUES ('prn_rar_purge', 'USER', 'CLIENT', 'Purge Test', true, false)
		ON CONFLICT DO NOTHING`)
	require.NoError(t, err)

	day := 24 * time.Hour
	now := time.Now().UTC()
	insert := func(id, status string, decidedAt *time.Time, expires time.Time) {
		by := (*string)(nil)
		if decidedAt != nil {
			s := "prn_admin"
			by = &s
		}
		_, err := pool.Exec(ctx, `INSERT INTO iam_reset_approval_requests
			(id, principal_id, status, decided_by, decided_at, expires_at) VALUES ($1,'prn_rar_purge',$2,$3,$4,$5)`,
			id, status, by, decidedAt, expires)
		require.NoError(t, err)
	}
	old, recent := now.Add(-100*day), now.Add(-10*day)
	insert("rar_pg_old_appr", "APPROVED", &old, old.Add(time.Hour))
	insert("rar_pg_old_deny", "DENIED", &old, old.Add(time.Hour))
	insert("rar_pg_old_pend", "PENDING", nil, old)
	insert("rar_pg_old_expd", "EXPIRED", nil, old)
	insert("rar_pg_new_appr", "APPROVED", &recent, recent.Add(time.Hour))
	insert("rar_pg_new_deny", "DENIED", &recent, recent.Add(time.Hour))
	insert("rar_pg_new_pend", "PENDING", nil, recent)
	insert("rar_pg_live", "PENDING", nil, now.Add(time.Hour))

	n, err := repo.PurgeExpired(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, int64(4))

	exists := func(id string) bool {
		var c int
		require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_reset_approval_requests WHERE id = $1`, id).Scan(&c))
		return c == 1
	}
	for _, id := range []string{"rar_pg_old_appr", "rar_pg_old_deny", "rar_pg_old_pend", "rar_pg_old_expd"} {
		require.False(t, exists(id), id+" ended over 90 days ago and goes")
	}
	for _, id := range []string{"rar_pg_new_appr", "rar_pg_new_deny", "rar_pg_new_pend", "rar_pg_live"} {
		require.True(t, exists(id), id+" is inside the audit window and stays")
	}
}
