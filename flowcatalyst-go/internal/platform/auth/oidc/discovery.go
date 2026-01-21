package oidc

import (
	"encoding/json"
	"net/http"

	"go.flowcatalyst.tech/internal/platform/auth/jwt"
)

// DiscoveryDocument represents the OpenID Connect discovery document
type DiscoveryDocument struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint,omitempty"`
	JwksURI                           string   `json:"jwks_uri"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	ResponseModesSupported            []string `json:"response_modes_supported,omitempty"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
}

// DiscoveryHandler handles OIDC discovery endpoints
type DiscoveryHandler struct {
	keyManager  *jwt.KeyManager
	issuer      string
	externalURL string
}

// NewDiscoveryHandler creates a new discovery handler
func NewDiscoveryHandler(keyManager *jwt.KeyManager, issuer, externalURL string) *DiscoveryHandler {
	return &DiscoveryHandler{
		keyManager:  keyManager,
		issuer:      issuer,
		externalURL: externalURL,
	}
}

// HandleDiscovery handles GET /.well-known/openid-configuration
func (h *DiscoveryHandler) HandleDiscovery(w http.ResponseWriter, r *http.Request) {
	baseURL := h.externalURL
	if baseURL == "" {
		// Fall back to request host
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		baseURL = scheme + "://" + r.Host
	}

	doc := DiscoveryDocument{
		Issuer:                h.issuer,
		AuthorizationEndpoint: baseURL + "/oauth/authorize",
		TokenEndpoint:         baseURL + "/oauth/token",
		JwksURI:               baseURL + "/.well-known/jwks.json",
		ScopesSupported: []string{
			"openid",
			"profile",
			"email",
		},
		ResponseTypesSupported: []string{
			"code",
			"token",
			"id_token",
			"code id_token",
		},
		ResponseModesSupported: []string{
			"query",
			"fragment",
		},
		GrantTypesSupported: []string{
			"authorization_code",
			"refresh_token",
			"client_credentials",
			"password",
		},
		SubjectTypesSupported: []string{
			"public",
		},
		IDTokenSigningAlgValuesSupported: []string{
			"RS256",
		},
		TokenEndpointAuthMethodsSupported: []string{
			"client_secret_basic",
			"client_secret_post",
		},
		CodeChallengeMethodsSupported: []string{
			"S256",
			"plain",
		},
		ClaimsSupported: []string{
			"sub",
			"iss",
			"aud",
			"exp",
			"iat",
			"nonce",
			"email",
			"name",
			"clients",
			"groups",
			"applications",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(doc)
}

// HandleJWKS handles GET /.well-known/jwks.json
func (h *DiscoveryHandler) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	jwks := h.keyManager.GetJWKS()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(jwks)
}
