//go:build integration

package fieldauth

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
)

// The per-client PIN-failure budget is split: failures from unenrolled keys
// count in field_pin_client_unenrolled and refuse only unenrolled keys, so an
// anonymous caller cannot fill the budget enrolled devices sign in under.

func splitLimits(unenrolled, enrolled uint32) *ratelimit.Policies {
	lim := roomyLimits()
	lim.FieldPinClientUnenrolled = ratelimit.Policy{Window: time.Hour, Limit: unenrolled}
	lim.FieldPinClient = ratelimit.Policy{Window: time.Hour, Limit: enrolled}
	return lim
}

// sprayUnenrolled makes n failed sign-ins for the client from fresh unenrolled
// keys, each with a random worker code.
func (e *env) sprayUnenrolled(n int) {
	e.t.Helper()
	for i := range n {
		d := e.newDevice()
		r := d.loginUn(e.codeA, fmt.Sprintf("nobody-%d-%d", time.Now().UnixNano(), i), "123456")
		require.Equal(e.t, http.StatusUnauthorized, r.Code, r.Body.String())
	}
}

func (e *env) bucketCount(bucket string) int {
	e.t.Helper()
	return e.q(`SELECT COUNT(*) FROM iam_rate_limit_events WHERE bucket = $1 AND key = $2`, bucket, e.clientID)
}

func assertClientThrottled(t *testing.T, r resp, msg string) {
	t.Helper()
	require.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())
	assert.NotEmpty(t, r.Header().Get("Retry-After"))
	assert.Equal(t, "TOO_MANY_ATTEMPTS", r.str("error"))
	assert.Equal(t, msg, r.str("message"))
}

func TestUnenrolledFailuresDoNotLockEnrolledDevicesOut(t *testing.T) {
	e := newEnv(t, envOpts{limits: splitLimits(3, 10000)})
	e.worker("ta1-real", goodPIN, false, e.codeA)
	enrolled := e.enrolled(fieldidentity.ModePersonal, e.codeA)

	e.sprayUnenrolled(3)
	assert.Equal(t, 3, e.bucketCount("field_pin_client_unenrolled"), "unenrolled failures count in their own bucket")
	assert.Equal(t, 0, e.bucketCount("field_pin_client"), "and not in the enrolled bucket")

	// An unenrolled login for the client is throttled, as before.
	un := e.newDevice()
	assertClientThrottled(t, un.loginUn(e.codeA, "ta1-real", goodPIN), "too many failed sign-ins; try again later")
	// An enrolled device with the right PIN still signs in.
	mustLogin(t, enrolled, e.codeA, "ta1-real", goodPIN)
	assert.Equal(t, 0, e.bucketCount("field_pin_client"), "the enrolled bucket is untouched")
}

func TestEnrolledBucketStillThrottlesEveryone(t *testing.T) {
	e := newEnv(t, envOpts{limits: splitLimits(10000, 3)})
	e.worker("ta2-real", goodPIN, false, e.codeA)
	a, b := e.enrolled(fieldidentity.ModePersonal, e.codeA), e.enrolled(fieldidentity.ModePersonal, e.codeA)
	for i := range 3 {
		require.Equal(t, http.StatusUnauthorized, a.login(e.codeA, fmt.Sprintf("nobody-%d", i), "123456").Code)
	}
	assert.Equal(t, 3, e.bucketCount("field_pin_client"))
	assert.Equal(t, 0, e.bucketCount("field_pin_client_unenrolled"))
	assertClientThrottled(t, b.login(e.codeA, "ta2-real", goodPIN), "too many failed sign-ins; try again later")
	// A full enrolled bucket also refuses unenrolled keys (kept as before).
	assertClientThrottled(t, e.newDevice().loginUn(e.codeA, "ta2-real", goodPIN), "too many failed sign-ins; try again later")
}

func TestUnenrolledBucketIsPerClient(t *testing.T) {
	c := newEnv(t, envOpts{limits: splitLimits(3, 10000)})
	d := newEnv(t, envOpts{limits: splitLimits(3, 10000)})
	d.worker("ta3-real", goodPIN, false, d.codeA)
	c.sprayUnenrolled(3)
	mustLoginUn(t, d.newDevice(), d.codeA, "ta3-real", goodPIN)
	assert.Equal(t, 0, d.bucketCount("field_pin_client_unenrolled"))
}

func TestUnenrolledBucketOtherEndpoints(t *testing.T) {
	e := newEnv(t, envOpts{limits: splitLimits(3, 10000)})
	e.worker("ta4-un", goodPIN, false, e.codeA)
	e.worker("ta4-en", goodPIN, false, e.codeA)
	e.supervisor("ta4-sup", goodPIN, e.codeA)
	e.worker("ta4-target", goodPIN, false, e.codeA)
	enrolled := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	supEnrolled := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	unKey := e.newDevice()

	unIn := mustLoginUn(t, unKey, e.codeA, "ta4-un", goodPIN)
	enIn := mustLogin(t, enrolled, e.codeA, "ta4-en", goodPIN)
	sup := mustLogin(t, supEnrolled, e.codeA, "ta4-sup", goodPIN)
	e.sprayUnenrolled(3)

	// PIN change: refused from an unenrolled key, allowed from an enrolled device.
	r := unKey.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "739204"}, unIn.access)
	assertClientThrottled(t, r, "too many failed PIN attempts; try again later")
	r = enrolled.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "739204"}, enIn.access)
	assert.Equal(t, http.StatusNoContent, r.Code, r.Body.String())

	// Act-as from an enrolled device is not refused.
	r = supEnrolled.callBearer("/field/auth/act-as", map[string]any{"workerCode": "ta4-target", "pin": goodPIN}, sup.access)
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())

	// Sign-in-code redemption knows no client from an unenrolled key (no
	// per-client bucket is consulted: unchanged); from an enrolled device it is
	// not refused by the unenrolled bucket.
	principal := e.principal(true)
	id := e.worker("ta4-code", goodPIN, false, e.codeA)
	r = enrolled.call("/field/auth/sign-in-code", map[string]any{"code": e.signInCode(id, e.codeA, principal)})
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())
}
