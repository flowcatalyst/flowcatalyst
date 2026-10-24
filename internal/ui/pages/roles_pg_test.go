//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The roles and permissions pages end to end against Postgres: the handlers
// run the real use cases (RolesOps over the unit of work), so the rows, the
// domain events, the audit records and, above all, the application
// confinement and the permission ceiling are the API's.
func TestRolesPagesAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := role.NewRepository(pool)
	catalog := role.NewPermissionRepo(pool)
	testpg.EnsureApplication(t, "uirole")
	testpg.EnsureApplication(t, "platform")
	store := PermissionStore{Roles: repo, Catalog: catalog}
	roles := &RolesPage{Store: repo, Ops: RolesOps{Repo: repo, UoW: testpg.NewUoW(t)}, Permissions: store, Apps: application.NewRepository(pool), Shell: shellForTest()}
	perms := &PermissionsPage{Catalogue: store, Apps: application.NewRepository(pool), Shell: shellForTest()}
	do := func(t *testing.T, q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, roles, perms)
		require.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusSeeOther, "%s %s: status %d: %s", q.method, q.target, rec.Code, rec.Body.String())
		return rec.Body.String()
	}
	// As: a manager of roles who holds role permissions only, no platform authority beyond them.
	as := func(q req) string {
		t.Helper()
		return serveMounted(t, rolesManager(), q, roles, perms).Body.String()
	}
	find := func(name string) *role.Role {
		t.Helper()
		r, err := repo.FindByName(ctx, name)
		require.NoError(t, err)
		return r
	}

	// Create, then edit with a permission set; both are the use cases'.
	body := do(t, rolesPost("/ui/roles/new", url.Values{"application": {"uirole"}, "roleName": {"picker"}, "displayName": {"Picker"}, "description": {"Picks"}}))
	require.Contains(t, body, "Role created successfully", body)
	picker := find("uirole:picker")
	require.NotNil(t, picker)
	assert.Equal(t, role.SourceDatabase, picker.Source)
	assert.Equal(t, "Picker", picker.DisplayName)
	assert.Equal(t, "Picks", deref(picker.Description))

	edit := "/ui/roles/uirole:picker/edit"
	body = do(t, rolesPost(edit, url.Values{"displayName": {"Picker 2"}, "description": {""}, "clientManaged": {"true"}, "permission": {"uirole:a:b:c", "uirole:a:b:d"}}))
	require.NotContains(t, body, "fc-flash-error", body)
	picker = find("uirole:picker")
	assert.Equal(t, "Picker 2", picker.DisplayName)
	assert.True(t, picker.ClientManaged)
	assert.ElementsMatch(t, []string{"uirole:a:b:c", "uirole:a:b:d"}, picker.Permissions)
	assert.Equal(t, 2, auditCount(t, picker.ID), "create and update are audited")

	// The editor offers the role's own permissions, the held ones checked.
	page := do(t, req{target: edit})
	assert.Contains(t, page, `name="permission" value="uirole:a:b:c" checked`)
	// Adding a permission in the editor writes nothing until Save.
	do(t, rolesPost(edit+"/add-permission", url.Values{"permission": {"uirole:a:b:c"}, "newContext": {"x"}, "newAggregate": {"y"}, "newAction": {"z"}}))
	assert.ElementsMatch(t, []string{"uirole:a:b:c", "uirole:a:b:d"}, find("uirole:picker").Permissions)

	// Confinement (F12): another application's permission is refused for a
	// caller who is not a super-admin; the role is unchanged.
	body = as(rolesPost(edit, url.Values{"displayName": {"Picker 2"}, "permission": {"uirole:a:b:c", "platform:iam:role:view"}}))
	assert.Contains(t, body, "does not belong to application", body)
	assert.ElementsMatch(t, []string{"uirole:a:b:c", "uirole:a:b:d"}, find("uirole:picker").Permissions)

	// The permission ceiling (F10): a platform role holding a permission the
	// manager lacks. The editor locks it and posts it back; saving with it
	// keeps it, a forged post without it is refused.
	do(t, rolesPost("/ui/roles/new", url.Values{"application": {"platform"}, "roleName": {"ui-pg-locked"}, "displayName": {"Locked"}}))
	locked := find("platform:ui-pg-locked")
	require.NotNil(t, locked)
	do(t, rolesPost("/ui/roles/platform:ui-pg-locked/edit", url.Values{"displayName": {"Locked"}, "permission": {"platform:iam:role:view", "platform:iam:user:delete"}}))
	require.ElementsMatch(t, []string{"platform:iam:role:view", "platform:iam:user:delete"}, find("platform:ui-pg-locked").Permissions)

	editPage := as(req{target: "/ui/roles/platform:ui-pg-locked/edit"})
	assert.Contains(t, editPage, `<input type="hidden" name="permission" value="platform:iam:user:delete">`)
	assert.NotContains(t, editPage, `type="checkbox" name="permission" value="platform:iam:user:delete"`)
	body = as(rolesPost("/ui/roles/platform:ui-pg-locked/edit", url.Values{"displayName": {"Locked 2"}, "permission": {"platform:iam:role:view", "platform:iam:user:delete"}}))
	require.NotContains(t, body, "you hold yourself", body)
	kept := find("platform:ui-pg-locked")
	assert.Equal(t, "Locked 2", kept.DisplayName)
	assert.ElementsMatch(t, []string{"platform:iam:role:view", "platform:iam:user:delete"}, kept.Permissions)

	body = as(rolesPost("/ui/roles/platform:ui-pg-locked/edit", url.Values{"displayName": {"Locked 3"}, "permission": {"platform:iam:role:view"}}))
	assert.Contains(t, body, "You may only add or remove permissions you hold yourself", body)
	assert.Contains(t, body, "platform:iam:user:delete", "the refusal names the permission")
	assert.Equal(t, "Locked 2", find("platform:ui-pg-locked").DisplayName, "a refused save changes nothing")

	// Deleting a role that holds it is refused too, and shown as a lock.
	assert.Contains(t, as(req{target: "/ui/roles"}), "so you cannot delete it")
	body = as(rolesPost("/ui/roles/platform:ui-pg-locked/delete", url.Values{}))
	assert.Contains(t, body, "You may only add or remove permissions you hold yourself", body)
	require.NotNil(t, find("platform:ui-pg-locked"))

	// Callers without the permission change nothing (forged posts).
	for _, ac := range [][]string{{rolesViewPerm}, {rolesViewPerm, rolesCreatePerm, rolesUpdatePerm}} {
		serveMounted(t, rolesUser(ac...), rolesPost("/ui/roles/uirole:picker/delete", url.Values{}), roles)
	}
	serveMounted(t, rolesClientUser(), rolesPost("/ui/roles/uirole:picker/delete", url.Values{}), roles)
	require.NotNil(t, find("uirole:picker"))

	// The delete is the use case's, and audited.
	id := picker.ID
	do(t, rolesPost("/ui/roles/uirole:picker/delete", url.Values{"drawer": {id}}))
	assert.Nil(t, find("uirole:picker"))
	assert.Equal(t, 3, auditCount(t, id), "create, update and delete are audited")
	do(t, rolesPost("/ui/roles/platform:ui-pg-locked/delete", url.Values{}))
	assert.Nil(t, find("platform:ui-pg-locked"))

	// The platform sync (all three permissions and the ceiling over what it
	// changes): a super-admin runs it, and the tally is reported.
	body = do(t, rolesPost("/ui/roles/sync-platform", url.Values{}))
	assert.Contains(t, body, "Platform Roles Synced", body)
	assert.NotNil(t, find("platform:super-admin"))
	assert.Equal(t, role.SourceCode, find("platform:super-admin").Source)

	// Permissions: defined once, idempotent, listed afterwards.
	form := url.Values{"application": {"uirole"}, "context": {"billing"}, "aggregate": {"invoice"}, "action": {"approve"}, "description": {"Approve invoices"}}
	body = as(rolesPost("/ui/permissions/new", form))
	assert.Contains(t, body, "Permission uirole:billing:invoice:approve created", body)
	p, err := catalog.FindByCode(ctx, "uirole:billing:invoice:approve")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "Approve invoices", deref(p.Description))
	assert.Contains(t, do(t, req{target: "/ui/permissions?app=uirole"}), "uirole:billing:invoice:approve")
	assert.Contains(t, do(t, rolesPost("/ui/permissions/new", form)), "This permission already exists.")
	forged := url.Values{"application": {"uirole"}, "context": {"billing"}, "aggregate": {"invoice"}, "action": {"forged"}}
	serveMounted(t, rolesUser(rolesViewPerm, rolesUpdatePerm), rolesPost("/ui/permissions/new", forged), perms)
	gone, err := catalog.FindByCode(ctx, "uirole:billing:invoice:forged")
	require.NoError(t, err)
	assert.Nil(t, gone, "a caller without role:create defines nothing")
	entries, err := store.Catalogue(ctx, "uirole")
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(entries, func(e bff.PermissionEntry) bool { return e.Permission == "uirole:billing:invoice:approve" }))
}
