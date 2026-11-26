//go:build integration

package fieldauth

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
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

// send is a request that touches no shared device state, for concurrent use.
func (d *device) send(path string, body any, nonce, accessToken string) resp {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", d.ip)
	req.Header.Set("DPoP", d.proof(http.MethodPost, path, accessToken, proofOpts{nonce: nonce}))
	if accessToken != "" {
		req.Header.Set("Authorization", "DPoP "+accessToken)
	}
	rec := httptest.NewRecorder()
	d.e.router.ServeHTTP(rec, req)
	return resp{rec}
}

func (e *env) adminRevoke(t *testing.T, workerID, keyID string) {
	t.Helper()
	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.RevokeUnlockKey(), fieldidentity.UnlockKeyRef{
		ClientID: e.clientID, ID: workerID, UnlockKeyID: keyID,
	}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)
}

// ── finding 1: a forged chain does not pass REQUIRED end to end ──────────

func TestUnlockKeyForgedChainRefusedUnderRequired(t *testing.T) {
	u := newUnlockEnv(t, nil)
	e := u.e
	sid := tokenPayload(t, u.si.access)["sid"].(string)
	appKey := newUnlockKey(t)
	genuine := attestationtest.Build(&appKey.key.PublicKey, attestationtest.Description{
		Version: 200, AttestationLevel: 1, KeyLevel: 1, Challenge: []byte("x"),
		Hardware: attestationtest.AuthList{Purposes: []int{2}, Algorithm: new(3), ECCurve: new(1), NoAuthRequired: true},
	})
	roots, err := attestation.ParseRootsPEM(genuine.RootPEM)
	require.NoError(t, err)
	e.s.Ops.AttestationRoots = roots
	e.policy("unlock_key_attestation", "REQUIRED")
	victim := newUnlockKey(t)
	forged := forgeLeaf(t, appKey, genuine, victim, attestation.Challenge(sid, u.workerID, u.d.id))
	r := victim.register(u.d, u.si, forged, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.Code, r.Body.String())
	assert.Equal(t, "ATTESTATION_REQUIRED", r.str("error"))
	assert.Contains(t, r.str("message"), "key description")
}

// ── finding 2 and the owner's revocation list: checked at each use ───────

func TestUnlockRequiredRefusesAnUnverifiedKeyAtUse(t *testing.T) {
	// A registration that read the policy before REQUIRED was set, committed
	// after the policy's revoke ran: the key is live and unverified.
	u, k := registered(t, nil)
	e := u.e
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	e.policy("unlock_key_attestation", "REQUIRED")
	r := u.d.challenge(k.id)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "UNLOCK_KEY_INVALID", r.str("error"))
	r = u.d.unlock(k.id, e.codeA, k.assertion(u.d, u.workerID, e.codeA, ch.str("challenge"), nil))
	assert.Equal(t, "UNLOCK_KEY_INVALID", r.str("error"), r.Body.String())
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND failure_reason = 'ATTESTATION_NOT_VERIFIED'`, u.workerID))
	assert.Zero(t, e.failures(u.workerID))
}

func TestUnlockRegistrationAndPolicyChangeDoNotCross(t *testing.T) {
	for range 5 {
		u := newUnlockEnv(t, nil)
		e := u.e
		roots, err := attestation.ParseRootsPEM(attestationtest.Build(&newUnlockKey(t).key.PublicKey, attestationtest.StrongBiometric(nil)).RootPEM)
		require.NoError(t, err)
		e.s.Ops.AttestationRoots = roots
		k := newUnlockKey(t)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); k.register(u.d, u.si, nil, nil) }()
		go func() {
			defer wg.Done()
			_, _ = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.PutPolicy(), fieldidentity.PutPolicyCommand{
				ClientID: e.clientID, UnlockKeysEnabled: true, UnlockKeyAttestation: fieldidentity.UnlockAttestationRequired,
			}, auth.NewExecutionContext(e.actx))
		}()
		wg.Wait()
		assert.Zero(t, e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE identity_id = $1 AND revoked_at IS NULL AND NOT attestation_verified`, u.workerID),
			"an unverified key survived a switch to REQUIRED")
	}
}

func TestUnlockStatusListRules(t *testing.T) {
	setup := func(t *testing.T) (*unlockEnv, *unlockKey, attestationtest.Chain, *attestation.StatusCache) {
		u := newUnlockEnv(t, nil)
		sid := tokenPayload(t, u.si.access)["sid"].(string)
		k := newUnlockKey(t)
		chain := attestationtest.Build(&k.key.PublicKey, attestationtest.StrongBiometric(attestation.Challenge(sid, u.workerID, u.d.id)))
		roots, err := attestation.ParseRootsPEM(chain.RootPEM)
		require.NoError(t, err)
		u.e.s.Ops.AttestationRoots = roots
		cache := attestation.NewStatusCache(time.Hour)
		u.e.s.Ops.AttestationStatus = cache
		return u, k, chain, cache
	}
	list := func(t *testing.T, doc string) *attestation.StatusList {
		l, err := attestation.ParseStatusList([]byte(doc))
		require.NoError(t, err)
		return l
	}
	t.Run("no usable list refuses a REQUIRED registration", func(t *testing.T) {
		u, k, chain, cache := setup(t)
		u.e.policy("unlock_key_attestation", "REQUIRED")
		r := k.register(u.d, u.si, chain.Base64, nil)
		assert.Equal(t, http.StatusServiceUnavailable, r.Code, r.Body.String())
		assert.Equal(t, "ATTESTATION_STATUS_UNAVAILABLE", r.str("error"))
		cache.Set(list(t, `{"entries":{}}`), time.Now().Add(-2*time.Hour)) // older than the maximum age
		r = k.register(u.d, u.si, chain.Base64, nil)
		assert.Equal(t, "ATTESTATION_STATUS_UNAVAILABLE", r.str("error"), "a stale copy is no copy")
	})
	t.Run("a listed certificate refuses a REQUIRED registration", func(t *testing.T) {
		u, k, chain, cache := setup(t)
		u.e.policy("unlock_key_attestation", "REQUIRED")
		cache.Set(list(t, `{"entries":{"3003":{"status":"REVOKED"}}}`), time.Now())
		r := k.register(u.d, u.si, chain.Base64, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, r.Code, r.Body.String())
		assert.Contains(t, r.str("message"), "status list")
	})
	t.Run("a refreshed list naming the chain revokes the key at use and ends its sessions", func(t *testing.T) {
		u, k, chain, cache := setup(t)
		e := u.e
		e.policy("unlock_key_attestation", "REQUIRED")
		cache.Set(list(t, `{"entries":{}}`), time.Now())
		require.Equal(t, http.StatusCreated, k.register(u.d, u.si, chain.Base64, nil).Code)
		r := k.fullUnlock(u.d, u.workerID, e.codeA)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		unlocked := r.str("refreshToken")

		cache.Set(list(t, `{"entries":{"2002":{"status":"SUSPENDED"}}}`), time.Now())
		r = u.d.challenge(k.id)
		assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		assert.Equal(t, "UNLOCK_KEY_INVALID", r.str("error"))
		revoked, reason := e.keyRevocation(k.id)
		assert.True(t, revoked)
		assert.Equal(t, "ATTESTATION_REVOKED", reason)
		assert.Equal(t, "INVALID_GRANT", u.d.refresh(unlocked).str("error"), "the key's session ended with it")
	})
	t.Run("under NONE the list is recorded, nothing is enforced", func(t *testing.T) {
		u, k, chain, cache := setup(t)
		cache.Set(list(t, `{"entries":{"3003":{"status":"REVOKED"}}}`), time.Now())
		r := k.register(u.d, u.si, chain.Base64, nil)
		require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
		att := r.json()["attestation"].(map[string]any)
		assert.Equal(t, "REVOKED", att["revocation"])
		assert.Equal(t, false, att["meetsRequirement"])
		assert.Equal(t, http.StatusOK, k.fullUnlock(u.d, u.workerID, u.e.codeA).Code)
	})
}

// ── finding 3: revoking a key ends the sessions it started ───────────────

func TestUnlockKeyRevocationEndsItsSessions(t *testing.T) {
	t.Run("administrator", func(t *testing.T) {
		u, k := registered(t, nil)
		r := k.fullUnlock(u.d, u.workerID, u.e.codeA)
		require.Equal(t, http.StatusOK, r.Code)
		pinSession := u.si.refresh
		u.e.adminRevoke(t, u.workerID, k.id)
		assert.Equal(t, "INVALID_GRANT", u.d.refresh(r.str("refreshToken")).str("error"))
		assert.Equal(t, http.StatusOK, u.d.refresh(pinSession).Code, "a PIN session is not the key's")
		assert.Equal(t, 1, u.e.q(`SELECT COUNT(*) FROM field_sessions WHERE revoked_reason = 'UNLOCK_KEY_REVOKED' AND identity_id = $1`, u.workerID))
	})
	t.Run("policy switched off", func(t *testing.T) {
		u, k := registered(t, nil)
		r := k.fullUnlock(u.d, u.workerID, u.e.codeA)
		require.Equal(t, http.StatusOK, r.Code)
		_, err := usecaseop.RunTx(u.e.actx, u.e.s.UoW, u.e.s.Ops.PutPolicy(), fieldidentity.PutPolicyCommand{ClientID: u.e.clientID},
			auth.NewExecutionContext(u.e.actx))
		require.NoError(t, err)
		assert.Equal(t, "INVALID_GRANT", u.d.refresh(r.str("refreshToken")).str("error"))
	})
	t.Run("the worker from the device keeps the session they are in", func(t *testing.T) {
		u, k := registered(t, nil)
		r := k.fullUnlock(u.d, u.workerID, u.e.codeA)
		require.Equal(t, http.StatusOK, r.Code)
		require.Equal(t, http.StatusNoContent, u.d.call("/field/auth/unlock-keys/revoke", map[string]any{"unlockKeyId": k.id}).Code)
		assert.Equal(t, http.StatusOK, u.d.refresh(r.str("refreshToken")).Code)
	})
}

// ── finding 5: a PIN that needs a supervisor's reset ─────────────────────

func TestUnlockRefusedWhileThePINNeedsAReset(t *testing.T) {
	u, k := registered(t, nil)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	u.e.exec(`UPDATE field_identities SET pin_pepper_id = 'retired-app-key' WHERE id = $1`, u.workerID)
	assert.Equal(t, "PIN_RESET_REQUIRED", u.d.login(u.e.codeA, "uk-1", goodPIN).str("error"), "the PIN sign-in refuses it")
	r := u.d.challenge(k.id)
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "PIN_RESET_REQUIRED", r.str("error"))
	r = u.d.unlock(k.id, u.e.codeA, k.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), nil))
	assert.Equal(t, "PIN_RESET_REQUIRED", r.str("error"), r.Body.String())
	assert.Zero(t, u.e.failures(u.workerID))
}

// ── finding 6 and the races of finding 8 ────────────────────────────────

func TestUnlockKeyConcurrentRevokesEmitOneEvent(t *testing.T) {
	u, k := registered(t, nil)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { ; u.e.adminRevoke(t, u.workerID, k.id) })
	}
	wg.Wait()
	assert.Equal(t, 1, u.e.eventCount(fieldidentity.UnlockKeyRevokedType))
}

func TestUnlocksRacingOneChallenge(t *testing.T) {
	u, k := registered(t, nil)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	a := k.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), nil)
	n1, n2 := u.d.fetchNonce(), u.d.fetchNonce()
	body := map[string]any{"unlockKeyId": k.id, "appCode": u.e.codeA, "assertion": a}
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i, n := range []string{n1, n2} {
		wg.Go(func() { ; codes[i] = u.d.send("/field/auth/unlock", body, n, "").Code })
	}
	wg.Wait()
	assert.ElementsMatch(t, []int{http.StatusOK, http.StatusUnauthorized}, codes, "one wins")
	assert.LessOrEqual(t, u.e.failures(u.workerID), 1, "at most one failure counted")
}

func TestTwoWorkersRegisteringOnOneDeviceAtOnce(t *testing.T) {
	u := newUnlockEnv(t, nil)
	e := u.e
	e.worker("uk-2", goodPIN, false, e.codeA)
	si2 := mustLogin(t, u.d, e.codeA, "uk-2", goodPIN)
	k1, k2 := newUnlockKey(t), newUnlockKey(t)
	sub2 := tokenPayload(t, si2.access)["sub"].(string)
	n1, n2 := u.d.fetchNonce(), u.d.fetchNonce()
	proof := func(k *unlockKey, sub, nonce string) string {
		return k.keyProof(u.d, sub, func(_, c map[string]any) { c["nonce"] = nonce })
	}
	b1 := map[string]any{"publicKeyJwk": k1.jwk(), "keyProof": proof(k1, u.workerID, n1)}
	b2 := map[string]any{"publicKeyJwk": k2.jwk(), "keyProof": proof(k2, sub2, n2)}
	codes := make([]int, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); codes[0] = u.d.send("/field/auth/unlock-keys", b1, n1, u.si.access).Code }()
	go func() { defer wg.Done(); codes[1] = u.d.send("/field/auth/unlock-keys", b2, n2, si2.access).Code }()
	wg.Wait()
	assert.ElementsMatch(t, []int{http.StatusCreated, http.StatusConflict}, codes)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE device_id = $1 AND revoked_at IS NULL`, u.d.id))
}

// ── more revocation paths of finding 8 ───────────────────────────────────

func TestPINChangeFromAnUnenrolledKeyRevokesEveryUnlockKey(t *testing.T) {
	u, k := registered(t, nil)
	un := u.e.newDevice()
	si := mustLoginUn(t, un, u.e.codeA, "uk-1", goodPIN)
	r := un.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "730418"}, si.access)
	require.Equal(t, http.StatusNoContent, r.Code, r.Body.String())
	revoked, reason := u.e.keyRevocation(k.id)
	assert.True(t, revoked)
	assert.Equal(t, "PIN_CHANGED", reason)
}

func TestHardLockThroughActAsRevokesTheSupervisorsKeys(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.policy("unlock_keys_enabled", true)
	e.policy("soft_lock_threshold", 10)
	supID := e.supervisor("uk-sup", goodPIN, e.codeA)
	e.worker("uk-tgt", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	sup := mustLogin(t, d, e.codeA, "uk-sup", goodPIN)
	k := newUnlockKey(t)
	require.Equal(t, http.StatusCreated, k.register(d, sup, nil, nil).Code)
	for range 10 {
		d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "uk-tgt", "pin": "000111"}, sup.access)
	}
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND hard_locked`, supID))
	revoked, reason := e.keyRevocation(k.id)
	assert.True(t, revoked)
	assert.Equal(t, "WORKER_LOCKED", reason)
	assert.Equal(t, 1, e.eventCount(fieldidentity.UnlockKeyRevokedType))
}

// forgeLeaf is the reviewer's forgery (attestation_test.go): a leaf for
// victim's key, with a key description claiming everything REQUIRED asks,
// signed by appKey (a genuinely attested key with no user authentication) and
// placed on top of appKey's genuine chain.
func forgeLeaf(t *testing.T, appKey *unlockKey, genuine attestationtest.Chain, victim *unlockKey, challenge []byte) []string {
	t.Helper()
	donor := attestationtest.Build(&victim.key.PublicKey, attestationtest.StrongBiometric(challenge))
	var ext pkix.Extension
	for _, e := range donor.Certs[0].Extensions {
		if e.Id.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 1, 17}) {
			ext = e
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "Android Keystore Key"},
		NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4102444800, 0), ExtraExtensions: []pkix.Extension{ext},
	}, genuine.Certs[0], &victim.key.PublicKey, appKey.key)
	require.NoError(t, err)
	return append([]string{base64.StdEncoding.EncodeToString(der)}, genuine.Base64...)
}
