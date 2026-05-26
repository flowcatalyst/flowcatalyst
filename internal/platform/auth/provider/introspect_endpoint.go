package provider

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// IntrospectEndpoint serves POST /oauth/introspect (RFC 7662). Returns
// JSON metadata about a token if active, or {"active":false} otherwise.
type IntrospectEndpoint struct{ provider *Provider }

// NewIntrospectEndpoint wires the handler.
func NewIntrospectEndpoint(p *Provider) *IntrospectEndpoint {
	return &IntrospectEndpoint{provider: p}
}

// RegisterRoutes mounts POST /oauth/introspect.
func (e *IntrospectEndpoint) RegisterRoutes(r chi.Router) {
	r.Post("/oauth/introspect", e.handle)
}

func (e *IntrospectEndpoint) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := NewSession()
	resp, err := e.provider.OAuth2.NewIntrospectionRequest(ctx, r, session)
	if err != nil {
		e.provider.OAuth2.WriteIntrospectionError(ctx, w, err)
		return
	}
	e.provider.OAuth2.WriteIntrospectionResponse(ctx, w, resp)
}
