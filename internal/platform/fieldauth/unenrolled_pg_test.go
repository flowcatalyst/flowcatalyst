//go:build integration

package fieldauth

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	sdkauth "github.com/flowcatalyst/go-sdk/auth"
)

// Device enrolment is opt-in (docs/field-worker-sign-in-options-spec.md §1): a
// worker code + PIN signs in from any key, which stays the credential's anchor.

func TestUnenrolledPINLoginTokenSessionAndPinChange(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	admin := e.adminAPI()
	workerID := e.worker("un-1", goodPIN, false, e.codeA)
	d := e.newDevice() // never enrolled

	in := mustLoginUn(t, d, e.codeA, "UN-1", goodPIN)
	claims := tokenPayload(t, in.access)
	assert.NotContains(t, claims, "device_id", "an unenrolled token names no device")
	assert.Equal(t, "UNENROLLED", claims["device_mode"])
	assert.Equal(t, d.jkt(), claims["cnf"].(map[string]any)["jkt"], "still bound to the proof key")
	assert.Equal(t, []any{"pin"}, claims["amr"])
	assert.Equal(t, workerID, claims["sub"])
	family := claims["sid"].(string)

	// The session is bound by jkt with no device row; SHARED absolute limit and
	// the policy default unenrolled idle limit.
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND device_id IS NULL AND jkt = $2
		AND idle_expires_at - issued_at BETWEEN INTERVAL '899 seconds' AND INTERVAL '901 seconds'
		AND absolute_expires_at - issued_at BETWEEN INTERVAL '43199 seconds' AND INTERVAL '43201 seconds'`, family, d.jkt()))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND outcome = 'SUCCESS'
		AND device_id IS NULL AND client_id = $2 AND auth_method = 'PIN'`, workerID, e.clientID))

	// Introspection skips the device check; an application's SDK verifier sees
	// no device.
	require.True(t, e.introspectActive(admin, in.access))
	jwks := e.jwksServer()
	v := sdkauth.NewFieldTokenVerifier(sdkauth.FieldTokenVerifierConfig{Issuer: issuer, AppCode: e.codeA, JWKSURL: jwks.URL})
	appURL := "https://app.example.test/api/orders"
	p, err := v.Verify(context.Background(), "DPoP "+in.access, d.proof(http.MethodPost, "/x", in.access, proofOpts{htu: appURL}), http.MethodPost, appURL)
	require.NoError(t, err)
	assert.Equal(t, "", p.DeviceID)
	assert.Equal(t, "UNENROLLED", p.DeviceMode)

	// Refresh on the same key keeps the session unenrolled.
	r := d.refresh(in.refresh)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.NotContains(t, tokenPayload(t, r.str("accessToken")), "device_id")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL AND device_id IS NULL AND jkt = $2`, family, d.jkt()))

	// The client's unenrolled idle limit governs new sessions.
	e.policy("unenrolled_idle_secs", 120)
	again := mustLoginUn(t, d, e.codeA, "un-1", goodPIN)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE refresh_hash = $1
		AND idle_expires_at - issued_at BETWEEN INTERVAL '119 seconds' AND INTERVAL '121 seconds'`, fieldidentity.HashToken(again.refresh)))

	// The PIN changes from an unenrolled key too.
	cur := r.str("accessToken")
	pr := d.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "739204"}, cur)
	require.Equal(t, http.StatusNoContent, pr.Code, pr.Body.String())
	mustLoginUn(t, d, e.codeA, "un-1", "739204")
	e.assertNoLeak(goodPIN, "739204", in.refresh)
}

func TestUnenrolledLoginRules(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	other := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("ur-1", goodPIN, false, e.codeA)
	d := e.newDevice()

	// A client is required from an unenrolled key.
	r := d.login(e.codeA, "ur-1", goodPIN)
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_REQUEST", r.str("error"))
	// Exactly one of workerCode / badgeToken.
	r = d.call("/field/auth/login", map[string]any{"appCode": e.codeA, "clientIdentifier": e.ident, "pin": goodPIN})
	assert.Equal(t, http.StatusBadRequest, r.Code)
	r = d.call("/field/auth/login", map[string]any{
		"appCode": e.codeA, "clientIdentifier": e.ident, "workerCode": "ur-1", "badgeToken": "FWB1X", "pin": goodPIN,
	})
	assert.Equal(t, http.StatusBadRequest, r.Code)

	// An unknown client is indistinguishable from a wrong PIN.
	wrong := d.loginUn(e.codeA, "ur-1", "000111")
	unknown := d.call("/field/auth/login", map[string]any{
		"appCode": e.codeA, "clientIdentifier": "no-such-client", "workerCode": "ur-1", "pin": goodPIN,
	})
	assert.Equal(t, http.StatusUnauthorized, unknown.Code, unknown.Body.String())
	assert.Equal(t, wrong.Code, unknown.Code)
	assert.Equal(t, wrong.Body.String(), unknown.Body.String(), "no client enumeration")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE failure_reason = 'UNKNOWN_CLIENT'`))

	// A worker of another client under this client's identifier does not exist.
	other.worker("ur-1", goodPIN, false, other.codeA)
	r = d.call("/field/auth/login", map[string]any{"appCode": other.codeA, "clientIdentifier": e.ident, "workerCode": "ur-other", "pin": goodPIN})
	assert.Equal(t, http.StatusUnauthorized, r.Code)

	// An enrolled device is bound to its client: naming another is CLIENT_MISMATCH.
	enrolled := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	r = enrolled.call("/field/auth/login", map[string]any{
		"appCode": e.codeA, "clientIdentifier": other.ident, "workerCode": "ur-1", "pin": goodPIN,
	})
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Equal(t, "CLIENT_MISMATCH", r.str("error"))
	r = enrolled.loginUn(e.codeA, "ur-1", goodPIN) // its own client's identifier is fine
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Contains(t, tokenPayload(t, r.str("accessToken")), "device_id", "an enrolled device still gets device_id")
	r = enrolled.login(e.codeA, "ur-1", goodPIN) // and no identifier at all
	assert.Equal(t, http.StatusOK, r.Code)
}

func TestRequireEnrolledDeviceRefusesUnenrolledKeys(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("re-1", goodPIN, false, e.codeA)
	d := e.newDevice()
	in := mustLoginUn(t, d, e.codeA, "re-1", goodPIN) // allowed until the policy says otherwise

	e.policy("require_enrolled_device", true)
	r := d.loginUn(e.codeA, "re-1", goodPIN)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"))
	assert.Equal(t, 0, e.q(`SELECT failed_since_success FROM field_identities WHERE worker_code = 'RE-1' AND client_id = $1`, e.clientID),
		"the refusal costs the worker nothing")
	// An unenrolled session cannot be refreshed once the client requires enrolment.
	r = d.refresh(in.refresh)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"))
	// The PIN cannot be changed from it either.
	r = d.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "739204"}, in.access)
	assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"))

	// An enrolled device is unaffected.
	enrolled := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	mustLogin(t, enrolled, e.codeA, "re-1", goodPIN)
}

func TestUnenrolledRefreshWithAnotherKeyRevokesFamily(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("rk-1", goodPIN, false, e.codeA)
	d := e.newDevice()
	var in signedIn

	for name, thief := range map[string]*device{"another unenrolled key": e.newDevice(), "an enrolled device": e.enrolled(fieldidentity.ModePersonal, e.codeA)} {
		// A fresh family per thief.
		in = mustLoginUn(t, d, e.codeA, "rk-1", goodPIN)
		family := tokenPayload(t, in.access)["sid"].(string)
		r := thief.refresh(in.refresh)
		assert.Equal(t, http.StatusUnauthorized, r.Code, "%s: %s", name, r.Body.String())
		assert.Equal(t, "INVALID_GRANT", r.str("error"), name)
		assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL`, family), "%s: the family is dead", name)
		assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_reason = 'REUSE'`, family), name)
		assert.Equal(t, http.StatusUnauthorized, d.refresh(in.refresh).Code, "%s: the owner's token died with it", name)
	}
}

func TestUnenrolledIPBucketThrottlesAndNotifies(t *testing.T) {
	lim := roomyLimits()
	lim.FieldPinIP = ratelimit.Policy{Window: time.Hour, Limit: 3}
	e := newEnv(t, envOpts{limits: lim})
	e.worker("ip-1", goodPIN, false, e.codeA)
	d := e.newDevice()

	for range 3 {
		r := d.loginUn(e.codeA, "ip-1", "000111")
		require.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	}
	// The IP bucket is full: even the right PIN is 429 with Retry-After.
	r := d.loginUn(e.codeA, "ip-1", goodPIN)
	require.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())
	assert.NotEmpty(t, r.Header().Get("Retry-After"))
	assert.Equal(t, "TOO_MANY_ATTEMPTS", r.str("error"))
	// Another address is unaffected.
	other := e.newDevice()
	mustLoginUn(t, other, e.codeA, "ip-1", goodPIN)
	// One notice, scope IP, with the address.
	for range 3 {
		_ = d.loginUn(e.codeA, "ip-1", goodPIN)
	}
	n := e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%"IP"%' AND data::text LIKE '%' || $2 || '%'`,
		fieldidentity.DeviceThrottledType, d.ip)
	assert.Equal(t, 1, n, "one field-device.throttled (scope IP) per address per window")
}

func TestEnrolledFailuresUseTheDeviceBucketNotTheIPBucket(t *testing.T) {
	lim := roomyLimits()
	lim.FieldPinDevice = ratelimit.Policy{Window: time.Hour, Limit: 3}
	lim.FieldPinIP = ratelimit.Policy{Window: time.Hour, Limit: 3}
	e := newEnv(t, envOpts{limits: lim})
	e.worker("eb-1", goodPIN, false, e.codeA)
	enrolled := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	// Same address as an unenrolled key.
	un := e.newDevice()
	un.ip = enrolled.ip

	for range 3 {
		assert.Equal(t, http.StatusUnauthorized, enrolled.login(e.codeA, "eb-1", "000111").Code)
	}
	r := enrolled.login(e.codeA, "eb-1", goodPIN)
	assert.Equal(t, http.StatusTooManyRequests, r.Code, "the enrolled device's own bucket is full")
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM iam_rate_limit_events WHERE bucket = 'field_pin_ip' AND key = $1`, enrolled.ip),
		"an enrolled device does not count against the IP bucket")
	mustLoginUn(t, un, e.codeA, "eb-1", goodPIN)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%"DEVICE"%' AND data::text LIKE '%' || $2 || '%'`,
		fieldidentity.DeviceThrottledType, enrolled.id))
}

func TestUnenrolledLogoutScopes(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("ul-1", goodPIN, false, e.codeA)
	d := e.newDevice()
	other := e.newDevice()
	in := mustLoginUn(t, d, e.codeA, "ul-1", goodPIN)

	// device-all needs an enrolled device.
	r := d.call("/field/auth/logout", map[string]any{"refreshToken": in.refresh, "scope": "device-all"})
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_REQUEST", r.str("error"))
	assert.Equal(t, http.StatusOK, d.refresh(in.refresh).Code, "nothing was revoked")

	// A token presented with another key is a quiet no-op, not a logout.
	in = mustLoginUn(t, d, e.codeA, "ul-1", goodPIN)
	family := tokenPayload(t, in.access)["sid"].(string)
	assert.Equal(t, http.StatusNoContent, other.call("/field/auth/logout", map[string]any{"refreshToken": in.refresh}).Code)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL`, family))
	// The owner's logout ends the family.
	assert.Equal(t, http.StatusNoContent, d.call("/field/auth/logout", map[string]any{"refreshToken": in.refresh}).Code)
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL`, family))
}

func TestKeyEnrolledAfterTheSessionKeepsUnenrolledLimits(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("ke-1", goodPIN, false, e.codeA)
	d := e.newDevice()
	in := mustLoginUn(t, d, e.codeA, "ke-1", goodPIN)
	family := tokenPayload(t, in.access)["sid"].(string)

	r := d.enrol(e.enrolCode(fieldidentity.ModePersonal, "", e.codeA))
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	// No mid-session upgrade: the refreshed token is still an unenrolled one.
	rr := d.refresh(in.refresh)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	claims := tokenPayload(t, rr.str("accessToken"))
	assert.NotContains(t, claims, "device_id")
	assert.Equal(t, "UNENROLLED", claims["device_mode"])
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL AND device_id IS NULL`, family))
	// The next sign-in is an enrolled one.
	next := mustLogin(t, d, e.codeA, "ke-1", goodPIN)
	assert.Equal(t, d.id, tokenPayload(t, next.access)["device_id"])
}

// A device the key has been enrolled as, then revoked, must not fall back to
// an unenrolled sign-in (the P1 device status checks still apply).
func TestRevokedDeviceKeyIsNotUnenrolled(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("rv-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_devices SET status = 'REVOKED' WHERE id = $1`, d.id)
	require.NoError(t, err)
	r := d.loginUn(e.codeA, "rv-1", goodPIN)
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "DEVICE_REVOKED", r.str("error"))
	_ = fmt.Sprint()
}
