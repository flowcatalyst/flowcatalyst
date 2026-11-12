//go:build integration

package mfa_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The purges remove expired email PINs and trusted devices and leave live ones.
func TestPurgeExpiredEmailPinsAndTrustedDevices(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := mfa.NewRepository(pool)
	const pid = "prn_mfapurge0001"
	now := time.Now().UTC()

	gone := mfa.NewEmailPin(pid, mfa.EmailPinLogin, "hash-gone", now.Add(-time.Minute))
	live := mfa.NewEmailPin(pid, mfa.EmailPinLogin, "hash-live", now.Add(time.Hour))
	require.NoError(t, repo.InsertEmailPin(ctx, gone))
	require.NoError(t, repo.InsertEmailPin(ctx, live))
	n, err := repo.PurgeExpiredEmailPins(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(1))
	assert.Equal(t, 0, count(t, `SELECT COUNT(*) FROM iam_mfa_email_pins WHERE id = $1`, gone.ID), "expired PIN removed")
	assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM iam_mfa_email_pins WHERE id = $1`, live.ID), "live PIN kept")

	dGone := mfa.NewTrustedDevice(pid, "dev-hash-gone", nil, now.Add(-time.Minute))
	dLive := mfa.NewTrustedDevice(pid, "dev-hash-live", nil, now.Add(time.Hour))
	require.NoError(t, repo.InsertTrustedDevice(ctx, dGone))
	require.NoError(t, repo.InsertTrustedDevice(ctx, dLive))
	n, err = repo.PurgeExpiredTrustedDevices(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(1))
	assert.Equal(t, 0, count(t, `SELECT COUNT(*) FROM iam_mfa_trusted_devices WHERE id = $1`, dGone.ID), "expired device removed")
	assert.Equal(t, 1, count(t, `SELECT COUNT(*) FROM iam_mfa_trusted_devices WHERE id = $1`, dLive.ID), "live device kept")
}

func count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}
