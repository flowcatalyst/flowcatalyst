//go:build integration

package pages

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	etops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs"
	openapiops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	procops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// The developer portal against Postgres: an application with a synced
// OpenAPI document (twice, the second dropping an operation), an event type
// with a schema and a process, read through the repositories the BFF reads.
func TestDeveloperPortalAgainstPostgres(t *testing.T) {
	ctx := testpg.AnchorCtx()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	appID := testpg.EnsureApplication(t, "uidevp")
	specs := openapispecs.NewRepository(pool)
	page := &Developer{
		Apps: application.NewRepository(pool), Specs: specs, EventTypes: eventtype.NewRepository(pool),
		Processes: process.NewRepository(pool), Shell: shellForTest(),
	}
	get := func(q req) string {
		t.Helper()
		rec := serveMounted(t, connUser(devViewPerm, procViewPerm), q, page)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", q.target, rec.Body.String())
		return rec.Body.String()
	}

	sync := func(spec string) {
		t.Helper()
		_, err := usecaseop.Run(ctx, uow, openapiops.SyncOpenApiSpec(specs),
			openapiops.SyncOpenApiSpecCommand{ApplicationID: appID, ApplicationCode: "uidevp", Spec: []byte(spec)}, testpg.TestEC())
		require.NoError(t, err)
	}
	sync(devOrdersSpec)

	// The rows are the other tests' business too (their lists are not empty
	// while these exist), so they go when the test does.
	etOps, procOps := EventTypeOps{Repo: eventtype.NewRepository(pool), UoW: uow}, ProcessOps{Repo: process.NewRepository(pool), UoW: uow}
	etID, err := etOps.Create(ctx, etops.CreateCommand{
		Code: "uidevp:orders:order:placed", Name: "Order placed", Schema: []byte(`{"type":"object","properties":{"sku":{"type":"string"}}}`),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = etOps.Archive(ctx, etID)
		_ = etOps.Delete(ctx, etID)
	})
	procID, err := procOps.Create(ctx, procops.CreateCommand{
		Code: "uidevp:orders:flow", Name: "Order flow", Body: "graph TD\n  A --> B",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = procOps.Archive(ctx, procID)
		_ = procOps.Delete(ctx, procID)
	})

	base := "/ui/developer/applications/" + appID
	assert.Contains(t, get(req{target: "/ui/developer"}), base)
	docs := get(req{target: base})
	assert.Contains(t, docs, "Orders API")
	assert.Contains(t, docs, "3 operations")
	assert.Contains(t, get(req{target: base + "/operation?m=post&p=/orders&n=2", datastar: true}), "Request body")

	ets := get(req{target: base + "/tabs/event-types?et=" + etID, datastar: true})
	assert.Contains(t, ets, "Order placed")
	assert.Contains(t, ets, "sku", "the stored schema is in the viewer")
	procs := get(req{target: base + "/tabs/processes", datastar: true})
	assert.Contains(t, procs, "Order flow")
	assert.Contains(t, get(req{target: base + "/tabs/processes?proc=" + procID, datastar: true}), "<fc-diagram")

	// A second sync that drops an operation is a new, breaking version; the old one is archived.
	sync(strings.Replace(devOrdersSpec, `"/health":{"get":{"summary":"Health","deprecated":true,"responses":{"200":{"description":"up"}}}}`, `"/status":{"get":{"summary":"Status","responses":{"200":{"description":"up"}}}}`, 1))
	versions := get(req{target: base + "/versions"})
	assert.Contains(t, versions, ">breaking<")
	assert.Contains(t, versions, ">ARCHIVED<")
	rec := serveMounted(t, connUser(devViewPerm), req{target: base + "/openapi.json"}, page)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "/status")
	assert.NotContains(t, rec.Body.String(), "/health")

	// The checks are the BFF's, against real rows too.
	for _, route := range []string{"/ui/developer", base, base + "/tabs/event-types", base + "/openapi.json"} {
		rec := serveMounted(t, connUser(), req{target: route}, page)
		assert.Equal(t, http.StatusForbidden, rec.Code, route)
		assert.NotContains(t, rec.Body.String(), "uidevp")
	}
	assert.Equal(t, http.StatusNotFound, serveMounted(t, connUser(devViewPerm), req{target: "/ui/developer/applications/app_missing"}, page).Code)
}

// The Developer Users page against Postgres: grant the role, set a
// credential (the secret is in the response once and only its keyed hash is
// stored), revoke it, remove the role. Every change is the principal API's
// use case and audited like it.
func TestDeveloperUsersAgainstPostgres(t *testing.T) {
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
		operations.CreateCommand{Email: "uidev-pg@example.com", Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	id := string(created.UserID)
	t.Cleanup(func() {
		_, _ = usecaseop.Run(ctx, uow, operations.DeleteUser(repo), operations.DeleteCommand{ID: created.UserID}, testpg.TestEC())
	})

	page := &DeveloperUsers{Store: repo, Ops: DeveloperUserOps{Repo: repo, Roles: roles, UoW: uow}, Shell: shellForTest()}
	post := func(action string, datastar bool) *httptest.ResponseRecorder {
		t.Helper()
		return serveMounted(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/developer-users/" + id + "/" + action, datastar: datastar, form: url.Values{}}, page)
	}
	roleNames := func() []string {
		p, err := repo.FindByID(ctx, ids.PrincipalID(id))
		require.NoError(t, err)
		return devRoleNames(p)
	}

	// Not a developer yet: the credential is refused by the use case, and the list does not show them.
	refused := post("credential", true)
	assert.Contains(t, refused.Body.String(), "fc-flash-error")
	assert.Contains(t, refused.Body.String(), "does not hold the developer role")
	assert.NotContains(t, serveMounted(t, anchorAdmin(), req{target: "/ui/developer-users"}, page).Body.String(), "uidev-pg@example.com")

	found := serveMounted(t, anchorAdmin(), req{target: "/ui/developer-users/grant/search?q=uidev-pg", datastar: true}, page).Body.String()
	assert.Contains(t, found, "uidev-pg@example.com")
	assert.NotContains(t, post("grant", true).Body.String(), "fc-flash-error")
	assert.Contains(t, roleNames(), developerRole)
	assert.NotZero(t, auditCount(t, id), "the role change is audited like the API's")
	assert.Contains(t, serveMounted(t, anchorAdmin(), req{target: "/ui/developer-users"}, page).Body.String(), "uidev-pg@example.com")
	assert.NotContains(t, serveMounted(t, anchorAdmin(), req{target: "/ui/developer-users/grant/search?q=uidev-pg", datastar: true}, page).Body.String(), "uidev-pg@example.com", "a developer is not offered again")

	set := post("credential", true)
	require.NotContains(t, set.Body.String(), "fc-flash-error", set.Body.String())
	assert.Equal(t, "no-store", set.Header().Get("Cache-Control"))
	secret := devBetween(set.Body.String(), `id="dev-secret-value"`, "</code>")
	require.NotEmpty(t, secret)
	secret = secret[strings.LastIndex(secret, ">")+1:]
	assert.Len(t, secret, 43, "32 random bytes, base64url")
	p, err := repo.FindByID(ctx, ids.PrincipalID(id))
	require.NoError(t, err)
	require.True(t, p.HasDevClientSecret())
	assert.NotContains(t, *p.UserIdentity.DevClientSecretRef, secret, "only a keyed hash is stored")
	assert.NotContains(t, serveMounted(t, anchorAdmin(), req{target: "/ui/developer-users"}, page).Body.String(), secret)
	assert.Contains(t, serveMounted(t, anchorAdmin(), req{target: "/ui/developer-users"}, page).Body.String(), ">Set<")

	rotated := post("credential", true)
	assert.Contains(t, rotated.Body.String(), "rotated for")
	assert.NotContains(t, rotated.Body.String(), secret, "a rotation shows the new value only")

	assert.NotContains(t, post("revoke", true).Body.String(), "fc-flash-error")
	p, _ = repo.FindByID(ctx, ids.PrincipalID(id))
	assert.False(t, p.HasDevClientSecret())

	// Removing the role takes a credential with it, so re-granting does not bring the old secret back.
	post("credential", true)
	assert.NotContains(t, post("remove-role", true).Body.String(), "fc-flash-error")
	p, _ = repo.FindByID(ctx, ids.PrincipalID(id))
	assert.NotContains(t, devRoleNames(p), developerRole)
	assert.False(t, p.HasDevClientSecret())

	// A caller without the permission is refused whatever the row.
	forged := serveMounted(t, connUser(permUserView), req{method: http.MethodPost, target: "/ui/developer-users/" + id + "/grant", datastar: true, form: url.Values{}}, page)
	assert.Contains(t, forged.Body.String(), "fc-flash-error")
	assert.NotContains(t, roleNames(), developerRole)
}

func devBetween(s, from, to string) string {
	i := strings.Index(s, from)
	if i < 0 {
		return ""
	}
	s = s[i:]
	j := strings.Index(s, to)
	if j < 0 {
		return ""
	}
	return s[:j]
}
