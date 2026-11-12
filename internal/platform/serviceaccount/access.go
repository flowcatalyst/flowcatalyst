package serviceaccount

import "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"

// CanView reports whether ac may read the service account sa. The view
// permission says what ac may read; this says which accounts.
//
//   - A non-anchor caller sees an account only if it is linked to at least one
//     client and the caller can access every client it is linked to: an account
//     linked to no client is platform-wide, and one reaching a client the caller
//     cannot is not the caller's to read.
//   - An application-scoped caller (one without all-applications access, such as
//     an application's own service account) additionally sees only accounts bound
//     to an application it can access, the rule operations.RequireApplicationReach
//     applies to writes.
//
// An anchor with all-applications access sees every account.
func CanView(ac *auth.AuthContext, sa *ServiceAccount) bool {
	if ac == nil || sa == nil {
		return false
	}
	if !ac.IsAnchor() {
		if len(sa.ClientIDs) == 0 {
			return false
		}
		for _, c := range sa.ClientIDs {
			if !ac.CanAccessClient(c) {
				return false
			}
		}
	}
	if ac.IsApplicationScoped() {
		return sa.ApplicationID != nil && ac.CanAccessApplication(string(*sa.ApplicationID))
	}
	return true
}

// FilterViewable returns the accounts ac may read under CanView.
func FilterViewable(ac *auth.AuthContext, accounts []ServiceAccount) []ServiceAccount {
	out := make([]ServiceAccount, 0, len(accounts))
	for i := range accounts {
		if CanView(ac, &accounts[i]) {
			out = append(out, accounts[i])
		}
	}
	return out
}
