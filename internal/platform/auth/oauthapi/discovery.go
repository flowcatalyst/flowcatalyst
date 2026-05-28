package oauthapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterDiscoveryRoutes mounts the OIDC discovery + JWKS endpoints.
func (s *State) RegisterDiscoveryRoutes(r chi.Router) {
	r.Get("/.well-known/openid-configuration", s.OpenIDConfiguration)
	r.Get("/.well-known/jwks.json", s.JWKS)
}

// openIDConfiguration is the OIDC discovery document, transcribed
// field-for-field from well_known_api.rs::OpenIdConfiguration. The
// endpoint URLs + advertised capabilities mirror Rust exactly (including
// end_session_endpoint and the hybrid response_types, which the token
// endpoint itself does not implement — the advertised list matches Rust).
type openIDConfiguration struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint,omitempty"`
	EndSessionEndpoint                string   `json:"end_session_endpoint,omitempty"`
	IntrospectionEndpoint             string   `json:"introspection_endpoint,omitempty"`
	RevocationEndpoint                string   `json:"revocation_endpoint,omitempty"`
	JwksURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	RequestParameterSupported         bool     `json:"request_parameter_supported"`
	RequestURIParameterSupported      bool     `json:"request_uri_parameter_supported"`
}

// OpenIDConfiguration serves GET /.well-known/openid-configuration.
func (s *State) OpenIDConfiguration(w http.ResponseWriter, _ *http.Request) {
	base := s.BaseURL
	writeJSON(w, http.StatusOK, openIDConfiguration{
		Issuer:                base,
		AuthorizationEndpoint: base + "/oauth/authorize",
		TokenEndpoint:         base + "/oauth/token",
		UserinfoEndpoint:      base + "/oauth/userinfo",
		EndSessionEndpoint:    base + "/auth/oidc/session/end",
		IntrospectionEndpoint: base + "/oauth/introspect",
		RevocationEndpoint:    base + "/oauth/revoke",
		JwksURI:               base + "/.well-known/jwks.json",
		ResponseTypesSupported: []string{
			"code", "token", "id_token",
			"code token", "code id_token", "token id_token",
			"code token id_token",
		},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ScopesSupported:                  []string{"openid", "profile", "email", "offline_access"},
		TokenEndpointAuthMethodsSupported: []string{
			"client_secret_basic", "client_secret_post",
		},
		GrantTypesSupported: []string{
			"authorization_code", "refresh_token", "client_credentials",
		},
		ClaimsSupported: []string{
			"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce",
			"name", "email", "email_verified", "acr", "amr", "azp",
			"type", "scope", "client_id", "roles", "applications", "clients",
		},
		CodeChallengeMethodsSupported: []string{"S256", "plain"},
		RequestParameterSupported:     false,
		RequestURIParameterSupported:  false,
	})
}

// jwkKey is one JSON Web Key (well_known_api.rs::JwkKey).
type jwkKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid,omitempty"`
	Alg string `json:"alg"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

// JWKS serves GET /.well-known/jwks.json, exposing the current key plus
// any previous keys (rotation) from authservice.
func (s *State) JWKS(w http.ResponseWriter, _ *http.Request) {
	all := s.Auth.AllJWKSKeys()
	keys := make([]jwkKey, 0, len(all))
	for _, k := range all {
		keys = append(keys, jwkKey{
			Kty: "RSA",
			Use: "sig",
			Kid: k.KeyID,
			Alg: "RS256",
			N:   k.Components.N,
			E:   k.Components.E,
		})
	}
	writeJSON(w, http.StatusOK, jwksResponse{Keys: keys})
}
