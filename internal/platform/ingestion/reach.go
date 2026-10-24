package ingestion

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// CheckReach reports whether the caller may act on this ingestion: below
// anchor tier only on the ingestions of its own clients, and an
// application-scoped principal (an application's service account, even at
// anchor tier) only on its own applications'. Holding the permission is the
// caller's separate gate: reach is not authority.
func CheckReach(ac *auth.AuthContext, i *Ingestion) error {
	clientID := string(i.ClientID)
	if err := auth.CheckScopeAccess(ac, &clientID); err != nil {
		return err
	}
	return CheckApplicationReach(ac, string(i.ApplicationID))
}

// CheckApplicationReach is the application axis of CheckReach, for a create
// that has resolved its application but has no ingestion yet.
func CheckApplicationReach(ac *auth.AuthContext, applicationID string) error {
	if !ac.CanAccessApplication(applicationID) {
		return usecase.Authorization("SCOPE_FORBIDDEN", "no access to this ingestion's application")
	}
	return nil
}

// ListReach returns the filters that confine a list to what the caller
// reaches: nil for an axis it reaches in full.
func ListReach(ac *auth.AuthContext) (clients, applications *[]string) {
	if !ac.IsAnchor() {
		c := append([]string{}, ac.Clients...)
		clients = &c
	}
	if ac.IsApplicationScoped() {
		a := append([]string{}, ac.Applications...)
		applications = &a
	}
	return clients, applications
}
