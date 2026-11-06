package client

import "context"

// FieldTokenIntrospectRequest — POST /api/field-tokens/introspect.
type FieldTokenIntrospectRequest struct {
	Token string `json:"token"`
}

// FieldTokenIntrospectResponse is the online check of a field access token.
// Only Active is set when the token is unusable (bad signature, expired,
// revoked session, suspended worker or device, or outside the caller's reach).
type FieldTokenIntrospectResponse struct {
	Active     bool     `json:"active"`
	Sub        string   `json:"sub,omitempty"`
	ClientID   string   `json:"clientId,omitempty"`
	WorkerCode string   `json:"workerCode,omitempty"`
	DeviceID   string   `json:"deviceId,omitempty"`
	AppCode    string   `json:"appCode,omitempty"`
	Amr        []string `json:"amr,omitempty"`
	AuthTime   string   `json:"authTime,omitempty"`
	Exp        string   `json:"exp,omitempty"`
	// Act is present when someone other than the worker is signed in as them;
	// Test is true for a test identity.
	Act  *FieldTokenActor `json:"act,omitempty"`
	Test bool             `json:"test,omitempty"`
	// LinkedPrincipalID is the platform user the worker is linked to, when
	// linked (audit correlation only; it grants nothing).
	LinkedPrincipalID string `json:"linkedPrincipalId,omitempty"`
}

// FieldTokenActor is the act claim: Typ is "field_worker" or "principal".
type FieldTokenActor struct {
	Sub string `json:"sub"`
	Typ string `json:"typ"`
}

// FieldTokensResource — /api/field-tokens/*.
type FieldTokensResource struct {
	c *FlowCatalystClient
}

// Introspect — POST /api/field-tokens/introspect. Use it when an offline
// signature check (auth.FieldTokenVerifier) is not enough and the session must
// still be live.
func (r *FieldTokensResource) Introspect(ctx context.Context, token string) (*FieldTokenIntrospectResponse, error) {
	var out FieldTokenIntrospectResponse
	if err := r.c.Post(ctx, "/api/field-tokens/introspect", &FieldTokenIntrospectRequest{Token: token}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
