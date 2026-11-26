//go:build integration

package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/tsid"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// These tests drive the field-worker protocol (/field/*) through the whole platform router
// (WirePlatform on embedded Postgres) with the requests a browser sends from an application's
// page on another origin: the protocol answers CORS from the platform's allowed-origins table
// (tnt_cors_allowed_origins), the same table and rule as the push gateway, with its own methods
// and headers (fieldauth.CORSRule), and never with credentials. A request without an Origin
// header (a phone) is answered as before.

const fieldPIN = "482915"

var fieldSeq atomic.Int64

// fcHarness is the wired platform with one client, application and worker.
type fcHarness struct {
	t        *testing.T
	h        http.Handler
	ident    string // client identifier
	app      string // application code
	worker   string // worker code
	allowed  string // an origin in the table
	notAllow string // an origin not in it
}

// newFieldCORSHarness wires the platform. withFallback mounts a stand-in for the console SPA as
// fc-server and fcdev do when it is embedded (mountFallback), which answers a method mismatch.
func newFieldCORSHarness(t *testing.T, withFallback bool) *fcHarness {
	t.Helper()
	// the PIN pepper; without it every field route answers 503
	t.Setenv("FLOWCATALYST_APP_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32))))
	ctx := context.Background()
	pool := testpg.Pool(t)
	n := fieldSeq.Add(1)
	stamp := time.Now().UnixNano() % 1_000_000_000

	fc := &fcHarness{
		t:        t,
		allowed:  fmt.Sprintf("https://field-app-%d-%d.example.com", n, stamp),
		notAllow: fmt.Sprintf("https://other-%d-%d.example.net", n, stamp),
		app:      fmt.Sprintf("fcors%d%d", n, stamp%100000),
		worker:   fmt.Sprintf("W%d", n),
	}
	addOrigin(t, fc.allowed)

	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	cfg := EnvCfg{
		JWTIssuer:              xoPublicBase,
		JWTSigningKeyPath:      keyPath,
		SessionTTLSecs:         3600,
		AccessTokenTTLSecs:     3600,
		RefreshTokenTTLSecs:    3600,
		FunctionsArtifactStore: "file://" + t.TempDir(),
	}
	r := chi.NewRouter()
	_, err = WirePlatform(r, pool, cfg, dispatch.Settings{})
	require.NoError(t, err)
	if withFallback {
		mountFallback(r, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("spa"))
		}), nil)
	}
	fc.h = r

	// A client, an application and a worker with a PIN.
	uow := testpg.NewUoW(t)
	repos := buildRepos(pool)
	fc.ident = fmt.Sprintf("fcors-co-%d-%d", n, stamp)
	ev, err := usecaseop.Run(ctx, uow, clientops.CreateClient(repos.clientRepo),
		clientops.CreateCommand{Name: "Field CORS Co " + fc.ident, Identifier: fc.ident}, testpg.TestEC())
	require.NoError(t, err)
	testpg.EnsureApplication(t, fc.app)
	actx := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"},
		AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	})
	ops := newFieldOps(repos)
	require.True(t, ops.Hasher.Configured())
	w, err := ops.RunEnsureWorker(actx, uow, fieldidentity.EnsureWorkerCommand{
		ClientID: ev.ClientID, WorkerCode: fc.worker, Name: "Worker " + fc.worker, AppCodes: []string{fc.app}, PIN: fieldPIN,
	}, auth.NewExecutionContext(actx))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE field_identities SET pin_change_required = FALSE WHERE id = $1`, w.ID)
	require.NoError(t, err)
	return fc
}

func addOrigin(t *testing.T, origin string) {
	t.Helper()
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO tnt_cors_allowed_origins (id, origin) VALUES ($1, $2)`, tsid.Generate(tsid.CorsOrigin), origin)
	require.NoError(t, err)
}

func removeOrigin(t *testing.T, origin string) {
	t.Helper()
	_, err := testpg.Pool(t).Exec(context.Background(), `DELETE FROM tnt_cors_allowed_origins WHERE origin = $1`, origin)
	require.NoError(t, err)
}

// fcDevice is a browser page's non-extractable P-256 key, as field-js's WebCryptoKeyProvider holds.
type fcDevice struct {
	t     *testing.T
	key   *ecdsa.PrivateKey
	nonce string
}

func newFCDevice(t *testing.T) *fcDevice {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return &fcDevice{t: t, key: k}
}

func fcB64JSON(t *testing.T, v any) string {
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// proof is the DPoP proof (RFC 9449, ES256) for POST {platform}{path} with the device's nonce.
func (d *fcDevice) proof(path string) string {
	d.t.Helper()
	pt, err := d.key.PublicKey.Bytes()
	require.NoError(d.t, err)
	jwk := map[string]string{
		"kty": "EC", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(pt[1:33]), "y": base64.RawURLEncoding.EncodeToString(pt[33:65]),
	}
	claims := map[string]any{
		"htm": http.MethodPost, "htu": xoPublicBase + path, "iat": time.Now().Unix(),
		"jti": fmt.Sprintf("fc-%d", fieldSeq.Add(1)),
	}
	if d.nonce != "" {
		claims["nonce"] = d.nonce
	}
	input := fcB64JSON(d.t, map[string]any{"alg": "ES256", "typ": "dpop+jwt", "jwk": jwk}) + "." + fcB64JSON(d.t, claims)
	digest := sha256.Sum256([]byte(input))
	der, err := ecdsa.SignASN1(rand.Reader, d.key, digest[:])
	require.NoError(d.t, err)
	var rs struct{ R, S *big.Int }
	_, err = asn1.Unmarshal(der, &rs)
	require.NoError(d.t, err)
	sig := append(rs.R.FillBytes(make([]byte, 32)), rs.S.FillBytes(make([]byte, 32))...)
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// post sends what field-js sends: POST with a JSON body, Accept, DPoP, and, from a page, the
// Origin and Sec-Fetch-* a browser adds to a cross-origin fetch. The DPoP-Nonce of the answer is
// the device's next nonce.
func (fc *fcHarness) post(d *fcDevice, path, origin string, body any) *httptest.ResponseRecorder {
	fc.t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(fc.t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Host = "platform.example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("DPoP", d.proof(path))
	if origin != "" {
		req.Header.Set("Origin", origin)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Sec-Fetch-Mode", "cors")
	}
	rec := httptest.NewRecorder()
	fc.h.ServeHTTP(rec, req)
	if n := rec.Header().Get("DPoP-Nonce"); n != "" {
		d.nonce = n
	}
	return rec
}

func (fc *fcHarness) preflight(path, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, path, nil)
	req.Host = "platform.example.com"
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,dpop")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	rec := httptest.NewRecorder()
	fc.h.ServeHTTP(rec, req)
	return rec
}

func (fc *fcHarness) login(d *fcDevice, origin string) *httptest.ResponseRecorder {
	return fc.post(d, "/field/auth/login", origin, map[string]string{
		"appCode": fc.app, "clientIdentifier": fc.ident, "workerCode": fc.worker, "pin": fieldPIN,
	})
}

func exposed(rec *httptest.ResponseRecorder) []string {
	var out []string
	for v := range strings.SplitSeq(rec.Header().Get("Access-Control-Expose-Headers"), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func assertNoCORSHeaders(t *testing.T, rec *httptest.ResponseRecorder, msg string) {
	t.Helper()
	for k := range rec.Header() {
		assert.False(t, strings.HasPrefix(k, "Access-Control-"), "%s: no CORS header, got %s", msg, k)
	}
}

var fieldPaths = []string{
	"/field/auth/nonce", "/field/devices/enrol", "/field/auth/login", "/field/auth/refresh",
	"/field/auth/pin", "/field/auth/logout", "/field/auth/act-as", "/field/auth/sign-in-code",
	// The worker's own photo: the set route's body is an image with an image Content-Type, which is
	// not a CORS-safelisted type, so the browser preflights it; the rule's Content-Type allowance
	// (any value) and its exposed ETag are all a page needs.
	"/field/worker/photo", "/field/worker/photo/set", "/field/worker/photo/remove",
	// Unlock keys (docs/field-unlock-keys-spec.md): a browser page cannot hold a biometric-gated key,
	// but the routes answer CORS as every /field/* route does.
	"/field/auth/unlock-keys", "/field/auth/unlock-keys/revoke", "/field/auth/unlock/challenge", "/field/auth/unlock",
}

func TestFieldCORSPreflight(t *testing.T) {
	fc := newFieldCORSHarness(t, true)
	for _, p := range fieldPaths {
		rec := fc.preflight(p, fc.allowed)
		require.Equal(t, http.StatusNoContent, rec.Code, p)
		h := rec.Header()
		assert.Equal(t, fc.allowed, h.Get("Access-Control-Allow-Origin"), p)
		assert.Equal(t, "POST", h.Get("Access-Control-Allow-Methods"), p)
		assert.Equal(t, "Authorization, Content-Type, DPoP", h.Get("Access-Control-Allow-Headers"), p)
		assert.Equal(t, "DPoP-Nonce, Retry-After, WWW-Authenticate, ETag", h.Get("Access-Control-Expose-Headers"), p)
		assert.Equal(t, "600", h.Get("Access-Control-Max-Age"), "the push gateway's preflight lifetime")
		assert.Equal(t, []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"}, h.Values("Vary"), p)
		assert.Empty(t, h.Values("Access-Control-Allow-Credentials"), "never credentials")
		assert.Empty(t, h.Get("X-Correlation-ID"), "answered before the protocol's middleware")
	}

	// An origin not in the table: refused, no CORS headers.
	rec := fc.preflight("/field/auth/login", fc.notAllow)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assertNoCORSHeaders(t, rec, "not allowed")
	assert.Equal(t, []string{"Origin"}, rec.Header().Values("Vary"))

	// The rule is the field protocol's only: an allowed origin gets nothing on the platform's
	// sign-in routes, whose login-CSRF guard relies on there being no preflight answer.
	rec = fc.preflight("/auth/login", fc.allowed)
	assertNoCORSHeaders(t, rec, "/auth/login")
	assert.NotEqual(t, http.StatusNoContent, rec.Code)
	rec = fc.preflight("/oauth/token", fc.allowed)
	assertNoCORSHeaders(t, rec, "/oauth/token")
}

func TestFieldCORSSignInFromAnAllowedOrigin(t *testing.T) {
	fc := newFieldCORSHarness(t, true)
	d := newFCDevice(t)

	// The nonce field-js primes with, then the sign-in: both reach the handlers and carry the
	// allow-origin, the exposed headers, and no allow-credentials.
	nonce := fc.post(d, "/field/auth/nonce", fc.allowed, map[string]any{})
	require.Equal(t, http.StatusOK, nonce.Code, nonce.Body.String())
	in := fc.login(d, fc.allowed)
	require.Equal(t, http.StatusOK, in.Code, in.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(in.Body.Bytes(), &body))
	assert.NotEmpty(t, body["accessToken"])
	assert.NotEmpty(t, body["refreshToken"])

	for name, rec := range map[string]*httptest.ResponseRecorder{"nonce": nonce, "login": in} {
		h := rec.Header()
		assert.Equal(t, fc.allowed, h.Get("Access-Control-Allow-Origin"), name)
		assert.Contains(t, exposed(rec), "DPoP-Nonce", "%s: the next nonce is readable by the page", name)
		assert.NotEmpty(t, h.Get("DPoP-Nonce"), name)
		assert.Contains(t, exposed(rec), "Retry-After", name)
		assert.Contains(t, exposed(rec), "WWW-Authenticate", name)
		assert.Empty(t, h.Values("Access-Control-Allow-Credentials"), name)
		assert.Contains(t, h.Values("Vary"), "Origin", name)
		assert.Empty(t, h.Get("Access-Control-Allow-Methods"), "%s: preflight headers only on the preflight", name)
		assert.Empty(t, h.Values("Set-Cookie"), "%s: no cookie", name)
	}

	// A refused call (an unusable nonce) is readable too: the challenge and the fresh nonce.
	d.nonce = "not-a-nonce"
	bad := fc.login(d, fc.allowed)
	assert.Equal(t, http.StatusUnauthorized, bad.Code)
	assert.Equal(t, fc.allowed, bad.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, bad.Header().Get("WWW-Authenticate"), "use_dpop_nonce")
	assert.NotEmpty(t, bad.Header().Get("DPoP-Nonce"))
}

func TestFieldCORSOriginNotInTheTable(t *testing.T) {
	fc := newFieldCORSHarness(t, true)

	// What a not-allowed page's request did before: the protocol answers it (the browser then
	// keeps the answer from the page, and never sends it at all without a preflight). Only the
	// CORS headers are missing.
	d := newFCDevice(t)
	require.Equal(t, http.StatusOK, fc.post(d, "/field/auth/nonce", fc.notAllow, map[string]any{}).Code)
	in := fc.login(d, fc.notAllow)
	require.Equal(t, http.StatusOK, in.Code, in.Body.String())
	assertNoCORSHeaders(t, in, "login")
	assert.NotEmpty(t, in.Header().Get("DPoP-Nonce"))
	assert.Equal(t, []string{"Origin"}, in.Header().Values("Vary"))
}

func TestFieldCORSWithoutAnOriginIsAnsweredAsBefore(t *testing.T) {
	fc := newFieldCORSHarness(t, true)

	// A phone: no Origin, no CORS header, no Vary.
	d := newFCDevice(t)
	nonce := fc.post(d, "/field/auth/nonce", "", map[string]any{})
	require.Equal(t, http.StatusOK, nonce.Code)
	in := fc.login(d, "")
	require.Equal(t, http.StatusOK, in.Code, in.Body.String())
	for _, rec := range []*httptest.ResponseRecorder{nonce, in} {
		assertNoCORSHeaders(t, rec, "phone")
		assert.Empty(t, rec.Header().Values("Vary"))
		assert.NotEmpty(t, rec.Header().Get("X-Correlation-ID"), "the protocol's middleware still runs")
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	}

	// OPTIONS without a preflight: the router's 405 as before, from the console's method-mismatch
	// handler when the SPA is mounted, and chi's own (Allow: POST) when it is not.
	opt := func(h http.Handler) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/field/auth/login", nil))
		return rec
	}
	rec := opt(fc.h)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Empty(t, rec.Header())
	assert.Empty(t, rec.Body.String())
	rec = opt(newFieldCORSHarness(t, false).h)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, http.Header{"Allow": {"POST"}}, rec.Header())

	// A path the protocol does not have, and a GET: the SPA fallback, as before.
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/field/nothing-here", nil),
		httptest.NewRequest(http.MethodGet, "/field/auth/login", nil),
	} {
		rec := httptest.NewRecorder()
		fc.h.ServeHTTP(rec, req)
		assert.Equal(t, "spa", rec.Body.String(), req.URL.Path)
	}
}

func TestFieldCORSFollowsTheTable(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the origin cache")
	}
	fc := newFieldCORSHarness(t, true)
	added := fc.notAllow
	require.Equal(t, http.StatusNoContent, fc.preflight("/field/auth/login", fc.allowed).Code)
	require.Equal(t, http.StatusForbidden, fc.preflight("/field/auth/login", added).Code)

	// The console adds one origin and removes the other.
	addOrigin(t, added)
	removeOrigin(t, fc.allowed)
	assert.Equal(t, http.StatusNoContent, fc.preflight("/field/auth/login", fc.allowed).Code, "cached")
	assert.Equal(t, http.StatusForbidden, fc.preflight("/field/auth/login", added).Code, "cached")

	time.Sleep(cors.OriginCacheTTL + time.Second)
	assert.Equal(t, http.StatusNoContent, fc.preflight("/field/auth/login", added).Code, "an added origin applies within the cache time")
	assert.Equal(t, http.StatusForbidden, fc.preflight("/field/auth/login", fc.allowed).Code, "a removed origin stops")
	d := newFCDevice(t)
	rec := fc.post(d, "/field/auth/nonce", fc.allowed, map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)
	assertNoCORSHeaders(t, rec, "removed origin")
}
