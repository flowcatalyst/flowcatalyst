package operations

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// CheckReach reports whether the caller may act on this function: the client
// axis first (a platform-owned function is anchor-only; below anchor only a
// client the caller reaches), then the application axis (an
// application-scoped principal, such as an application's service account,
// only on its own applications' functions). Holding the permission is the
// caller's separate gate: reach is not authority.
func CheckReach(ac *auth.AuthContext, f *function.Function) error {
	if err := auth.CheckScopeAccess(ac, f.ClientID); err != nil {
		return err
	}
	return CheckApplicationReach(ac, f.ApplicationID)
}

// CheckApplicationReach is the application axis of CheckReach, for a create
// that has an application but no function yet.
func CheckApplicationReach(ac *auth.AuthContext, applicationID string) error {
	if !ac.CanAccessApplication(applicationID) {
		return usecase.Authorization("SCOPE_FORBIDDEN", "no access to this function's application")
	}
	return nil
}
