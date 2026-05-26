package provider

import (
	"github.com/ory/fosite"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
)

// fositeClient adapts auth.OAuthClient onto fosite.Client. We don't
// implement the OpenIDConnectClient extras yet — that's needed when we
// add the authorization_code flow + ID-token issuance.
type fositeClient struct{ c *auth.OAuthClient }

// AdaptClient wraps an auth.OAuthClient as a fosite.Client.
func AdaptClient(c *auth.OAuthClient) fosite.Client { return &fositeClient{c: c} }

func (a *fositeClient) GetID() string { return a.c.ClientID }

func (a *fositeClient) GetHashedSecret() []byte {
	if a.c.SecretHash == nil {
		return nil
	}
	return []byte(*a.c.SecretHash)
}

func (a *fositeClient) GetRedirectURIs() []string { return a.c.RedirectURIs }

func (a *fositeClient) GetGrantTypes() fosite.Arguments {
	if len(a.c.GrantTypes) == 0 {
		return fosite.Arguments{"authorization_code"}
	}
	return fosite.Arguments(a.c.GrantTypes)
}

// GetResponseTypes is required for authorization_code; we default to
// "code" until we surface this on auth.OAuthClient.
func (a *fositeClient) GetResponseTypes() fosite.Arguments {
	return fosite.Arguments{"code"}
}

func (a *fositeClient) GetScopes() fosite.Arguments { return fosite.Arguments(a.c.Scopes) }

func (a *fositeClient) IsPublic() bool { return a.c.ClientType == auth.OAuthClientPublic }

// GetAudience is empty for now — FlowCatalyst doesn't pin audience per
// client. Set this once we add an Audience column to oauth_clients.
func (a *fositeClient) GetAudience() fosite.Arguments { return nil }
