package identityprovider

import (
	"context"
	"strings"
)

// PortalIdPForDomain finds an identity provider bound to the given client's
// PORTAL plane (PortalClientID) that owns the email domain. Only
// portal-bound IdPs participate — an employee-plane IdP never serves a
// portal. Returns nil when no bound IdP owns the domain (→ password auth).
func PortalIdPForDomain(ctx context.Context, repo *Repository, portalClientID, domain string) (*IdentityProvider, error) {
	idps, err := repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range idps {
		idp := &idps[i]
		if idp.PortalClientID == nil || *idp.PortalClientID != portalClientID {
			continue
		}
		for _, d := range idp.AllowedEmailDomains {
			if strings.EqualFold(d, domain) {
				return idp, nil
			}
		}
	}
	return nil, nil
}
