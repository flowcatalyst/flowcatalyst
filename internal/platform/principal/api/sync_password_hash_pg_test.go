//go:build integration

package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// F13 (owner ruling 2026-09-25, as Java's PrincipalApiTest
// aSyncNeverReplacesAnExistingPasswordHashAndSaysSo and Rust decision #22):
// POST /api/principals/sync never applies a passwordHash to a user that
// already exists, not even for an anchor; the rest of the entry applies, a
// user the sync creates still takes its hash, and the response names the
// ignored emails in passwordHashIgnored (omitted when none).
func TestSyncUsers_NeverReplacesAnExistingPasswordHashAndSaysSo(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: principal.NewRepository(pool), Roles: role.NewRepository(pool), UoW: testpg.NewUoW(t)}
	anchor := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "p_synchash_anchor",
		Scope:       auth.ScopeAnchor,
		Permissions: []string{"platform:*:*:*"},
	})
	sync := func(body string) SyncUsersResponse {
		t.Helper()
		var req SyncUsersRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		out, err := s.syncUsers(anchor, &apicommon.In[SyncUsersRequest]{Body: req})
		require.NoError(t, err)
		return out.Body
	}

	const keep, fresh = "hashkeep-f13@example.test", "hashnew-f13@example.test"
	sync(`{"principals":[{"email":"` + keep + `","name":"Keep","passwordHash":"$2y$10$originalhashvalue"}]}`)

	got := sync(`{"principals":[` +
		`{"email":"HashKeep-F13@Example.test","name":"Kept","passwordHash":"$2y$10$replacementhashvalue"},` +
		`{"email":"` + fresh + `","name":"New","passwordHash":"$2y$10$migratedhashvalue"}]}`)
	assert.Equal(t, []string{keep}, got.PasswordHashIgnored)
	assert.Equal(t, uint32(1), got.Created)
	assert.Equal(t, uint32(1), got.Updated)

	kept, err := s.Repo.FindByEmail(ctx, keep)
	require.NoError(t, err)
	require.NotNil(t, kept.UserIdentity.PasswordHash)
	assert.Equal(t, "$2y$10$originalhashvalue", *kept.UserIdentity.PasswordHash)
	assert.Equal(t, "Kept", kept.Name, "the rest of the entry applies")
	created, err := s.Repo.FindByEmail(ctx, fresh)
	require.NoError(t, err)
	require.NotNil(t, created.UserIdentity.PasswordHash)
	assert.Equal(t, "$2y$10$migratedhashvalue", *created.UserIdentity.PasswordHash)

	noHash := sync(`{"principals":[{"email":"` + keep + `","name":"K"}]}`)
	raw, err := json.Marshal(noHash)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "passwordHashIgnored", "omitted when nothing was ignored")
}
