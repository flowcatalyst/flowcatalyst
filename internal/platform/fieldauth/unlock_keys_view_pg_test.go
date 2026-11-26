//go:build integration

package fieldauth

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
)

// Every answer that carries a session says what the session may do with
// unlock keys (docs/field-unlock-keys-spec.md section 2.3): the library reads
// it instead of probing the registration route.

type unlockKeysAnswer struct {
	allowed, attestationRequired bool
	registerExpiresIn            float64 // 0 when absent
	present                      bool    // registerExpiresIn present
}

func unlockKeysOf(t *testing.T, body map[string]any) unlockKeysAnswer {
	t.Helper()
	raw, ok := body["unlockKeys"].(map[string]any)
	require.True(t, ok, "the answer carries unlockKeys: %v", body)
	out := unlockKeysAnswer{allowed: raw["allowed"].(bool), attestationRequired: raw["attestationRequired"].(bool)}
	if v, ok := raw["registerExpiresIn"]; ok {
		out.present, out.registerExpiresIn = true, v.(float64)
	}
	return out
}

func TestTokenAnswersSayWhetherUnlockKeysAreAllowed(t *testing.T) {
	t.Run("policy off, PERSONAL device", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		e.worker("uv-1", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		got := unlockKeysOf(t, mustLogin(t, d, e.codeA, "uv-1", goodPIN).body)
		assert.Equal(t, unlockKeysAnswer{}, got)
	})
	t.Run("policy on, PERSONAL device, just signed in with the PIN", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		got := unlockKeysOf(t, u.si.body)
		assert.True(t, got.allowed)
		assert.False(t, got.attestationRequired)
		require.True(t, got.present)
		assert.Greater(t, got.registerExpiresIn, float64(590))
		assert.LessOrEqual(t, got.registerExpiresIn, float64(600))
	})
	t.Run("REQUIRED", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		u.e.policy("unlock_key_attestation", "REQUIRED")
		got := unlockKeysOf(t, u.d.refresh(u.si.refresh).json())
		assert.True(t, got.allowed)
		assert.True(t, got.attestationRequired)
	})
	t.Run("SHARED device", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		e.policy("unlock_keys_enabled", true)
		e.policy("unlock_key_attestation", "REQUIRED")
		e.worker("uv-2", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModeShared, e.codeA)
		assert.Equal(t, unlockKeysAnswer{}, unlockKeysOf(t, mustLogin(t, d, e.codeA, "uv-2", goodPIN).body))
	})
	t.Run("unenrolled key", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		e.policy("unlock_keys_enabled", true)
		e.worker("uv-3", goodPIN, false, e.codeA)
		d := e.newDevice()
		assert.Equal(t, unlockKeysAnswer{}, unlockKeysOf(t, mustLoginUn(t, d, e.codeA, "uv-3", goodPIN).body))
	})
	t.Run("act-as", func(t *testing.T) {
		a := newActAsEnv(t, roomyLimits())
		a.e.policy("unlock_keys_enabled", true)
		r := a.actAs(map[string]any{"workerCode": "tgt-1", "pin": goodPIN})
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.Equal(t, unlockKeysAnswer{}, unlockKeysOf(t, r.json()))
		// The supervisor's own session on the same PERSONAL device is allowed.
		assert.True(t, unlockKeysOf(t, a.d.refresh(a.sup.refresh).json()).allowed)
	})
	t.Run("console sign-in code", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		code := u.e.signInCode(u.workerID, u.e.codeA, u.e.principal(true))
		r := u.d.call("/field/auth/sign-in-code", map[string]any{"code": code})
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.Equal(t, unlockKeysAnswer{}, unlockKeysOf(t, r.json()))
	})
	t.Run("a policy change shows at the next refresh", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		require.True(t, unlockKeysOf(t, u.si.body).allowed)
		u.e.policy("unlock_keys_enabled", false)
		r := u.d.refresh(u.si.refresh)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.Equal(t, unlockKeysAnswer{}, unlockKeysOf(t, r.json()))
		u.e.policy("unlock_keys_enabled", true)
		r = u.d.refresh(r.str("refreshToken"))
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.True(t, unlockKeysOf(t, r.json()).allowed)
	})
	t.Run("a session the unlock began cannot register", func(t *testing.T) {
		u, k := registered(t, nil)
		r := k.fullUnlock(u.d, u.workerID, u.e.codeA)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		got := unlockKeysOf(t, r.json())
		assert.True(t, got.allowed)
		assert.False(t, got.present)
	})
	t.Run("the registration window passes", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		u.e.exec(`UPDATE field_sessions SET auth_time = auth_time - interval '9 minutes 30 seconds' WHERE identity_id = $1`, u.workerID)
		r := u.d.refresh(u.si.refresh)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		got := unlockKeysOf(t, r.json())
		require.True(t, got.present)
		assert.LessOrEqual(t, got.registerExpiresIn, float64(30), "the refresh keeps the sign-in's deadline")
		u.e.exec(`UPDATE field_sessions SET auth_time = auth_time - interval '1 minute' WHERE identity_id = $1`, u.workerID)
		r = u.d.refresh(r.str("refreshToken"))
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		got = unlockKeysOf(t, r.json())
		assert.True(t, got.allowed)
		assert.False(t, got.present)
	})
	t.Run("a pending PIN change", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		e.policy("unlock_keys_enabled", true)
		e.worker("uv-4", goodPIN, true, e.codeA)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		got := unlockKeysOf(t, mustLogin(t, d, e.codeA, "uv-4", goodPIN).body)
		assert.True(t, got.allowed)
		assert.False(t, got.present)
	})
}

// A registration with an empty body, which nothing legitimate sends now that
// libraries read unlockKeys, never counts against the worker: with the policy
// off it is refused 403 and recorded as an attempt (the sign-in history shows
// it); with the policy on it is 400 and recorded as nothing. Neither touches
// the worker's failure counter or the failure buckets.
func TestAnEmptyRegistrationNeverCountsTowardsTheLockout(t *testing.T) {
	for _, on := range []bool{false, true} {
		e := newEnv(t, envOpts{})
		e.policy("unlock_keys_enabled", on)
		workerID := e.worker("uv-5", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		si := mustLogin(t, d, e.codeA, "uv-5", goodPIN)
		for range 15 { // more than the default failure budgets
			r := d.callBearer("/field/auth/unlock-keys", map[string]any{}, si.access)
			if on {
				assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
				assert.Equal(t, "INVALID_REQUEST", r.str("error"))
			} else {
				assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
				assert.Equal(t, "UNLOCK_KEYS_DISABLED", r.str("error"))
			}
		}
		want := 0
		if !on {
			want = 15
		}
		assert.Equal(t, want, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND auth_method = 'UNLOCK_KEY_REGISTER'`, workerID))
		assert.Zero(t, e.failures(workerID))
		r := d.login(e.codeA, "uv-5", goodPIN)
		assert.Equal(t, http.StatusOK, r.Code, "the worker and the device are not throttled: %s", r.Body.String())
	}
}
