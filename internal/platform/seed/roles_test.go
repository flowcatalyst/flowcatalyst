package seed

import (
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
