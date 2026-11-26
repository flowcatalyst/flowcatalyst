//go:build integration

package fieldauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The lock order of a worker's credentials (fieldidentity lock_order.go): the
// worker rows first, then the device, keys and sessions. These tests are the
// review's two reproductions (a refresh escaping a key's revocation; a
// suspension deadlocking with an administrator's revoke) made into tests of
// the real operations.

// unlocked is a registered key with a session it signed in.
type unlocked struct {
	u       *unlockEnv
	k       *unlockKey
	refresh string // the key's session's refresh token
}

func newUnlocked(t *testing.T) unlocked {
	t.Helper()
	u, k := registered(t, nil)
	r := k.fullUnlock(u.d, u.workerID, u.e.codeA)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	return unlocked{u: u, k: k, refresh: r.str("refreshToken")}
}

// liveKeySessions is how many live sessions the key signed in.
func (e *env) liveKeySessions(keyID string) int {
	return e.q(`SELECT COUNT(*) FROM field_sessions WHERE unlock_key_id = $1 AND revoked_at IS NULL`, keyID)
}

// The review's race: a refresh holding the parent session while a key's
// revocation runs inserted a child the revocation never saw. The refresh takes
// the worker's row first, as every revocation now does, so the revocation
// waits for the refresh and then ends the child too.
func TestRefreshOverlappingAKeyRevocationDoesNotOutliveIt(t *testing.T) {
	x := newUnlocked(t)
	e := x.u.e
	ctx := context.Background()

	// The first half of a refresh, through the protocol's own transaction:
	// the worker, then the session.
	tx, err := e.s.Ops.R.Sessions.BeginAuth(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	ident, err := tx.LockIdentityByID(ctx, x.u.workerID)
	require.NoError(t, err)
	require.NotNil(t, ident)
	parent, err := tx.LockSessionByHash(ctx, fieldidentity.HashToken(x.refresh))
	require.NoError(t, err)
	require.NotNil(t, parent)

	done := make(chan error, 1)
	go func() {
		_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.RevokeUnlockKey(), fieldidentity.UnlockKeyRef{
			ClientID: e.clientID, ID: x.u.workerID, UnlockKeyID: x.k.id,
		}, auth.NewExecutionContext(e.actx))
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("the revocation did not wait for the refresh holding the worker (err %v)", err)
	case <-time.After(300 * time.Millisecond):
	}

	// The second half: rotate and insert the child, as refresh does.
	now := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, tx.Rotate(ctx, parent.ID, now))
	child := *parent
	child.ID, child.RefreshHash = fieldidentity.NewSessionID(), fieldidentity.HashToken("fwr_child-"+parent.ID)
	child.IssuedAt, child.RevokedAt, child.RevokedReason, child.RotatedAt = now, nil, "", nil
	require.NoError(t, tx.InsertSession(ctx, &child))
	require.NoError(t, tx.Commit(ctx))

	require.NoError(t, <-done)
	assert.Zero(t, e.liveKeySessions(x.k.id), "the child the refresh made ended with the key")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE id = $1 AND revoked_reason = 'UNLOCK_KEY_REVOKED'`, child.ID))
}

// A refresh checks, under the worker's lock, that the session's key may still
// keep it going, also when nothing ended the session.
func TestRefreshChecksTheSessionsUnlockKey(t *testing.T) {
	cases := []struct {
		name   string
		change func(x unlocked)
		ok     bool
	}{
		{"live key", func(unlocked) {}, true},
		{"revoked by an administrator", func(x unlocked) {
			x.u.e.exec(`UPDATE field_unlock_keys SET revoked_at = NOW(), revoked_reason = 'ADMIN' WHERE id = $1`, x.k.id)
		}, false},
		{"revoked by the status list", func(x unlocked) {
			x.u.e.exec(`UPDATE field_unlock_keys SET revoked_at = NOW(), revoked_reason = 'ATTESTATION_REVOKED' WHERE id = $1`, x.k.id)
		}, false},
		{"revoked by the phone (keeps its sessions)", func(x unlocked) {
			x.u.e.exec(`UPDATE field_unlock_keys SET revoked_at = NOW(), revoked_reason = 'DEVICE' WHERE id = $1`, x.k.id)
		}, true},
		{"replaced (keeps its sessions)", func(x unlocked) {
			x.u.e.exec(`UPDATE field_unlock_keys SET revoked_at = NOW(), revoked_reason = 'REPLACED' WHERE id = $1`, x.k.id)
		}, true},
		{"deleted", func(x unlocked) {
			x.u.e.exec(`DELETE FROM field_unlock_keys WHERE id = $1`, x.k.id)
		}, false},
		{"unlock keys switched off", func(x unlocked) { x.u.e.policy("unlock_keys_enabled", false) }, false},
		{"REQUIRED and the key was not verified", func(x unlocked) { x.u.e.policy("unlock_key_attestation", "REQUIRED") }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := newUnlocked(t)
			tc.change(x)
			r := x.u.d.refresh(x.refresh)
			if tc.ok {
				assert.Equal(t, http.StatusOK, r.Code, r.Body.String())
			} else {
				assert.Equal(t, "INVALID_GRANT", r.str("error"), r.Body.String())
			}
		})
	}
}

// heldWorker holds a worker's row as another transaction would, and probes
// whether the operation waiting on it has locked any of the worker's keys or
// sessions already (it must not have: the worker comes first).
type heldWorker struct {
	t  *testing.T
	tx pgx.Tx
}

func holdWorkers(t *testing.T, ids ...string) *heldWorker {
	t.Helper()
	tx, err := testpg.Pool(t).Begin(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	_, err = tx.Exec(context.Background(), `SELECT 1 FROM field_identities WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids)
	require.NoError(t, err)
	return &heldWorker{t: t, tx: tx}
}

// free reports whether every row the query selects can be locked at once.
func (h *heldWorker) free(sql string, args ...any) bool {
	h.t.Helper()
	ctx := context.Background()
	_, err := h.tx.Exec(ctx, `SAVEPOINT probe`)
	require.NoError(h.t, err)
	rows, err := h.tx.Query(ctx, sql+` FOR UPDATE NOWAIT`, args...)
	if err == nil {
		for rows.Next() {
		}
		err = rows.Err()
		rows.Close()
	}
	if err != nil {
		var pgErr *pgconn.PgError
		require.True(h.t, errors.As(err, &pgErr) && pgErr.Code == "55P03", "probe: %v", err)
		_, rerr := h.tx.Exec(ctx, `ROLLBACK TO SAVEPOINT probe`)
		require.NoError(h.t, rerr)
		return false
	}
	_, err = h.tx.Exec(ctx, `RELEASE SAVEPOINT probe`)
	require.NoError(h.t, err)
	return true
}

func (h *heldWorker) release() { require.NoError(h.t, h.tx.Commit(context.Background())) }

// Every operation that changes a worker's keys or sessions takes the worker's
// row before any key or session row: held at the worker, it has locked
// neither, and it completes once the worker is released. One order for all is
// what keeps the session-first operations (suspend, PIN reset, PIN change,
// device revoke) and the key-first ones (an administrator's revoke, the
// policy's and the status list's) from deadlocking.
func TestEveryCredentialChangeTakesTheWorkerFirst(t *testing.T) {
	// Each entry prepares what needs no lock, then returns the operation.
	type op func(t *testing.T, x unlocked) func() error
	ops := map[string]op{
		"suspend": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.SuspendWorker(),
					fieldidentity.WorkerRef{ClientID: x.u.e.clientID, ID: x.u.workerID}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"PIN reset": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.ResetWorkerPin(),
					fieldidentity.ResetPinCommand{ClientID: x.u.e.clientID, ID: x.u.workerID},
					auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"PIN change": func(t *testing.T, x unlocked) func() error {
			return func() error {
				fam := tokenPayload(t, x.u.si.access)["sid"].(string)
				_, err := usecaseop.RunTx(context.Background(), x.u.e.s.UoW, x.u.e.s.Ops.ChangeOwnPin(),
					fieldidentity.ChangeOwnPinCommand{WorkerID: x.u.workerID, FamilyID: fam, DeviceID: x.u.d.id, NewPIN: "730418"},
					auth.NewExecutionContext(context.Background()))
				return err
			}
		},
		"device revoke": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.RevokeDevice(),
					fieldidentity.DeviceRef{ClientID: x.u.e.clientID, ID: x.u.d.id}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"administrator's key revoke": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.RevokeUnlockKey(), fieldidentity.UnlockKeyRef{
					ClientID: x.u.e.clientID, ID: x.u.workerID, UnlockKeyID: x.k.id,
				}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"status list revoke": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(context.Background(), x.u.e.s.UoW, x.u.e.s.Ops.RevokeUnlockKeyForAttestation(),
					fieldidentity.RevokeForAttestationCommand{UnlockKeyID: x.k.id, Serial: "2002"},
					auth.NewExecutionContext(context.Background()))
				return err
			}
		},
		"the phone's own revoke": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(context.Background(), x.u.e.s.UoW, x.u.e.s.Ops.RevokeOwnUnlockKey(),
					fieldidentity.RevokeOwnUnlockKeyCommand{DeviceID: x.u.d.id, UnlockKeyID: x.k.id},
					auth.NewExecutionContext(context.Background()))
				return err
			}
		},
		"policy switched off": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.PutPolicy(),
					fieldidentity.PutPolicyCommand{ClientID: x.u.e.clientID}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"refresh reuse revokes the family": func(t *testing.T, x unlocked) func() error {
			return func() error {
				fam := tokenPayload(t, x.u.si.access)["sid"].(string)
				_, err := usecaseop.RunTx(context.Background(), x.u.e.s.UoW, x.u.e.s.Ops.RevokeSessionFamilyOnReuse(),
					fieldidentity.RevokeFamilyCommand{ClientID: x.u.e.clientID, WorkerID: x.u.workerID, DeviceID: x.u.d.id, FamilyID: fam},
					auth.NewExecutionContext(context.Background()))
				return err
			}
		},
		"refresh": func(t *testing.T, x unlocked) func() error {
			return func() error {
				if r := x.u.d.refresh(x.refresh); r.Code != http.StatusOK {
					return fmt.Errorf("refresh: %d %s", r.Code, r.Body.String())
				}
				return nil
			}
		},
		"sign-out of the device": func(t *testing.T, x unlocked) func() error {
			return func() error {
				if r := x.u.d.call("/field/auth/logout", map[string]any{"refreshToken": x.refresh, "scope": "device-all"}); r.Code != http.StatusNoContent {
					return fmt.Errorf("logout: %d %s", r.Code, r.Body.String())
				}
				return nil
			}
		},
		"act-as (the target)": func(t *testing.T, x unlocked) func() error {
			x.u.e.supervisor("ord-sup", goodPIN, x.u.e.codeA)
			d := x.u.e.enrolled(fieldidentity.ModePersonal, x.u.e.codeA)
			sup := mustLogin(t, d, x.u.e.codeA, "ord-sup", goodPIN)
			return func() error {
				if r := d.callBearer("/field/auth/act-as", map[string]any{"workerId": x.u.workerID, "pin": goodPIN}, sup.access); r.Code != http.StatusOK {
					return fmt.Errorf("act-as: %d %s", r.Code, r.Body.String())
				}
				return nil
			}
		},
		"deletion": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.DeleteWorker(),
					fieldidentity.WorkerRef{ClientID: x.u.e.clientID, ID: x.u.workerID}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"update (no longer a test identity, no longer acts)": func(t *testing.T, x unlocked) func() error {
			// A test identity with a live sign-in code: unmarking expires it.
			x.u.e.exec(`UPDATE field_identities SET is_test = TRUE, can_impersonate = TRUE WHERE id = $1`, x.u.workerID)
			x.u.e.signInCode(x.u.workerID, x.u.e.codeA, x.u.e.principal(true))
			off := false
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.UpdateWorker(), fieldidentity.UpdateWorkerCommand{
					ClientID: x.u.e.clientID, ID: x.u.workerID, Name: "Renamed",
					CanImpersonate: &off, IsTest: &off,
				}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"application access removed": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.RevokeWorkerApp(), fieldidentity.WorkerAppCommand{
					ClientID: x.u.e.clientID, ID: x.u.workerID, AppCode: x.u.e.codeA,
				}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"key registration": func(t *testing.T, x unlocked) func() error {
			k2 := newUnlockKey(t)
			return func() error {
				if r := k2.register(x.u.d, x.u.si, nil, nil); r.Code != http.StatusCreated {
					return fmt.Errorf("register: %d %s", r.Code, r.Body.String())
				}
				return nil
			}
		},
		"console sign-in code redeemed": func(t *testing.T, x unlocked) func() error {
			code := x.u.e.signInCode(x.u.workerID, x.u.e.codeA, x.u.e.principal(true))
			d := x.u.e.newDevice()
			return func() error {
				if r := d.call("/field/auth/sign-in-code", map[string]any{"code": code}); r.Code != http.StatusOK {
					return fmt.Errorf("sign-in code: %d %s", r.Code, r.Body.String())
				}
				return nil
			}
		},
		"an application's override switching unlock keys off": func(t *testing.T, x unlocked) func() error {
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.PatchPolicyOverride(), fieldidentity.PatchPolicyOverrideCommand{
					ClientID: x.u.e.clientID, ApplicationID: x.u.e.appA,
					Set: map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: false},
				}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"a lock that overrules an override": func(t *testing.T, x unlocked) func() error {
			x.u.e.override(t, x.u.e.appB, map[fieldidentity.PolicyMember]any{fieldidentity.MemberUnlockKeysEnabled: false})
			return func() error {
				_, err := usecaseop.RunTx(x.u.e.actx, x.u.e.s.UoW, x.u.e.s.Ops.SetPolicyLocks(), fieldidentity.SetPolicyLocksCommand{
					ClientID: x.u.e.clientID, LockedMembers: []fieldidentity.PolicyMember{fieldidentity.MemberUnlockKeysEnabled},
				}, auth.NewExecutionContext(x.u.e.actx))
				return err
			}
		},
		"unlock": func(t *testing.T, x unlocked) func() error {
			ch := x.u.d.challenge(x.k.id)
			require.Equal(t, http.StatusOK, ch.Code, ch.Body.String())
			assertion := x.k.assertion(x.u.d, x.u.workerID, x.u.e.codeA, ch.str("challenge"), nil)
			return func() error {
				if r := x.u.d.unlock(x.k.id, x.u.e.codeA, assertion); r.Code != http.StatusOK {
					return fmt.Errorf("unlock: %d %s", r.Code, r.Body.String())
				}
				return nil
			}
		},
	}
	for name, o := range ops {
		t.Run(name, func(t *testing.T) {
			x := newUnlocked(t)
			do := o(t, x)
			h := holdWorkers(t, x.u.workerID)
			done := make(chan error, 1)
			go func() { done <- do() }()
			select {
			case err := <-done:
				t.Fatalf("%s did not wait for the worker's row (err %v)", name, err)
			case <-time.After(300 * time.Millisecond):
			}
			assert.True(t, h.free(`SELECT 1 FROM field_unlock_keys WHERE identity_id = $1`, x.u.workerID),
				"%s locked a key before the worker", name)
			assert.True(t, h.free(`SELECT 1 FROM field_sessions WHERE identity_id = $1`, x.u.workerID),
				"%s locked a session before the worker", name)
			assert.True(t, h.free(`SELECT 1 FROM field_sign_in_codes WHERE identity_id = $1`, x.u.workerID),
				"%s locked a sign-in code before the worker", name)
			h.release()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(10 * time.Second):
				t.Fatalf("%s did not finish after the worker was released", name)
			}
		})
	}
}

// The review's deadlock, under load: a suspension, a PIN reset, a device's
// revocation, an administrator's revoke of the key, the policy switched off
// and a refresh of the key's session, all at once. None may fail with a
// deadlock (40P01), and nothing the key signed in may survive.
func TestCredentialChangesAtOnceDoNotDeadlock(t *testing.T) {
	for round := range 6 {
		t.Run(fmt.Sprintf("round %d", round), func(t *testing.T) {
			x := newUnlocked(t)
			e := x.u.e
			ec := auth.NewExecutionContext(e.actx)
			worker := fieldidentity.WorkerRef{ClientID: e.clientID, ID: x.u.workerID}
			start := make(chan struct{})
			var mu sync.Mutex
			var errs []error
			var wg sync.WaitGroup
			for _, f := range []func() error{
				func() error {
					_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.SuspendWorker(), worker, ec)
					return err
				},
				func() error {
					_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.ResetWorkerPin(), fieldidentity.ResetPinCommand{WorkerRef: worker}, ec)
					return err
				},
				func() error {
					_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.RevokeDevice(), fieldidentity.DeviceRef{ClientID: e.clientID, ID: x.u.d.id}, ec)
					return err
				},
				func() error {
					_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.RevokeUnlockKey(),
						fieldidentity.UnlockKeyRef{WorkerRef: worker, UnlockKeyID: x.k.id}, ec)
					return err
				},
				func() error {
					_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.PutPolicy(), fieldidentity.PutPolicyCommand{ClientID: e.clientID}, ec)
					return err
				},
				func() error {
					_ = x.u.d.send("/field/auth/refresh", map[string]any{"refreshToken": x.refresh}, x.u.d.fetchNonce(), "")
					return nil
				},
			} {
				wg.Go(func() {
					<-start
					if err := f(); err != nil {
						mu.Lock()
						errs = append(errs, err)
						mu.Unlock()
					}
				})
			}
			close(start)
			wg.Wait()
			for _, err := range errs {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "40P01" {
					t.Errorf("DEADLOCK: %v", err)
				} else {
					t.Errorf("unexpected error: %v", err)
				}
			}
			assert.Zero(t, e.liveKeySessions(x.k.id))
			revoked, _ := e.keyRevocation(x.k.id)
			assert.True(t, revoked)
		})
	}
}
