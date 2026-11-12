package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Delivery credentials. A webhook or scheduled-job firing carries the bearer
// token and HMAC signature of a service account (the subscription's own, its
// connection's, or the application's). Whoever sets or changes the URL, or the
// credentials, of signed work must therefore be allowed to use those
// credentials, or they could point the delivery at a URL they own and collect
// a token and signatures they were never given. These are the two checks every
// such write makes (subscriptions, connections, scheduled jobs).

// MayWieldServiceAccount reports (nil) whether ac may name sa as the signer of
// a delivery. sa nil is SERVICE_ACCOUNT_NOT_FOUND and an inactive account is
// SERVICE_ACCOUNT_INACTIVE (400). Otherwise:
//
//   - an anchor with all-applications access may use any account;
//   - an application-scoped anchor (an application's own service account) only
//     an account bound to an application it can access;
//   - anyone else only an account linked to at least one client, all of which
//     the caller can access, and bound to no application or one the caller can
//     access. An account linked to no client (every application-provisioned
//     account) is anchor-only.
//
// Refusal is 403 SCOPE_FORBIDDEN.
func MayWieldServiceAccount(ac *auth.AuthContext, sa *serviceaccount.ServiceAccount) error {
	if sa == nil {
		return usecase.Validation("SERVICE_ACCOUNT_NOT_FOUND", "service account does not exist")
	}
	if !sa.Active {
		return usecase.Validation("SERVICE_ACCOUNT_INACTIVE", "service account '"+sa.Code+"' is inactive")
	}
	forbidden := usecase.Authorization("SCOPE_FORBIDDEN", "no access to service account '"+sa.Code+"'")
	var appID string
	if sa.ApplicationID != nil {
		appID = string(*sa.ApplicationID)
	}
	if ac.IsAnchor() {
		if !ac.IsApplicationScoped() {
			return nil
		}
		if sa.ApplicationID != nil && ac.CanAccessApplication(appID) {
			return nil
		}
		return forbidden
	}
	if len(sa.ClientIDs) == 0 {
		return forbidden
	}
	for _, cid := range sa.ClientIDs {
		if !ac.CanAccessClient(cid) {
			return forbidden
		}
	}
	if sa.ApplicationID != nil && !ac.CanAccessApplication(appID) {
		return forbidden
	}
	return nil
}

// MayWieldApplicationIdentity reports (nil) whether ac may have deliveries of
// the application applicationID signed by the application's own service
// account: an anchor with all-applications access, or an application-scoped
// anchor bound to that application. Anyone else is refused with 403
// APPLICATION_ACCESS_REQUIRED.
func MayWieldApplicationIdentity(ac *auth.AuthContext, applicationID string) error {
	if ac.IsAnchor() && ac.CanAccessApplication(applicationID) {
		return nil
	}
	return usecase.Authorization("APPLICATION_ACCESS_REQUIRED", "not authorised to deliver as this application")
}

// RequireWieldableAccount loads the service account id and applies
// MayWieldServiceAccount to the caller in ctx. An id that names no account is
// SERVICE_ACCOUNT_NOT_FOUND.
func RequireWieldableAccount(ctx context.Context, repo *serviceaccount.Repository, id string) (*serviceaccount.ServiceAccount, error) {
	sa, err := repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "service account lookup failed", err)
	}
	if err := MayWieldServiceAccount(auth.FromContext(ctx), sa); err != nil {
		return nil, err
	}
	return sa, nil
}
