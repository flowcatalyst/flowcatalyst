package fieldauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Unlock keys (docs/field-unlock-keys-spec.md): a worker registers a key their
// PERSONAL phone keeps behind a strong biometric, and later signs in again by
// signing a challenge with it. Four routes: register (bearer), challenge,
// unlock and revoke (the enrolled device's proof).

func sharedDevice(c *call) {
	fail(c.w, http.StatusForbidden, fieldidentity.CodeSharedDevice, "unlock keys are not allowed on a shared device")
}

func unlockKeysDisabled(c *call) {
	fail(c.w, http.StatusForbidden, fieldidentity.CodeUnlockKeysDisabled, "unlock keys are switched off for this client")
}

func unlockKeyInvalid(c *call) {
	fail(c.w, http.StatusUnauthorized, fieldidentity.CodeUnlockKeyInvalid, "the unlock key is unknown or revoked; sign in with the PIN")
}

func invalidUnlockProof(c *call) {
	fail(c.w, http.StatusUnauthorized, fieldidentity.CodeInvalidUnlockProof, "the unlock signature does not verify; sign in with the PIN")
}

func pinReauthRequired(c *call) {
	fail(c.w, http.StatusUnauthorized, fieldidentity.CodePINReauthRequired, "sign in with the PIN on this device; the unlock key works again afterwards")
}

func workerLocked(c *call) {
	fail(c.w, http.StatusLocked, fieldidentity.CodeWorkerLocked, "this worker is locked; ask a supervisor to unlock them")
}

func workerSoftLocked(c *call, until, now time.Time) {
	retryAfter(c.w, uint32(until.Sub(now).Seconds())+1)
	fail(c.w, http.StatusTooManyRequests, fieldidentity.CodeWorkerLockedTemp, "too many failed attempts; try again later")
}

// personalDevice answers the refusal when the call is not from an enrolled
// PERSONAL device, reporting whether it may go on.
func personalDevice(c *call, refuse func(string)) bool {
	switch {
	case c.device == nil:
		refuse("UNKNOWN_DEVICE")
		unknownDevice(c)
		return false
	case c.device.Mode != fieldidentity.ModePersonal:
		refuse("SHARED_DEVICE")
		sharedDevice(c)
		return false
	}
	return true
}

// attestationJSON is the FieldUnlockKeyAttestation wire shape.
func attestationJSON(s attestation.Summary, verified bool) map[string]any {
	out := map[string]any{
		"format": string(s.Format), "verdict": string(s.Verdict), "challengeMatches": s.ChallengeMatches,
		"authPerUse": s.AuthPerUse, "biometricOnly": s.BiometricOnly, "signP256": s.SignP256,
		"meetsRequirement": verified,
	}
	if s.SecurityLevel != "" {
		out["securityLevel"] = string(s.SecurityLevel)
	}
	if s.AttestationSecurityLevel != "" {
		out["attestationSecurityLevel"] = string(s.AttestationSecurityLevel)
	}
	if s.RootFingerprint != "" {
		out["rootFingerprint"] = s.RootFingerprint
	}
	if s.AttestationVersion != 0 {
		out["attestationVersion"] = s.AttestationVersion
	}
	for k, v := range map[string]string{
		"provisioning": string(s.Provisioning), "dates": string(s.Dates), "datesProblem": s.DatesProblem,
		"revocation": string(s.Revocation),
	} {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// pinNeedsReset answers 403 PIN_RESET_REQUIRED for a worker whose PIN was set
// under a retired app key (or who has none), as the PIN sign-in does, and
// reports whether it did.
func (s *State) pinNeedsReset(c *call, ident *fieldidentity.Identity, refuse func(string)) bool {
	if !s.Ops.Hasher.Retired(ident.StoredPIN()) && ident.HasPIN() {
		return false
	}
	refuse("PIN_RESET_REQUIRED")
	pinResetRequired(c, ident.ID)
	return true
}

// attestationStanding is how a key stands against the policy's attestation
// rule at a challenge or an unlock.
type attestationStanding int

const (
	// standingOK: the rule admits the key (NONE, or REQUIRED and met).
	standingOK attestationStanding = iota
	// standingNotVerified: REQUIRED, and the key was registered without a
	// verified attestation (under the older policy).
	standingNotVerified
	// standingStatusUnavailable: REQUIRED with the status list on, and the
	// platform has no usable copy of it: the key cannot be checked now.
	standingStatusUnavailable
	// standingListed: a usable status list names a certificate of the key's
	// chain.
	standingListed
)

// keyStanding is the key's standing under pol at the call's time, with the
// listed serial for standingListed.
func (s *State) keyStanding(c *call, key *fieldidentity.UnlockKey, pol fieldidentity.Policy) (attestationStanding, string) {
	if pol.UnlockKeyAttestation == fieldidentity.UnlockAttestationNone {
		return standingOK, ""
	}
	// REQUIRED, or a rule this version does not know (read as REQUIRED, never
	// as NONE).
	if !key.AttestationVerified {
		return standingNotVerified, ""
	}
	if s.Ops.AttestationStatus == nil {
		return standingOK, "" // the operator switched the list off (FC_FIELD_ATTESTATION_STATUS_URL empty)
	}
	l, ok := s.Ops.AttestationStatus.Usable(c.now)
	if !ok {
		return standingStatusUnavailable, ""
	}
	if serial, listed := attestation.ListedSerial(l, key.Attestation.Serials); listed {
		return standingListed, serial
	}
	return standingOK, ""
}

// attestationStatusUnavailable is 503 ATTESTATION_STATUS_UNAVAILABLE: under
// REQUIRED the platform cannot check the key against the status list now. The
// key is not revoked; the library signs the worker in with the PIN for now and
// tries the key again later.
func attestationStatusUnavailable(c *call) {
	retryAfter(c.w, 300)
	fail(c.w, http.StatusServiceUnavailable, fieldidentity.CodeAttestationStatusUnavailable,
		"the attestation status list is unavailable; sign in with the PIN for now")
}

// keyMeetsPolicy answers the refusal for a key the policy's attestation rule
// does not admit now and reports whether the key may go on: one registered
// without a verified attestation is invalid (the library falls back to the
// PIN); one that cannot be checked because the status list is unavailable is
// 503 ATTESTATION_STATUS_UNAVAILABLE and stays registered; one whose chain the
// status list names is revoked (ATTESTATION_REVOKED, its sessions with it) and
// invalid.
func (s *State) keyMeetsPolicy(c *call, key *fieldidentity.UnlockKey, pol fieldidentity.Policy, refuse func(string)) bool {
	standing, serial := s.keyStanding(c, key, pol)
	return s.answerStanding(c, key, standing, serial, refuse)
}

// answerStanding answers the refusal for a standing already computed (so the
// answer and the decision to refuse read the status list once) and reports
// whether the key may go on.
func (s *State) answerStanding(c *call, key *fieldidentity.UnlockKey, standing attestationStanding, serial string, refuse func(string)) bool {
	switch standing {
	case standingOK:
		return true
	case standingNotVerified:
		refuse("ATTESTATION_NOT_VERIFIED")
		unlockKeyInvalid(c)
	case standingStatusUnavailable:
		refuse(fieldidentity.CodeAttestationStatusUnavailable)
		attestationStatusUnavailable(c)
	case standingListed:
		if _, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.RevokeUnlockKeyForAttestation(),
			fieldidentity.RevokeForAttestationCommand{UnlockKeyID: key.ID, Serial: serial},
			usecase.NewExecutionContext(fieldidentity.PrincipalAttestationStatus)); err != nil {
			s.internal(c, "revoke for attestation", err)
			return false
		}
		refuse("ATTESTATION_REVOKED")
		unlockKeyInvalid(c)
	}
	return false
}

// noteLockout emits field-unlock-key:revoked for the keys a hard lock reached
// in tx revoked, after its commit. Best effort: the revocation is committed.
func (s *State) noteLockout(c *call, tx *fieldidentity.AuthTx, ident *fieldidentity.Identity) {
	n := tx.UnlockKeysRevokedByLock()
	if n == 0 {
		return
	}
	if _, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.NoteUnlockKeysRevoked(), fieldidentity.UnlockKeysRevokedNotice{
		ClientID: ident.ClientID, WorkerID: ident.ID, Reason: fieldidentity.UnlockRevokedWorkerLocked, Count: n,
	}, usecase.NewExecutionContext(ident.ID)); err != nil {
		slog.Warn("field unlock-key revocation event failed", "worker", ident.ID, "err", err)
	}
}

// ── register ─────────────────────────────────────────────────────────────

type registerUnlockKeyRequest struct {
	PublicKeyJWK     json.RawMessage `json:"publicKeyJwk"`
	KeyProof         string          `json:"keyProof"`
	AttestationChain []string        `json:"attestationChain"`
}

// registerUnlockKey is POST /field/auth/unlock-keys (bearer). The order is the
// spec's section 2.1: the device and the token here, the key proof here, then
// the use case (the policy, the worker, the session's PIN sign-in, one key per
// device, the key's thumbprint, the attestation).
func (s *State) registerUnlockKey(c *call) {
	claims := c.claims
	attempt := fieldidentity.LoginAttempt{
		AuthMethod: fieldidentity.MethodUnlockKeyRegister, IdentityID: claims.Subject, WorkerCode: claims.WorkerCode,
		ClientID: claims.ClientID,
	}
	refuse := func(reason string) {
		attempt.Outcome, attempt.FailureReason = fieldidentity.OutcomeFailure, reason
		s.attempt(c, attempt)
	}
	if !personalDevice(c, refuse) {
		return
	}
	switch {
	case claims.PCR:
		refuse("PIN_CHANGE_REQUIRED")
		c.w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		fail(c.w, http.StatusUnauthorized, "invalid_token", "a PIN-change-only token cannot register an unlock key")
		return
	case claims.DeviceID != c.device.ID:
		refuse("SESSION_NOT_ON_DEVICE")
		fail(c.w, http.StatusForbidden, fieldidentity.CodePINSignInRequired, "sign in with the PIN on this device first")
		return
	case claims.Act != nil:
		refuse("IMPERSONATING")
		impersonating(c)
		return
	}
	pol, err := s.Ops.R.Policies.Effective(c.r.Context(), c.device.ClientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	if !pol.UnlockKeysEnabled {
		refuse("UNLOCK_KEYS_DISABLED")
		unlockKeysDisabled(c)
		return
	}
	var req registerUnlockKeyRequest
	if !c.decode(&req) {
		return
	}
	if len(req.PublicKeyJWK) == 0 || strings.TrimSpace(req.KeyProof) == "" {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "publicKeyJwk and keyProof are required")
		return
	}
	if len(req.AttestationChain) > attestation.MaxCertificates {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "attestationChain has too many certificates")
		return
	}
	pub, canonical, jkt, err := ParseECJWK(req.PublicKeyJWK)
	if err != nil {
		fail(c.w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", strings.TrimPrefix(err.Error(), ErrProof.Error()+": "))
		return
	}
	if jkt == c.device.JKT {
		fail(c.w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", "the unlock key must not be the device key")
		return
	}
	if _, err := VerifyUnlockJWS(req.KeyProof, pub, UnlockExpect{
		Type: KeyProofType, Aud: strings.TrimRight(s.BaseURL, "/") + unlockKeysPath, Nonce: c.proof.Nonce,
		Sub: claims.Subject, DeviceID: c.device.ID,
	}, c.now); err != nil {
		refuse("BAD_KEY_PROOF")
		invalidUnlockProof(c)
		return
	}
	res, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.RegisterUnlockKey(), fieldidentity.RegisterUnlockKeyCommand{
		WorkerID: claims.Subject, DeviceID: c.device.ID, SessionID: claims.SessionID, JKT: jkt,
		PublicKeyJWK: canonical, PublicKey: pub, AttestationChain: req.AttestationChain,
	}, usecase.NewExecutionContext(claims.Subject))
	if err != nil {
		ue := usecase.AsError(err)
		if ue != nil && ue.Code == fieldidentity.CodeAttestationStatusUnavailable {
			refuse(ue.Code)
			fail(c.w, http.StatusServiceUnavailable, ue.Code, ue.Message)
			return
		}
		if ue == nil || ue.Kind == usecase.KindInternal {
			useCaseError(c.w, err)
			return
		}
		refuse(ue.Code)
		if ue.Code == fieldidentity.CodeWorkerLockedTemp {
			if until, ok := ue.Details["lockedUntil"].(time.Time); ok {
				workerSoftLocked(c, until, c.now)
				return
			}
		}
		useCaseError(c.w, err)
		return
	}
	attempt.Outcome = fieldidentity.OutcomeSuccess
	s.attempt(c, attempt)
	body := map[string]any{
		"unlockKeyId": res.Key.ID, "deviceId": res.Key.DeviceID, "workerId": res.Key.IdentityID,
		"createdAt":   jsontime.New(res.Key.CreatedAt),
		"attestation": attestationJSON(res.Key.Attestation, res.Key.AttestationVerified),
	}
	if d, ok := res.Key.PINDeadline(res.Policy); ok {
		body["pinRequiredBy"] = jsontime.New(d)
	}
	if res.Replaced != "" {
		body["replacedUnlockKeyId"] = res.Replaced
	}
	writeJSON(c.w, http.StatusCreated, body)
}

// ── challenge ────────────────────────────────────────────────────────────

type unlockKeyRef struct {
	UnlockKeyID string `json:"unlockKeyId"`
}

// liveKeyOnDevice loads the named key when it is live and this device's.
func (s *State) liveKeyOnDevice(c *call, id string) (*fieldidentity.UnlockKey, bool) {
	k, err := s.Ops.R.UnlockKeys.FindByID(c.r.Context(), strings.TrimSpace(id))
	if err != nil {
		s.internal(c, "find unlock key", err)
		return nil, false
	}
	if k == nil || !k.Live() || k.DeviceID != c.device.ID {
		return nil, true
	}
	return k, true
}

// unlockChallenge is POST /field/auth/unlock/challenge (section 4.3): the
// library asks before it prompts for a biometric, so a key that cannot sign the
// worker in is refused before the worker is asked for a finger.
func (s *State) unlockChallenge(c *call) {
	if !personalDevice(c, func(string) {}) {
		return
	}
	ctx := c.r.Context()
	if pol := s.Limits.FieldUnlockChallenge; pol.Limit > 0 && s.RateLimit != nil {
		if rej := ratelimit.Enforce(ctx, s.RateLimit, ratelimit.BucketFieldUnlockChallenge, c.device.ID, pol); rej != nil {
			retryAfter(c.w, rej.RetryAfterSecs)
			fail(c.w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "too many unlock attempts; try again later")
			return
		}
	}
	if _, secs, hit := s.throttled(c, c.device.ClientID); hit {
		retryAfter(c.w, secs)
		fail(c.w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "too many failed sign-ins; try again later")
		return
	}
	var req unlockKeyRef
	if !c.decode(&req) {
		return
	}
	if strings.TrimSpace(req.UnlockKeyID) == "" {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "unlockKeyId is required")
		return
	}
	key, ok := s.liveKeyOnDevice(c, req.UnlockKeyID)
	if !ok {
		return
	}
	if key == nil {
		unlockKeyInvalid(c)
		return
	}
	pol, err := s.Ops.R.Policies.Effective(ctx, c.device.ClientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	if !pol.UnlockKeysEnabled {
		unlockKeysDisabled(c)
		return
	}
	ident, err := s.Ops.R.Identities.FindByID(ctx, key.IdentityID)
	if err != nil {
		s.internal(c, "find worker", err)
		return
	}
	switch {
	case ident == nil || ident.Status != fieldidentity.StatusActive:
		unlockKeyInvalid(c)
		return
	case ident.HardLocked:
		workerLocked(c)
		return
	case ident.LockedUntil != nil && ident.LockedUntil.After(c.now):
		workerSoftLocked(c, *ident.LockedUntil, c.now)
		return
	}
	if s.pinNeedsReset(c, ident, func(string) {}) || !s.keyMeetsPolicy(c, key, pol, func(string) {}) {
		return
	}
	if ident.PINChangeRequired || key.PINDue(pol, c.now) {
		pinReauthRequired(c)
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		s.internal(c, "challenge", err)
		return
	}
	challenge := base64.RawURLEncoding.EncodeToString(b)
	exp := c.now.Add(fieldidentity.UnlockChallengeTTL)
	if err := s.Ops.R.UnlockKeys.StoreChallenge(ctx, challenge, key.ID, c.device.ID, exp); err != nil {
		s.internal(c, "store challenge", err)
		return
	}
	body := map[string]any{"challenge": challenge, "expiresAt": jsontime.New(exp)}
	if d, ok := key.PINDeadline(pol); ok {
		body["pinRequiredBy"] = jsontime.New(d)
	}
	writeJSON(c.w, http.StatusOK, body)
}

// ── unlock ───────────────────────────────────────────────────────────────

type unlockRequest struct {
	UnlockKeyID string `json:"unlockKeyId"`
	AppCode     string `json:"appCode"`
	Assertion   string `json:"assertion"`
}

// unlock is POST /field/auth/unlock (section 4.5): a new session for the key's
// worker when the key signed a fresh challenge. A bad signature and a replayed
// challenge count as one failure of the worker, as a wrong PIN does.
func (s *State) unlock(c *call) {
	attempt := fieldidentity.LoginAttempt{AuthMethod: fieldidentity.MethodUnlockKey}
	refuse := func(reason string) {
		attempt.Outcome, attempt.FailureReason = fieldidentity.OutcomeFailure, reason
		s.attempt(c, attempt)
	}
	if !personalDevice(c, refuse) {
		return
	}
	clientID := c.device.ClientID
	if s.refuseThrottled(c, clientID, "too many failed sign-ins; try again later", refuse) {
		return
	}
	var req unlockRequest
	if !c.decode(&req) {
		return
	}
	req.UnlockKeyID, req.AppCode = strings.TrimSpace(req.UnlockKeyID), strings.TrimSpace(req.AppCode)
	if req.UnlockKeyID == "" || req.AppCode == "" || strings.TrimSpace(req.Assertion) == "" {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "unlockKeyId, appCode and assertion are required")
		return
	}
	ctx := c.r.Context()
	key, ok := s.liveKeyOnDevice(c, req.UnlockKeyID)
	if !ok {
		return
	}
	if key == nil {
		s.noteFailure(c, clientID)
		refuse("UNKNOWN_UNLOCK_KEY")
		unlockKeyInvalid(c)
		return
	}
	attempt.IdentityID = key.IdentityID
	pol, err := s.Ops.R.Policies.Effective(ctx, clientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	if !pol.UnlockKeysEnabled {
		refuse("UNLOCK_KEYS_DISABLED")
		unlockKeysDisabled(c)
		return
	}

	tx, err := s.Ops.R.Sessions.BeginAuth(ctx)
	if err != nil {
		s.internal(c, "begin", err)
		return
	}
	defer tx.Rollback(ctx)
	ident, err := tx.LockIdentityByID(ctx, key.IdentityID)
	if err != nil {
		s.internal(c, "lock identity", err)
		return
	}
	if ident == nil || ident.Status != fieldidentity.StatusActive {
		tx.Rollback(ctx)
		refuse("WORKER_UNAVAILABLE")
		unlockKeyInvalid(c)
		return
	}
	attempt.WorkerCode = ident.WorkerCode
	locked, err := tx.LockUnlockKey(ctx, key.ID)
	if err != nil {
		s.internal(c, "lock unlock key", err)
		return
	}
	if locked == nil || !locked.Live() {
		tx.Rollback(ctx)
		refuse("UNLOCK_KEY_REVOKED")
		unlockKeyInvalid(c)
		return
	}
	key = locked
	now := time.Now().UTC()
	if ident.HardLocked {
		tx.Rollback(ctx)
		refuse("WORKER_LOCKED")
		workerLocked(c)
		return
	}
	if ident.LockedUntil != nil && ident.LockedUntil.After(now) {
		tx.Rollback(ctx)
		refuse("WORKER_SOFT_LOCKED")
		workerSoftLocked(c, *ident.LockedUntil, now)
		return
	}
	// The rules that hold apart from the signature: a PIN a supervisor must
	// reset (the PIN sign-in refuses it, so the unlock does too), and the
	// policy's attestation rule (it is checked at each use, not only when the
	// policy changes). The transaction is ended first: a revocation the status
	// list causes is written by its own operation.
	// The standing is read once: a status fetch landing between a second read
	// and the decision could otherwise refuse with no answer.
	standing, serial := s.keyStanding(c, key, pol)
	if s.Ops.Hasher.Retired(ident.StoredPIN()) || !ident.HasPIN() || standing != standingOK {
		tx.Rollback(ctx)
		if !s.pinNeedsReset(c, ident, refuse) && s.answerStanding(c, key, standing, serial, refuse) {
			// Unreachable: the PIN needs no reset, so the standing refused.
			s.internal(c, "unlock refusal", errors.New("no refusal for a refused unlock"))
		}
		return
	}

	// failed counts the attempt against the worker and the buckets, commits it
	// and answers INVALID_UNLOCK_PROOF.
	failed := func(reason string) {
		if err := tx.RecordFailure(ctx, ident, pol, now); err != nil {
			s.internal(c, "record failure", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			s.internal(c, "commit failure", err)
			return
		}
		s.noteFailure(c, clientID)
		s.noteLockout(c, tx, ident)
		refuse(reason)
		invalidUnlockProof(c)
	}
	pub, _, _, err := ParseECJWK([]byte(key.PublicKeyJWK))
	if err != nil {
		s.internal(c, "stored unlock key", err)
		return
	}
	claims, err := VerifyUnlockJWS(req.Assertion, pub, UnlockExpect{
		Type: UnlockAssertionType, KeyID: key.ID, Aud: strings.TrimRight(s.BaseURL, "/") + unlockPath,
		Sub: ident.ID, DeviceID: c.device.ID, App: req.AppCode,
	}, now)
	if err != nil {
		failed("BAD_ASSERTION")
		return
	}
	ch, err := tx.ConsumeUnlockChallenge(ctx, claims.Nonce)
	if err != nil {
		s.internal(c, "consume challenge", err)
		return
	}
	switch {
	case ch == nil:
		failed("CHALLENGE_UNKNOWN")
		return
	case ch.UnlockKeyID != key.ID || ch.DeviceID != c.device.ID:
		failed("CHALLENGE_FOREIGN")
		return
	case !now.Before(ch.ExpiresAt):
		if err := tx.Commit(ctx); err != nil {
			s.internal(c, "commit challenge", err)
			return
		}
		refuse("CHALLENGE_EXPIRED")
		fail(c.w, http.StatusUnauthorized, fieldidentity.CodeChallengeExpired, "the challenge has expired; ask for a new one")
		return
	}
	// commitRefusal keeps the consumed challenge and answers the refusal.
	commitRefusal := func(reason string, answer func(*call)) {
		if err := tx.Commit(ctx); err != nil {
			s.internal(c, "commit challenge", err)
			return
		}
		refuse(reason)
		answer(c)
	}
	deadline, forced := key.PINDeadline(pol)
	if ident.PINChangeRequired || key.PINDue(pol, now) {
		commitRefusal("PIN_REAUTH_REQUIRED", pinReauthRequired)
		return
	}

	app, err := s.findApp(c, req.AppCode)
	if err != nil {
		s.internal(c, "application", err)
		return
	}
	dev := c.device
	reason := ""
	switch {
	case app == nil || !app.Active:
		reason = "APP_UNAVAILABLE"
	case !ident.HasApp(string(app.ID)):
		reason = "NO_GRANT"
	case !dev.HasApp(string(app.ID)):
		reason = "DEVICE_APP_NOT_ENROLLED"
	case dev.AssignedIdentityID != nil && *dev.AssignedIdentityID != ident.ID:
		reason = "DEVICE_ASSIGNED_TO_OTHER"
	}
	cl, err := s.activeClient(c, clientID)
	if err != nil {
		s.internal(c, "client", err)
		return
	}
	if reason == "" && cl == nil {
		reason = "CLIENT_INACTIVE"
	}
	if app != nil {
		attempt.ApplicationID = string(app.ID)
	}
	if reason != "" {
		commitRefusal(reason, noAppAccess)
		return
	}

	if err := tx.RecordSuccess(ctx, ident, nil, now); err != nil {
		s.internal(c, "record success", err)
		return
	}
	token, hash, err := newRefreshToken()
	if err != nil {
		s.internal(c, "refresh token", err)
		return
	}
	at := now.Truncate(time.Microsecond)
	sess := fieldidentity.NewSession(at, ident, string(app.ID), dev, c.proof.JKT, hash, pol)
	sess.AMR = append([]string(nil), fieldidentity.UnlockAMR...)
	sess.UnlockKeyID = key.ID
	if forced {
		sess.CapAbsolute(deadline)
	}
	if err := tx.InsertSession(ctx, sess); err != nil {
		s.internal(c, "insert session", err)
		return
	}
	if err := tx.TouchUnlockKeyUsed(ctx, key.ID, at); err != nil {
		s.internal(c, "touch unlock key", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(c, "commit unlock", err)
		return
	}
	attempt.Outcome = fieldidentity.OutcomeSuccess
	s.attempt(c, attempt)
	s.issue(c, ident, cl, app, sess, token, nil)
}

// ── revoke from the device ───────────────────────────────────────────────

// revokeUnlockKey is POST /field/auth/unlock-keys/revoke: any holder of the
// device key may revoke a key on the device. Unknown, foreign and revoked keys
// are a quiet 204, as logout.
func (s *State) revokeUnlockKey(c *call) {
	if c.device == nil {
		unknownDevice(c)
		return
	}
	var req unlockKeyRef
	if !c.decode(&req) {
		return
	}
	if strings.TrimSpace(req.UnlockKeyID) == "" {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "unlockKeyId is required")
		return
	}
	// The revocation is audited as the key's worker (the device acts for them);
	// a key that is not this device's changes nothing whoever is named.
	principal := fieldidentity.PrincipalDevice
	if k, err := s.Ops.R.UnlockKeys.FindByID(c.r.Context(), strings.TrimSpace(req.UnlockKeyID)); err != nil {
		s.internal(c, "find unlock key", err)
		return
	} else if k != nil && k.DeviceID == c.device.ID {
		principal = k.IdentityID
	}
	if _, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.RevokeOwnUnlockKey(), fieldidentity.RevokeOwnUnlockKeyCommand{
		DeviceID: c.device.ID, UnlockKeyID: req.UnlockKeyID,
	}, usecase.NewExecutionContext(principal)); err != nil {
		useCaseError(c.w, err)
		return
	}
	c.w.WriteHeader(http.StatusNoContent)
}
