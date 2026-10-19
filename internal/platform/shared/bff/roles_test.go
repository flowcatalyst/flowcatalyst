package bff

import "testing"

func TestBuiltinPermissionsListTheDispatchJobWriteActions(t *testing.T) {
	have := map[string]bool{}
	for _, p := range builtinPermissions() {
		have[p.Permission] = true
	}
	for _, want := range []string{
		"platform:messaging:dispatch-job:view",
		"platform:messaging:dispatch-job:requeue",
		"platform:messaging:dispatch-job:redrive-completed",
		"platform:messaging:dispatch-job:cancel",
		"platform:messaging:dispatch-job:complete",
		"platform:messaging:dispatch-job:edit-payload",
	} {
		if !have[want] {
			t.Errorf("builtin permission catalogue lacks %q", want)
		}
	}
}
