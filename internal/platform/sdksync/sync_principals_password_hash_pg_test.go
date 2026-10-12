//go:build integration

package sdksync

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// F13 on the application route (POST /api/applications/{appCode}/principals/sync):
// an existing user's hash is never replaced, a new user takes its hash, and
// the shared sync result carries passwordHashIgnored (omitted when none, so
// the other sync routes' responses are unchanged).
func TestSyncPrincipals_Endpoint_NeverReplacesAnExistingPasswordHash(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	appCode := "syncpwkeep"
	_, err := usecaseop.Run(ctx, uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Sync PW Keep"}, testpg.TestEC())
	require.NoError(t, err)
	s := &State{Apps: application.NewRepository(pool), Principals: principal.NewRepository(pool), Roles: role.NewRepository(pool), UoW: uow}
	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "p_syncpwkeep", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"}, AllApplications: true,
	})
	sync := func(body string) SyncResultResponse {
		t.Helper()
		var req syncPrincipalsRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		out, err := s.syncPrincipals(caller, &syncPrincipalsInput{AppCode: appCode, Body: req})
		require.NoError(t, err)
		return out.Body
	}

	const keep, fresh = "keep-f13-app@example.test", "new-f13-app@example.test"
	sync(`{"principals":[{"email":"` + keep + `","name":"Keep","passwordHash":"$2y$10$originalhashvalue"}]}`)
	got := sync(`{"principals":[` +
		`{"email":"` + keep + `","name":"Kept","passwordHash":"$2y$10$replacementhashvalue"},` +
		`{"email":"` + fresh + `","name":"New","passwordHash":"$2y$10$migratedhashvalue"}]}`)
	assert.Equal(t, []string{keep}, got.PasswordHashIgnored)

	var stored string
	require.NoError(t, pool.QueryRow(ctx, `SELECT password_hash FROM iam_principals WHERE email = $1`, keep).Scan(&stored))
	assert.Equal(t, "$2y$10$originalhashvalue", stored)
	require.NoError(t, pool.QueryRow(ctx, `SELECT password_hash FROM iam_principals WHERE email = $1`, fresh).Scan(&stored))
	assert.Equal(t, "$2y$10$migratedhashvalue", stored)

	raw, err := json.Marshal(sync(`{"principals":[{"email":"` + keep + `","name":"K"}]}`))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "passwordHashIgnored", "omitted when nothing was ignored")
}
