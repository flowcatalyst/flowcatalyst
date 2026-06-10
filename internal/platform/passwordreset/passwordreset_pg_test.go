//go:build integration

package passwordreset_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFactorAttempts pins the brute-force counter behind the factor-gated
// confirm: attempts increment atomically, persist across reads, and the
// principal's token set burns cleanly (the handler's response to hitting the
// ceiling).
func TestFactorAttempts(t *testing.T) {
	ctx := context.Background()
	repo := passwordreset.NewRepository(testpg.Pool(t))

	const pid = "prn_resetattempt1"
	tok := passwordreset.New(pid, "hash_resetattempt_test_1", time.Now().UTC().Add(15*time.Minute))
	tok.RequiresFactor = true
	require.NoError(t, repo.Insert(ctx, tok))

	// Fresh token starts at zero.
	found, err := repo.FindByTokenHash(ctx, tok.TokenHash)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.True(t, found.RequiresFactor)
	assert.Equal(t, 0, found.FactorAttempts)

	// Five wrong guesses: the counter is atomic and monotonic.
	for want := 1; want <= 5; want++ {
		n, err := repo.IncrementFactorAttempts(ctx, tok.ID)
		require.NoError(t, err)
		assert.Equal(t, want, n)
	}
	found, err = repo.FindByTokenHash(ctx, tok.TokenHash)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, 5, found.FactorAttempts, "attempts persist on the row")

	// Ceiling response: burn the whole token set for the principal.
	require.NoError(t, repo.DeleteByPrincipalID(ctx, pid))
	found, err = repo.FindByTokenHash(ctx, tok.TokenHash)
	require.NoError(t, err)
	assert.Nil(t, found, "burned token must be gone")
}

// TestConsumeRoundTrip sanity-checks the single-use consume path with the
// widened column set (purpose, reset_2fa, requires_factor, factor_attempts).
func TestConsumeRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := passwordreset.NewRepository(testpg.Pool(t))

	const pid = "prn_resetconsume1"
	tok := passwordreset.New(pid, "hash_resetconsume_test1", time.Now().UTC().Add(15*time.Minute))
	tok.Reset2FA = true
	tok.RequiresFactor = true
	require.NoError(t, repo.Insert(ctx, tok))

	got, err := repo.Consume(ctx, tok.TokenHash)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, pid, got.PrincipalID)
	assert.True(t, got.Reset2FA)
	assert.True(t, got.RequiresFactor)
	assert.Equal(t, 0, got.FactorAttempts)

	// Single use: a second consume finds nothing.
	again, err := repo.Consume(ctx, tok.TokenHash)
	require.NoError(t, err)
	assert.Nil(t, again)
}
