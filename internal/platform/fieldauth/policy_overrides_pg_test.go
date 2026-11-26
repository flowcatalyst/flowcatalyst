//go:build integration

package fieldauth

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Per-application overrides on the device protocol
// (docs/field-policy-application-overrides.md sections 4 and 7).

func (e *env) override(t *testing.T, appID string, set map[fieldidentity.PolicyMember]any, clear ...fieldidentity.PolicyMember) {
	t.Helper()
	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.PatchPolicyOverride(), fieldidentity.PatchPolicyOverrideCommand{
		ClientID: e.clientID, ApplicationID: appID, Set: set, Clear: clear,
	}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)
}

func (e *env) locks(t *testing.T, members ...fieldidentity.PolicyMember) {
	t.Helper()
	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.SetPolicyLocks(), fieldidentity.SetPolicyLocksCommand{
		ClientID: e.clientID, LockedMembers: members,
	}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)
}

// keyedApps is a worker with a registered key and a key-signed session in
// each of the two applications.
type keyedApps struct {
	u        *unlockEnv
	k        *unlockKey
	inA, inB string // refresh tokens of the key's sessions in A and B
}

func newKeyedApps(t *testing.T) keyedApps {
	t.Helper()
	u, k := registered(t, nil)
	a := k.fullUnlock(u.d, u.workerID, u.e.codeA)
	require.Equal(t, http.StatusOK, a.Code, a.Body.String())
	b := k.fullUnlock(u.d, u.workerID, u.e.codeB)
	require.Equal(t, http.StatusOK, b.Code, b.Body.String())
	return keyedApps{u: u, k: k, inA: a.str("refreshToken"), inB: b.str("refreshToken")}
}

func (x keyedApps) live(t *testing.T, token string) bool {
	t.Helper()
	return x.u.e.q(`SELECT COUNT(*) FROM field_sessions WHERE refresh_hash = $1 AND revoked_at IS NULL`, fieldidentity.HashToken(token)) == 1
}

// The one change to 0.31.0, from both sides.
func TestSwitchingUnlockKeysOffClientWideUnderOverrides(t *testing.T) {
	t.Run("no override: every key revoked and its sessions ended, as in 0.31.0", func(t *testing.T) {
		x := newKeyedApps(t)
		x.u.e.putPolicy(t, fieldidentity.PutPolicyCommand{})
		revoked, reason := x.u.e.keyRevocation(x.k.id)
		assert.True(t, revoked)
		assert.Equal(t, "POLICY_DISABLED", reason)
		assert.False(t, x.live(t, x.inA))
		assert.False(t, x.live(t, x.inB))
	})
	t.Run("A keeps them on: A's session and the key stay, B's key-signed session ends", func(t *testing.T) {
		x := newKeyedApps(t)
		e := x.u.e
		e.override(t, e.appA, map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: true})
		e.putPolicy(t, fieldidentity.PutPolicyCommand{})
		revoked, _ := e.keyRevocation(x.k.id)
		assert.False(t, revoked, "the key serves A")
		assert.True(t, x.live(t, x.inA))
		assert.False(t, x.live(t, x.inB))
		assert.Equal(t, http.StatusOK, x.u.d.refresh(x.inA).Code)
		assert.Equal(t, "INVALID_GRANT", x.u.d.refresh(x.inB).str("error"))

		// The key still signs in to A, not to B.
		assert.Equal(t, http.StatusOK, x.k.fullUnlock(x.u.d, x.u.workerID, e.codeA).Code)
		ch := x.u.d.call("/field/auth/unlock/challenge", map[string]any{"unlockKeyId": x.k.id, "appCode": e.codeB})
		assert.Equal(t, http.StatusForbidden, ch.Code, ch.Body.String())
		assert.Equal(t, "UNLOCK_KEYS_DISABLED", ch.str("error"))
		ch = x.u.d.challenge(x.k.id) // no appCode: some application has them on
		require.Equal(t, http.StatusOK, ch.Code, ch.Body.String())
		r := x.u.d.unlock(x.k.id, e.codeB, x.k.assertion(x.u.d, x.u.workerID, e.codeB, ch.str("challenge"), nil))
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "UNLOCK_KEYS_DISABLED", r.str("error"), "the unlock, which names its application, applies its rule")
		assert.Zero(t, e.failures(x.u.workerID))

		// The last "on" goes: every key revoked, every key-signed session ended.
		e.override(t, e.appA, nil, fieldidentity.MemberUnlockKeysEnabled)
		revoked, reason := e.keyRevocation(x.k.id)
		assert.True(t, revoked)
		assert.Equal(t, "POLICY_DISABLED", reason)
		assert.Zero(t, e.q(`SELECT COUNT(*) FROM field_sessions WHERE unlock_key_id = $1 AND revoked_at IS NULL`, x.k.id))
	})
}

// One application switched off while the client has them on: only its
// key-signed sessions end, the key stays; on again, it signs in there again.
func TestOneApplicationSwitchesUnlockKeysOff(t *testing.T) {
	x := newKeyedApps(t)
	e := x.u.e
	e.override(t, e.appB, map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: false})
	revoked, _ := e.keyRevocation(x.k.id)
	assert.False(t, revoked)
	assert.True(t, x.live(t, x.inA))
	assert.False(t, x.live(t, x.inB))
	si := mustLogin(t, x.u.d, e.codeB, "uk-1", goodPIN)
	assert.False(t, unlockKeysOf(t, si.body).allowed, "B's token answers say off")
	si = mustLogin(t, x.u.d, e.codeA, "uk-1", goodPIN)
	assert.True(t, unlockKeysOf(t, si.body).allowed)

	e.override(t, e.appB, nil, fieldidentity.MemberUnlockKeysEnabled)
	assert.Equal(t, http.StatusOK, x.k.fullUnlock(x.u.d, x.u.workerID, e.codeB).Code)
}

// Client-wide off, one application on: a worker registers a key in that
// application's session, and it signs in there only.
func TestAnApplicationTurnsUnlockKeysOnForItself(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("ov-1", goodPIN, false, e.codeA, e.codeB)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA, e.codeB)
	e.override(t, e.appA, map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: true})

	inB := mustLogin(t, d, e.codeB, "ov-1", goodPIN)
	assert.False(t, unlockKeysOf(t, inB.body).allowed)
	k := newUnlockKey(t)
	r := k.register(d, inB, nil, nil)
	assert.Equal(t, "UNLOCK_KEYS_DISABLED", r.str("error"), "B's session cannot register")

	inA := mustLogin(t, d, e.codeA, "ov-1", goodPIN)
	assert.True(t, unlockKeysOf(t, inA.body).allowed)
	r = k.register(d, inA, nil, nil)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	assert.Equal(t, http.StatusOK, k.fullUnlock(d, workerID, e.codeA).Code)
}

// A lock keeps an override from applying, with its effect on keys and
// sessions at each change; unlocking restores the override.
func TestLockingAnOverriddenMember(t *testing.T) {
	t.Run("unlockKeysEnabled: locking the only 'on' revokes the keys", func(t *testing.T) {
		x := newKeyedApps(t)
		e := x.u.e
		e.override(t, e.appA, map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: true})
		e.putPolicy(t, fieldidentity.PutPolicyCommand{}) // client off: A keeps the key
		revoked, _ := e.keyRevocation(x.k.id)
		require.False(t, revoked)
		e.locks(t, fieldidentity.MemberUnlockKeysEnabled)
		revoked, reason := e.keyRevocation(x.k.id)
		assert.True(t, revoked, "with A's override overruled no application has them on")
		assert.Equal(t, "POLICY_DISABLED", reason)
		assert.False(t, x.live(t, x.inA))
		e.locks(t)
		si := mustLogin(t, x.u.d, e.codeA, "uk-1", goodPIN)
		assert.True(t, unlockKeysOf(t, si.body).allowed, "unlocked, A's kept override applies again (a new key is registered)")
	})
	t.Run("unlockKeysEnabled: locking an 'off' override lets its sessions be; unlocking ends them", func(t *testing.T) {
		x := newKeyedApps(t)
		e := x.u.e
		e.override(t, e.appB, map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: false})
		require.False(t, x.live(t, x.inB))
		b := x.k.fullUnlock(x.u.d, x.u.workerID, e.codeB)
		assert.Equal(t, http.StatusForbidden, b.Code)
		e.locks(t, fieldidentity.MemberUnlockKeysEnabled) // B follows the client again: on
		b = x.k.fullUnlock(x.u.d, x.u.workerID, e.codeB)
		require.Equal(t, http.StatusOK, b.Code, b.Body.String())
		e.locks(t) // B's off applies again: its key-signed session ends
		assert.False(t, x.live(t, b.str("refreshToken")))
		assert.True(t, x.live(t, x.inA))
		revoked, _ := e.keyRevocation(x.k.id)
		assert.False(t, revoked)
	})
}

// The other overridable members on the protocol: an enrolled device only for
// one application, and its own idle limit.
func TestOverridesOfSignInRulesAndLimits(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.worker("ov-2", goodPIN, false, e.codeA, e.codeB)
	e.override(t, e.appA, map[fieldidentity.PolicyMember]any{
		fieldidentity.MemberRequireEnrolledDevice: true, fieldidentity.MemberUnenrolledIdleSecs: 120,
	})
	e.override(t, e.appB, map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnenrolledIdleSecs: 300})
	un := e.newDevice()
	assert.Equal(t, "UNKNOWN_DEVICE", un.loginUn(e.codeA, "ov-2", goodPIN).str("error"), "A needs an enrolled device")
	si := mustLoginUn(t, un, e.codeB, "ov-2", goodPIN)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE refresh_hash = $1
		AND idle_expires_at BETWEEN issued_at + interval '299 seconds' AND issued_at + interval '301 seconds'`,
		fieldidentity.HashToken(si.refresh)), "B's own idle limit")
}
