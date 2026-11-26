package fieldidentity

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Unlock keys (docs/field-unlock-keys-spec.md): a key pair a worker's PERSONAL
// phone made in its secure hardware, usable only after a strong biometric. The
// platform holds the public half and signs the worker in again when the key
// signs a fresh challenge.

// UnlockKeyRevokeReason is why an unlock key ended (field_unlock_keys.revoked_reason).
type UnlockKeyRevokeReason string

const (
	// UnlockRevokedReplaced: the same worker registered a new key on the device.
	UnlockRevokedReplaced UnlockKeyRevokeReason = "REPLACED"
	// UnlockRevokedPINChanged: the worker changed their PIN on another device.
	UnlockRevokedPINChanged UnlockKeyRevokeReason = "PIN_CHANGED"
	// UnlockRevokedPINReset: an administrator reset the worker's PIN.
	UnlockRevokedPINReset UnlockKeyRevokeReason = "PIN_RESET"
	// UnlockRevokedWorkerSuspended: the worker was suspended.
	UnlockRevokedWorkerSuspended UnlockKeyRevokeReason = "WORKER_SUSPENDED"
	// UnlockRevokedWorkerLocked: the worker reached the hard lock.
	UnlockRevokedWorkerLocked UnlockKeyRevokeReason = "WORKER_LOCKED"
	// UnlockRevokedDeviceRevoked: the device was revoked.
	UnlockRevokedDeviceRevoked UnlockKeyRevokeReason = "DEVICE_REVOKED"
	// UnlockRevokedPolicyDisabled: the client switched unlock keys off.
	UnlockRevokedPolicyDisabled UnlockKeyRevokeReason = "POLICY_DISABLED"
	// UnlockRevokedPolicyChanged: the client started requiring an attestation
	// the key did not meet.
	UnlockRevokedPolicyChanged UnlockKeyRevokeReason = "POLICY_CHANGED"
	// UnlockRevokedAdmin: an administrator revoked the key.
	UnlockRevokedAdmin UnlockKeyRevokeReason = "ADMIN"
	// UnlockRevokedDevice: the device (the worker or the library) revoked it.
	UnlockRevokedDevice UnlockKeyRevokeReason = "DEVICE"
	// UnlockRevokedAttestationRevoked: the attestation key status list names a
	// certificate of the key's chain (under the policy's REQUIRED).
	UnlockRevokedAttestationRevoked UnlockKeyRevokeReason = "ATTESTATION_REVOKED"
)

// EndsSessions reports whether revoking a key for r also ends the live
// sessions the key signed in, whenever they began and whether or not the key
// was still live (docs/field-unlock-keys-spec.md section 7): an
// administrator's revoke, the policy's and the attestation status list's do.
// The other reasons leave the sessions to the operation that revoked the key:
// a PIN reset, a suspension, a device's revocation and a PIN change on another
// device end them by worker or by device themselves; a replacement (the same
// worker on the same phone, who has just proved their PIN there), the
// worker's own revoke from the phone (switching the feature off, not signing
// out) and a hard lock (which ends no PIN session either) keep them. A refresh
// of a session whose key was revoked for a reason that ends sessions is
// refused.
func (r UnlockKeyRevokeReason) EndsSessions() bool {
	switch r {
	case UnlockRevokedAdmin, UnlockRevokedPolicyDisabled, UnlockRevokedPolicyChanged, UnlockRevokedAttestationRevoked:
		return true
	case UnlockRevokedReplaced, UnlockRevokedPINChanged, UnlockRevokedPINReset, UnlockRevokedWorkerSuspended,
		UnlockRevokedWorkerLocked, UnlockRevokedDeviceRevoked, UnlockRevokedDevice:
		return false
	}
	return true // a reason this version does not know: end, never keep
}

// RegistrationDeadline is the last moment the session may register an unlock
// key on the device (docs/field-unlock-keys-spec.md D1): its PIN sign-in plus
// [UnlockKeyRegistrationWindow], for the worker's own session signed in with
// the PIN on that device. It reports false for a session that can never
// register (another device or none, an act-as or console sign-in code
// session, a session an unlock or a badge alone began). Refreshes keep the
// sign-in's time, so every token of the session gives the same deadline.
func RegistrationDeadline(s *Session, deviceID string) (time.Time, bool) {
	if s == nil || deviceID == "" || s.DeviceID != deviceID || s.Impersonated() || !SignedInWithPIN(s.AMR) {
		return time.Time{}, false
	}
	return s.AuthTime.Add(UnlockKeyRegistrationWindow), true
}

// UnlockKeyAttestationRule is the policy's unlockKeyAttestation.
type UnlockKeyAttestationRule string

const (
	// UnlockAttestationNone records the attestation and enforces nothing (the
	// default).
	UnlockAttestationNone UnlockKeyAttestationRule = "NONE"
	// UnlockAttestationRequired refuses a key whose attestation does not meet
	// [attestation.Summary.MeetsRequirement].
	UnlockAttestationRequired UnlockKeyAttestationRule = "REQUIRED"
)

// Valid reports whether r is a known rule.
func (r UnlockKeyAttestationRule) Valid() bool {
	switch r {
	case UnlockAttestationNone, UnlockAttestationRequired:
		return true
	}
	return false
}

// Sign-in methods of the unlock-key routes (field_login_attempts.auth_method).
const (
	MethodUnlockKey         = "UNLOCK_KEY"
	MethodUnlockKeyRegister = "UNLOCK_KEY_REGISTER"
)

// Error codes of the unlock-key routes (docs/field-unlock-keys-spec.md section 10).
const (
	CodeSharedDevice         = "SHARED_DEVICE"
	CodeUnlockKeysDisabled   = "UNLOCK_KEYS_DISABLED"
	CodePINSignInRequired    = "PIN_SIGN_IN_REQUIRED"
	CodeUnlockKeyOtherWorker = "UNLOCK_KEY_OTHER_WORKER"
	CodeUnlockKeyExists      = "UNLOCK_KEY_EXISTS"
	CodeInvalidUnlockProof   = "INVALID_UNLOCK_PROOF"
	CodeUnlockKeyInvalid     = "UNLOCK_KEY_INVALID"
	CodeChallengeExpired     = "CHALLENGE_EXPIRED"
	CodePINReauthRequired    = "PIN_REAUTH_REQUIRED"
	CodeWorkerLocked         = "WORKER_LOCKED"
	CodeWorkerLockedTemp     = "WORKER_LOCKED_TEMPORARILY"
	// CodeAttestationStatusUnavailable: under REQUIRED, no usable copy of the
	// attestation key status list (never fetched, or older than its maximum age).
	CodeAttestationStatusUnavailable = "ATTESTATION_STATUS_UNAVAILABLE"
	// CodePINResetRequired: the worker's PIN was set under a retired app key; a
	// supervisor must reset it (as the PIN sign-in answers).
	CodePINResetRequired = "PIN_RESET_REQUIRED"
)

// PrincipalDevice is the execution-context principal of a device acting with
// no worker known (a revoke naming a key that is not the device's).
const PrincipalDevice = "field:device"

// UnlockAMR is the amr of a session signed in by an unlock key: a hardware-held
// key and user verification (docs/field-worker-identity-design.md section 3.4).
var UnlockAMR = []string{"hwk", "uv"}

// UnlockKeyRegistrationWindow is how long after a PIN sign-in its session may
// register an unlock key (docs/field-unlock-keys-spec.md D1).
const UnlockKeyRegistrationWindow = 10 * time.Minute

// UnlockChallengeTTL is how long an unlock challenge can be used (once).
const UnlockChallengeTTL = 5 * time.Minute

// UnlockKey is one registered unlock key of a worker on a device.
type UnlockKey struct {
	ID         string
	ClientID   string
	IdentityID string
	DeviceID   string
	JKT        string
	// PublicKeyJWK is the canonical public JWK; AttestationChain the chain as
	// sent ("" when none). Neither is ever on the wire, in an event or a log.
	PublicKeyJWK     string
	AttestationChain string
	// Attestation is the parsed summary; AttestationVerified whether it met the
	// REQUIRED rule when the key was registered.
	Attestation         attestation.Summary
	AttestationVerified bool
	// PINVerifiedAt is the last time the worker proved their PIN on the device.
	PINVerifiedAt time.Time
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	RevokedAt     *time.Time
	RevokedReason UnlockKeyRevokeReason
	RevokedBy     string

	fresh bool
	// replaces is the worker's previous live key on the device, revoked
	// REPLACED by Persist before the new key is inserted.
	replaces string
}

// IDStr satisfies usecase.HasID.
func (k UnlockKey) IDStr() string { return k.ID }

// Live reports whether the key has not been revoked.
func (k *UnlockKey) Live() bool { return k.RevokedAt == nil }

// PINDeadline is when the worker must prove their PIN again on the device
// before the key signs them in: PINVerifiedAt plus the policy's pinReauthDays.
// ok is false when the policy forces no periodic PIN (pinReauthDays 0, the
// default; docs/field-unlock-keys-spec.md section 4.4).
func (k *UnlockKey) PINDeadline(p Policy) (deadline time.Time, ok bool) {
	if p.PINReauthDays <= 0 {
		return time.Time{}, false
	}
	return k.PINVerifiedAt.Add(time.Duration(p.PINReauthDays) * 24 * time.Hour), true
}

// PINDue reports whether the policy's periodic PIN is due at now.
func (k *UnlockKey) PINDue(p Policy, now time.Time) bool {
	d, ok := k.PINDeadline(p)
	return ok && !now.Before(d)
}

// NewUnlockKey builds a key to register. replaces is the worker's own live
// key on the device ("" when none).
func NewUnlockKey(ident *Identity, dev *Device, jkt, jwk, chain string, summary attestation.Summary,
	verified bool, pinVerifiedAt, now time.Time, replaces string,
) *UnlockKey {
	return &UnlockKey{
		ID: tsid.Generate(tsid.FieldUnlockKey), ClientID: ident.ClientID, IdentityID: ident.ID, DeviceID: dev.ID,
		JKT: jkt, PublicKeyJWK: jwk, AttestationChain: chain, Attestation: summary, AttestationVerified: verified,
		PINVerifiedAt: pinVerifiedAt, CreatedAt: now, fresh: true, replaces: replaces,
	}
}

// Revoke marks the key revoked for reason by who (a principal id, or the
// operation), reporting whether it was live.
func (k *UnlockKey) Revoke(reason UnlockKeyRevokeReason, by string, at time.Time) bool {
	if !k.Live() {
		return false
	}
	k.RevokedAt, k.RevokedReason, k.RevokedBy = &at, reason, by
	return true
}

// SignedInWithPIN reports whether a session's amr is a PIN sign-in by the
// worker themself: ["pin"] or ["badge","pin"], nothing more (no unlock, no
// impersonation, no badge alone).
func SignedInWithPIN(amr []string) bool {
	switch len(amr) {
	case 1:
		return amr[0] == "pin"
	case 2:
		return amr[0] == "badge" && amr[1] == "pin"
	}
	return false
}
