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
)

// An act-as never holds another worker's row while the supervisor's PIN is
// hashed, and never locks a worker of another client at all
// (fieldidentity lock_order.go).

// sendAsync runs the request in the background.
func sendAsync(f func() resp) <-chan resp {
	out := make(chan resp, 1)
	go func() { out <- f() }()
	return out
}

func waitResp(t *testing.T, ch <-chan resp) resp {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("the act-as did not finish")
		return resp{}
	}
}

// A worker id of another client is refused as before (403 NO_APP_ACCESS,
// TARGET_OTHER_CLIENT recorded) without its row ever being locked: the
// request finishes while that row is held, and, held at the supervisor, the
// request has not locked it either.
func TestActAsNeverLocksAWorkerOfAnotherClient(t *testing.T) {
	a := newActAsEnv(t, roomyLimits())
	other := newEnv(t, envOpts{limits: roomyLimits()})
	foreign := other.worker("tgt-x", goodPIN, false, other.codeA)

	held := holdWorkers(t, foreign)
	r := waitResp(t, sendAsync(func() resp { return a.actAs(map[string]any{"workerId": foreign, "pin": goodPIN}) }))
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "NO_APP_ACCESS", r.str("error"))
	held.release()

	sup := holdWorkers(t, a.supID)
	ch := sendAsync(func() resp { return a.actAs(map[string]any{"workerId": foreign, "pin": goodPIN}) })
	time.Sleep(300 * time.Millisecond)
	assert.True(t, sup.free(`SELECT 1 FROM field_identities WHERE id = $1`, foreign), "the foreign worker was locked")
	sup.release()
	r = waitResp(t, ch)
	assert.Equal(t, "NO_APP_ACCESS", r.str("error"))
	assert.Equal(t, 2, a.e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE auth_method = 'ACT_AS' AND failure_reason = 'TARGET_OTHER_CLIENT' AND actor_id = $1`, a.supID))
	assert.Zero(t, a.e.failures(a.supID), "a right PIN counts nothing")
}

// A wrong PIN is answered and counted without the target's row: the request
// finishes while the target is held.
func TestActAsWithAWrongPINNeverLocksTheTarget(t *testing.T) {
	a := newActAsEnv(t, roomyLimits())
	held := holdWorkers(t, a.targetID)
	r := waitResp(t, sendAsync(func() resp { return a.actAs(map[string]any{"workerCode": "tgt-1", "pin": "000999"}) }))
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
	held.release()
	assert.Equal(t, 1, a.e.failures(a.supID), "the wrong PIN is counted")

	// A right PIN does wait for the target (both are locked, in id order,
	// after the PIN), and then succeeds.
	held = holdWorkers(t, a.targetID)
	ch := sendAsync(func() resp { return a.actAs(map[string]any{"workerCode": "tgt-1", "pin": goodPIN}) })
	select {
	case r := <-ch:
		t.Fatalf("the act-as did not wait for the target: %d %s", r.Code, r.Body.String())
	case <-time.After(300 * time.Millisecond):
	}
	held.release()
	r = waitResp(t, ch)
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Zero(t, a.e.failures(a.supID), "the right PIN reset the counter")
}

// What the PIN check decided is checked again under both locks: a supervisor
// whose PIN changed, or who lost the power to act, while the act-as waited for
// the target is refused.
func TestActAsRechecksTheSupervisorUnderBothLocks(t *testing.T) {
	for name, change := range map[string]string{
		"PIN changed":       `UPDATE field_identities SET pin_hash = pin_hash || 'x' WHERE id = $1`,
		"no longer may act": `UPDATE field_identities SET can_impersonate = FALSE WHERE id = $1`,
		"suspended":         `UPDATE field_identities SET status = 'DISABLED' WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, envOpts{limits: roomyLimits()})
			// The target is made first, so its id sorts first: the second
			// transaction waits for it before it locks the supervisor.
			targetID := e.worker("tgt-r", goodPIN, false, e.codeA)
			supID := e.supervisor("sup-r", goodPIN, e.codeA)
			require.Less(t, targetID, supID)
			d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
			sup := mustLogin(t, d, e.codeA, "sup-r", goodPIN)

			held := holdWorkers(t, targetID)
			ch := sendAsync(func() resp {
				return d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "tgt-r", "pin": goodPIN}, sup.access)
			})
			// Wait until the act-as is blocked on a row lock: its PIN has
			// verified and its second transaction waits for the target.
			require.Eventually(t, func() bool {
				return e.q(`SELECT COUNT(*) FROM pg_locks WHERE NOT granted`) > 0
			}, 10*time.Second, 10*time.Millisecond)
			_, err := held.tx.Exec(context.Background(), change, supID)
			require.NoError(t, err)
			held.release()
			r := waitResp(t, ch)
			assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
			assert.Equal(t, "IMPERSONATION_FORBIDDEN", r.str("error"))
			assert.Zero(t, e.q(`SELECT COUNT(*) FROM field_sessions WHERE actor_id = $1`, supID))
		})
	}
}
