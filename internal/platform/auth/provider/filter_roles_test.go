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
			// The prod bug: bare sync name resolves via the short-name fallback
			// and is emitted as the app-local short name — the HR app's
			// bare-keyed catalogue resolves it.
			name:   "bare sync name resolves to short name",
			roles:  []string{"hr-manager"},
			appIDs: []string{appHR},
			want:   []string{"hr-manager"},
		},
		{
			// Console-assigned prefixed name resolves via FindByName and is
			// ALSO emitted as the short name — consistent with the sync case.
			name:   "prefixed console name emitted as short name",
			roles:  []string{"hr:hr-manager"},
			appIDs: []string{appHR},
			want:   []string{"hr-manager"},
		},
		{
			// Malformed multi-colon role, prefixed form: only the first ":"
			// after the app code is the delimiter → short name keeps its colon.
			name:   "multi-colon prefixed role keeps its inner colon",
			roles:  []string{"logistics_portal:dashboard:user"},
			appIDs: []string{appLog},
			want:   []string{"dashboard:user"},
		},
		{
			// Malformed multi-colon role, bare form resolves the same way.
			name:   "multi-colon bare role resolves to same short name",
			roles:  []string{"dashboard:user"},
			appIDs: []string{appLog},
			want:   []string{"dashboard:user"},
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
			name:   "mixed set keeps only in-app roles as short names",
			roles:  []string{"hr-manager", "viewer", "platform:admin"},
			appIDs: []string{appHR},
			want:   []string{"hr-manager"},
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
