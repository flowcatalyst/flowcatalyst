//go:build integration

package fieldauth

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	fieldidentityapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	sdkauth "github.com/flowcatalyst/go-sdk/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

const goodPIN = "482915"

func roomyLimits() *ratelimit.Policies {
	p := ratelimit.PoliciesFromEnv()
	p.FieldPinDevice = ratelimit.Policy{Window: time.Hour, Limit: 10000}
	p.FieldPinClient = ratelimit.Policy{Window: time.Hour, Limit: 10000}
	return &p
}

// ── happy path through the admin API ─────────────────────────────────────

// adminAPI mounts the huma admin surface (with the real introspect verifier)
// behind an administrator.
func (e *env) adminAPI() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), authContext())))
		})
	})
	fieldidentityapi.Register(humachi.New(r, apiroute.PlatformAPIConfig("t", "1")), &fieldidentityapi.State{
		Ops: e.s.Ops, UoW: e.s.UoW, BaseURL: baseURL, Verifier: TokenVerifier{Auth: e.auth},
	})
	return r
}

func adminPost(t *testing.T, h http.Handler, path string, body any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Less(t, rec.Code, 300, rec.Body.String())
	var m map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m))
	return m
}

func (e *env) introspectActive(h http.Handler, token string) bool {
	e.t.Helper()
	return adminPost(e.t, h, "/api/field-tokens/introspect", map[string]any{"token": token})["active"] == true
}

// jwksServer serves the platform's JWKS the way /.well-known/jwks.json does,
// so the SDK verifier can be run against tokens the platform minted.
func (e *env) jwksServer() *httptest.Server {
	var keys []map[string]string
	for _, k := range e.auth.AllJWKSKeys() {
		keys = append(keys, map[string]string{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": k.KeyID, "n": k.Components.N, "e": k.Components.E})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	e.t.Cleanup(srv.Close)
	return srv
}

func TestHappyPathAdminToLogoutAndSDKVerify(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	admin := e.adminAPI()

	// Create the worker and an enrolment code through the admin API.
	w := adminPost(t, admin, "/api/field-workers", map[string]any{
		"clientId": e.clientID, "workerCode": "hp-1", "name": "Happy Path", "appCodes": []string{e.codeA}, "pin": goodPIN,
	})
	workerID := w["id"].(string)
	code := adminPost(t, admin, "/api/field-devices/enrolment-codes", map[string]any{
		"clientId": e.clientID, "mode": "PERSONAL", "appCodes": []string{e.codeA},
	})["code"].(string)
	// The supervisor-entered PIN forces a change; clear it so the worker can work.
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET pin_change_required = FALSE WHERE id = $1`, workerID)
	require.NoError(t, err)

	d := e.newDevice()
	enrol := d.enrol(code)
	require.Equal(t, http.StatusCreated, enrol.Code, enrol.Body.String())
	assert.Equal(t, e.clientID, enrol.str("clientId"))
	assert.Equal(t, "PERSONAL", enrol.str("mode"))
	assert.Equal(t, []any{e.codeA}, enrol.json()["appCodes"])

	in := mustLogin(t, d, e.codeA, "HP-1", goodPIN)
	assert.Equal(t, "DPoP", in.body["tokenType"])
	assert.True(t, strings.HasPrefix(in.refresh, "fwr_"))
	assert.Equal(t, false, in.body["pinChangeRequired"])
	claims := tokenPayload(t, in.access)
	assert.Equal(t, "field", claims["token_use"])
	assert.Equal(t, e.codeA, claims["aud"])
	assert.Equal(t, workerID, claims["sub"])
	assert.Equal(t, e.clientID, claims["client_id"])
	assert.Equal(t, e.ident, claims["client_identifier"])
	assert.Equal(t, "HP-1", claims["worker_code"])
	assert.Equal(t, d.id, claims["device_id"])
	assert.Equal(t, "PERSONAL", claims["device_mode"])
	assert.Equal(t, []any{"pin"}, claims["amr"])
	assert.Equal(t, d.jkt(), claims["cnf"].(map[string]any)["jkt"])
	for _, absent := range []string{"roles", "scope", "applications", "clients", "pcr"} {
		assert.NotContains(t, claims, absent)
	}
	assert.Equal(t, "fc-field+jwt", tokenHeader(t, in.access)["typ"])

	// introspect: active
	require.True(t, e.introspectActive(admin, in.access))

	// An application backend verifies the same token with the SDK.
	jwks := e.jwksServer()
	v := sdkauth.NewFieldTokenVerifier(sdkauth.FieldTokenVerifierConfig{Issuer: issuer, AppCode: e.codeA, JWKSURL: jwks.URL})
	appURL := "https://app.example.test/api/orders"
	proof := d.proof(http.MethodPost, "/x", in.access, proofOpts{htu: appURL})
	p, err := v.Verify(context.Background(), "DPoP "+in.access, proof, http.MethodPost, appURL)
	require.NoError(t, err, "the SDK verifies a token the platform minted (RS256)")
	assert.Equal(t, workerID, p.WorkerID)
	assert.Equal(t, e.ident, p.ClientIdentifier)
	assert.Equal(t, "HP-1", p.WorkerCode)

	// refresh → a new token pair, old one inactive-able
	r := d.refresh(in.refresh)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.NotEqual(t, in.refresh, r.str("refreshToken"))
	newAccess, newRefresh := r.str("accessToken"), r.str("refreshToken")
	require.True(t, e.introspectActive(admin, newAccess))

	// logout, then introspect says inactive for both access tokens.
	lo := d.call("/field/auth/logout", map[string]any{"refreshToken": newRefresh, "scope": "session"})
	require.Equal(t, http.StatusNoContent, lo.Code, lo.Body.String())
	assert.False(t, e.introspectActive(admin, newAccess))
	assert.False(t, e.introspectActive(admin, in.access), "same family")
	// Idempotent.
	assert.Equal(t, http.StatusNoContent, d.call("/field/auth/logout", map[string]any{"refreshToken": newRefresh}).Code)
	// The refresh token is dead.
	assert.Equal(t, http.StatusUnauthorized, d.refresh(newRefresh).Code)

	// Sign-ins were recorded; nothing secret is in events, audit or attempts.
	assert.GreaterOrEqual(t, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND outcome = 'SUCCESS'`, workerID), 1)
	assert.Equal(t, 1, e.eventCount(fieldidentity.DeviceEnrolledType))
	e.assertNoLeak(goodPIN, code, in.refresh, newRefresh)
}

func tokenPayload(t *testing.T, tok string) map[string]any {
	t.Helper()
	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func tokenHeader(t *testing.T, tok string) map[string]any {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[0])
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// ── brute force ──────────────────────────────────────────────────────────

func TestBruteForceSoftThenHardLock(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("bf-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	expireSoftLock := func() {
		_, err := testpg.Pool(t).Exec(context.Background(),
			`UPDATE field_identities SET locked_until = NOW() - INTERVAL '1 second' WHERE id = $1`, workerID)
		require.NoError(t, err)
	}

	// Five wrong PINs: soft lock at the threshold.
	for i := 1; i <= 5; i++ {
		r := d.login(e.codeA, "bf-1", "000111")
		require.Equal(t, http.StatusUnauthorized, r.Code, "attempt %d: %s", i, r.Body.String())
		assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
	}
	// Soft locked: even the right PIN gets 429 + Retry-After, and is not verified.
	r := d.login(e.codeA, "bf-1", goodPIN)
	require.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())
	assert.NotEmpty(t, r.Header().Get("Retry-After"))
	assert.Equal(t, "WORKER_LOCKED_TEMPORARILY", r.str("error"))

	// Failures past the threshold re-arm the soft lock; the tenth hard-locks.
	for i := 6; i <= 10; i++ {
		expireSoftLock()
		r := d.login(e.codeA, "bf-1", "000111")
		require.Equal(t, http.StatusUnauthorized, r.Code, "attempt %d", i)
	}
	expireSoftLock()
	r = d.login(e.codeA, "bf-1", goodPIN)
	require.Equal(t, http.StatusLocked, r.Code, r.Body.String())
	assert.Equal(t, "WORKER_LOCKED", r.str("error"))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND hard_locked`, workerID))

	// An administrator's unlock clears both; the worker signs in again.
	_, err := usecaseop.Run(e.actx, e.s.UoW, e.s.Ops.UnlockWorker(),
		fieldidentity.WorkerRef{ClientID: e.clientID, ID: workerID}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)
	mustLogin(t, d, e.codeA, "bf-1", goodPIN)
	assert.Equal(t, 0, e.q(`SELECT failed_since_success FROM field_identities WHERE id = $1`, workerID), "a success resets the counter")
	// Every attempt is in the log; the wrong PIN itself is nowhere.
	assert.GreaterOrEqual(t, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND outcome = 'FAILURE'`, workerID), 10)
	e.assertNoLeak("000111", goodPIN)
}

// ── spray ────────────────────────────────────────────────────────────────

func TestSprayTripsDeviceBucketAcrossWorkerCodes(t *testing.T) {
	lim := roomyLimits()
	lim.FieldPinDevice = ratelimit.Policy{Window: time.Hour, Limit: 5}
	e := newEnv(t, envOpts{limits: lim})
	e.worker("sp-real", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	other := e.enrolled(fieldidentity.ModePersonal, e.codeA)

	// Five guesses, each against a different (unknown) worker code, each a 401.
	for i := range 5 {
		r := d.login(e.codeA, "ghost-"+string(rune('a'+i)), "123456")
		require.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	}
	// The bucket is full: the next attempt on this device is 429 whatever the
	// worker and PIN, including the real worker's right PIN.
	r := d.login(e.codeA, "sp-real", goodPIN)
	require.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())
	assert.NotEmpty(t, r.Header().Get("Retry-After"))
	assert.Equal(t, "TOO_MANY_ATTEMPTS", r.str("error"))
	// Another device of the same client is unaffected.
	mustLogin(t, other, e.codeA, "sp-real", goodPIN)
	// One throttled event for the device, however many attempts follow.
	for range 3 {
		_ = d.login(e.codeA, "sp-real", goodPIN)
	}
	n := e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%' || $2 || '%'`, fieldidentity.DeviceThrottledType, d.id)
	assert.Equal(t, 1, n, "one field-device.throttled per device per window")
}

func TestClientBucketThrottlesEveryDevice(t *testing.T) {
	lim := roomyLimits()
	lim.FieldPinClient = ratelimit.Policy{Window: time.Hour, Limit: 4}
	e := newEnv(t, envOpts{limits: lim})
	e.worker("cb-real", goodPIN, false, e.codeA)
	a, b := e.enrolled(fieldidentity.ModePersonal, e.codeA), e.enrolled(fieldidentity.ModePersonal, e.codeA)
	for range 2 {
		_ = a.login(e.codeA, "nobody-a", "123456")
		_ = b.login(e.codeA, "nobody-b", "123456")
	}
	r := b.login(e.codeA, "cb-real", goodPIN)
	assert.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())
	assert.Equal(t, http.StatusTooManyRequests, a.login(e.codeA, "cb-real", goodPIN).Code)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%CLIENT%' AND data::text LIKE '%' || $2 || '%'`,
		fieldidentity.DeviceThrottledType, e.clientID))
}

// ── refresh ──────────────────────────────────────────────────────────────

func TestRefreshRotation(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("rr-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "rr-1", goodPIN)
	family := tokenPayload(t, in.access)["sid"].(string)

	r := d.refresh(in.refresh)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	next := r.str("refreshToken")
	assert.NotEqual(t, in.refresh, next)
	assert.Equal(t, family, tokenPayload(t, r.str("accessToken"))["sid"], "the family survives rotation")

	assert.Equal(t, 2, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1`, family))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL`, family), "one live row")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_reason = 'ROTATED' AND rotated_at IS NOT NULL`, family))
	// The child inherits the absolute deadline and amr.
	assert.Equal(t, 1, e.q(`SELECT COUNT(DISTINCT absolute_expires_at) FROM field_sessions WHERE family_id = $1`, family))
	// The chain continues.
	assert.Equal(t, http.StatusOK, d.refresh(next).Code)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1`, workerID))
	e.assertNoLeak(in.refresh, next)
}

func TestRefreshReuseKillsFamily(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("ru-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "ru-1", goodPIN)
	family := tokenPayload(t, in.access)["sid"].(string)
	r1 := d.refresh(in.refresh)
	require.Equal(t, http.StatusOK, r1.Code)
	child := r1.str("refreshToken")

	// The rotated token comes back after the grace window: theft.
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_sessions SET rotated_at = NOW() - INTERVAL '2 minutes' WHERE family_id = $1 AND revoked_reason = 'ROTATED'`, family)
	require.NoError(t, err)
	r := d.refresh(in.refresh)
	require.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_GRANT", r.str("error"))

	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL`, family), "the whole family is dead")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_reason = 'REUSE'`, family))
	// The legitimate child dies with it.
	assert.Equal(t, http.StatusUnauthorized, d.refresh(child).Code)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%' || $2 || '%'`, fieldidentity.SessionRevokedType, family))
}

func TestRefreshRetryGraceWithin60Seconds(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("rg-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "rg-1", goodPIN)
	family := tokenPayload(t, in.access)["sid"].(string)

	lost := d.refresh(in.refresh) // the response is "lost": the client never sees it
	require.Equal(t, http.StatusOK, lost.Code)
	retry := d.refresh(in.refresh) // the client retries with the token it still has
	require.Equal(t, http.StatusOK, retry.Code, "within 60s a retry is not theft: %s", retry.Body.String())
	assert.NotEqual(t, lost.str("refreshToken"), retry.str("refreshToken"))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL`, family), "still one live session")
	// The retry's child works; the lost child was rotated away by the retry.
	assert.Equal(t, http.StatusOK, d.refresh(retry.str("refreshToken")).Code)
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%' || $2 || '%'`, fieldidentity.SessionRevokedType, family),
		"a retry is not a revocation")

	// The same rotated token from ANOTHER enrolled device is theft, at once.
	thief := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	r := thief.refresh(in.refresh)
	require.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL`, family))
}

func TestRefreshHonoursIdleAndAbsoluteDeadlines(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("dl-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModeShared, e.codeA)
	in := mustLogin(t, d, e.codeA, "dl-1", goodPIN)
	family := tokenPayload(t, in.access)["sid"].(string)
	// SHARED: 15 minute idle, 12 hour absolute.
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1
		AND idle_expires_at - issued_at BETWEEN INTERVAL '899 seconds' AND INTERVAL '901 seconds'
		AND absolute_expires_at - issued_at BETWEEN INTERVAL '43199 seconds' AND INTERVAL '43201 seconds'`, family))

	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_sessions SET idle_expires_at = NOW() - INTERVAL '1 second' WHERE family_id = $1`, family)
	require.NoError(t, err)
	r := d.refresh(in.refresh)
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "SESSION_EXPIRED", r.str("error"))

	in = mustLogin(t, d, e.codeA, "dl-1", goodPIN)
	_, err = testpg.Pool(t).Exec(context.Background(), `UPDATE field_sessions SET absolute_expires_at = NOW() - INTERVAL '1 second' WHERE refresh_hash = $1`,
		fieldidentity.HashToken(in.refresh))
	require.NoError(t, err)
	r = d.refresh(in.refresh)
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "SESSION_EXPIRED", r.str("error"))
}

// ── revoked / suspended ──────────────────────────────────────────────────

func TestRevokedDeviceRefusedForLoginAndRefresh(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("rd-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "rd-1", goodPIN)

	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.RevokeDevice(),
		fieldidentity.DeviceRef{ClientID: e.clientID, ID: d.id}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)

	r := d.login(e.codeA, "rd-1", goodPIN)
	assert.Equal(t, http.StatusForbidden, r.Code)
	assert.Equal(t, "DEVICE_REVOKED", r.str("error"))
	r = d.refresh(in.refresh)
	assert.Equal(t, http.StatusForbidden, r.Code)
	assert.Equal(t, "DEVICE_REVOKED", r.str("error"))
}

func TestSuspendedDeviceRefused(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("sd-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.SuspendDevice(),
		fieldidentity.DeviceRef{ClientID: e.clientID, ID: d.id}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)
	r := d.login(e.codeA, "sd-1", goodPIN)
	assert.Equal(t, http.StatusForbidden, r.Code)
	assert.Equal(t, "DEVICE_SUSPENDED", r.str("error"))
}

func TestSuspendedWorkerRefused(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("sw-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "sw-1", goodPIN)

	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.SuspendWorker(),
		fieldidentity.WorkerRef{ClientID: e.clientID, ID: workerID}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)

	r := d.login(e.codeA, "sw-1", goodPIN) // the right PIN, still refused
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"), "indistinguishable from a wrong PIN")
	r = d.refresh(in.refresh)
	assert.Equal(t, http.StatusUnauthorized, r.Code, "the suspension revoked the family")
}

func TestAccessChecksAfterPINAnswerUniform403(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	mine := e.worker("ac-mine", goodPIN, false, e.codeA, e.codeB)
	e.worker("ac-other", goodPIN, false, e.codeA)
	e.worker("ac-noapp", goodPIN, false, e.codeB)
	d := e.newDevice()
	code, err := fieldidentity.NewEnrolmentCodeValue()
	require.NoError(t, err)
	_, err = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.CreateEnrolmentCode(), fieldidentity.CreateEnrolmentCodeCommand{
		ClientID: e.clientID, Mode: "PERSONAL", AppCodes: []string{e.codeA}, AssignedWorkerID: mine, Code: code,
	}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, d.enrol(code).Code)

	// Assigned worker, enrolled app: fine. The same worker on an app the
	// device is not enrolled for, a worker without the grant, and a worker the
	// device is not assigned to all get the same 403.
	mustLogin(t, d, e.codeA, "ac-mine", goodPIN)
	for _, c := range []struct{ app, worker string }{
		{e.codeB, "ac-mine"}, {e.codeA, "ac-noapp"}, {e.codeA, "ac-other"}, {"no-such-app", "ac-mine"},
	} {
		r := d.login(c.app, c.worker, goodPIN)
		assert.Equal(t, http.StatusForbidden, r.Code, "%+v: %s", c, r.Body.String())
		assert.Equal(t, "NO_APP_ACCESS", r.str("error"), "%+v", c)
	}
	// With a WRONG PIN the same requests are 401: access is only checked after
	// the PIN verifies, so it cannot be probed without it.
	assert.Equal(t, http.StatusUnauthorized, d.login(e.codeB, "ac-mine", "000111").Code)
	// The reason is in the log, not the response.
	assert.GreaterOrEqual(t, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE failure_reason IN ('NO_GRANT','DEVICE_APP_NOT_ENROLLED','DEVICE_ASSIGNED_TO_OTHER','APP_UNAVAILABLE')`), 4)
}

// ── proof, nonce, key ────────────────────────────────────────────────────

func TestUsedNonceRefused(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("un-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	d.fetchNonce()
	body := map[string]any{"appCode": e.codeA, "workerCode": "un-1", "pin": goodPIN}
	proof := d.proof(http.MethodPost, "/field/auth/login", "", proofOpts{})
	first := d.raw(http.MethodPost, "/field/auth/login", body, proof, "")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	freshA := first.Header().Get("DPoP-Nonce")
	require.NotEmpty(t, freshA, "a success carries a fresh nonce")

	// The identical proof, replayed, carries a used nonce.
	replay := d.raw(http.MethodPost, "/field/auth/login", body, proof, "")
	require.Equal(t, http.StatusUnauthorized, replay.Code, replay.Body.String())
	assert.Equal(t, "use_dpop_nonce", replay.str("error"))
	assert.Equal(t, `DPoP error="use_dpop_nonce"`, replay.Header().Get("WWW-Authenticate"))
	freshB := replay.Header().Get("DPoP-Nonce")
	assert.NotEmpty(t, freshB, "an error carries a fresh nonce too")
	assert.NotEqual(t, freshA, freshB)
	// The client retries once with the fresh nonce and succeeds.
	d.nonce = freshB
	retry := d.raw(http.MethodPost, "/field/auth/login", body, d.proof(http.MethodPost, "/field/auth/login", "", proofOpts{}), "")
	assert.Equal(t, http.StatusOK, retry.Code, retry.Body.String())

	// No nonce at all, and an expired one, are the same refusal.
	none := d.raw(http.MethodPost, "/field/auth/login", body, d.proof(http.MethodPost, "/field/auth/login", "", proofOpts{nonce: "-"}), "")
	assert.Equal(t, "use_dpop_nonce", none.str("error"))
	n := d.fetchNonce()
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_nonces SET expires_at = NOW() - INTERVAL '1 second' WHERE nonce_hash = $1`, nonceHashForTest(n))
	require.NoError(t, err)
	expired := d.raw(http.MethodPost, "/field/auth/login", body, d.proof(http.MethodPost, "/field/auth/login", "", proofOpts{nonce: n}), "")
	assert.Equal(t, "use_dpop_nonce", expired.str("error"))
}

func nonceHashForTest(n string) []byte {
	sum := sha256.Sum256([]byte(n))
	return sum[:]
}

func signRS256(t *testing.T, privPEM []byte, claims jwt.MapClaims) string {
	t.Helper()
	block, _ := pem.Decode(privPEM)
	require.NotNil(t, block)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)
	out, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	require.NoError(t, err)
	return out
}

func TestWrongKeyProofRefused(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("wk-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	attacker := e.newDevice()
	body := map[string]any{"appCode": e.codeA, "workerCode": "wk-1", "pin": goodPIN}
	path := "/field/auth/login"

	// 1. The attacker signs with their own key but presents the victim device's
	//    public key in the header: the signature does not verify.
	d.fetchNonce()
	r := d.raw(http.MethodPost, path, body, attacker.proof(http.MethodPost, path, "", proofOpts{nonce: d.nonce, jwkOf: d}), "")
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "invalid_dpop_proof", r.str("error"))

	// 2. A valid proof from a key that was never enrolled.
	attacker.nonce = r.Header().Get("DPoP-Nonce")
	r = attacker.raw(http.MethodPost, path, body, attacker.proof(http.MethodPost, path, "", proofOpts{}), "")
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"))

	// 3. The right device but a stolen proof for another method / URL.
	d.nonce = r.Header().Get("DPoP-Nonce")
	r = d.raw(http.MethodPost, path, body, d.proof(http.MethodPost, path, "", proofOpts{htu: baseURL + "/field/auth/refresh"}), "")
	assert.Equal(t, "invalid_dpop_proof", r.str("error"), "htu mismatch")
	d.nonce = r.Header().Get("DPoP-Nonce")
	r = d.raw(http.MethodPost, path, body, d.proof(http.MethodPost, path, "", proofOpts{htm: "GET"}), "")
	assert.Equal(t, "invalid_dpop_proof", r.str("error"), "htm mismatch")
	d.nonce = r.Header().Get("DPoP-Nonce")
	// 4. No proof header at all.
	r = d.raw(http.MethodPost, path, body, "", "")
	assert.Equal(t, "invalid_dpop_proof", r.str("error"))
	// 5. A proof more than a day off the clock.
	d.nonce = r.Header().Get("DPoP-Nonce")
	r = d.raw(http.MethodPost, path, body, d.proof(http.MethodPost, path, "", proofOpts{iat: time.Now().Add(-48 * time.Hour)}), "")
	assert.Equal(t, "invalid_dpop_proof", r.str("error"))
	// And a skewed-but-within-a-day device clock is fine.
	d.nonce = r.Header().Get("DPoP-Nonce")
	r = d.raw(http.MethodPost, path, body, d.proof(http.MethodPost, path, "", proofOpts{iat: time.Now().Add(-6 * time.Hour)}), "")
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())
}

func TestEnrolmentRules(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	code := e.enrolCode(fieldidentity.ModeShared, "", e.codeA)

	// The proof key must be the key being enrolled.
	d, other := e.newDevice(), e.newDevice()
	r := d.call("/field/devices/enrol", map[string]any{
		"code": code, "publicKeyJwk": other.jwk(), "platform": "ios", "model": "Phone",
	})
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "invalid_dpop_proof", r.str("error"))

	// A bad code burns nothing and enrols nothing.
	r = d.call("/field/devices/enrol", map[string]any{"code": "WRONGCODE9", "publicKeyJwk": d.jwk(), "platform": "ios", "model": "Phone"})
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "INVALID_ENROLMENT_CODE", r.str("error"))

	// require_attestation cannot be satisfied in P1 and does not consume the code.
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO field_policies (client_id, require_attestation) VALUES ($1, TRUE)
		 ON CONFLICT (client_id) DO UPDATE SET require_attestation = TRUE`, e.clientID)
	require.NoError(t, err)
	r = d.enrol(code)
	assert.Equal(t, http.StatusUnprocessableEntity, r.Code, r.Body.String())
	assert.Equal(t, "ATTESTATION_REQUIRED", r.str("error"))
	_, err = testpg.Pool(t).Exec(context.Background(), `UPDATE field_policies SET require_attestation = FALSE WHERE client_id = $1`, e.clientID)
	require.NoError(t, err)

	// Now it enrols (the code survived), a SHARED device with the code's apps.
	r = d.enrol(code)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	assert.Equal(t, "SHARED", r.str("mode"))
	// Single use: a second device cannot use it.
	again := e.newDevice().enrol(code)
	assert.Equal(t, http.StatusUnauthorized, again.Code)
	assert.Equal(t, "INVALID_ENROLMENT_CODE", again.str("error"))
	// The same key cannot enrol twice.
	dup := d.enrol(e.enrolCode(fieldidentity.ModePersonal, "", e.codeA))
	assert.Equal(t, http.StatusConflict, dup.Code, dup.Body.String())
	assert.Equal(t, "DEVICE_KEY_EXISTS", dup.str("error"))
	// An expired code is refused.
	expired := e.enrolCode(fieldidentity.ModePersonal, "", e.codeA)
	_, err = testpg.Pool(t).Exec(context.Background(), `UPDATE field_enrolment_codes SET expires_at = NOW() - INTERVAL '1 minute' WHERE code_hash = $1`, fieldidentity.HashToken(expired))
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, e.newDevice().enrol(expired).Code)
	// The attestation chain is stored raw with the reported level.
	withChain := e.newDevice()
	chainCode := e.enrolCode(fieldidentity.ModePersonal, "", e.codeA)
	r = withChain.call("/field/devices/enrol", map[string]any{
		"code": chainCode, "publicKeyJwk": withChain.jwk(), "platform": "android", "model": "Pixel",
		"attestationChain": []string{"cert1", "cert2"},
	})
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_devices WHERE id = $1 AND attestation_level = 'SOFTWARE' AND attestation_chain LIKE '%cert2%'`, r.str("deviceId")))
	e.assertNoLeak(code, chainCode)
}

// ── field token is not an API credential ─────────────────────────────────

func TestFieldTokenRejectedAsAPIBearer(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("ab-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "ab-1", goodPIN)

	// The platform's own API middleware, over the SAME signing key, issuer and
	// (so that nothing else can be the cause) an audience equal to the field
	// token's aud. Only token_use stands between the token and the API.
	prov, err := provider.NewProvider(provider.Config{Issuer: issuer, Audience: e.codeA, SigningKey: e.privPEM}, nil, nil)
	require.NoError(t, err)
	reached := false
	h := platformmw.Authenticator(platformmw.AuthConfig{Provider: prov})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = auth.FromContext(r.Context()) != nil
	}))
	hit := func(token string) int {
		reached = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	assert.Equal(t, http.StatusUnauthorized, hit(in.access), "a field token is not an API bearer")
	assert.False(t, reached)

	// Control: an API token with the same key and claims shape does pass, so the
	// 401 above is the token_use allowlist and not a broken fixture.
	api := jwt.MapClaims{
		"iss": issuer, "aud": e.codeA, "sub": "prn_x", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
		"type": "SERVICE", "tier": "ANCHOR", "token_use": "api",
	}
	signed := signRS256(t, e.privPEM, api)
	assert.Equal(t, http.StatusOK, hit(signed))
	assert.True(t, reached)
}

// ── pcr ──────────────────────────────────────────────────────────────────

func TestPCRTokenOnlyWorksOnPinEndpoint(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	admin := e.adminAPI()
	workerID := e.worker("pc-1", goodPIN, true, e.codeA) // supervisor-set PIN: change required
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	second := e.enrolled(fieldidentity.ModePersonal, e.codeA)

	in := mustLogin(t, d, e.codeA, "pc-1", goodPIN)
	assert.Equal(t, true, in.body["pinChangeRequired"])
	assert.Equal(t, true, tokenPayload(t, in.access)["pcr"])
	other := mustLogin(t, second, e.codeA, "pc-1", goodPIN) // a second family, to be revoked by the change

	// Not an API credential, not an application credential, not introspect-active.
	jwks := e.jwksServer()
	v := sdkauth.NewFieldTokenVerifier(sdkauth.FieldTokenVerifierConfig{Issuer: issuer, AppCode: e.codeA, JWKSURL: jwks.URL})
	appURL := "https://app.example.test/api/orders"
	_, err := v.Verify(context.Background(), "DPoP "+in.access, d.proof(http.MethodPost, "/x", in.access, proofOpts{htu: appURL}), http.MethodPost, appURL)
	var fe *sdkauth.FieldError
	require.ErrorAs(t, err, &fe)
	assert.Equal(t, sdkauth.FieldReasonPCR, fe.Reason)
	assert.False(t, e.introspectActive(admin, in.access), "a pcr token is never active for an app")

	// It does work on /field/auth/pin: wrong current PIN, weak new PIN, then success.
	r := d.callBearer("/field/auth/pin", map[string]any{"currentPin": "000111", "newPin": "739204"}, in.access)
	assert.Equal(t, http.StatusUnauthorized, r.Code, "wrong current PIN")
	r = d.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "123456"}, in.access)
	require.Equal(t, http.StatusUnprocessableEntity, r.Code, r.Body.String())
	assert.Equal(t, "PIN_POLICY", r.str("error"))
	assert.NotEmpty(t, r.json()["violations"])
	r = d.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": goodPIN}, in.access)
	assert.Equal(t, http.StatusUnprocessableEntity, r.Code, "unchanged PIN")
	// No bearer, wrong scheme and a mismatched ath are refused.
	assert.Equal(t, http.StatusUnauthorized, d.call("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "739204"}).Code)
	d.nonce = d.raw(http.MethodPost, "/field/auth/nonce", nil, d.proof(http.MethodPost, "/field/auth/nonce", "", proofOpts{nonce: "-"}), "").Header().Get("DPoP-Nonce")
	r = d.raw(http.MethodPost, "/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "739204"},
		d.proof(http.MethodPost, "/field/auth/pin", "", proofOpts{ath: AccessTokenHash("another token")}), in.access)
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "invalid_dpop_proof", r.str("error"))

	r = d.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "739204"}, in.access)
	require.Equal(t, http.StatusNoContent, r.Code, r.Body.String())

	// The other family was revoked (PIN_CHANGED); this one survives.
	assert.Equal(t, http.StatusUnauthorized, second.refresh(other.refresh).Code)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE identity_id = $1 AND revoked_reason = 'PIN_CHANGED'`, workerID))
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND pin_change_required`, workerID))
	// Refreshing yields a token without pcr that an application accepts.
	rr := d.refresh(in.refresh)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, false, rr.json()["pinChangeRequired"])
	assert.NotContains(t, tokenPayload(t, rr.str("accessToken")), "pcr")
	proof := d.proof(http.MethodPost, "/x", rr.str("accessToken"), proofOpts{htu: appURL})
	_, err = v.Verify(context.Background(), "DPoP "+rr.str("accessToken"), proof, http.MethodPost, appURL)
	require.NoError(t, err)
	assert.True(t, e.introspectActive(admin, rr.str("accessToken")))
	// The new PIN signs in; the old one does not.
	mustLogin(t, d, e.codeA, "pc-1", "739204")
	assert.Equal(t, http.StatusUnauthorized, d.login(e.codeA, "pc-1", goodPIN).Code)
	e.assertNoLeak(goodPIN, "739204")
}

// ── legacy hashes, logout scope, 503 ─────────────────────────────────────

func TestLegacyPinIsRehashedOnFirstSuccess(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("lg-1", "739204", false, e.codeA)
	legacy, err := passwordhash.HashWithParams(goodPIN, pin.Params)
	require.NoError(t, err)
	_, err = testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET pin_hash = $2, pin_scheme = 'LEGACY' WHERE id = $1`, workerID, legacy)
	require.NoError(t, err)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)

	assert.Equal(t, http.StatusUnauthorized, d.login(e.codeA, "lg-1", "000111").Code)
	mustLogin(t, d, e.codeA, "lg-1", goodPIN)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND pin_scheme = 'PEPPER_V1' AND pin_hash <> $2`, workerID, legacy))
	mustLogin(t, d, e.codeA, "lg-1", goodPIN) // and the rehashed PIN verifies
}

func TestLogoutScopes(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("lo-a", goodPIN, false, e.codeA)
	e.worker("lo-b", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModeShared, e.codeA)
	other := e.enrolled(fieldidentity.ModeShared, e.codeA)
	a := mustLogin(t, d, e.codeA, "lo-a", goodPIN)
	b := mustLogin(t, d, e.codeA, "lo-b", goodPIN)
	o := mustLogin(t, other, e.codeA, "lo-a", goodPIN)

	assert.Equal(t, http.StatusNoContent, d.call("/field/auth/logout", map[string]any{"refreshToken": a.refresh, "scope": "session"}).Code)
	assert.Equal(t, http.StatusUnauthorized, d.refresh(a.refresh).Code)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE device_id = $1 AND revoked_at IS NULL`, d.id), "lo-b is still in")

	// device-all: the SHARED hand-over ends every session on THIS device only.
	assert.Equal(t, http.StatusNoContent, d.call("/field/auth/logout", map[string]any{"refreshToken": b.refresh, "scope": "device-all"}).Code)
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_sessions WHERE device_id = $1 AND revoked_at IS NULL`, d.id))
	assert.Equal(t, http.StatusOK, other.refresh(o.refresh).Code)
	// A token from another device is a quiet no-op.
	assert.Equal(t, http.StatusNoContent, d.call("/field/auth/logout", map[string]any{"refreshToken": o.refresh, "scope": "device-all"}).Code)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE device_id = $1 AND revoked_at IS NULL`, other.id))
	assert.Equal(t, http.StatusBadRequest, d.call("/field/auth/logout", map[string]any{"refreshToken": o.refresh, "scope": "everything"}).Code)
}

func TestMissingAppKeyAnswers503(t *testing.T) {
	e := newEnv(t, envOpts{noAppKey: true})
	d := e.newDevice()
	for _, path := range []string{"/field/auth/nonce", "/field/devices/enrol", "/field/auth/login", "/field/auth/refresh", "/field/auth/pin", "/field/auth/logout"} {
		r := d.raw(http.MethodPost, path, map[string]any{}, d.proof(http.MethodPost, path, "", proofOpts{}), "")
		assert.Equal(t, http.StatusServiceUnavailable, r.Code, path)
		assert.Equal(t, fieldidentity.CodeUnconfigured, r.str("error"), path)
	}
}
