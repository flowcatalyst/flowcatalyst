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
