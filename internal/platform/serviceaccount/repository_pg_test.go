//go:build integration

package serviceaccount

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

// TestReencryptLegacyDoesNotClobberAConcurrentRotation is the hazard the
// compare-and-set exists for.
//
// A reader loads a row still holding plaintext P and schedules an upgrade. In
// between, a rotation stores a NEW secret Q. A blind UPDATE would overwrite Q
// with an encryption of the stale P — and every delivery signed with Q would
// then fail verification at the subscriber, with nothing in our logs to say
// why. The upgrade must therefore only fire while the column still holds
// exactly the value the reader saw.
func TestReencryptLegacyDoesNotClobberAConcurrentRotation(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := NewRepository(pool)
	require.NotNil(t, repo.enc, "the test key must be configured")

	const id = "sa_clobber_test01"
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type, wh_signing_secret_ref)
		 VALUES ($1, 'clobber-code', 'Clobber', true, 'BEARER_TOKEN', 'stale-plaintext')`, id)
	require.NoError(t, err)

	// The row as a reader saw it — plaintext, so an upgrade is warranted.
	stale := "stale-plaintext"
	seen := dbq.IamServiceAccount{ID: id, WhSigningSecretRef: &stale}

	// A rotation lands first.
	rotated := "encrypted:rotated-by-someone-else"
	_, err = pool.Exec(ctx,
		`UPDATE iam_service_accounts SET wh_signing_secret_ref = $2 WHERE id = $1`, id, rotated)
	require.NoError(t, err)

	// Now the stale reader's upgrade fires. It must find nothing to update.
	repo.reencryptLegacySecrets(ctx, seen)

	var after *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT wh_signing_secret_ref FROM iam_service_accounts WHERE id = $1`, id).Scan(&after))
	require.NotNil(t, after)
	assert.Equal(t, rotated, *after,
		"the rotation survives; the stale reader's upgrade is a no-op")
}

// TestReencryptLegacyUpgradesAnUntouchedRow is the other half: when nothing
// raced, the plaintext is replaced with ciphertext of the same value.
func TestReencryptLegacyUpgradesAnUntouchedRow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := NewRepository(pool)

	const id = "sa_upgrade_test01"
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type, wh_auth_token_ref)
		 VALUES ($1, 'upgrade-code', 'Upgrade', true, 'BEARER_TOKEN', 'plain-token')`, id)
	require.NoError(t, err)

	plain := "plain-token"
	repo.reencryptLegacySecrets(ctx, dbq.IamServiceAccount{ID: id, WhAuthTokenRef: &plain})

	var after *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT wh_auth_token_ref FROM iam_service_accounts WHERE id = $1`, id).Scan(&after))
	require.NotNil(t, after)
	assert.NotEqual(t, plain, *after, "no longer plaintext")

	pt, err := repo.enc.Decrypt(*after)
	require.NoError(t, err)
	assert.Equal(t, plain, pt, "and it decrypts back to the same secret")
}

// TestFindByID_CorruptWebhookAuthTypeFailsLoudly is the X-06 read boundary:
// a row whose wh_auth_type column holds a value that isn't one of the known
// WebhookAuthType constants (junk written before validation existed, or
// hand-edited) must fail the read with a distinct error, never round-trip as
// that literal string and never silently coerce to NONE.
func TestFindByID_CorruptWebhookAuthTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := NewRepository(pool)

	const id = "sa_corrupt_test01"
	// Migration 051 forbids this value at the write boundary, which is the
	// point of the constraint. The loud read path is defence in depth for a
	// value that predates it (or survives the constraint being dropped), so
	// the row is seeded with the constraint briefly off — which is exactly
	// the situation being simulated. The seeded row is deleted first: cleanups
	// run LIFO, so this one fires before the helper restores the constraint,
	// which would otherwise fail to validate against the still-corrupt row.
	testpg.WithConstraintDropped(t, pool, "iam_service_accounts", "chk_iam_service_accounts_wh_auth_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type)
			 VALUES ($1, 'corrupt-code', 'Corrupt', true, 'NOT_A_REAL_AUTH_TYPE')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM iam_service_accounts WHERE id = $1`, id)
		})
	})

	sa, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt wh_auth_type must fail the read, not round-trip it")
	assert.Nil(t, sa)
	assert.Contains(t, err.Error(), "CORRUPT_WEBHOOK_AUTH_TYPE")
}

// TestFindAll_CorruptWebhookAuthTypeFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned with 200 OK.
func TestFindAll_CorruptWebhookAuthTypeFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := NewRepository(pool)

	const goodID = "sa_cl_good0123456" // 17 chars, matches VARCHAR(17)
	const badID = "sa_cl_bad00123456"  // 17 chars, matches VARCHAR(17)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type)
		 VALUES ($1, 'corruptlist-good', 'Good', true, 'NONE')`, goodID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM iam_service_accounts WHERE id = $1`, goodID)
	})
	// See the note in TestFindByID_CorruptWebhookAuthTypeFailsLoudly: seeded
	// with the constraint briefly dropped, and removed before it is restored.
	testpg.WithConstraintDropped(t, pool, "iam_service_accounts", "chk_iam_service_accounts_wh_auth_type", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type)
			 VALUES ($1, 'corruptlist-bad', 'Bad', true, 'NOT_A_REAL_AUTH_TYPE')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM iam_service_accounts WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindAll(ctx)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_WEBHOOK_AUTH_TYPE")
}
