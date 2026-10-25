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

// The dispatch-job write actions are their own permissions (they were all
// dispatch-job:view). The built-in messaging-admin role holds the four ordinary
// ones; redrive-completed (re-delivering jobs that already succeeded) is held
// by no built-in role but super-admin's wildcard; the read-only viewer role
// holds none. Existing deployments get the grants from migration 085.
func TestDispatchJobWritePermissionsInBuiltInRoles(t *testing.T) {
	writes := []string{
		"platform:messaging:dispatch-job:requeue",
		"platform:messaging:dispatch-job:cancel",
		"platform:messaging:dispatch-job:complete",
		"platform:messaging:dispatch-job:edit-payload",
	}
	const redrive = "platform:messaging:dispatch-job:redrive-completed"
	seen := map[string]bool{}
	for _, r := range PlatformRoles() {
		seen[r.Name] = true
		if slices.Contains(r.Permissions, redrive) {
			t.Errorf("role %s holds %s; no built-in role should", r.Name, redrive)
		}
		switch r.Name {
		case "platform:messaging-admin":
			for _, p := range append([]string{"platform:messaging:dispatch-job:view"}, writes...) {
				if !slices.Contains(r.Permissions, p) {
					t.Errorf("messaging-admin missing %q", p)
				}
			}
		case "platform:viewer", "platform:admin-readonly", "platform:application-service":
			for _, p := range writes {
				if slices.Contains(r.Permissions, p) {
					t.Errorf("role %s must not hold %q", r.Name, p)
				}
			}
		}
	}
	if !seen["platform:messaging-admin"] {
		t.Fatal("messaging-admin role not found")
	}
}

// TestFunctionTracePermissionsInBuiltInRoles pins who holds the trace
// permissions (docs/function-trace-design.md section 1): operators switch
// tracing on and read bodies, readers read records with bodies withheld, the
// CI deployer role stays minimal. Existing deployments get the same from
// migration 087.
func TestFunctionTracePermissionsInBuiltInRoles(t *testing.T) {
	want := map[string][]string{
		"platform:messaging-admin":     {"platform:function:trace:view", "platform:function:trace:manage", "platform:function:trace:view-bodies"},
		"platform:viewer":              {"platform:function:trace:view"},
		"platform:function-publisher":  {"platform:function:trace:view"},
		"platform:function-deployer":   nil,
		"platform:admin":               nil,
		"platform:super-admin":         nil, // wildcard
		"platform:function-runner":     nil,
		"platform:application-service": nil,
	}
	got := map[string][]string{}
	for _, r := range PlatformRoles() {
		for _, p := range r.Permissions {
			if strings.HasPrefix(p, "platform:function:trace:") {
				got[r.Name] = append(got[r.Name], p)
			}
		}
	}
	for role, perms := range want {
		slices.Sort(perms)
		g := got[role]
		slices.Sort(g)
		if !slices.Equal(g, perms) {
			t.Errorf("%s holds trace permissions %v, want %v", role, g, perms)
		}
	}
}

// TestIngestionRoles pins who holds the ingestion permissions
// (docs/ingestion-design.md section 4): the admin holds every human grant but
// route, route authoring is a role of its own, and only the runtime's role
// holds the runtime API.
func TestIngestionRoles(t *testing.T) {
	const (
		view     = "platform:ingestion:ingestion:view"
		create   = "platform:ingestion:ingestion:create"
		update   = "platform:ingestion:ingestion:update"
		del      = "platform:ingestion:ingestion:delete"
		promote  = "platform:ingestion:ingestion:promote"
		test     = "platform:ingestion:ingestion:test"
		route    = "platform:ingestion:ingestion:route"
		apply    = "platform:ingestion:set:apply"
		operate  = "platform:ingestion:runtime:operate"
		identity = "platform:ingestion:ingestion:runtime-identity"
		secrets  = "platform:ingestion:ingestion:bind-secrets"
	)
	want := map[string][]string{
		"platform:ingestion-admin":        {view, create, update, del, promote, test, apply, identity, secrets},
		"platform:ingestion-operator":     {view, update, test},
		"platform:ingestion-viewer":       {view},
		"platform:ingestion-route-author": {view, create, update, route},
		"platform:ingestion-runtime":      {operate},
	}
	got := map[string][]string{}
	for _, r := range PlatformRoles() {
		for _, p := range r.Permissions {
			if strings.HasPrefix(p, "platform:ingestion:") {
				got[r.Name] = append(got[r.Name], p)
			}
		}
	}
	for role, perms := range want {
		slices.Sort(perms)
		g := got[role]
		slices.Sort(g)
		if !slices.Equal(g, perms) {
			t.Errorf("%s holds %v, want %v", role, g, perms)
		}
		delete(got, role)
	}
	for role, perms := range got {
		t.Errorf("%s must not hold ingestion permissions, has %v", role, perms)
	}
}
