//go:build integration

package pages

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The profile's developer credential against Postgres: the user's own record
// and the principal use cases, with the platform's /auth/* endpoints faked
// (they are the login endpoint's, tested there).
func TestProfileDeveloperCredentialAgainstPostgres(t *testing.T) {
	t.Setenv("FLOWCATALYST_APP_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	ctx := testpg.AnchorCtx()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	repo := principal.NewRepository(pool)
	roles := role.NewRepository(pool)
	testpg.EnsureApplication(t, "platform")
	if existing, err := roles.FindByName(ctx, developerRole); err != nil || existing == nil {
		_, err := usecaseop.Run(ctx, uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: "platform", RoleName: "developer", DisplayName: "platform developer"}, testpg.TestEC())
		require.NoError(t, err)
	}
	created, err := usecaseop.Run(ctx, uow, operations.CreateUser(repo),
		operations.CreateCommand{Email: "uiprofile-pg@example.com", Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	id := string(created.UserID)
	t.Cleanup(func() {
		_, _ = usecaseop.Run(ctx, uow, operations.DeleteUser(repo), operations.DeleteCommand{ID: created.UserID}, testpg.TestEC())
	})
	ops := DeveloperUserOps{Repo: repo, Roles: roles, UoW: uow}

	up := newFakeAuthEndpoints()
	up.me["principalId"], up.me["email"], up.me["name"] = id, "uiprofile-pg@example.com", "Profile Pg"
	page := &Profile{Upstream: up, Principals: repo, Credentials: ops, Shell: shellForTest()}
	self := &auth.AuthContext{
		PrincipalID: id, Scope: auth.ScopeAnchor, Roles: []string{developerRole}, Email: "uiprofile-pg@example.com",
		PrincipalType: auth.PrincipalTypeUser, Permissions: []string{"platform:developer:api-credential:manage"},
	}
	post := func(action string) string {
		t.Helper()
		rec := serveMounted(t, self, req{method: http.MethodPost, target: "/ui/profile/" + action, form: url.Values{}, headers: map[string]string{"Sec-Fetch-Site": "same-origin"}}, page)
		return rec.Header().Get("Location") + "|" + rec.Body.String()
	}

	// Not a developer: the section shows (the permission says so) but the use case refuses.
	refused := post("developer-credential")
	assert.Contains(t, refused, "developer role")
	p, err := repo.FindByID(ctx, ids.PrincipalID(id))
	require.NoError(t, err)
	assert.False(t, p.HasDevClientSecret())

	require.NoError(t, ops.GrantRole(ctx, id))
	set := post("developer-credential")
	secret := devBetween(set, `id="dev-secret-value"`, "</code>")
	require.NotEmpty(t, secret, set)
	secret = secret[strings.LastIndex(secret, ">")+1:]
	assert.Len(t, secret, 43, "32 random bytes, base64url")
	p, err = repo.FindByID(ctx, ids.PrincipalID(id))
	require.NoError(t, err)
	require.True(t, p.HasDevClientSecret())
	assert.NotContains(t, *p.UserIdentity.DevClientSecretRef, secret, "only a keyed hash is stored")

	// Seen again: a credential is set, and the secret is nowhere.
	view := serveMounted(t, self, req{target: "/ui/profile"}, page).Body.String()
	assert.Contains(t, view, "Credential set")
	assert.Contains(t, view, "Rotate Secret")
	assert.NotContains(t, view, secret)

	rotated := post("developer-credential")
	assert.NotContains(t, rotated, secret, "a rotation shows the new value only")
	assert.Contains(t, rotated, `id="dev-secret-value"`)

	assert.True(t, strings.HasPrefix(post("developer-credential/revoke"), "/ui/profile|"))
	p, err = repo.FindByID(ctx, ids.PrincipalID(id))
	require.NoError(t, err)
	assert.False(t, p.HasDevClientSecret())
	assert.Contains(t, serveMounted(t, self, req{target: "/ui/profile"}, page).Body.String(), "No credential set")
}
