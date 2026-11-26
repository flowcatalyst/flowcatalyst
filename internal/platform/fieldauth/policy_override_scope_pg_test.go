//go:build integration

package fieldauth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// An application's override applies only to a worker who holds the
// application, of an application active and enabled for the client, and is
// decided before any credential is checked
// (docs/field-policy-application-overrides.md section 1). The review's
// reproduction: application B loosens the enrolled-device rule, and a worker
// who holds only A is attacked from an unenrolled key naming B.
func TestAnOverrideNeverReachesAWorkerOutsideTheApplication(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.policy("require_enrolled_device", true)
	w := e.worker("RV-1", goodPIN, false, e.codeA) // holds A only
	e.worker("RV-B", goodPIN, false, e.codeB)      // holds B
	d := e.newDevice()
	e.override(t, e.appB, map[fieldidentity.PolicyMember]any{fieldidentity.MemberRequireEnrolledDevice: false})

	for range 12 {
		r := d.loginUn(e.codeB, "RV-1", "000000")
		require.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		require.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
	}
	assert.Zero(t, e.failures(w), "nothing counted against a worker the override does not apply to")
	assert.Zero(t, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND (hard_locked OR locked_until IS NOT NULL)`, w))
	assert.Equal(t, 12, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND failure_reason = 'OVERRIDE_NOT_APPLICABLE'`, w))

	// No PIN oracle: the right PIN through B answers as a wrong one.
	r := d.loginUn(e.codeB, "RV-1", goodPIN)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))

	// The worker's own application, on an enrolled device, is untouched.
	dev := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	assert.Equal(t, http.StatusOK, dev.login(e.codeA, "RV-1", goodPIN).Code)
	// And the override does what it is for, for B's own workers.
	assert.Equal(t, http.StatusOK, d.loginUn(e.codeB, "RV-B", goodPIN).Code)
}

// An override of an application that is inactive, or not enabled for the
// client, applies nowhere.
func TestAnOverrideOfAnApplicationThatDoesNotServeTheClientDoesNotApply(t *testing.T) {
	for name, change := range map[string]string{
		"inactive":    `UPDATE app_applications SET active = FALSE WHERE id = $1`,
		"not enabled": `UPDATE app_client_configs SET enabled = FALSE WHERE application_id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, envOpts{limits: roomyLimits()})
			e.policy("require_enrolled_device", true)
			e.worker("RV-2", goodPIN, false, e.codeB)
			e.override(t, e.appB, map[fieldidentity.PolicyMember]any{fieldidentity.MemberRequireEnrolledDevice: false})
			d := e.newDevice()
			require.Equal(t, http.StatusOK, d.loginUn(e.codeB, "RV-2", goodPIN).Code)
			e.exec(change, e.appB)
			t.Cleanup(func() { e.exec(`UPDATE app_applications SET active = TRUE WHERE id = $1`, e.appB) })
			assert.Equal(t, "UNKNOWN_DEVICE", d.loginUn(e.codeB, "RV-2", goodPIN).str("error"))
		})
	}
}

// A badge alone (badge-only sign-in) never resets the PIN's failure counter,
// whether the client allows it or an application's override does; and the
// override's badge-only applies only to a worker who holds the application.
func TestBadgeOnlySignInKeepsThePINFailureCounter(t *testing.T) {
	for _, viaOverride := range []bool{false, true} {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		w := e.worker("BO-1", goodPIN, false, e.codeA)
		other := e.worker("BO-2", goodPIN, false, e.codeB)
		if viaOverride {
			e.override(t, e.appA, map[fieldidentity.PolicyMember]any{fieldidentity.MemberBadgeOnlyLogin: true})
		} else {
			e.policy("badge_only_login", true)
		}
		d := e.enrolled(fieldidentity.ModeShared, e.codeA, e.codeB)
		for range 2 {
			require.Equal(t, http.StatusUnauthorized, d.login(e.codeA, "BO-1", "000000").Code)
		}
		require.Equal(t, 2, e.failures(w))
		r := d.loginBadge(e.codeA, e.badge(w), "")
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.Equal(t, 2, e.failures(w), "the badge alone verified no PIN (override %v)", viaOverride)
		r = d.login(e.codeA, "BO-1", goodPIN)
		require.Equal(t, http.StatusOK, r.Code)
		assert.Zero(t, e.failures(w), "a PIN sign-in resets it")
		if viaOverride {
			// A worker who does not hold A gets the client's rule: a PIN.
			r = d.loginBadge(e.codeA, e.badge(other), "")
			assert.Equal(t, "PIN_REQUIRED", r.str("error"), r.Body.String())
		}
	}
}

// An override change racing an unlock: the unlock decides under the worker's
// lock, so it either ran before (and the change ends its session) or after
// (and is refused); a key-signed session of an application that is now off
// never survives.
func TestAnOverrideChangeRacingAnUnlock(t *testing.T) {
	u, k := registered(t, nil)
	e := u.e
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	assertion := k.assertion(u.d, u.workerID, e.codeA, ch.str("challenge"), nil)

	held := holdWorkers(t, u.workerID)
	patched := make(chan error, 1)
	go func() {
		_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.PatchPolicyOverride(), fieldidentity.PatchPolicyOverrideCommand{
			ClientID: e.clientID, ApplicationID: e.appA, Set: map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: false},
		}, auth.NewExecutionContext(e.actx))
		patched <- err
	}()
	waitForLockWaiters(t, e, 1) // the change waits for the worker
	unlocked := sendAsync(func() resp { return u.d.unlock(k.id, e.codeA, assertion) })
	waitForLockWaiters(t, e, 2) // and the unlock behind it
	held.release()
	require.NoError(t, <-patched)
	r := waitResp(t, unlocked)
	assert.Equal(t, "UNLOCK_KEYS_DISABLED", r.str("error"), "the unlock read the policy again under the lock: %s", r.Body.String())
	assert.Zero(t, e.q(`SELECT COUNT(*) FROM field_sessions WHERE unlock_key_id = $1 AND application_id = $2 AND revoked_at IS NULL`, k.id, e.appA))
}

// An override change racing a registration: both take the client's policy
// lock, so the registration reads the policy the change left.
func TestAnOverrideChangeRacingARegistration(t *testing.T) {
	u := newUnlockEnv(t, nil)
	e := u.e
	k := newUnlockKey(t)
	held, err := testpg.Pool(t).Begin(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Rollback(context.Background()) })
	_, err = held.Exec(context.Background(), `SELECT pg_advisory_xact_lock(hashtext('field-policy:' || $1::text))`, e.clientID)
	require.NoError(t, err)
	patched := make(chan error, 1)
	go func() {
		_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.PatchPolicyOverride(), fieldidentity.PatchPolicyOverrideCommand{
			ClientID: e.clientID, ApplicationID: e.appA, Set: map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: false},
		}, auth.NewExecutionContext(e.actx))
		patched <- err
	}()
	waitForLockWaiters(t, e, 1) // the change waits for the policy lock
	registered := sendAsync(func() resp { return k.register(u.d, u.si, nil, nil) })
	waitForLockWaiters(t, e, 2) // and the registration behind it
	require.NoError(t, held.Commit(context.Background()))
	require.NoError(t, <-patched)
	r := waitResp(t, registered)
	assert.Equal(t, "UNLOCK_KEYS_DISABLED", r.str("error"), r.Body.String())
	assert.Zero(t, e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE identity_id = $1`, u.workerID))
}

// An override of an application that no longer serves the client keeps no key
// alive: its key-signed sessions stop refreshing, the challenge without appCode
// is refused, and the next change of where unlock keys are on revokes the keys.
func TestAnOverrideOfAnInactiveApplicationKeepsNoKeyAlive(t *testing.T) {
	x := newKeyedApps(t)
	e := x.u.e
	e.override(t, e.appA, map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: true})
	e.putPolicy(t, fieldidentity.PutPolicyCommand{}) // client off: A keeps the key
	revoked, _ := e.keyRevocation(x.k.id)
	require.False(t, revoked)
	e.exec(`UPDATE app_applications SET active = FALSE WHERE id = $1`, e.appA)
	t.Cleanup(func() { e.exec(`UPDATE app_applications SET active = TRUE WHERE id = $1`, e.appA) })
	assert.Equal(t, "UNLOCK_KEYS_DISABLED", x.u.d.challenge(x.k.id).str("error"))
	assert.Equal(t, "INVALID_GRANT", x.u.d.refresh(x.inA).str("error"))
	e.locks(t, fieldidentity.MemberUnlockKeysEnabled)
	revoked, reason := e.keyRevocation(x.k.id)
	assert.True(t, revoked, "the inactive application's override counts nowhere")
	assert.Equal(t, "POLICY_DISABLED", reason)
}

// waitForLockWaiters waits until n transactions wait on a lock.
func waitForLockWaiters(t *testing.T, e *env, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var waiting int
		_ = testpg.Pool(t).QueryRow(context.Background(), `SELECT COUNT(DISTINCT pid) FROM pg_locks WHERE NOT granted`).Scan(&waiting)
		return waiting >= n
	}, 10*time.Second, 10*time.Millisecond)
	_ = e
}
