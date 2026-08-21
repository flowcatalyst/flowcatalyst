package identityprovider

import (
	"context"
	"strings"
)

// OIDCProviderForDomain finds the external (OIDC) identity provider that owns
// the email domain. Domain ownership is the ONLY criterion: an IdP's job is
// authentication — proving a person owns an email at their org — so any login
// attempt for an owned domain routes to its IdP regardless of which surface
// (employee login, any client's portal) the attempt came from. Which client's
// portal identity a successful portal login materialises is decided by the
// FLOW (the portal-flagged OAuth client), never by the IdP.
//
// Returns nil when no OIDC provider owns the domain (→ password auth).
// INTERNAL providers own password domains and never participate.
func OIDCProviderForDomain(ctx context.Context, repo *Repository, domain string) (*IdentityProvider, error) {
	idps, err := repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range idps {
		idp := &idps[i]
		if idp.Type != TypeOIDC {
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
