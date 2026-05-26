package provider

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ory/fosite"
)

// TokenEndpoint serves POST /oauth/token. It delegates the OAuth2
// protocol entirely to fosite — our only inputs are (a) the storage +
// hasher fosite was constructed with, and (b) a per-request hook to
// stamp FlowCatalyst extra claims onto the session before fosite signs
// the JWT.
type TokenEndpoint struct {
	provider *Provider
}

// NewTokenEndpoint wires the handler.
func NewTokenEndpoint(p *Provider) *TokenEndpoint { return &TokenEndpoint{provider: p} }

// RegisterRoutes mounts POST /oauth/token on the supplied router.
func (e *TokenEndpoint) RegisterRoutes(r chi.Router) {
	r.Post("/oauth/token", e.handle)
}

func (e *TokenEndpoint) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := NewSession()

	accessRequest, err := e.provider.OAuth2.NewAccessRequest(ctx, r, session)
	if err != nil {
		e.provider.OAuth2.WriteAccessError(ctx, w, accessRequest, err)
		return
	}

	// For client_credentials, set subject to the OAuth client's owning
	// principal and populate our Claims onto the JWT session. fosite's
	// ClientCredentialsGrantHandler doesn't pick a subject otherwise.
	if accessRequest.GetGrantTypes().ExactOne("client_credentials") {
		client, ok := accessRequest.GetClient().(*fositeClient)
		if !ok || client.c.PrincipalID == nil || *client.c.PrincipalID == "" {
			e.provider.OAuth2.WriteAccessError(ctx, w, accessRequest,
				fosite.ErrInvalidClient.WithHint("Client has no owning principal."))
			return
		}
		claims, err := BuildClaims(ctx, e.provider.cfg, e.provider.principals, e.provider.roles, *client.c.PrincipalID)
		if err != nil {
			e.provider.OAuth2.WriteAccessError(ctx, w, accessRequest,
				fosite.ErrServerError.WithWrap(err).WithDescription("Principal resolution failed."))
			return
		}
		session.applyClaims(claims)

		// Mirror requested-scope onto granted-scope (fosite expects the
		// grant handler to set this; for client_credentials, narrowing
		// already happened in fosite's scope validator).
		for _, s := range accessRequest.GetRequestedScopes() {
			accessRequest.GrantScope(s)
		}
		for _, a := range accessRequest.GetRequestedAudience() {
			accessRequest.GrantAudience(a)
		}
	}

	response, err := e.provider.OAuth2.NewAccessResponse(ctx, accessRequest)
	if err != nil {
		e.provider.OAuth2.WriteAccessError(ctx, w, accessRequest, err)
		return
	}
	e.provider.OAuth2.WriteAccessResponse(ctx, w, accessRequest, response)
}
