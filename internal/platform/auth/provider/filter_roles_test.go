package provider

import (
	"context"
	"reflect"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
)

// fakeRoleLookup stands in for *role.Repository so the filter logic is testable
// without a database. byName is the exact iam_roles.name index; byShortApps
// simulates RoleFindByShortNameInApps (bare short name → role, scoped to the
// role's own application).
type fakeRoleLookup struct {
	byName     map[string]*role.Role
	byShortApp map[string]*role.Role
}

func (f fakeRoleLookup) FindByName(_ context.Context, name string) (*role.Role, error) {
	return f.byName[name], nil
}

func (f fakeRoleLookup) FindByShortNameInApps(_ context.Context, shortName string, appIDs []string) (*role.Role, error) {
	r := f.byShortApp[shortName]
	if r == nil || r.ApplicationID == nil {
		return nil, nil
	}
	for _, id := range appIDs {
		if id == *r.ApplicationID {
			return r, nil
		}
	}
	return nil, nil
}

func TestFilterRolesForApplications(t *testing.T) {
	appHR, appBilling, appLog := "app_hr", "app_billing", "app_logistics"
	hrManager := &role.Role{Name: "hr:hr-manager", ApplicationID: &appHR, ApplicationCode: "hr"}
	billingViewer := &role.Role{Name: "billing:viewer", ApplicationID: &appBilling, ApplicationCode: "billing"}
	platformAdmin := &role.Role{Name: "platform:admin"} // ApplicationID nil
	// Malformed: the role's own short name contains a colon.
	logDash := &role.Role{Name: "logistics_portal:dashboard:user", ApplicationID: &appLog, ApplicationCode: "logistics_portal"}

	lookup := fakeRoleLookup{
		byName: map[string]*role.Role{
			"hr:hr-manager":                   hrManager,
			"billing:viewer":                  billingViewer,
			"platform:admin":                  platformAdmin,
			"logistics_portal:dashboard:user": logDash,
		},
		// SDK-synced principals carry the bare short name only.
		byShortApp: map[string]*role.Role{
			"hr-manager":     hrManager,
			"viewer":         billingViewer,
			"dashboard:user": logDash,
		},
	}

	cases := []struct {
		name   string
		roles  []string
		appIDs []string
		want   []string
	}{
		{
			// A bare assignment still resolves via the short-name fallback, but
			// is emitted CANONICALLY — narrowing normalises the spelling rather
			// than propagating whichever form happened to be stored.
			name:   "bare sync name emitted canonically",
			roles:  []string{"hr-manager"},
			appIDs: []string{appHR},
			want:   []string{"hr:hr-manager"},
		},
		{
			// Console-assigned prefixed name resolves via FindByName and is
			// emitted unchanged — both storage forms converge on one spelling.
			name:   "prefixed console name emitted unchanged",
			roles:  []string{"hr:hr-manager"},
			appIDs: []string{appHR},
			want:   []string{"hr:hr-manager"},
		},
		{
			// Multi-colon role names (legacy app codes that predate the
			// convention) pass through whole — nothing is stripped, so there is
			// no delimiter ambiguity left to get wrong.
			name:   "multi-colon role passes through whole",
			roles:  []string{"logistics_portal:dashboard:user"},
			appIDs: []string{appLog},
			want:   []string{"logistics_portal:dashboard:user"},
		},
		{
			// Its bare form resolves to the same canonical name.
			name:   "multi-colon bare role resolves canonically",
			roles:  []string{"dashboard:user"},
			appIDs: []string{appLog},
			want:   []string{"logistics_portal:dashboard:user"},
		},
		{
			// A role belonging to another app is dropped (no privilege bleed).
			name:   "bare name for another app is dropped",
			roles:  []string{"viewer"},
			appIDs: []string{appHR},
			want:   []string{},
		},
		{
			// Platform roles (no application) never belong on an app-scoped RP.
			name:   "platform role dropped",
			roles:  []string{"platform:admin"},
			appIDs: []string{appHR},
			want:   []string{},
		},
		{
			name:   "unknown name dropped",
			roles:  []string{"does-not-exist"},
			appIDs: []string{appHR},
			want:   []string{},
		},
		{
			name:   "mixed set keeps only in-app roles, canonically named",
			roles:  []string{"hr-manager", "viewer", "platform:admin"},
			appIDs: []string{appHR},
			want:   []string{"hr:hr-manager"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filterRolesForApplications(context.Background(), lookup, tc.roles, tc.appIDs)
			if err != nil {
				t.Fatalf("filterRolesForApplications: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
