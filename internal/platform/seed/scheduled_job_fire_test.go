package seed

import (
	"slices"
	"testing"
)

// The application-service role carries the scheduled-job fire grant (look up and
// fire its own application's jobs). Existing deployments get it from migration 100.
func TestApplicationServiceRoleCarriesScheduledJobFire(t *testing.T) {
	for _, r := range PlatformRoles() {
		if r.Name == "platform:application-service" {
			if !slices.Contains(r.Permissions, "platform:application-service:scheduled-job:fire") {
				t.Error("application-service role missing scheduled-job:fire")
			}
			return
		}
	}
	t.Fatal("platform:application-service role not found")
}
