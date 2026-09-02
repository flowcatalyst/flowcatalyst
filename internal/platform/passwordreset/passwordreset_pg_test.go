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

// TestFindByTokenHash_CorruptPurposeFailsLoudly is the X-06 read boundary: a
// row whose purpose column holds a value that isn't one of the known Purpose
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to "reset".
func TestFindByTokenHash_CorruptPurposeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := passwordreset.NewRepository(pool)

	const id = "prt_corrupt_test1"
	const hash = "hash_corrupt_purpose_test_1"
	// The purpose column is CHECK-constrained since migration 051; drop it
	// for the seed insert to simulate a row written before the constraint
	// existed (or hand-edited out of band) — the exact scenario this read
	// boundary defends against.
	testpg.WithConstraintDropped(t, pool, "iam_password_reset_tokens", "chk_iam_password_reset_tokens_purpose", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO iam_password_reset_tokens (id, principal_id, token_hash, purpose, expires_at)
			 VALUES ($1, 'prn_corrupttest01', $2, 'NOT_A_REAL_PURPOSE', NOW() + interval '15 minutes')`,
			id, hash)
		require.NoError(t, err)
		// The row must be gone before WithConstraintDropped restores the
		// constraint (t.Cleanup runs LIFO, so this fires first), otherwise
		// re-adding the constraint fails against the still-corrupt row.
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM iam_password_reset_tokens WHERE id = $1`, id)
		})
	})

	tok, err := repo.FindByTokenHash(ctx, hash)
	require.Error(t, err, "a corrupt purpose must fail the read, not round-trip it")
	assert.Nil(t, tok)
	assert.Contains(t, err.Error(), "CORRUPT_PASSWORD_RESET_PURPOSE")
}
