// Package authservice is a hand-rolled, 1:1 port of the Rust
// crates/fc-platform/src/auth/auth_service.rs. It owns JWT token
// generation and validation for the OAuth/OIDC surface (the former
// fosite-backed JWT strategy was removed — see ADR-0001).
//
// It supports RS256 (RSA) for production and HS256 (HMAC) for
// development, plus JWT key rotation: it signs with the current key and
// validates against the current key and any previous keys. The JWKS
// endpoint exposes all active public keys so clients can verify tokens
// signed by either key during rotation.
//
// The claim shapes (AccessTokenClaims, IDTokenClaims) are transcribed
// field-for-field from the Rust structs so the wire contract — what
// introspection echoes and what consumers decode — matches exactly.
package authservice

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Sentinel errors mirroring the relevant PlatformError variants the Rust
// auth_service returns. Callers translate these to HTTP status codes
// (TokenExpired/InvalidToken → 401).
var (
	// ErrTokenExpired indicates the JWT's exp has passed.
	ErrTokenExpired = errors.New("token expired")
	// ErrInvalidToken indicates a signature/issuer/audience failure.
	ErrInvalidToken = errors.New("invalid token")
)

// RsaPublicKeyComponents holds the base64url (no-pad) modulus and
// exponent of an RSA public key, ready for a JWKS entry.
type RsaPublicKeyComponents struct {
	// N is the modulus, base64url-encoded (no padding).
	N string
	// E is the exponent, base64url-encoded (no padding).
	E string
}

// AccessTokenClaims is the JWT payload for access tokens. The embedded
// RegisteredClaims supplies iss/sub/exp/iat/nbf/jti; the custom Aud
// field shadows RegisteredClaims.Audience (same "aud" json tag at a
// shallower depth) so audiences serialize as a bare string — matching
// Rust's `aud: String` rather than golang-jwt's default array form.
type AccessTokenClaims struct {
	jwt.RegisteredClaims

	// Aud is the single-valued audience (bare string on the wire).
	Aud string `json:"aud"`

	// PrincipalType is "USER" or "SERVICE" (Rust serde rename: "type").
	PrincipalType string `json:"type"`

	// Tier is the tenancy tier: "ANCHOR" | "PARTNER" | "CLIENT". (Formerly
	// carried on the "scope" claim; renamed so "scope" can hold real OAuth
	// scopes — see the Scope field below. This diverges from the Rust wire
	// contract by design.)
	Tier string `json:"tier"`

	// Scope is the granted OAuth scope: a space-delimited list of permission
	// codes (the principal's ceiling, optionally narrowed to the requested
	// scope at mint time). Omitted when the token carries no scope claim, in
	// which case permissions are derived downstream from Roles.
	Scope string `json:"scope,omitempty"`

	// Email is the user email; omitted for SERVICE principals.
	Email *string `json:"email,omitempty"`

	// Name is the principal display name (always present).
	Name string `json:"name"`

	// Clients is the client access list: ["*"] for anchor, "id:identifier"
	// pairs otherwise. Always emitted (possibly empty array).
	Clients []string `json:"clients"`

	// Roles is the assigned role names. Always emitted.
	Roles []string `json:"roles"`

	// Applications is the application codes derived from role-name
	// prefixes. Always emitted.
	Applications []string `json:"applications"`
}

// IDTokenClaims is the JWT payload for OIDC ID tokens. As in Rust, it
// omits nbf/jti (the embedded RegisteredClaims fields stay unset and are
// dropped by omitempty) and carries the OIDC standard claims plus the
// FlowCatalyst custom claims.
type IDTokenClaims struct {
	jwt.RegisteredClaims

	// Aud is the relying-party client_id (bare string).
	Aud string `json:"aud"`

	AuthTime      *int64   `json:"auth_time,omitempty"`
	Nonce         *string  `json:"nonce,omitempty"`
	Name          *string  `json:"name,omitempty"`
	Email         *string  `json:"email,omitempty"`
	EmailVerified *bool    `json:"email_verified,omitempty"`
	UpdatedAt     *int64   `json:"updated_at,omitempty"`
	ACR           *string  `json:"acr,omitempty"`
	AMR           []string `json:"amr,omitempty"`
	AZP           *string  `json:"azp,omitempty"`

	// PrincipalType is "USER" or "SERVICE" (serde rename: "type").
	PrincipalType string `json:"type"`
	// Tier is the tenancy tier (ANCHOR|PARTNER|CLIENT). Renamed from the
	// former "scope" claim — see AccessTokenClaims.Tier.
	Tier string `json:"tier"`
	// ClientID is the home client id; omitted when absent.
	ClientID *string `json:"client_id,omitempty"`
	// Roles, Applications, Clients always emitted (possibly empty).
	Roles        []string `json:"roles"`
	Applications []string `json:"applications"`
	Clients      []string `json:"clients"`
}

// Config bundles the construction-time settings, mirroring Rust's
// AuthConfig. TTLs are in seconds.
type Config struct {
	// RSAPrivateKeyPEM / RSAPublicKeyPEM enable RS256 when both are set.
	RSAPrivateKeyPEM string
	// RSAPublicKeyPEM is the current verification key (PEM).
	RSAPublicKeyPEM string
	// RSAPublicKeyPreviousPEM enables validation-only rotation when set.
	RSAPublicKeyPreviousPEM string

	// SecretKey is the HS256 fallback secret (used when RSA keys absent).
	SecretKey string

	// Issuer is the JWT `iss` claim.
	Issuer string
	// Audience is the access-token `aud` claim and the value validated on
	// access-token verification.
	Audience string

	// AccessTokenExpirySecs is the access-token lifetime (default 3600).
	AccessTokenExpirySecs int64
	// SessionTokenExpirySecs is the session-cookie lifetime (default 86400).
	SessionTokenExpirySecs int64
	// RefreshTokenExpirySecs is the refresh-token lifetime (default 30d).
	RefreshTokenExpirySecs int64
}

// DefaultConfig returns the canonical defaults, matching Rust's
// AuthConfig::default.
func DefaultConfig() Config {
	return Config{
		Issuer:                 "flowcatalyst",
		Audience:               "flowcatalyst",
		AccessTokenExpirySecs:  3600,
		SessionTokenExpirySecs: 86400,
		RefreshTokenExpirySecs: 86400 * 30,
	}
}

func (c *Config) applyDefaults() {
	if c.Issuer == "" {
		c.Issuer = "flowcatalyst"
	}
	if c.Audience == "" {
		c.Audience = "flowcatalyst"
	}
	if c.AccessTokenExpirySecs == 0 {
		c.AccessTokenExpirySecs = 3600
	}
	if c.SessionTokenExpirySecs == 0 {
		c.SessionTokenExpirySecs = 86400
	}
	if c.RefreshTokenExpirySecs == 0 {
		c.RefreshTokenExpirySecs = 86400 * 30
	}
}

// keyEntry is a single verification key with its JWKS metadata.
type keyEntry struct {
	verifyKey     any // *rsa.PublicKey (RS256) or []byte (HS256)
	keyID         string
	rsaComponents *RsaPublicKeyComponents
}

// AuthService manages token generation + validation. Construct via New,
// NewWithRSA, or NewWithSecret.
type AuthService struct {
	config Config

	algorithm     string // "RS256" | "HS256"
	signingMethod jwt.SigningMethod
	signKey       any // *rsa.PrivateKey (RS256) or []byte (HS256)

	currentVerify any // *rsa.PublicKey (RS256) or []byte (HS256)
	keyID         string
	rsaComponents *RsaPublicKeyComponents

	previousKeys []keyEntry
}

// NewWithRSA builds an RS256 service from PEM key material.
func NewWithRSA(config Config, privateKeyPEM, publicKeyPEM string) (*AuthService, error) {
	config.applyDefaults()

	priv, err := parseRSAPrivateKey([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("invalid RSA private key: %w", err)
	}
	pub, err := parseRSAPublicKey([]byte(publicKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("invalid RSA public key: %w", err)
	}
	components, err := extractRSAComponents(publicKeyPEM)
	if err != nil {
		return nil, err
	}

	return &AuthService{
		config:        config,
		algorithm:     "RS256",
		signingMethod: jwt.SigningMethodRS256,
		signKey:       priv,
		currentVerify: pub,
		keyID:         generateKeyID(publicKeyPEM),
		rsaComponents: components,
	}, nil
}

// NewWithSecret builds an HS256 service from the config's SecretKey.
func NewWithSecret(config Config) *AuthService {
	config.applyDefaults()
	secret := []byte(config.SecretKey)
	return &AuthService{
		config:        config,
		algorithm:     "HS256",
		signingMethod: jwt.SigningMethodHS256,
		signKey:       secret,
		currentVerify: secret,
	}
}

// New builds a service using RSA when an RSA private key is present
// (deriving the public key if not supplied), falling back to HS256
// otherwise. Loads the previous RSA key for rotation when configured.
func New(config Config) (*AuthService, error) {
	// Derive the public PEM from the private key when only the private is
	// configured (the common case in this codebase, which loads a single
	// signing key). The kid is then this service's own — clients read it
	// from the JWKS, so it need not match any external value.
	// RSA configured: it MUST succeed, or we fail closed. Silently downgrading
	// to HS256 on a bad key — and, when no SecretKey is set, to an EMPTY-secret
	// HS256 — would turn a key misconfiguration into trivially forgeable
	// tokens. Refuse to start instead. (This is a deliberate divergence from
	// the Rust port's warn-and-fall-back behaviour.)
	if config.RSAPrivateKeyPEM != "" {
		if config.RSAPublicKeyPEM == "" {
			pub, err := publicPEMFromPrivatePEM(config.RSAPrivateKeyPEM)
			if err != nil {
				return nil, fmt.Errorf("derive RSA public key from private key: %w", err)
			}
			config.RSAPublicKeyPEM = pub
		}
		svc, err := NewWithRSA(config, config.RSAPrivateKeyPEM, config.RSAPublicKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("initialise RS256 signer: %w", err)
		}
		if config.RSAPublicKeyPreviousPEM != "" {
			if err := svc.AddPreviousRSAKey(config.RSAPublicKeyPreviousPEM); err != nil {
				return nil, fmt.Errorf("load previous RSA key: %w", err)
			}
		}
		return svc, nil
	}
	// No RSA configured → HS256 (development only). Refuse an empty secret,
	// which would make every token forgeable by anyone.
	if config.SecretKey == "" {
		return nil, errors.New("no signing key configured: set an RSA private key (production) or a non-empty HS256 SecretKey (development)")
	}
	return NewWithSecret(config), nil
}

// AddPreviousRSAKey registers a validation-only previous RSA public key
// for rotation. The key is also exposed in JWKS.
func (s *AuthService) AddPreviousRSAKey(publicKeyPEM string) error {
	pub, err := parseRSAPublicKey([]byte(publicKeyPEM))
	if err != nil {
		return fmt.Errorf("invalid previous RSA public key: %w", err)
	}
	components, err := extractRSAComponents(publicKeyPEM)
	if err != nil {
		return err
	}
	s.previousKeys = append(s.previousKeys, keyEntry{
		verifyKey:     pub,
		keyID:         generateKeyID(publicKeyPEM),
		rsaComponents: components,
	})
	return nil
}

// KeyID returns the current key id, or "" for HS256.
func (s *AuthService) KeyID() string { return s.keyID }

// Algorithm returns "RS256" or "HS256".
func (s *AuthService) Algorithm() string { return s.algorithm }

// RSAComponents returns the current key's JWKS components, or nil for HS256.
func (s *AuthService) RSAComponents() *RsaPublicKeyComponents { return s.rsaComponents }

// JWKSKey pairs a key id with its RSA components for the JWKS endpoint.
type JWKSKey struct {
	KeyID      string
	Components RsaPublicKeyComponents
}

// AllJWKSKeys returns the current key plus any previous keys (rotation).
// Empty for HS256.
func (s *AuthService) AllJWKSKeys() []JWKSKey {
	var keys []JWKSKey
	if s.keyID != "" && s.rsaComponents != nil {
		keys = append(keys, JWKSKey{KeyID: s.keyID, Components: *s.rsaComponents})
	}
	for _, prev := range s.previousKeys {
		if prev.rsaComponents != nil {
			keys = append(keys, JWKSKey{KeyID: prev.keyID, Components: *prev.rsaComponents})
		}
	}
	return keys
}

// GenerateAccessToken mints a short-lived access token for API calls.
func (s *AuthService) GenerateAccessToken(p *principal.Principal) (string, error) {
	return s.generateTokenWithExpiry(p, s.config.AccessTokenExpirySecs, nil)
}

// GenerateAccessTokenWithScope mints a short-lived access token whose "scope"
// claim carries the supplied granted permissions (space-delimited). Callers
// compute the granted set as the principal's permission ceiling intersected
// with any requested scope; passing nil/empty is equivalent to
// GenerateAccessToken (no scope claim).
func (s *AuthService) GenerateAccessTokenWithScope(p *principal.Principal, scope []string) (string, error) {
	return s.generateTokenWithExpiry(p, s.config.AccessTokenExpirySecs, scope)
}

// GenerateSessionToken mints a longer-lived token for cookie sessions.
func (s *AuthService) GenerateSessionToken(p *principal.Principal) (string, error) {
	return s.generateTokenWithExpiry(p, s.config.SessionTokenExpirySecs, nil)
}

func (s *AuthService) generateTokenWithExpiry(p *principal.Principal, expirySecs int64, scope []string) (string, error) {
	now := time.Now().UTC()
	exp := now.Add(time.Duration(expirySecs) * time.Second)

	claims := AccessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   p.ID,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        tsid.GenerateUntyped(),
		},
		Aud:           s.config.Audience,
		PrincipalType: string(p.Type),
		Tier:          string(p.Scope),
		Scope:         strings.Join(scope, " "),
		Email:         principalEmail(p),
		Name:          p.Name,
		Clients:       buildClients(p),
		Roles:         roleNames(p),
		Applications:  buildApplications(roleNames(p)),
	}
	return s.sign(claims)
}

// GenerateIDToken mints an OIDC ID token addressed to clientID, echoing
// the supplied nonce when present.
func (s *AuthService) GenerateIDToken(p *principal.Principal, clientID string, nonce *string) (string, error) {
	now := time.Now().UTC()
	exp := now.Add(time.Duration(s.config.AccessTokenExpirySecs) * time.Second)
	nowUnix := now.Unix()

	email := principalEmail(p)
	var emailVerified *bool
	if email != nil {
		v := true
		emailVerified = &v
	}
	name := p.Name
	azp := clientID

	claims := IDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   p.ID,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Aud:           clientID,
		AuthTime:      &nowUnix,
		Nonce:         nonce,
		Name:          &name,
		Email:         email,
		EmailVerified: emailVerified,
		UpdatedAt:     &nowUnix,
		AZP:           &azp,
		PrincipalType: string(p.Type),
		Tier:          string(p.Scope),
		ClientID:      p.ClientID,
		Roles:         roleNames(p),
		Applications:  buildApplications(roleNames(p)),
		Clients:       buildClients(p),
	}
	return s.sign(claims)
}

// sign serializes and signs the supplied claims with the current key,
// stamping the kid header when using RS256.
func (s *AuthService) sign(claims jwt.Claims) (string, error) {
	tok := jwt.NewWithClaims(s.signingMethod, claims)
	if s.keyID != "" {
		tok.Header["kid"] = s.keyID
	}
	signed, err := tok.SignedString(s.signKey)
	if err != nil {
		return "", fmt.Errorf("encode JWT: %w", err)
	}
	return signed, nil
}

// ValidateToken verifies an access token's signature, issuer, audience,
// and expiry, trying the current key first then previous keys (rotation).
func (s *AuthService) ValidateToken(token string) (*AccessTokenClaims, error) {
	verifyKeys := make([]any, 0, 1+len(s.previousKeys))
	verifyKeys = append(verifyKeys, s.currentVerify)
	for _, k := range s.previousKeys {
		verifyKeys = append(verifyKeys, k.verifyKey)
	}

	var lastErr error
	for _, vk := range verifyKeys {
		claims := &AccessTokenClaims{}
		_, err := jwt.ParseWithClaims(token, claims,
			func(*jwt.Token) (any, error) { return vk, nil },
			jwt.WithValidMethods([]string{s.algorithm}),
			jwt.WithIssuer(s.config.Issuer),
		)
		if err == nil {
			// aud lives in a custom field (RegisteredClaims.Audience is
			// shadowed), so audience must be checked here rather than via
			// jwt.WithAudience.
			if claims.Aud != s.config.Audience {
				return nil, fmt.Errorf("%w: audience mismatch", ErrInvalidToken)
			}
			return claims, nil
		}
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		lastErr = err
	}
	return nil, fmt.Errorf("%w: %v", ErrInvalidToken, lastErr)
}

// HasClientAccess reports whether the claims grant access to clientID,
// handling both plain ids and "id:identifier" pairs and the "*" wildcard.
func (s *AuthService) HasClientAccess(claims *AccessTokenClaims, clientID string) bool {
	for _, c := range claims.Clients {
		if c == "*" || c == clientID || strings.HasPrefix(c, clientID+":") {
			return true
		}
	}
	return false
}

// HasRole reports whether the claims carry the named role.
func (s *AuthService) HasRole(claims *AccessTokenClaims, role string) bool {
	for _, r := range claims.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsAnchor reports whether the claims are for an anchor-tier principal.
func (s *AuthService) IsAnchor(claims *AccessTokenClaims) bool { return claims.Tier == "ANCHOR" }

// ─── principal → claim helpers (1:1 with Rust) ──────────────────────────

func principalEmail(p *principal.Principal) *string {
	if p.UserIdentity != nil && p.UserIdentity.Email != "" {
		e := p.UserIdentity.Email
		return &e
	}
	return nil
}

func roleNames(p *principal.Principal) []string {
	out := make([]string, 0, len(p.Roles))
	for _, ra := range p.Roles {
		out = append(out, ra.Role)
	}
	return out
}

// buildClients mirrors the Rust client-access list construction:
// anchor → ["*"]; partner → assigned clients mapped to "id:identifier";
// client → the home client id mapped likewise.
func buildClients(p *principal.Principal) []string {
	switch p.Scope {
	case principal.ScopeAnchor:
		return []string{"*"}
	case principal.ScopePartner:
		out := make([]string, 0, len(p.AssignedClients))
		for _, id := range p.AssignedClients {
			out = append(out, clientPair(p, id))
		}
		return out
	default: // CLIENT
		out := make([]string, 0, 1)
		if p.ClientID != nil {
			out = append(out, clientPair(p, *p.ClientID))
		}
		return out
	}
}

func clientPair(p *principal.Principal, id string) string {
	if ident, ok := p.ClientIdentifierMap[id]; ok {
		return id + ":" + ident
	}
	return id
}

// buildApplications extracts app codes from role names that contain a
// ':' separator (e.g. "operant:admin" → "operant"), de-duplicated.
// First-seen order is preserved (Rust uses an unordered HashSet, so any
// order is wire-compatible).
func buildApplications(roles []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, role := range roles {
		idx := strings.IndexByte(role, ':')
		if idx <= 0 {
			continue
		}
		app := role[:idx]
		if _, ok := seen[app]; ok {
			continue
		}
		seen[app] = struct{}{}
		out = append(out, app)
	}
	return out
}

// ExtractBearerToken returns the token after a "Bearer " prefix, or ""
// when the header isn't a bearer credential.
func ExtractBearerToken(authHeader string) string {
	const prefix = "Bearer "
	if strings.HasPrefix(authHeader, prefix) {
		return authHeader[len(prefix):]
	}
	return ""
}

// ─── key parsing / JWKS helpers ─────────────────────────────────────────

func generateKeyID(publicKeyPEM string) string {
	h := sha256.Sum256([]byte(publicKeyPEM))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

func extractRSAComponents(publicKeyPEM string) (*RsaPublicKeyComponents, error) {
	pub, err := parseRSAPublicKey([]byte(publicKeyPEM))
	if err != nil {
		return nil, err
	}
	return &RsaPublicKeyComponents{
		N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}, nil
}

func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	any8, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs8: %w", err)
	}
	rsaKey, ok := any8.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return rsaKey, nil
}

// publicPEMFromPrivatePEM derives the PKIX public-key PEM from an RSA
// private-key PEM.
func publicPEMFromPrivatePEM(privPEM string) (string, error) {
	priv, err := parseRSAPrivateKey([]byte(privPEM))
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

func parseRSAPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
		return nil, errors.New("public key is not RSA")
	}
	if rsaPub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return rsaPub, nil
	}
	return nil, errors.New("unparseable RSA public key")
}
