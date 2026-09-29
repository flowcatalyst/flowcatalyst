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
