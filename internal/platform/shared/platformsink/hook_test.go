package platformsink

import "testing"

func TestAffectsRouterConfig(t *testing.T) {
	for typ, want := range map[string]bool{
		"platform:admin:client:created":          true,
		"platform:admin:client:deleted":          true,
		"platform:admin:dispatch-pool:created":   true,
		"platform:admin:dispatch-pools:synced":   true,
		"platform:admin:subscription:created":    true,
		"platform:admin:subscription:synced":     true,
		"platform:admin:connection:created":      false,
		"platform:admin:event-type:created":      false,
		"platform:iam:user:created":              false,
		"platform:admin:scheduled-job:created":   false,
		"platform:admin:clientele:created":       false,
		"platform:admin:subscriptions-other:foo": false,
	} {
		if got := affectsRouterConfig(typ); got != want {
			t.Errorf("affectsRouterConfig(%q) = %v, want %v", typ, got, want)
		}
	}
}
