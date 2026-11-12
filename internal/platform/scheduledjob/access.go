package scheduledjob

import "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"

// CanViewJob reports whether ac may read a job's firings: a client's job needs
// access to that client; a platform-level job (no client) is anchor-only. The
// view permission says what ac may read; this says for which tenants. Shared by
// the API and BFF instance reads so both answer alike.
func CanViewJob(ac *auth.AuthContext, j *ScheduledJob) bool {
	if j.ClientID == nil {
		return ac.IsAnchor()
	}
	return ac.CanAccessClient(string(*j.ClientID))
}

// CanViewInstance is CanViewJob for one firing (and its logs).
func CanViewInstance(ac *auth.AuthContext, inst *ScheduledJobInstance) bool {
	if inst.ClientID == nil {
		return ac.IsAnchor()
	}
	return ac.CanAccessClient(*inst.ClientID)
}
