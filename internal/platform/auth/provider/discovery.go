package provider

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// DiscoveryEndpoint serves OIDC discovery + JWKS endpoints. It exposes
// the metadata SDK consumers (and IDP bridges) need to discover the
// /oauth/token, /oauth/authorize, jwks_uri, etc. endpoints, plus the
// public key set used to verify our JWTs.
type DiscoveryEndpoint struct {
	cfg     Config
	pubKey  *rsa.PublicKey
	baseURL string
}

// NewDiscoveryEndpoint wires the handler. baseURL is the issuer URL
// (e.g. https://flowcatalyst.example.com) — discovery endpoints
// advertise their own URLs derived from this.
func NewDiscoveryEndpoint(cfg Config, baseURL string) (*DiscoveryEndpoint, error) {
	pub, err := parseRSAPublicHalf(cfg.SigningKey)
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		baseURL = cfg.Issuer
	}
	return &DiscoveryEndpoint{cfg: cfg, pubKey: pub, baseURL: baseURL}, nil
}

// RegisterRoutes mounts the discovery + JWKS endpoints.
func (e *DiscoveryEndpoint) RegisterRoutes(r chi.Router) {
	r.Get("/.well-known/openid-configuration", e.handleDiscovery)
	r.Get("/.well-known/jwks.json", e.handleJWKS)
}

func (e *DiscoveryEndpoint) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]any{
		"issuer":                 e.cfg.Issuer,
		"token_endpoint":         e.baseURL + "/oauth/token",
		"authorization_endpoint": e.baseURL + "/oauth/authorize",
		"revocation_endpoint":    e.baseURL + "/oauth/revoke",
		"introspection_endpoint": e.baseURL + "/oauth/introspect",
		"jwks_uri":               e.baseURL + "/.well-known/jwks.json",
		"userinfo_endpoint":      e.baseURL + "/oauth/userinfo",
		"grant_types_supported": []string{
			"client_credentials",
			"refresh_token",
			// authorization_code lands once the authorize endpoint is wired.
		},
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{"openid", "profile", "email"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func (e *DiscoveryEndpoint) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	jwk := map[string]any{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"n":   base64URL(e.pubKey.N.Bytes()),
		"e":   base64URL(big.NewInt(int64(e.pubKey.E)).Bytes()),
	}
	if e.cfg.SigningKeyID != "" {
		jwk["kid"] = e.cfg.SigningKeyID
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jwk}})
}

// parseRSAPublicHalf re-uses the PEM private key to derive the public
// half. We don't store a separate public-key file: the JWKS endpoint
// emits the same key fosite signs with.
func parseRSAPublicHalf(pemBytes []byte) (*rsa.PublicKey, error) {
	if len(pemBytes) == 0 {
		return nil, errors.New("signing key is empty")
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return &k.PublicKey, nil
	}
	any8, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := any8.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return &rsaKey.PublicKey, nil
}

// base64URL emits no-pad URL-safe base64 — the JWK encoding (RFC 7515).
func base64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
