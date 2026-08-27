package oauthapi

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// mintInteractiveAccessToken picks the access-token shape for an interactive
// login (authorization_code and its refresh). Default: an identity-only
// token (token_use=identity, rejected as an API bearer). For an OAuth client
// flagged apiAccess — trusted first-party apps opted in per client — it
// mints an authority-bearing token (token_use=api) narrowed to the client:
//
//   - roles: only those bound to the client's ApplicationIDs (platform-wide
//     roles drop out), via the same FilterRolesForApplications the id_token
//     narrowing uses. An unscoped client (no ApplicationIDs) keeps the full
//     role set — mirroring the id_token backward-compat rule.
//   - applications claim: the user's accessible applications ∩ the client's;
//     AllApplications is forced false on a scoped client.
//   - scope claim: granted permissions derived from the NARROWED roles,
//     further intersected with the requested scope (grantedScope).
//
// So an app-scoped client can never mint authority beyond its own
// applications, however broad the user's platform access is.
func (s *State) mintInteractiveAccessToken(ctx context.Context, p *principal.Principal, client *auth.OAuthClient, requestedScope string) (string, error) {
	if client == nil {
		return s.Auth.GenerateIdentityAccessToken(p)
	}
	if !client.APIAccess {
		return s.Auth.GenerateIdentityAccessTokenFor(p, client.ClientID)
	}
	narrowed := p
	if len(client.ApplicationIDs) > 0 && s.FilterRolesForApplications != nil {
		kept, err := s.FilterRolesForApplications(ctx, roleNamesOf(p), client.ApplicationIDs)
		if err != nil {
			return "", err
		}
		keep := make(map[string]struct{}, len(kept))
		for _, n := range kept {
			keep[n] = struct{}{}
		}
		clone := *p
		clone.Roles = nil
		for _, ra := range p.Roles {
			if _, ok := keep[ra.Role]; ok {
				clone.Roles = append(clone.Roles, ra)
			}
		}
		clone.AllApplications = false
		clone.AccessibleApplicationIDs = intersectApps(p, client.ApplicationIDs)
		narrowed = &clone
	}
	granted, _, err := s.grantedScope(ctx, narrowed, requestedScope)
	if err != nil {
		return "", err
	}
	return s.Auth.GenerateAccessTokenWithScopeFor(narrowed, granted, client.ClientID)
}

// intersectApps returns the client's application ids the user can actually
// access: all of them when the user holds AllApplications, else the
// intersection with the user's explicit access list.
func intersectApps(p *principal.Principal, clientApps []string) []string {
	if p.AllApplications {
		return append([]string(nil), clientApps...)
	}
	userApps := make(map[string]struct{}, len(p.AccessibleApplicationIDs))
	for _, id := range p.AccessibleApplicationIDs {
		userApps[id] = struct{}{}
	}
	out := make([]string, 0, len(clientApps))
	for _, id := range clientApps {
		if _, ok := userApps[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
