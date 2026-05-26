package provider

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RevokeEndpoint serves POST /oauth/revoke (RFC 7009). fosite handles
// the full request lifecycle — we only thread the request through.
type RevokeEndpoint struct{ provider *Provider }

// NewRevokeEndpoint wires the handler.
func NewRevokeEndpoint(p *Provider) *RevokeEndpoint { return &RevokeEndpoint{provider: p} }

// RegisterRoutes mounts POST /oauth/revoke.
func (e *RevokeEndpoint) RegisterRoutes(r chi.Router) {
	r.Post("/oauth/revoke", e.handle)
}

func (e *RevokeEndpoint) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := e.provider.OAuth2.NewRevocationRequest(ctx, r)
	e.provider.OAuth2.WriteRevocationResponse(ctx, w, err)
}
