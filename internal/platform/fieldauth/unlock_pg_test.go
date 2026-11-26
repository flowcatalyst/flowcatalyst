//go:build integration

package fieldauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation/attestationtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── helpers ──────────────────────────────────────────────────────────────

// unlockKey is a phone's biometric-gated key, held by the test.
type unlockKey struct {
	t   *testing.T
	key *ecdsa.PrivateKey
	id  string // once registered
}

func newUnlockKey(t *testing.T) *unlockKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return &unlockKey{t: t, key: k}
}

func (u *unlockKey) jwk() map[string]string {
	pt, err := u.key.PublicKey.Bytes()
	require.NoError(u.t, err)
	return map[string]string{
		"kty": "EC", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(pt[1:33]), "y": base64.RawURLEncoding.EncodeToString(pt[33:65]),
	}
}

// signJWS signs header.claims with key, raw r||s.
func signJWS(t *testing.T, key *ecdsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	input := b64json(t, header) + "." + b64json(t, claims)
	digest := sha256.Sum256([]byte(input))
	der, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	require.NoError(t, err)
	var rs struct{ R, S *big.Int }
	_, err = asn1.Unmarshal(der, &rs)
	require.NoError(t, err)
	return input + "." + base64.RawURLEncoding.EncodeToString(append(rs.R.FillBytes(make([]byte, 32)), rs.S.FillBytes(make([]byte, 32))...))
}

// keyProof is the registration's key proof over the device's next nonce.
func (u *unlockKey) keyProof(d *device, workerID string, mutate func(h, c map[string]any)) string {
	if d.nonce == "" {
		d.fetchNonce()
	}
	h := map[string]any{"typ": KeyProofType, "alg": "ES256"}
	c := map[string]any{
		"aud": baseURL + "/field/auth/unlock-keys", "nonce": d.nonce, "sub": workerID,
		"device_id": d.id, "iat": time.Now().Unix(),
	}
	if mutate != nil {
		mutate(h, c)
	}
	return signJWS(u.t, u.key, h, c)
}

func (u *unlockKey) assertion(d *device, workerID, app, challenge string, mutate func(h, c map[string]any)) string {
	h := map[string]any{"typ": UnlockAssertionType, "alg": "ES256", "kid": u.id}
	c := map[string]any{
		"aud": baseURL + "/field/auth/unlock", "nonce": challenge, "sub": workerID,
		"device_id": d.id, "app": app, "iat": time.Now().Unix(),
	}
	if mutate != nil {
		mutate(h, c)
	}
	return signJWS(u.t, u.key, h, c)
}

// register registers u for the signed-in worker on d.
func (u *unlockKey) register(d *device, si signedIn, chain []string, mutate func(h, c map[string]any)) resp {
	u.t.Helper()
	sub, _ := tokenPayload(u.t, si.access)["sub"].(string)
	body := map[string]any{"publicKeyJwk": u.jwk(), "keyProof": u.keyProof(d, sub, mutate)}
	if chain != nil {
		body["attestationChain"] = chain
	}
	r := d.callBearer("/field/auth/unlock-keys", body, si.access)
	if r.Code == http.StatusCreated {
		u.id = r.str("unlockKeyId")
	}
	return r
}

func (d *device) challenge(keyID string) resp {
	return d.call("/field/auth/unlock/challenge", map[string]any{"unlockKeyId": keyID})
}

func (d *device) unlock(keyID, app, assertion string) resp {
	return d.call("/field/auth/unlock", map[string]any{"unlockKeyId": keyID, "appCode": app, "assertion": assertion})
}

// fullUnlock runs challenge then unlock with u.
func (u *unlockKey) fullUnlock(d *device, workerID, app string) resp {
	u.t.Helper()
	ch := d.challenge(u.id)
	require.Equal(u.t, http.StatusOK, ch.Code, ch.Body.String())
	return d.unlock(u.id, app, u.assertion(d, workerID, app, ch.str("challenge"), nil))
}

// unlockEnv is a client with unlock keys on, a worker signed in with the PIN
// on an enrolled PERSONAL device.
type unlockEnv struct {
	e        *env
	d        *device
	workerID string
	si       signedIn
}

func newUnlockEnv(t *testing.T, limits *ratelimit.Policies) *unlockEnv {
	t.Helper()
	if limits == nil {
		limits = roomyLimits()
	}
	e := newEnv(t, envOpts{limits: limits})
	e.policy("unlock_keys_enabled", true)
	workerID := e.worker("uk-1", goodPIN, false, e.codeA, e.codeB)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA, e.codeB)
	return &unlockEnv{e: e, d: d, workerID: workerID, si: mustLogin(t, d, e.codeA, "uk-1", goodPIN)}
}

// registered is newUnlockEnv with a registered key.
func registered(t *testing.T, limits *ratelimit.Policies) (*unlockEnv, *unlockKey) {
	t.Helper()
	u := newUnlockEnv(t, limits)
	k := newUnlockKey(t)
	r := k.register(u.d, u.si, nil, nil)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	return u, k
}

func (e *env) keyRevocation(keyID string) (revoked bool, reason string) {
	e.t.Helper()
	var r *string
	require.NoError(e.t, testpg.Pool(e.t).QueryRow(context.Background(),
		`SELECT revoked_reason FROM field_unlock_keys WHERE id = $1`, keyID).Scan(&r))
	if r == nil {
		return false, ""
	}
	return true, *r
}

func (e *env) failures(workerID string) int {
	return e.q(`SELECT failed_since_success FROM field_identities WHERE id = $1`, workerID)
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	_, err := testpg.Pool(e.t).Exec(context.Background(), sql, args...)
	require.NoError(e.t, err)
}

// ── the whole path ───────────────────────────────────────────────────────

func TestUnlockKeyRegisterChallengeUnlock(t *testing.T) {
	u := newUnlockEnv(t, nil)
	e := u.e
	k := newUnlockKey(t)

	r := k.register(u.d, u.si, nil, nil)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	body := r.json()
	assert.Equal(t, u.d.id, body["deviceId"])
	assert.Equal(t, u.workerID, body["workerId"])
	assert.NotContains(t, body, "replacedUnlockKeyId")
	att := body["attestation"].(map[string]any)
	assert.Equal(t, "NONE", att["format"])
	assert.Equal(t, "NONE", att["verdict"])
	assert.Equal(t, false, att["meetsRequirement"])
	assert.Equal(t, 1, e.eventCount(fieldidentity.UnlockKeyRegisteredType))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND auth_method = 'UNLOCK_KEY_REGISTER' AND outcome = 'SUCCESS'`, u.workerID))

	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code, ch.Body.String())
	assert.NotEmpty(t, ch.str("challenge"))
	assert.NotContains(t, ch.json(), "pinRequiredBy", "the default forces no periodic PIN")
	assertion := k.assertion(u.d, u.workerID, e.codeB, ch.str("challenge"), nil)
	r = u.d.unlock(k.id, e.codeB, assertion)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	claims := tokenPayload(t, r.str("accessToken"))
	assert.Equal(t, []any{"hwk", "uv"}, claims["amr"])
	assert.Equal(t, u.workerID, claims["sub"])
	assert.Equal(t, e.codeB, claims["aud"])
	assert.Equal(t, u.d.id, claims["device_id"])
	assert.Equal(t, "PERSONAL", claims["device_mode"])
	assert.NotContains(t, claims, "pcr")
	assert.NotContains(t, claims, "act")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE id = $1 AND last_used_at IS NOT NULL`, k.id))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND auth_method = 'UNLOCK_KEY' AND outcome = 'SUCCESS'`, u.workerID))

	// The unlocked session follows the PERSONAL limits (no PIN deadline by
	// default); it refreshes and keeps its amr.
	var abs, pinVerified time.Time
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT s.absolute_expires_at, k.pin_verified_at FROM field_sessions s, field_unlock_keys k
		 WHERE s.family_id = $1 AND k.id = $2 ORDER BY s.issued_at DESC LIMIT 1`, claims["sid"], k.id).Scan(&abs, &pinVerified))
	assert.False(t, abs.After(pinVerified.Add(7*24*time.Hour)))
	ref := u.d.refresh(r.str("refreshToken"))
	require.Equal(t, http.StatusOK, ref.Code, ref.Body.String())
	assert.Equal(t, []any{"hwk", "uv"}, tokenPayload(t, ref.str("accessToken"))["amr"])

	// Neither the key nor the challenge nor the assertion is in an event, the
	// audit or the attempts.
	e.assertNoLeak(k.jwk()["x"], ch.str("challenge"), assertion)
}

func TestUnlockKeyRegisterReplacesTheWorkersOwnKey(t *testing.T) {
	u, first := registered(t, nil)
	si := mustLogin(t, u.d, u.e.codeA, "uk-1", goodPIN)
	second := newUnlockKey(t)
	r := second.register(u.d, si, nil, nil)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	assert.Equal(t, first.id, r.str("replacedUnlockKeyId"))
	revoked, reason := u.e.keyRevocation(first.id)
	assert.True(t, revoked)
	assert.Equal(t, "REPLACED", reason)
	assert.Equal(t, 1, u.e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE device_id = $1 AND revoked_at IS NULL`, u.d.id))
}

// ── registration refusals (spec section 2.1) ─────────────────────────────

func TestUnlockKeyRegisterRefusals(t *testing.T) {
	t.Run("shared device", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		e.policy("unlock_keys_enabled", true)
		e.worker("uk-s", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModeShared, e.codeA)
		r := newUnlockKey(t).register(d, mustLogin(t, d, e.codeA, "uk-s", goodPIN), nil, nil)
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "SHARED_DEVICE", r.str("error"))
	})
	t.Run("unenrolled key", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		e.policy("unlock_keys_enabled", true)
		e.worker("uk-u", goodPIN, false, e.codeA)
		d := e.newDevice()
		r := newUnlockKey(t).register(d, mustLoginUn(t, d, e.codeA, "uk-u", goodPIN), nil, nil)
		assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"))
	})
	t.Run("session began before the key was enrolled", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		e.policy("unlock_keys_enabled", true)
		e.worker("uk-e", goodPIN, false, e.codeA)
		d := e.newDevice()
		si := mustLoginUn(t, d, e.codeA, "uk-e", goodPIN)
		require.Equal(t, http.StatusCreated, d.enrol(e.enrolCode(fieldidentity.ModePersonal, "", e.codeA)).Code)
		r := newUnlockKey(t).register(d, si, nil, nil)
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "PIN_SIGN_IN_REQUIRED", r.str("error"))
	})
	t.Run("policy off", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		u.e.policy("unlock_keys_enabled", false)
		r := newUnlockKey(t).register(u.d, u.si, nil, nil)
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "UNLOCK_KEYS_DISABLED", r.str("error"))
		assert.Equal(t, 1, u.e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND auth_method = 'UNLOCK_KEY_REGISTER' AND failure_reason = 'UNLOCK_KEYS_DISABLED'`, u.workerID))
	})
	t.Run("impersonated", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		e := u.e
		e.supervisor("uk-sup", goodPIN, e.codeA)
		sup := mustLogin(t, u.d, e.codeA, "uk-sup", goodPIN)
		r := u.d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "uk-1", "pin": goodPIN}, sup.access)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		imp := signedIn{access: r.str("accessToken")}
		r = newUnlockKey(t).register(u.d, imp, nil, nil)
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "IMPERSONATING", r.str("error"))
	})
	t.Run("temporary PIN", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		e.policy("unlock_keys_enabled", true)
		e.worker("uk-t", goodPIN, true, e.codeA)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		r := newUnlockKey(t).register(d, mustLogin(t, d, e.codeA, "uk-t", goodPIN), nil, nil)
		assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		assert.Equal(t, "invalid_token", r.str("error"))
	})
	t.Run("PIN sign-in too old", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		u.e.exec(`UPDATE field_sessions SET auth_time = NOW() - INTERVAL '11 minutes' WHERE identity_id = $1`, u.workerID)
		r := newUnlockKey(t).register(u.d, u.si, nil, nil)
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "PIN_SIGN_IN_REQUIRED", r.str("error"))
	})
	t.Run("session from an unlock", func(t *testing.T) {
		u, k := registered(t, nil)
		r := k.fullUnlock(u.d, u.workerID, u.e.codeA)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		r = newUnlockKey(t).register(u.d, signedIn{access: r.str("accessToken")}, nil, nil)
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "PIN_SIGN_IN_REQUIRED", r.str("error"))
	})
	t.Run("badge only", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		u.e.policy("badge_only_login", true)
		token := u.e.badge(u.workerID)
		r := u.d.loginBadge(u.e.codeA, token, "")
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		r = newUnlockKey(t).register(u.d, signedIn{access: r.str("accessToken")}, nil, nil)
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "PIN_SIGN_IN_REQUIRED", r.str("error"))
	})
	t.Run("hard locked worker", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		u.e.exec(`UPDATE field_identities SET hard_locked = TRUE WHERE id = $1`, u.workerID)
		r := newUnlockKey(t).register(u.d, u.si, nil, nil)
		assert.Equal(t, http.StatusLocked, r.Code, r.Body.String())
		assert.Equal(t, "WORKER_LOCKED", r.str("error"))
	})
	t.Run("the device key", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		k := &unlockKey{t: t, key: u.d.key}
		r := k.register(u.d, u.si, nil, nil)
		assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
		assert.Equal(t, "INVALID_PUBLIC_KEY", r.str("error"))
	})
	badProofs := map[string]func(h, c map[string]any){
		"another nonce":   func(_, c map[string]any) { c["nonce"] = "not-this-requests-nonce" },
		"another worker":  func(_, c map[string]any) { c["sub"] = "fwu_0SOMEONEELSE00" },
		"another device":  func(_, c map[string]any) { c["device_id"] = "fwd_0OTHERDEVICE00" },
		"unlock audience": func(_, c map[string]any) { c["aud"] = baseURL + "/field/auth/unlock" },
		"unlock typ":      func(h, _ map[string]any) { h["typ"] = UnlockAssertionType },
		"dpop typ":        func(h, _ map[string]any) { h["typ"] = "dpop+jwt" },
		"a kid":           func(h, _ map[string]any) { h["kid"] = "fwk_0" },
		"stale iat":       func(_, c map[string]any) { c["iat"] = time.Now().Add(-48 * time.Hour).Unix() },
	}
	for name, mutate := range badProofs {
		t.Run("key proof: "+name, func(t *testing.T) {
			u := newUnlockEnv(t, nil)
			r := newUnlockKey(t).register(u.d, u.si, nil, mutate)
			assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
			assert.Equal(t, "INVALID_UNLOCK_PROOF", r.str("error"))
			assert.Zero(t, u.e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE identity_id = $1`, u.workerID))
		})
	}
	t.Run("key proof signed by another key", func(t *testing.T) {
		u := newUnlockEnv(t, nil)
		k, other := newUnlockKey(t), newUnlockKey(t)
		body := map[string]any{"publicKeyJwk": k.jwk(), "keyProof": other.keyProof(u.d, u.workerID, nil)}
		r := u.d.callBearer("/field/auth/unlock-keys", body, u.si.access)
		assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		assert.Equal(t, "INVALID_UNLOCK_PROOF", r.str("error"))
	})
	t.Run("another worker holds the device's key", func(t *testing.T) {
		u, _ := registered(t, nil)
		u.e.worker("uk-2", goodPIN, false, u.e.codeA)
		r := newUnlockKey(t).register(u.d, mustLogin(t, u.d, u.e.codeA, "uk-2", goodPIN), nil, nil)
		assert.Equal(t, http.StatusConflict, r.Code, r.Body.String())
		assert.Equal(t, "UNLOCK_KEY_OTHER_WORKER", r.str("error"))
	})
	t.Run("the same key twice", func(t *testing.T) {
		u, k := registered(t, nil)
		si := mustLogin(t, u.d, u.e.codeA, "uk-1", goodPIN)
		r := k.register(u.d, si, nil, nil)
		assert.Equal(t, http.StatusConflict, r.Code, r.Body.String())
		assert.Equal(t, "UNLOCK_KEY_EXISTS", r.str("error"))
	})
}

// ── attestation ──────────────────────────────────────────────────────────

func TestUnlockKeyAttestationRecordedAndEnforcedOnlyByPolicy(t *testing.T) {
	u := newUnlockEnv(t, nil)
	e := u.e
	sid := tokenPayload(t, u.si.access)["sid"].(string)
	k := newUnlockKey(t)
	chain := attestationtest.Build(&k.key.PublicKey, attestationtest.StrongBiometric(attestation.Challenge(sid, u.workerID, u.d.id)))
	roots, err := attestation.ParseRootsPEM(chain.RootPEM)
	require.NoError(t, err)

	// Default (NONE): a chain from an untrusted root is parsed, recorded, accepted.
	r := k.register(u.d, u.si, chain.Base64, nil)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	att := r.json()["attestation"].(map[string]any)
	assert.Equal(t, "ANDROID_KEY", att["format"])
	assert.Equal(t, "UNTRUSTED", att["verdict"])
	assert.Equal(t, "TEE", att["securityLevel"])
	assert.Equal(t, true, att["authPerUse"])
	assert.Equal(t, true, att["challengeMatches"])
	assert.Equal(t, false, att["meetsRequirement"])

	// REQUIRED with the root trusted: a key without a chain is refused, one with
	// it is accepted and meets the requirement.
	e.s.Ops.AttestationRoots = roots
	e.policy("unlock_key_attestation", "REQUIRED")
	si := mustLogin(t, u.d, e.codeA, "uk-1", goodPIN)
	r = newUnlockKey(t).register(u.d, si, nil, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.Code, r.Body.String())
	assert.Equal(t, "ATTESTATION_REQUIRED", r.str("error"))

	sid = tokenPayload(t, si.access)["sid"].(string)
	k2 := newUnlockKey(t)
	desc := attestationtest.StrongBiometric(attestation.Challenge(sid, u.workerID, u.d.id))
	desc.Hardware.AuthTimeout = new(300) // not per use
	weak := attestationtest.Build(&k2.key.PublicKey, desc)
	e.s.Ops.AttestationRoots, err = attestation.ParseRootsPEM(append(chain.RootPEM, weak.RootPEM...))
	require.NoError(t, err)
	r = k2.register(u.d, si, weak.Base64, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.Code, "a trusted chain that allows a timeout is refused: %s", r.Body.String())

	k3 := newUnlockKey(t)
	good := attestationtest.Build(&k3.key.PublicKey, attestationtest.StrongBiometric(attestation.Challenge(sid, u.workerID, u.d.id)))
	e.s.Ops.AttestationRoots, err = attestation.ParseRootsPEM(good.RootPEM)
	require.NoError(t, err)
	r = k3.register(u.d, si, good.Base64, nil)
	require.Equal(t, http.StatusCreated, r.Code, r.Body.String())
	att = r.json()["attestation"].(map[string]any)
	assert.Equal(t, "TRUSTED", att["verdict"])
	assert.Equal(t, true, att["meetsRequirement"])
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE id = $1 AND attestation_verified`, k3.id))
	e.assertNoLeak(good.Base64[0])
}

// ── challenge and unlock refusals (spec sections 4.3 and 4.5) ────────────

func TestUnlockWrongKeyCountsAsAFailure(t *testing.T) {
	u, k := registered(t, nil)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	forged := &unlockKey{t: t, key: newUnlockKey(t).key, id: k.id}
	r := u.d.unlock(k.id, u.e.codeA, forged.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), nil))
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_UNLOCK_PROOF", r.str("error"))
	assert.Equal(t, 1, u.e.failures(u.workerID))
	assert.Equal(t, 1, u.e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND auth_method = 'UNLOCK_KEY' AND failure_reason = 'BAD_ASSERTION'`, u.workerID))
}

func TestUnlockAssertionMustNameEverything(t *testing.T) {
	cases := map[string]func(h, c map[string]any){
		"another app":      func(_, c map[string]any) { c["app"] = "other-app" },
		"another worker":   func(_, c map[string]any) { c["sub"] = "fwu_0SOMEONEELSE00" },
		"another device":   func(_, c map[string]any) { c["device_id"] = "fwd_0OTHERDEVICE00" },
		"another audience": func(_, c map[string]any) { c["aud"] = "https://other.example.test/field/auth/unlock" },
		"key proof typ":    func(h, _ map[string]any) { h["typ"] = KeyProofType },
		"no kid":           func(h, _ map[string]any) { delete(h, "kid") },
		"no iat":           func(_, c map[string]any) { delete(c, "iat") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			u, k := registered(t, nil)
			ch := u.d.challenge(k.id)
			require.Equal(t, http.StatusOK, ch.Code)
			r := u.d.unlock(k.id, u.e.codeA, k.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), mutate))
			assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
			assert.Equal(t, "INVALID_UNLOCK_PROOF", r.str("error"))
			assert.Equal(t, 1, u.e.failures(u.workerID))
		})
	}
}

func TestUnlockReplayedChallengeCountsAsAFailure(t *testing.T) {
	u, k := registered(t, nil)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	a := k.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), nil)
	require.Equal(t, http.StatusOK, u.d.unlock(k.id, u.e.codeA, a).Code)
	r := u.d.unlock(k.id, u.e.codeA, a)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_UNLOCK_PROOF", r.str("error"))
	assert.Equal(t, 1, u.e.failures(u.workerID), "the replay is one failure")
	assert.Equal(t, 1, u.e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND failure_reason = 'CHALLENGE_UNKNOWN'`, u.workerID))
}

func TestUnlockExpiredChallengeIsNotCounted(t *testing.T) {
	u, k := registered(t, nil)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	u.e.exec(`UPDATE field_unlock_challenges SET expires_at = NOW() - INTERVAL '1 second'`)
	r := u.d.unlock(k.id, u.e.codeA, k.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), nil))
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "CHALLENGE_EXPIRED", r.str("error"))
	assert.Zero(t, u.e.failures(u.workerID))
	assert.Zero(t, u.e.q(`SELECT COUNT(*) FROM field_unlock_challenges WHERE unlock_key_id = $1`, k.id), "consumed")
}

func TestUnlockChallengeOfAnotherKeyIsRefused(t *testing.T) {
	u, k := registered(t, nil)
	e := u.e
	// A second worker's key on a second device: its challenge cannot unlock k.
	e.worker("uk-2", goodPIN, false, e.codeA)
	d2 := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	k2 := newUnlockKey(t)
	require.Equal(t, http.StatusCreated, k2.register(d2, mustLogin(t, d2, e.codeA, "uk-2", goodPIN), nil, nil).Code)
	other := d2.challenge(k2.id)
	require.Equal(t, http.StatusOK, other.Code)
	r := u.d.unlock(k.id, e.codeA, k.assertion(u.d, u.workerID, e.codeA, other.str("challenge"), nil))
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_UNLOCK_PROOF", r.str("error"))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND failure_reason = 'CHALLENGE_FOREIGN'`, u.workerID))

	// k used from d2 (another device) is no key at all there.
	assert.Equal(t, "UNLOCK_KEY_INVALID", d2.challenge(k.id).str("error"))
	assert.Equal(t, "UNLOCK_KEY_INVALID", d2.unlock(k.id, e.codeA, "x.y.z").str("error"))
}

func TestUnlockRevokedAndUnknownKeys(t *testing.T) {
	u, k := registered(t, nil)
	r := u.d.call("/field/auth/unlock-keys/revoke", map[string]any{"unlockKeyId": k.id})
	require.Equal(t, http.StatusNoContent, r.Code, r.Body.String())
	_, reason := u.e.keyRevocation(k.id)
	assert.Equal(t, "DEVICE", reason)
	for _, id := range []string{k.id, "fwk_0UNKNOWNKEY000"} {
		r := u.d.challenge(id)
		assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		assert.Equal(t, "UNLOCK_KEY_INVALID", r.str("error"))
		r = u.d.unlock(id, u.e.codeA, k.assertion(u.d, u.workerID, u.e.codeA, "c", nil))
		assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		assert.Equal(t, "UNLOCK_KEY_INVALID", r.str("error"))
	}
	assert.Zero(t, u.e.failures(u.workerID), "no worker failure without a live key")
	// A revoke of an unknown or already revoked key is a quiet 204.
	assert.Equal(t, http.StatusNoContent, u.d.call("/field/auth/unlock-keys/revoke", map[string]any{"unlockKeyId": k.id}).Code)
	assert.Equal(t, http.StatusNoContent, u.d.call("/field/auth/unlock-keys/revoke", map[string]any{"unlockKeyId": "fwk_0NOPE"}).Code)
	assert.Equal(t, 1, u.e.eventCount(fieldidentity.UnlockKeyRevokedType))
}

func TestUnlockRevokedBetweenChallengeAndUnlock(t *testing.T) {
	u, k := registered(t, nil)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	u.e.exec(`UPDATE field_unlock_keys SET revoked_at = NOW(), revoked_reason = 'ADMIN' WHERE id = $1`, k.id)
	r := u.d.unlock(k.id, u.e.codeA, k.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), nil))
	assert.Equal(t, "UNLOCK_KEY_INVALID", r.str("error"), r.Body.String())
}

func TestUnlockForeignDeviceKeyRevokeChangesNothing(t *testing.T) {
	u, k := registered(t, nil)
	other := u.e.enrolled(fieldidentity.ModePersonal, u.e.codeA)
	assert.Equal(t, http.StatusNoContent, other.call("/field/auth/unlock-keys/revoke", map[string]any{"unlockKeyId": k.id}).Code)
	revoked, _ := u.e.keyRevocation(k.id)
	assert.False(t, revoked)
}

func TestUnlockOnSharedOrUnenrolledDevice(t *testing.T) {
	u, k := registered(t, nil)
	shared := u.e.enrolled(fieldidentity.ModeShared, u.e.codeA)
	un := u.e.newDevice()
	for _, path := range []string{"/field/auth/unlock/challenge", "/field/auth/unlock"} {
		body := map[string]any{"unlockKeyId": k.id, "appCode": u.e.codeA, "assertion": "x.y.z"}
		r := shared.call(path, body)
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "SHARED_DEVICE", r.str("error"))
		r = un.call(path, body)
		assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"))
	}
}

func TestUnlockPolicyOffRefusesEvenALiveKey(t *testing.T) {
	u, k := registered(t, nil)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	u.e.policy("unlock_keys_enabled", false) // a direct change: the key stays live
	assert.Equal(t, "UNLOCK_KEYS_DISABLED", u.d.challenge(k.id).str("error"))
	r := u.d.unlock(k.id, u.e.codeA, k.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), nil))
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "UNLOCK_KEYS_DISABLED", r.str("error"))
}

func TestUnlockDefaultForcesNoPINByTime(t *testing.T) {
	u, k := registered(t, nil)
	u.e.exec(`UPDATE field_unlock_keys SET pin_verified_at = NOW() - INTERVAL '400 days' WHERE id = $1`, k.id)
	r := k.fullUnlock(u.d, u.workerID, u.e.codeA)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	var abs time.Time
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT absolute_expires_at FROM field_sessions WHERE family_id = $1`, tokenPayload(t, r.str("accessToken"))["sid"]).Scan(&abs))
	assert.True(t, abs.After(time.Now().Add(13*time.Hour)), "no PIN deadline caps the session: the PERSONAL shift limit applies")
}

func TestUnlockPINIntervalBoundary(t *testing.T) {
	u, k := registered(t, nil)
	e := u.e
	e.policy("pin_reauth_days", 7)
	e.exec(`UPDATE field_unlock_keys SET pin_verified_at = NOW() - INTERVAL '7 days' + INTERVAL '2 minutes' WHERE id = $1`, k.id)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code, "inside the interval: %s", ch.Body.String())
	assert.NotEmpty(t, ch.str("pinRequiredBy"))
	r := u.d.unlock(k.id, e.codeA, k.assertion(u.d, u.workerID, e.codeA, ch.str("challenge"), nil))
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	var abs, pinVerified time.Time
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT s.absolute_expires_at, k.pin_verified_at FROM field_sessions s, field_unlock_keys k
		 WHERE s.family_id = $1 AND k.id = $2`, tokenPayload(t, r.str("accessToken"))["sid"], k.id).Scan(&abs, &pinVerified))
	assert.False(t, abs.After(pinVerified.Add(7*24*time.Hour)), "the session ends by the PIN deadline")

	e.exec(`UPDATE field_unlock_keys SET pin_verified_at = NOW() - INTERVAL '7 days' - INTERVAL '1 second' WHERE id = $1`, k.id)
	assert.Equal(t, "PIN_REAUTH_REQUIRED", u.d.challenge(k.id).str("error"), "past the interval")
}

func TestUnlockForcedPINDeadline(t *testing.T) {
	u, k := registered(t, nil)
	e := u.e
	e.policy("pin_reauth_days", 7)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	e.exec(`UPDATE field_unlock_keys SET pin_verified_at = NOW() - INTERVAL '8 days' WHERE id = $1`, k.id)

	r := u.d.challenge(k.id)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "PIN_REAUTH_REQUIRED", r.str("error"))
	r = u.d.unlock(k.id, e.codeA, k.assertion(u.d, u.workerID, e.codeA, ch.str("challenge"), nil))
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "PIN_REAUTH_REQUIRED", r.str("error"))
	revoked, _ := e.keyRevocation(k.id)
	assert.False(t, revoked, "the key is kept")
	assert.Zero(t, e.failures(u.workerID))

	// A longer pinReauthDays applies at once.
	e.policy("pin_reauth_days", 30)
	assert.Equal(t, http.StatusOK, u.d.challenge(k.id).Code)
	e.policy("pin_reauth_days", 7)

	// A PIN sign-in on the device restarts the clock; the key works again.
	mustLogin(t, u.d, e.codeA, "uk-1", goodPIN)
	r = k.fullUnlock(u.d, u.workerID, e.codeA)
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())

	// A pending PIN change is the same refusal.
	e.exec(`UPDATE field_identities SET pin_change_required = TRUE WHERE id = $1`, u.workerID)
	assert.Equal(t, "PIN_REAUTH_REQUIRED", u.d.challenge(k.id).str("error"))
}

func TestUnlockLocksAndLockout(t *testing.T) {
	u, k := registered(t, nil)
	e := u.e
	// A soft lock refuses before any signature is checked.
	e.exec(`UPDATE field_identities SET locked_until = NOW() + INTERVAL '5 minutes' WHERE id = $1`, u.workerID)
	r := u.d.challenge(k.id)
	assert.Equal(t, http.StatusTooManyRequests, r.Code)
	assert.Equal(t, "WORKER_LOCKED_TEMPORARILY", r.str("error"))
	assert.NotEmpty(t, r.Header().Get("Retry-After"))
	e.exec(`UPDATE field_identities SET locked_until = NULL WHERE id = $1`, u.workerID)

	// Bad assertions are failures: the soft lock at 5, then the hard lock at 10
	// revokes the key (WORKER_LOCKED) and emits the event.
	e.policy("soft_lock_secs", 30)
	forged := &unlockKey{t: t, key: newUnlockKey(t).key, id: k.id}
	for range 10 {
		e.exec(`UPDATE field_identities SET locked_until = NULL WHERE id = $1`, u.workerID)
		ch := u.d.challenge(k.id)
		if ch.Code != http.StatusOK {
			break
		}
		u.d.unlock(k.id, e.codeA, forged.assertion(u.d, u.workerID, e.codeA, ch.str("challenge"), nil))
	}
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND hard_locked`, u.workerID))
	revoked, reason := e.keyRevocation(k.id)
	assert.True(t, revoked)
	assert.Equal(t, "WORKER_LOCKED", reason)
	assert.Equal(t, 1, e.eventCount(fieldidentity.UnlockKeyRevokedType))
	assert.Equal(t, "UNLOCK_KEY_INVALID", u.d.challenge(k.id).str("error"))
}

func TestUnlockHardLockByWrongPINsRevokesTheKeys(t *testing.T) {
	u, k := registered(t, nil)
	e := u.e
	e.policy("soft_lock_threshold", 10)
	for range 10 {
		u.d.login(e.codeA, "uk-1", "000111")
	}
	revoked, reason := e.keyRevocation(k.id)
	assert.True(t, revoked)
	assert.Equal(t, "WORKER_LOCKED", reason)
}

func TestUnlockLockedWorkerIsRefused(t *testing.T) {
	u, k := registered(t, nil)
	ch := u.d.challenge(k.id)
	require.Equal(t, http.StatusOK, ch.Code)
	u.e.exec(`UPDATE field_identities SET hard_locked = TRUE WHERE id = $1`, u.workerID)
	r := u.d.unlock(k.id, u.e.codeA, k.assertion(u.d, u.workerID, u.e.codeA, ch.str("challenge"), nil))
	assert.Equal(t, http.StatusLocked, r.Code, r.Body.String())
	assert.Equal(t, "WORKER_LOCKED", r.str("error"))
	assert.Equal(t, "WORKER_LOCKED", u.d.challenge(k.id).str("error"))
}

func TestUnlockAccessChecksAfterTheSignature(t *testing.T) {
	u, k := registered(t, nil)
	e := u.e
	// An application that does not exist (or that the worker or device lacks): 403.
	r := k.fullUnlock(u.d, u.workerID, "no-such-app")
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "NO_APP_ACCESS", r.str("error"))

	// A grant removed keeps the key; that application is refused, the other works.
	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.RevokeWorkerApp(), fieldidentity.WorkerAppCommand{
		ClientID: e.clientID, ID: u.workerID, AppCode: e.codeB,
	}, auth.NewExecutionContext(e.actx))
	require.NoError(t, err)
	revoked, _ := e.keyRevocation(k.id)
	assert.False(t, revoked)
	assert.Equal(t, "NO_APP_ACCESS", k.fullUnlock(u.d, u.workerID, e.codeB).str("error"))
	assert.Equal(t, http.StatusOK, k.fullUnlock(u.d, u.workerID, e.codeA).Code)
	assert.Zero(t, e.failures(u.workerID))
}

func TestUnlockChallengeRateLimit(t *testing.T) {
	limits := roomyLimits()
	limits.FieldUnlockChallenge = ratelimit.Policy{Window: time.Hour, Limit: 2}
	u, k := registered(t, limits)
	assert.Equal(t, http.StatusOK, u.d.challenge(k.id).Code)
	assert.Equal(t, http.StatusOK, u.d.challenge(k.id).Code)
	r := u.d.challenge(k.id)
	assert.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())
	assert.Equal(t, "TOO_MANY_ATTEMPTS", r.str("error"))
	assert.NotEmpty(t, r.Header().Get("Retry-After"))
}

func TestUnlockThrottledDeviceIsRefused(t *testing.T) {
	limits := roomyLimits()
	limits.FieldPinDevice = ratelimit.Policy{Window: time.Hour, Limit: 2}
	u, k := registered(t, limits)
	e := u.e
	e.worker("uk-x", goodPIN, false, e.codeA)
	for range 2 {
		u.d.login(e.codeA, "uk-x", "000111")
	}
	assert.Equal(t, "TOO_MANY_ATTEMPTS", u.d.challenge(k.id).str("error"))
	assert.Equal(t, "TOO_MANY_ATTEMPTS", u.d.unlock(k.id, e.codeA, "x.y.z").str("error"))
}

// ── revocation triggers (spec section 7) ─────────────────────────────────

func TestUnlockKeyRevocationTriggers(t *testing.T) {
	runAdmin := func(t *testing.T, e *env, fn func(ctx context.Context) error) {
		t.Helper()
		require.NoError(t, fn(e.actx))
	}
	ref := func(e *env, id string) fieldidentity.WorkerRef {
		return fieldidentity.WorkerRef{ClientID: e.clientID, ID: id}
	}
	cases := map[string]struct {
		reason string
		act    func(t *testing.T, u *unlockEnv)
	}{
		"PIN reset": {"PIN_RESET", func(t *testing.T, u *unlockEnv) {
			runAdmin(t, u.e, func(ctx context.Context) error {
				_, err := usecaseop.RunTx(ctx, u.e.s.UoW, u.e.s.Ops.ResetWorkerPin(),
					fieldidentity.ResetPinCommand{WorkerRef: ref(u.e, u.workerID)}, auth.NewExecutionContext(ctx))
				return err
			})
		}},
		"worker suspended": {"WORKER_SUSPENDED", func(t *testing.T, u *unlockEnv) {
			runAdmin(t, u.e, func(ctx context.Context) error {
				_, err := usecaseop.RunTx(ctx, u.e.s.UoW, u.e.s.Ops.SuspendWorker(), ref(u.e, u.workerID), auth.NewExecutionContext(ctx))
				return err
			})
		}},
		"device revoked": {"DEVICE_REVOKED", func(t *testing.T, u *unlockEnv) {
			runAdmin(t, u.e, func(ctx context.Context) error {
				_, err := usecaseop.RunTx(ctx, u.e.s.UoW, u.e.s.Ops.RevokeDevice(),
					fieldidentity.DeviceRef{ClientID: u.e.clientID, ID: u.d.id}, auth.NewExecutionContext(ctx))
				return err
			})
		}},
		"policy switched off": {"POLICY_DISABLED", func(t *testing.T, u *unlockEnv) {
			runAdmin(t, u.e, func(ctx context.Context) error {
				_, err := usecaseop.RunTx(ctx, u.e.s.UoW, u.e.s.Ops.PutPolicy(),
					fieldidentity.PutPolicyCommand{ClientID: u.e.clientID}, auth.NewExecutionContext(ctx))
				return err
			})
		}},
		"administrator": {"ADMIN", func(t *testing.T, u *unlockEnv) {
			k, err := u.e.s.Ops.R.UnlockKeys.LiveByDevice(context.Background(), u.d.id)
			require.NoError(t, err)
			runAdmin(t, u.e, func(ctx context.Context) error {
				_, err := usecaseop.RunTx(ctx, u.e.s.UoW, u.e.s.Ops.RevokeUnlockKey(),
					fieldidentity.UnlockKeyRef{WorkerRef: ref(u.e, u.workerID), UnlockKeyID: k.ID}, auth.NewExecutionContext(ctx))
				return err
			})
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			u, k := registered(t, nil)
			tc.act(t, u)
			revoked, reason := u.e.keyRevocation(k.id)
			assert.True(t, revoked)
			assert.Equal(t, tc.reason, reason)
			assert.Equal(t, 1, u.e.eventCount(fieldidentity.UnlockKeyRevokedType))
		})
	}

	t.Run("device suspended keeps the key", func(t *testing.T) {
		u, k := registered(t, nil)
		_, err := usecaseop.RunTx(u.e.actx, u.e.s.UoW, u.e.s.Ops.SuspendDevice(),
			fieldidentity.DeviceRef{ClientID: u.e.clientID, ID: u.d.id}, auth.NewExecutionContext(u.e.actx))
		require.NoError(t, err)
		revoked, _ := u.e.keyRevocation(k.id)
		assert.False(t, revoked)
	})
	t.Run("worker deleted removes the keys", func(t *testing.T) {
		u, k := registered(t, nil)
		require.Equal(t, http.StatusOK, u.d.challenge(k.id).Code)
		_, err := usecaseop.RunTx(u.e.actx, u.e.s.UoW, u.e.s.Ops.DeleteWorker(), ref(u.e, u.workerID), auth.NewExecutionContext(u.e.actx))
		require.NoError(t, err)
		assert.Zero(t, u.e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE id = $1`, k.id))
		assert.Zero(t, u.e.q(`SELECT COUNT(*) FROM field_unlock_challenges WHERE unlock_key_id = $1`, k.id))
	})
	t.Run("PIN change keeps this device's key and revokes the others", func(t *testing.T) {
		u, here := registered(t, nil)
		e := u.e
		d2 := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		there := newUnlockKey(t)
		require.Equal(t, http.StatusCreated, there.register(d2, mustLogin(t, d2, e.codeA, "uk-1", goodPIN), nil, nil).Code)

		si := mustLogin(t, u.d, e.codeA, "uk-1", goodPIN)
		e.exec(`UPDATE field_unlock_keys SET pin_verified_at = NOW() - INTERVAL '3 days' WHERE id = $1`, here.id)
		r := u.d.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "730418"}, si.access)
		require.Equal(t, http.StatusNoContent, r.Code, r.Body.String())
		revoked, _ := e.keyRevocation(here.id)
		assert.False(t, revoked)
		assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_unlock_keys WHERE id = $1 AND pin_verified_at > NOW() - INTERVAL '1 minute'`, here.id))
		revoked, reason := e.keyRevocation(there.id)
		assert.True(t, revoked)
		assert.Equal(t, "PIN_CHANGED", reason)
	})
	t.Run("attestation required revokes the unverified keys", func(t *testing.T) {
		u, k := registered(t, nil)
		e := u.e
		roots, err := attestation.ParseRootsPEM(attestationtest.Build(&newUnlockKey(t).key.PublicKey,
			attestationtest.StrongBiometric(nil)).RootPEM)
		require.NoError(t, err)
		e.s.Ops.AttestationRoots = roots
		_, err = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.PutPolicy(), fieldidentity.PutPolicyCommand{
			ClientID: e.clientID, UnlockKeysEnabled: true, UnlockKeyAttestation: fieldidentity.UnlockAttestationRequired,
		}, auth.NewExecutionContext(e.actx))
		require.NoError(t, err)
		revoked, reason := e.keyRevocation(k.id)
		assert.True(t, revoked)
		assert.Equal(t, "POLICY_CHANGED", reason)
	})
}

func TestPolicyRequiringAttestationNeedsRoots(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.PutPolicy(), fieldidentity.PutPolicyCommand{
		ClientID: e.clientID, UnlockKeysEnabled: true, UnlockKeyAttestation: fieldidentity.UnlockAttestationRequired,
	}, auth.NewExecutionContext(e.actx))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "POLICY_INVALID")
}

// An unlock proves the key and a biometric, not the PIN: it leaves the PIN's
// failure counter as it is, so alternating unlocks with PIN guesses still
// reaches the hard lock; a PIN sign-in resets it.
func TestUnlockDoesNotResetThePINFailureCounter(t *testing.T) {
	u, k := registered(t, nil)
	for range 3 {
		require.Equal(t, http.StatusUnauthorized, u.d.login(u.e.codeA, "uk-1", "000999").Code)
	}
	require.Equal(t, 3, u.e.failures(u.workerID))
	r := k.fullUnlock(u.d, u.workerID, u.e.codeA)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, 3, u.e.failures(u.workerID), "the unlock verified no PIN")
	assert.Equal(t, 1, u.e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND last_login_at IS NOT NULL`, u.workerID))
	require.Equal(t, http.StatusOK, u.d.login(u.e.codeA, "uk-1", goodPIN).Code)
	assert.Zero(t, u.e.failures(u.workerID), "a PIN sign-in resets it")
}
