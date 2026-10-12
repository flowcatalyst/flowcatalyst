//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// bcryptHash mints a Laravel-style bcrypt ($2a$) hash — the format a migrated
// app ships and the login flow accepts (passwordhash.Verify) before re-encoding
// to native argon2id on first success.
func bcryptHash(t *testing.T, plaintext string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	require.NoError(t, err)
	return string(h)
}

// A synced principal carrying a passwordHash keeps that hash verbatim so the
// migrated user logs in with their existing password; a later hash-less sync
// must NOT wipe it; and a sync carrying a new hash leaves it too (F13).
func TestSyncPrincipals_PasswordHashCarry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	email := "prn-sync-pwhash@example.com"
	origHash := bcryptHash(t, "laravel-original-pw")

	// 1. New user created via sync with a pre-hashed credential.
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncPrincipals(repo, role.NewRepository(testpg.Pool(t)), nil), operations.SyncPrincipalsCommand{
		ApplicationCode: "syncpw",
		Principals: []operations.SyncPrincipalInput{{
			Email: email, Name: "Synced User", Active: true, PasswordHash: new(origHash),
		}},
	}, ec)
	require.NoError(t, err)

	got, err := repo.FindByEmail(ctx, email)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.UserIdentity.PasswordHash)
	assert.Equal(t, origHash, *got.UserIdentity.PasswordHash, "hash stored verbatim (not re-hashed)")
	assert.NoError(t, passwordhash.Verify("laravel-original-pw", *got.UserIdentity.PasswordHash),
		"the migrated password verifies at login")

	// 2. Re-sync the SAME user with NO hash (a roles-only sync) — must preserve.
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncPrincipals(repo, role.NewRepository(testpg.Pool(t)), nil), operations.SyncPrincipalsCommand{
		ApplicationCode: "syncpw",
		Principals: []operations.SyncPrincipalInput{{
			Email: email, Name: "Synced User Renamed", Active: true,
		}},
	}, ec)
	require.NoError(t, err)

	got, err = repo.FindByEmail(ctx, email)
	require.NoError(t, err)
	require.NotNil(t, got.UserIdentity.PasswordHash)
	assert.Equal(t, origHash, *got.UserIdentity.PasswordHash, "hash-less sync must not wipe the password")
	assert.Equal(t, "Synced User Renamed", got.Name, "other fields still update")

	// 3. Re-sync with a NEW hash (F13, owner ruling 2026-09-25 as Java and
	// Rust): a sync never applies a hash to an existing user, whoever the
	// caller (an anchor here); the rest of the entry still applies.
	newHash := bcryptHash(t, "laravel-changed-pw")
	ignored, err := operations.PasswordHashesIgnored(ctx, repo, []operations.SyncPrincipalInput{
		{Email: "  PRN-Sync-PWHash@Example.com ", PasswordHash: new(newHash)},
		{Email: email, PasswordHash: new(newHash)},
		{Email: email},
		{Email: "prn-sync-pwhash-new@example.com", PasswordHash: new(newHash)},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{email}, ignored,
		"the existing user's email, normalised, once; a hash-less entry and a new user are not listed")

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncPrincipals(repo, role.NewRepository(testpg.Pool(t)), nil), operations.SyncPrincipalsCommand{
		ApplicationCode: "syncpw",
		Principals: []operations.SyncPrincipalInput{{
			Email: email, Name: "Synced User Renamed Again", Active: true, PasswordHash: new(newHash),
		}},
	}, ec)
	require.NoError(t, err)

	got, err = repo.FindByEmail(ctx, email)
	require.NoError(t, err)
	require.NotNil(t, got.UserIdentity.PasswordHash)
	assert.Equal(t, origHash, *got.UserIdentity.PasswordHash, "a hash on the sync never replaces an existing user's")
	assert.NoError(t, passwordhash.Verify("laravel-original-pw", *got.UserIdentity.PasswordHash))
	assert.Equal(t, "Synced User Renamed Again", got.Name, "the rest of the entry applies")
}

// A synced user with no passwordHash at all stays password-less (the OIDC
// default) — unchanged from the pre-feature behaviour.
func TestSyncPrincipals_NoHash_StaysPasswordless(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	email := "prn-sync-nopw@example.com"
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncPrincipals(repo, role.NewRepository(testpg.Pool(t)), nil), operations.SyncPrincipalsCommand{
		ApplicationCode: "syncpw",
		Principals: []operations.SyncPrincipalInput{{
			Email: email, Name: "No Password", Active: true,
		}},
	}, testpg.TestEC())
	require.NoError(t, err)

	got, err := repo.FindByEmail(ctx, email)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.UserIdentity)
	assert.Nil(t, got.UserIdentity.PasswordHash, "no hash supplied → no password (OIDC-style)")
}
