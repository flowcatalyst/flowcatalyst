//go:build integration

package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestSyncUsers_Endpoint_NoAppCode drives POST /api/principals/sync end-to-end:
// it decodes the raw wire JSON (proving `passwordHash` lands on the body),
// runs the handler with NO application code, and asserts the user is created
// with the migrated hash persisted verbatim — the whole point of the
// application-less endpoint.
func TestSyncUsers_Endpoint_NoAppCode(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: principal.NewRepository(pool), UoW: testpg.NewUoW(t)}

	authCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "p_syncusers_http",
		Scope:       auth.ScopeAnchor, // anchor clears CanSyncPrincipals
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("migrated-pw"), bcrypt.DefaultCost)
	require.NoError(t, err)
	hashStr := string(hash)

	// Build the body from raw JSON — no appCode anywhere in sight.
	rawBody := `{"principals":[{"email":"syncusers-noapp@example.com","name":"No App User","roles":["admin"],"passwordHash":` +
		mustJSON(t, hashStr) + `}]}`
	var req SyncUsersRequest
	require.NoError(t, json.Unmarshal([]byte(rawBody), &req))
	require.NotNil(t, req.Principals[0].PasswordHash, "passwordHash must decode onto the body")

	out, err := s.syncUsers(authCtx, &apicommon.In[SyncUsersRequest]{Body: req})
	require.NoError(t, err)
	assert.Equal(t, uint32(1), out.Body.Created)
	assert.Equal(t, []string{"syncusers-noapp@example.com"}, out.Body.SyncedEmails)

	got, err := s.Repo.FindByEmail(ctx, "syncusers-noapp@example.com")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.UserIdentity.PasswordHash)
	assert.Equal(t, hashStr, *got.UserIdentity.PasswordHash, "hash stored verbatim from the app-less endpoint")
	assert.NoError(t, passwordhash.Verify("migrated-pw", *got.UserIdentity.PasswordHash))
}

// TestSyncUsers_Endpoint_RequiresAuth pins that an unauthenticated caller is
// rejected before any write.
func TestSyncUsers_Endpoint_RequiresAuth(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: principal.NewRepository(pool), UoW: testpg.NewUoW(t)}

	_, err := s.syncUsers(ctx, &apicommon.In[SyncUsersRequest]{Body: SyncUsersRequest{
		Principals: []SyncUserInput{{Email: "syncusers-noauth@example.com", Name: "X"}},
	}})
	require.Error(t, err, "no auth context → rejected")
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	require.NoError(t, err)
	return string(b)
}
