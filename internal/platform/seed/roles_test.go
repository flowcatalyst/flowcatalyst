package seed

import (
	"slices"
	"strings"
	"testing"
)

// TestPlatformPermissionsHaveFourSegments pins the permission shape every
// platform permission shares — platform:<subdomain>:<resource>:<action> —
// because wildcard matching is segment-wise with equal segment counts: a
// three-segment permission is one platform:*:*:* (super-admin) can never
// match. The function permissions were once three segments and super-admin
// could not so much as view a function.
func TestPlatformPermissionsHaveFourSegments(t *testing.T) {
	for _, r := range PlatformRoles() {
		for _, p := range r.Permissions {
			if n := len(strings.Split(p, ":")); n != 4 {
				t.Errorf("role %s: permission %q has %d segments, want 4", r.Name, p, n)
			}
		}
	}
}

// TestApplicationServiceRoleCarriesBatchIngestPermissions pins the owner
// ruling that each application delivers its own outbox with its own
// service-account credentials: the built-in application-service role holds
// the three batch-ingest permissions. Existing deployments get them from
// migration 069 (the seeder never rewrites an existing role).
func TestApplicationServiceRoleCarriesBatchIngestPermissions(t *testing.T) {
	want := []string{
		"platform:messaging:batch:events-write",
		"platform:messaging:batch:dispatch-jobs-write",
		"platform:admin:batch:audit-logs-write",
		// F19: an application syncs its own dispatch pools.
		"platform:messaging:dispatch-pool:sync",
	}
	for _, r := range PlatformRoles() {
		if r.Name != "platform:application-service" {
			continue
		}
		for _, p := range want {
			if !slices.Contains(r.Permissions, p) {
				t.Errorf("application-service role missing %q", p)
			}
		}
		return
	}
	t.Fatal("platform:application-service role not found")
}

// TestApplicationServiceRoleCarriesAdminPagePermissions pins F20: the built-in
// application-service role holds the permissions an application's own admin
// pages need, and no console (platform:iam / platform:messaging view)
// permission that would lift the application confinement. Existing
// deployments get them from migration 080.
func TestApplicationServiceRoleCarriesAdminPagePermissions(t *testing.T) {
	want := []string{
		"platform:application-service:event:view",
		"platform:application-service:dispatch-job:view",
		"platform:application-service:user:view",
		"platform:application-service:user:create",
		"platform:application-service:invitation:view",
		"platform:application-service:invitation:manage",
	}
	unconfining := []string{
		"platform:messaging:event:view", "platform:messaging:dispatch-job:view",
		"platform:iam:user:view", "platform:iam:user:create", "platform:*:*:*",
	}
	for _, r := range PlatformRoles() {
		if r.Name != "platform:application-service" {
			continue
		}
		for _, p := range want {
			if !slices.Contains(r.Permissions, p) {
				t.Errorf("application-service role missing %q", p)
			}
		}
		for _, p := range unconfining {
			if slices.Contains(r.Permissions, p) {
				t.Errorf("application-service role must not hold %q (it lifts the application confinement)", p)
			}
		}
		return
	}
	t.Fatal("platform:application-service role not found")
}

// TestFunctionDeployerRole pins F26: the CI deploy role holds exactly what
// `fcdev fn deploy --create --client <code>` needs, and nothing that widens it
// (no secret manage, no invoke, no domain/route manage, no wildcard).
func TestFunctionDeployerRole(t *testing.T) {
	want := []string{
		"platform:function:function:view",
		"platform:function:function:manage",
		"platform:function:version:publish",
		"platform:function:alias:promote",
		"platform:admin:client:view",
		"platform:admin:application:view",
	}
	for _, r := range PlatformRoles() {
		if r.Name != "platform:function-deployer" {
			continue
		}
		got := slices.Clone(r.Permissions)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("function-deployer permissions = %v, want %v", got, want)
		}
		if r.Description == nil || !strings.Contains(*r.Description, "DELETING") ||
			!strings.Contains(*r.Description, "function-publisher") {
			t.Errorf("description must state the delete caveat and point to function-publisher: %v", r.Description)
		}
		return
	}
	t.Fatal("platform:function-deployer role not found")
}
