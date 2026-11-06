package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// Field-worker tokens. A field worker signs in from a device with a PIN or a
// badge (the device may or may not be enrolled); the platform issues a
// short-lived access token bound to the device key (RFC 9449 DPoP style). A
// token on a key that is not an enrolled device has no device_id and
// device_mode "UNENROLLED". When someone else is signed in as the worker (a
// supervisor, or an admin through a console sign-in code) the token carries an
// act claim: the principal's Actor field. Test identities carry test. The application backend verifies, per request, the
// token AND a proof signed by the same device key:
//
//	v := auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{
//	    Issuer:  "https://auth.flowcatalyst.io",
//	    AppCode: "my-app",
//	})
//	p, err := v.Verify(ctx, r.Header.Get("Authorization"), r.Header.Get("DPoP"), r.Method, fullURL)
//
// or, with net/http, auth.FieldMiddleware(v) (which requires BaseURL in the
// config and compares htu with BaseURL + path, never the Host header). Nonce checking is NOT done here:
// an app that needs replay protection beyond the proof's iat window uses the
// framework's stateless nonce (docs/field-worker-identity-design.md §6.1). A
// PIN-change-only (pcr) token is refused: it is valid on the platform's
// /field/auth/pin only.

// Reasons a field credential is refused. They are the stable strings the
// shared fixtures (api/field-fixtures) expect.
const (
	FieldReasonMalformed     = "malformed"
	FieldReasonWrongTyp      = "wrong_typ"
	FieldReasonWrongTokenUse = "wrong_token_use"
	FieldReasonWrongAud      = "wrong_aud"
	FieldReasonExpired       = "expired"
	FieldReasonInvalidToken  = "invalid_token"
	FieldReasonPCR           = "pcr"
	FieldReasonMissingProof  = "missing_proof"
	FieldReasonInvalidProof  = "invalid_proof"
	FieldReasonJKTMismatch   = "jkt_mismatch"
	FieldReasonHTMMismatch   = "htm_mismatch"
	FieldReasonHTUMismatch   = "htu_mismatch"
	FieldReasonATHMismatch   = "ath_mismatch"
	FieldReasonStaleProof    = "stale_proof"
)

// FieldError is a refused field credential; Reason is one of the FieldReason*
// constants.
type FieldError struct {
	Reason string
	Detail string
}

func (e *FieldError) Error() string {
	if e.Detail == "" {
		return "field credential refused: " + e.Reason
	}
	return "field credential refused: " + e.Reason + ": " + e.Detail
}

// Is lets errors.Is(err, ErrInvalidToken) match any refused field credential.
func (e *FieldError) Is(target error) bool {
	var t *Error
	return errors.As(target, &t) && t.Kind == KindInvalidToken
}

func fieldErr(reason, detail string) error { return &FieldError{Reason: reason, Detail: detail} }

// FieldTokenVerifierConfig configures a FieldTokenVerifier. Issuer and AppCode
// are required.
type FieldTokenVerifierConfig struct {
	// Issuer is the FlowCatalyst base URL and the tokens' iss claim.
	Issuer string
	// AppCode is this application's code; tokens must be addressed to it (aud).
	AppCode string
	// JWKSURL skips OIDC discovery and fetches keys from this URL.
	JWKSURL string
	// HTTPClient is the transport for discovery and JWKS (default 15 s timeout).
	HTTPClient *http.Client
	// JWKSRefreshInterval defaults to 1 hour.
	JWKSRefreshInterval time.Duration
	// ClockSkew bounds exp/nbf checks (default 60 s).
	ClockSkew time.Duration
	// ProofMaxAge is how far a proof's iat may be from now, either way
	// (default 5 minutes). Widen it for devices with bad clocks.
	ProofMaxAge time.Duration
	// BaseURL is this service's public base URL. [FieldMiddleware] REQUIRES it
	// (it panics at construction without one): the proof's htu is compared
	// against BaseURL + the request path, never against the Host header or
	// X-Forwarded-* headers, which a client can set. Direct [FieldTokenVerifier.Verify]
	// callers pass the full URL themselves and may leave it empty.
	BaseURL string
	// Now overrides the clock (tests, fixtures).
	Now func() time.Time
}

// FieldPrincipal is a verified field worker on a verified device.
type FieldPrincipal struct {
	WorkerID         string
	ClientID         string
	ClientIdentifier string
	WorkerCode       string
	Name             string
	AppCode          string
	// DeviceID is empty on a key that is not an enrolled device (DeviceMode is
	// then "UNENROLLED").
	DeviceID   string
	DeviceMode string
	// SessionID is the login family (sid).
	SessionID string
	AMR       []string
	AuthTime  time.Time
	ExpiresAt time.Time
	// JKT is the thumbprint of the device key the request was proven with.
	JKT     string
	TokenID string
	// Actor is non-nil when someone other than the worker is signed in as them
	// (the act claim); the other fields are still the WORKER's.
	Actor *FieldActor
	// Test marks a test identity.
	Test bool
}

// FieldActor is who is acting as the worker (RFC 8693 act claim). Type is
// "field_worker" (a supervisor) or "principal" (a platform user).
type FieldActor struct {
	ID   string
	Type string
	Name string
}

// FieldTokenVerifier verifies field access tokens and their DPoP proofs.
type FieldTokenVerifier struct {
	cfg     FieldTokenVerifierConfig
	cache   *JwksCache
	jwksURL string
	once    sync.Once
	initErr error
}

// NewFieldTokenVerifier builds a verifier.
func NewFieldTokenVerifier(cfg FieldTokenVerifierConfig) *FieldTokenVerifier {
	if cfg.JWKSRefreshInterval == 0 {
		cfg.JWKSRefreshInterval = time.Hour
	}
	if cfg.ClockSkew == 0 {
		cfg.ClockSkew = 60 * time.Second
	}
	if cfg.ProofMaxAge == 0 {
		cfg.ProofMaxAge = 5 * time.Minute
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = defaultHTTPClient()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Issuer = strings.TrimRight(cfg.Issuer, "/")
	return &FieldTokenVerifier{cfg: cfg, cache: NewJwksCache(cfg.HTTPClient, cfg.JWKSRefreshInterval)}
}

func (v *FieldTokenVerifier) keys(ctx context.Context) (jwk.Set, error) {
	v.once.Do(func() {
		u := v.cfg.JWKSURL
		if u == "" {
			var err error
			if u, err = discoverJWKSURL(ctx, v.cfg.HTTPClient, v.cfg.Issuer); err != nil {
				v.initErr = err
				return
			}
		}
		v.jwksURL = u
		if err := v.cache.Register(u); err != nil {
			v.initErr = newErr(KindDiscovery, "register JWKS URL: "+err.Error())
		}
	})
	if v.initErr != nil {
		return nil, v.initErr
	}
	set, err := v.cache.Get(ctx, v.jwksURL)
	if err != nil {
		return nil, newErr(KindDiscovery, "fetch JWKS: "+err.Error())
	}
	return set, nil
}

type fieldClaims struct {
	Iss              string   `json:"iss"`
	Sub              string   `json:"sub"`
	Aud              Audience `json:"aud"`
	Exp              int64    `json:"exp"`
	Jti              string   `json:"jti"`
	TokenUse         string   `json:"token_use"`
	ClientID         string   `json:"client_id"`
	ClientIdentifier string   `json:"client_identifier"`
	WorkerCode       string   `json:"worker_code"`
	Name             string   `json:"name"`
	DeviceID         string   `json:"device_id"`
	DeviceMode       string   `json:"device_mode"`
	SID              string   `json:"sid"`
	AMR              []string `json:"amr"`
	AuthTime         int64    `json:"auth_time"`
	Cnf              struct {
		JKT string `json:"jkt"`
	} `json:"cnf"`
	PCR bool `json:"pcr"`
	Act *struct {
		Sub  string `json:"sub"`
		Typ  string `json:"typ"`
		Name string `json:"name"`
	} `json:"act"`
	Test bool `json:"test"`
}

// Verify checks a request's credentials: the Authorization header ("DPoP
// <token>"), the DPoP proof header, and the request's method and full URL
// (scheme, host and path; any query is ignored). It returns the verified
// principal or a [*FieldError] (also an [ErrInvalidToken]) naming the reason;
// discovery and key-fetch failures are [ErrDiscovery].
func (v *FieldTokenVerifier) Verify(ctx context.Context, authorizationHeader, dpopHeader, method, requestURL string) (*FieldPrincipal, error) {
	scheme, token, _ := strings.Cut(strings.TrimSpace(authorizationHeader), " ")
	token = strings.TrimSpace(token)
	if !strings.EqualFold(scheme, "DPoP") || token == "" {
		return nil, fieldErr(FieldReasonMalformed, "Authorization must be \"DPoP <token>\"")
	}
	msg, err := jws.Parse([]byte(token))
	if err != nil || len(msg.Signatures()) != 1 {
		return nil, fieldErr(FieldReasonMalformed, "not a compact JWS")
	}
	if typ := msg.Signatures()[0].ProtectedHeaders().Type(); typ != "fc-field+jwt" {
		return nil, fieldErr(FieldReasonWrongTyp, "typ is "+typ)
	}
	// The platform signs field access tokens RS256 only; refuse any other alg
	// (HS256, none, ES256, ...) before touching the keys.
	if alg := msg.Signatures()[0].ProtectedHeaders().Algorithm(); alg != jwa.RS256 {
		return nil, fieldErr(FieldReasonInvalidToken, "alg must be RS256, got "+alg.String())
	}
	set, err := v.keys(ctx)
	if err != nil {
		return nil, err
	}
	now := v.cfg.Now()
	if _, err := jwt.ParseString(token,
		jwt.WithKeySet(set, jws.WithRequireKid(false)),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAcceptableSkew(v.cfg.ClockSkew),
		jwt.WithClock(jwt.ClockFunc(func() time.Time { return now })),
	); err != nil {
		if errors.Is(err, jwt.ErrTokenExpired()) {
			return nil, fieldErr(FieldReasonExpired, "")
		}
		return nil, fieldErr(FieldReasonInvalidToken, err.Error())
	}
	var c fieldClaims
	if err := json.Unmarshal(msg.Payload(), &c); err != nil {
		return nil, fieldErr(FieldReasonMalformed, "claims are not JSON")
	}
	if c.Exp == 0 {
		return nil, fieldErr(FieldReasonInvalidToken, "the token has no exp")
	}
	if c.TokenUse != "field" {
		return nil, fieldErr(FieldReasonWrongTokenUse, "token_use is "+c.TokenUse)
	}
	if string(c.Aud) != v.cfg.AppCode {
		return nil, fieldErr(FieldReasonWrongAud, "aud is "+string(c.Aud))
	}
	if c.PCR {
		return nil, fieldErr(FieldReasonPCR, "the token may only change the PIN")
	}
	if c.Cnf.JKT == "" {
		return nil, fieldErr(FieldReasonJKTMismatch, "the token carries no cnf.jkt")
	}
	if strings.TrimSpace(dpopHeader) == "" {
		return nil, fieldErr(FieldReasonMissingProof, "")
	}
	proof, err := parseFieldProof(dpopHeader)
	if err != nil {
		return nil, err
	}
	if proof.jkt != c.Cnf.JKT {
		return nil, fieldErr(FieldReasonJKTMismatch, "")
	}
	if proof.htm != method {
		return nil, fieldErr(FieldReasonHTMMismatch, "")
	}
	if !sameURL(proof.htu, requestURL) {
		return nil, fieldErr(FieldReasonHTUMismatch, "")
	}
	sum := sha256.Sum256([]byte(token))
	if proof.ath != base64.RawURLEncoding.EncodeToString(sum[:]) {
		return nil, fieldErr(FieldReasonATHMismatch, "")
	}
	if d := now.Sub(time.Unix(proof.iat, 0)); d > v.cfg.ProofMaxAge || d < -v.cfg.ProofMaxAge {
		return nil, fieldErr(FieldReasonStaleProof, "")
	}
	p := &FieldPrincipal{
		WorkerID: c.Sub, ClientID: c.ClientID, ClientIdentifier: c.ClientIdentifier, WorkerCode: c.WorkerCode,
		Name: c.Name, AppCode: string(c.Aud), DeviceID: c.DeviceID, DeviceMode: c.DeviceMode, SessionID: c.SID,
		AMR: c.AMR, AuthTime: time.Unix(c.AuthTime, 0).UTC(), ExpiresAt: time.Unix(c.Exp, 0).UTC(),
		JKT: proof.jkt, TokenID: c.Jti, Test: c.Test,
	}
	if c.Act != nil && c.Act.Sub != "" {
		p.Actor = &FieldActor{ID: c.Act.Sub, Type: c.Act.Typ, Name: c.Act.Name}
	}
	return p, nil
}

// sameURL compares scheme, host (case-insensitive) and path; query and
// fragment are ignored on both sides.
func sameURL(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host) && ua.Path == ub.Path
}

type fieldProof struct {
	jkt, htm, htu, ath string
	iat                int64
}

func parseFieldProof(compact string) (*fieldProof, error) {
	bad := func(detail string) error { return fieldErr(FieldReasonInvalidProof, detail) }
	parts := strings.Split(strings.TrimSpace(compact), ".")
	if len(parts) != 3 {
		return nil, bad("not a compact JWS")
	}
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, bad("header is not base64url")
	}
	var hdr struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
		JWK struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
			D   string `json:"d"`
		} `json:"jwk"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return nil, bad("header is not JSON")
	}
	if hdr.Typ != "dpop+jwt" || hdr.Alg != "ES256" {
		return nil, bad("typ must be dpop+jwt and alg ES256")
	}
	if hdr.JWK.Kty != "EC" || hdr.JWK.Crv != "P-256" || hdr.JWK.D != "" {
		return nil, bad("jwk must be a public EC P-256 key")
	}
	x, errX := base64.RawURLEncoding.DecodeString(hdr.JWK.X)
	y, errY := base64.RawURLEncoding.DecodeString(hdr.JWK.Y)
	if errX != nil || errY != nil || len(x) != 32 || len(y) != 32 {
		return nil, bad("jwk coordinates are malformed")
	}
	point := append(append([]byte{4}, x...), y...)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
	if err != nil {
		return nil, bad("jwk point is not on the curve")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, bad("signature is malformed")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return nil, bad("signature does not verify")
	}
	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, bad("claims are not base64url")
	}
	var c struct {
		HTM string `json:"htm"`
		HTU string `json:"htu"`
		IAT *int64 `json:"iat"`
		JTI string `json:"jti"`
		ATH string `json:"ath"`
	}
	if err := json.Unmarshal(claimBytes, &c); err != nil || c.IAT == nil || c.JTI == "" {
		return nil, bad("claims need iat and jti")
	}
	canonical := fmt.Sprintf(`{"crv":"P-256","kty":"EC","x":"%s","y":"%s"}`,
		base64.RawURLEncoding.EncodeToString(x), base64.RawURLEncoding.EncodeToString(y))
	tp := sha256.Sum256([]byte(canonical))
	return &fieldProof{
		jkt: base64.RawURLEncoding.EncodeToString(tp[:]), htm: c.HTM, htu: c.HTU, ath: c.ATH, iat: *c.IAT,
	}, nil
}

// ── net/http middleware ──────────────────────────────────────────────────

type fieldCtxKey struct{}

// FieldPrincipalFrom returns the principal [FieldMiddleware] stored.
func FieldPrincipalFrom(ctx context.Context) (*FieldPrincipal, bool) {
	p, ok := ctx.Value(fieldCtxKey{}).(*FieldPrincipal)
	return p, ok
}

// FieldMiddleware verifies every request with v and puts the [FieldPrincipal]
// in the request context ([FieldPrincipalFrom]). A refused request is a 401
// {"error": <reason>} with WWW-Authenticate: DPoP; a key-fetch failure is 503.
// The request URL is v's BaseURL plus the request path; the Host header and
// X-Forwarded-* headers are never used. FieldMiddleware panics when v has no
// BaseURL (a wiring error, caught at startup rather than as 401s at runtime).
func FieldMiddleware(v *FieldTokenVerifier) func(http.Handler) http.Handler {
	if strings.TrimRight(v.cfg.BaseURL, "/") == "" {
		panic("auth.FieldMiddleware: FieldTokenVerifierConfig.BaseURL is required (the public base URL the proof's htu is compared against)")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := v.Verify(r.Context(), r.Header.Get("Authorization"), r.Header.Get("DPoP"), r.Method, v.requestURL(r))
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				if fe, ok := errors.AsType[*FieldError](err); ok {
					w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": fe.Reason})
					return
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unavailable"})
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), fieldCtxKey{}, p)))
		})
	}
}

func (v *FieldTokenVerifier) requestURL(r *http.Request) string {
	return strings.TrimRight(v.cfg.BaseURL, "/") + r.URL.Path
}
