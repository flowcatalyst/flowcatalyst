//go:build integration

package fieldauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation/attestationtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Ending a key's sessions is by key: whoever revoked the key and whenever,
// what an administrator's revoke, the policy's revocations and the status
// list's end is every live session the key signed in
// (docs/field-unlock-keys-spec.md section 7).

func (x unlocked) phoneRevokes(t *testing.T) {
	t.Helper()
	require.Equal(t, http.StatusNoContent, x.u.d.call("/field/auth/unlock-keys/revoke", map[string]any{"unlockKeyId": x.k.id}).Code)
	revoked, reason := x.u.e.keyRevocation(x.k.id)
	require.True(t, revoked)
	require.Equal(t, "DEVICE", reason)
}

// refreshed refreshes the key's session and keeps the new token.
func (x *unlocked) refreshed(t *testing.T) {
	t.Helper()
	r := x.u.d.refresh(x.refresh)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	x.refresh = r.str("refreshToken")
}

func (e *env) putPolicy(t *testing.T, cmd fieldidentity.PutPolicyCommand) {
	t.Helper()
	cmd.ClientID = e.clientID
	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.PutPolicy(), cmd, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)
}

func TestAnAdministratorEndsTheSessionsOfAKeyThePhoneRevoked(t *testing.T) {
	x := newUnlocked(t)
	e := x.u.e
	x.phoneRevokes(t)
	x.refreshed(t) // the phone's own revoke keeps the session the worker is in
	counts, err := e.s.Ops.R.UnlockKeys.LiveSessionCounts(context.Background(), []string{x.k.id}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, int64(1), counts[x.k.id], "the console and the API show the revoked key's live session")
	revokedEvents, sessionEvents := e.eventCount(fieldidentity.UnlockKeyRevokedType), e.eventCount(fieldidentity.SessionRevokedType)

	e.adminRevoke(t, x.u.workerID, x.k.id)
	assert.Zero(t, e.liveKeySessions(x.k.id))
	assert.Equal(t, "INVALID_GRANT", x.u.d.refresh(x.refresh).str("error"))
	_, reason := e.keyRevocation(x.k.id)
	assert.Equal(t, "DEVICE", reason, "the key keeps the revocation it had")
	assert.Equal(t, revokedEvents, e.eventCount(fieldidentity.UnlockKeyRevokedType), "the key was not revoked again")
	assert.Equal(t, sessionEvents+1, e.eventCount(fieldidentity.SessionRevokedType), "the sessions' end is an event")

	// Again: nothing left to end, nothing emitted.
	e.adminRevoke(t, x.u.workerID, x.k.id)
	assert.Equal(t, sessionEvents+1, e.eventCount(fieldidentity.SessionRevokedType))
}

// A replacement keeps the old key's sessions (the same worker on the same
// phone, who has just proved their PIN there); switching unlock keys off ends
// them, as it ends every session any key of the client signed in.
func TestAReplacedKeysSessionsEndWhenUnlockKeysAreSwitchedOff(t *testing.T) {
	x := newUnlocked(t)
	e := x.u.e
	k2 := newUnlockKey(t)
	require.Equal(t, http.StatusCreated, k2.register(x.u.d, x.u.si, nil, nil).Code)
	_, reason := e.keyRevocation(x.k.id)
	require.Equal(t, "REPLACED", reason)
	x.refreshed(t)
	r := k2.fullUnlock(x.u.d, x.u.workerID, e.codeA)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())

	e.putPolicy(t, fieldidentity.PutPolicyCommand{}) // unlock keys off
	assert.Zero(t, e.liveKeySessions(x.k.id), "the replaced key's session ended")
	assert.Zero(t, e.liveKeySessions(k2.id), "the live key's session ended")
	assert.Equal(t, "INVALID_GRANT", x.u.d.refresh(x.refresh).str("error"))
	assert.Equal(t, http.StatusOK, x.u.d.refresh(x.u.si.refresh).Code, "a PIN session is no key's")
}

// Starting to require attestation ends the sessions of every key registered
// without a verified one, also a key the phone revoked already.
func TestRequiringAttestationEndsTheSessionsOfUnverifiedKeys(t *testing.T) {
	x := newUnlocked(t)
	e := x.u.e
	chain := attestationtest.Build(&newUnlockKey(t).key.PublicKey, attestationtest.StrongBiometric([]byte("any")))
	roots, err := attestation.ParseRootsPEM(chain.RootPEM)
	require.NoError(t, err)
	e.s.Ops.AttestationRoots = roots
	x.phoneRevokes(t)
	x.refreshed(t)

	e.putPolicy(t, fieldidentity.PutPolicyCommand{UnlockKeysEnabled: true, UnlockKeyAttestation: fieldidentity.UnlockAttestationRequired})
	assert.Zero(t, e.liveKeySessions(x.k.id))
	assert.Equal(t, "INVALID_GRANT", x.u.d.refresh(x.refresh).str("error"))
}

// attestedUnlock is a key registered with a trusted chain under REQUIRED and a
// session it signed in, with the status list's cache.
func attestedUnlock(t *testing.T) (unlocked, *attestation.StatusCache) {
	t.Helper()
	u := newUnlockEnv(t, nil)
	sid := tokenPayload(t, u.si.access)["sid"].(string)
	k := newUnlockKey(t)
	chain := attestationtest.Build(&k.key.PublicKey, attestationtest.StrongBiometric(attestation.Challenge(sid, u.workerID, u.d.id)))
	roots, err := attestation.ParseRootsPEM(chain.RootPEM)
	require.NoError(t, err)
	u.e.s.Ops.AttestationRoots = roots
	cache := attestation.NewStatusCache(time.Hour)
	empty, err := attestation.ParseStatusList([]byte(`{"entries":{}}`))
	require.NoError(t, err)
	cache.Set(empty, time.Now())
	u.e.s.Ops.AttestationStatus = cache
	u.e.policy("unlock_key_attestation", "REQUIRED")
	r := k.register(u.d, u.si, chain.Base64, nil)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	r = k.fullUnlock(u.d, u.workerID, u.e.codeA)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	return unlocked{u: u, k: k, refresh: r.str("refreshToken")}, cache
}

func listing(t *testing.T, doc string) *attestation.StatusList {
	t.Helper()
	l, err := attestation.ParseStatusList([]byte(doc))
	require.NoError(t, err)
	return l
}

// The check after each fetch reaches the sessions a key already started,
// which refresh without the key: a listed chain revokes a live key and ends
// its sessions, and ends the sessions of a key the phone revoked.
func TestTheStatusListCheckAfterAFetchEndsTheKeysSessions(t *testing.T) {
	t.Run("a live key", func(t *testing.T) {
		x, _ := attestedUnlock(t)
		e := x.u.e
		n, err := e.s.Ops.SweepAttestationStatus(context.Background(), e.s.UoW, listing(t, `{"entries":{"abcdef":{"status":"REVOKED"}}}`))
		require.NoError(t, err)
		assert.Zero(t, n, "a serial of no registered chain")
		assert.Equal(t, 1, e.liveKeySessions(x.k.id))
		n, err = e.s.Ops.SweepAttestationStatus(context.Background(), e.s.UoW, listing(t, `{"entries":{"02002":{"status":"SUSPENDED"}}}`))
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		_, reason := e.keyRevocation(x.k.id)
		assert.Equal(t, "ATTESTATION_REVOKED", reason)
		assert.Zero(t, e.liveKeySessions(x.k.id))
		n, err = e.s.Ops.SweepAttestationStatus(context.Background(), e.s.UoW, listing(t, `{"entries":{"2002":{"status":"REVOKED"}}}`))
		require.NoError(t, err)
		assert.Zero(t, n, "nothing left: the key is revoked and has no session")
	})
	t.Run("a key the phone revoked", func(t *testing.T) {
		x, _ := attestedUnlock(t)
		e := x.u.e
		x.phoneRevokes(t)
		x.refreshed(t)
		n, err := e.s.Ops.SweepAttestationStatus(context.Background(), e.s.UoW, listing(t, `{"entries":{"3003":{"status":"REVOKED"}}}`))
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Zero(t, e.liveKeySessions(x.k.id))
		_, reason := e.keyRevocation(x.k.id)
		assert.Equal(t, "DEVICE", reason)
	})
	t.Run("under NONE nothing is checked", func(t *testing.T) {
		x, _ := attestedUnlock(t)
		e := x.u.e
		e.policy("unlock_key_attestation", "NONE")
		n, err := e.s.Ops.SweepAttestationStatus(context.Background(), e.s.UoW, listing(t, `{"entries":{"3003":{"status":"REVOKED"}}}`))
		require.NoError(t, err)
		assert.Zero(t, n)
		assert.Equal(t, 1, e.liveKeySessions(x.k.id))
	})
}

// Under REQUIRED with the list on and no usable copy, a challenge and an
// unlock are refused with a retryable answer and the key stays registered;
// with the list off, or under NONE, nothing changes.
func TestUnlockWithNoUsableStatusList(t *testing.T) {
	t.Run("REQUIRED refuses, retryable, and keeps the key", func(t *testing.T) {
		x, cache := attestedUnlock(t)
		e := x.u.e
		ch := x.u.d.challenge(x.k.id)
		require.Equal(t, http.StatusOK, ch.Code, ch.Body.String())
		assertion := x.k.assertion(x.u.d, x.u.workerID, e.codeA, ch.str("challenge"), nil)

		cache.Set(listing(t, `{"entries":{}}`), time.Now().Add(-2*time.Hour)) // stale
		r := x.u.d.challenge(x.k.id)
		assert.Equal(t, http.StatusServiceUnavailable, r.Code, r.Body.String())
		assert.Equal(t, "ATTESTATION_STATUS_UNAVAILABLE", r.str("error"))
		assert.NotEmpty(t, r.Header().Get("Retry-After"))
		r = x.u.d.unlock(x.k.id, e.codeA, assertion)
		assert.Equal(t, http.StatusServiceUnavailable, r.Code, r.Body.String())
		assert.Equal(t, "ATTESTATION_STATUS_UNAVAILABLE", r.str("error"))
		revoked, _ := e.keyRevocation(x.k.id)
		assert.False(t, revoked, "the key is not revoked for the platform's outage")
		assert.Zero(t, e.failures(x.u.workerID), "not the worker's failure")
		x.refreshed(t) // a session the key started goes on

		cache.Set(listing(t, `{"entries":{}}`), time.Now())
		assert.Equal(t, http.StatusOK, x.k.fullUnlock(x.u.d, x.u.workerID, e.codeA).Code, "usable again")
	})
	t.Run("the list switched off", func(t *testing.T) {
		x, _ := attestedUnlock(t)
		x.u.e.s.Ops.AttestationStatus = nil
		assert.Equal(t, http.StatusOK, x.k.fullUnlock(x.u.d, x.u.workerID, x.u.e.codeA).Code)
	})
	t.Run("NONE", func(t *testing.T) {
		x, cache := attestedUnlock(t)
		x.u.e.policy("unlock_key_attestation", "NONE")
		cache.Set(listing(t, `{"entries":{}}`), time.Now().Add(-2*time.Hour))
		assert.Equal(t, http.StatusOK, x.k.fullUnlock(x.u.d, x.u.workerID, x.u.e.codeA).Code)
	})
}

// The unlock reads the key's standing once and answers from it: every
// refused standing writes its refusal (never an empty 200), and while the
// status list's copy turns usable and stale under it, every unlock answers
// either a session or a refusal.
func TestUnlockAnswersEveryStanding(t *testing.T) {
	x, _ := attestedUnlock(t)
	s := x.u.e.s
	key, err := s.Ops.R.UnlockKeys.FindByID(context.Background(), x.k.id)
	require.NoError(t, err)
	for standing, want := range map[attestationStanding]string{
		standingNotVerified:       "UNLOCK_KEY_INVALID",
		standingStatusUnavailable: "ATTESTATION_STATUS_UNAVAILABLE",
		standingListed:            "UNLOCK_KEY_INVALID",
	} {
		rec := httptest.NewRecorder()
		c := &call{w: rec, r: httptest.NewRequest(http.MethodPost, "/field/auth/unlock", nil), now: time.Now()}
		assert.False(t, s.answerStanding(c, key, standing, "2002", func(string) {}), want)
		assert.Equal(t, want, resp{rec}.str("error"), "standing %d", standing)
	}
	rec := httptest.NewRecorder()
	c := &call{w: rec, r: httptest.NewRequest(http.MethodPost, "/field/auth/unlock", nil), now: time.Now()}
	assert.True(t, s.answerStanding(c, key, standingOK, "", func(string) {}))
	assert.Zero(t, rec.Body.Len(), "an admitted key writes nothing")

	// The key above was revoked by the standingListed case; a fresh one for
	// the flips.
	y, cache := attestedUnlock(t)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		fresh := true
		for {
			select {
			case <-stop:
				return
			default:
			}
			at := time.Now()
			if !fresh {
				at = at.Add(-2 * time.Hour)
			}
			cache.Set(listing(t, `{"entries":{}}`), at)
			fresh = !fresh
		}
	}()
	for range 20 {
		ch := y.u.d.challenge(y.k.id)
		if ch.Code != http.StatusOK {
			require.Equal(t, http.StatusServiceUnavailable, ch.Code, ch.Body.String())
			continue
		}
		r := y.u.d.unlock(y.k.id, y.u.e.codeA, y.k.assertion(y.u.d, y.u.workerID, y.u.e.codeA, ch.str("challenge"), nil))
		switch r.Code {
		case http.StatusOK:
			assert.NotEmpty(t, r.str("refreshToken"), "a 200 carries a session")
		case http.StatusServiceUnavailable:
			assert.Equal(t, "ATTESTATION_STATUS_UNAVAILABLE", r.str("error"))
		default:
			t.Errorf("unexpected answer %d %s", r.Code, r.Body.String())
		}
	}
	close(stop)
	<-done
}
