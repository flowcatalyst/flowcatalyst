package fieldidentity

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// RegisterUnlockKeyCommand registers a worker's unlock key on the device the
// protocol layer authenticated (docs/field-unlock-keys-spec.md section 4.1).
// The protocol layer has verified the DPoP proof, the access token and the key
// proof; the key material is never audited.
type RegisterUnlockKeyCommand struct {
	WorkerID string `json:"workerId"`
	DeviceID string `json:"deviceId"`
	// SessionID is the access token's sid: the login family that registers.
	SessionID        string           `json:"sessionId"`
	JKT              string           `json:"-"`
	PublicKeyJWK     string           `json:"-"`
	PublicKey        *ecdsa.PublicKey `json:"-"`
	AttestationChain []string         `json:"-"`
}

// RegisteredUnlockKey is the stored key and the key it replaced ("" when none).
type RegisteredUnlockKey struct {
	Key      *UnlockKey
	Replaced string
	Policy   Policy
}

// RegisterUnlockKey stores a worker's unlock key: the client allows unlock
// keys, the device is an ACTIVE PERSONAL one, the worker is ACTIVE, unlocked
// and has no pending PIN change, the registering session is the worker's own
// PIN sign-in on this device within [UnlockKeyRegistrationWindow], no other
// worker holds a key on the device, and the attestation meets the policy. The
// worker's own previous key on the device is revoked REPLACED in the same
// transaction.
func (o *Ops) RegisterUnlockKey() usecaseop.TxOperation[RegisterUnlockKeyCommand, RegisteredUnlockKey] {
	return usecaseop.TxOperation[RegisterUnlockKeyCommand, RegisteredUnlockKey]{
		Name: "RegisterFieldUnlockKey",
		Validate: func(_ context.Context, cmd RegisterUnlockKeyCommand) error {
			if cmd.WorkerID == "" || cmd.DeviceID == "" || cmd.SessionID == "" || cmd.JKT == "" ||
				cmd.PublicKeyJWK == "" || cmd.PublicKey == nil {
				return usecase.Validation("INVALID_REQUEST", "the worker, device, session and key are required")
			}
			if len(cmd.AttestationChain) > attestation.MaxCertificates {
				return usecase.Validation("INVALID_REQUEST", "attestationChain has too many certificates")
			}
			return nil
		},
		// The protocol layer authenticated the worker (bearer + proof + key proof).
		Authorize: usecaseop.Public[RegisterUnlockKeyCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RegisterUnlockKeyCommand, ec usecase.ExecutionContext) (RegisteredUnlockKey, error) {
			var zero RegisteredUnlockKey
			now := time.Now().UTC()
			dev, err := o.R.Devices.FindByID(ctx, cmd.DeviceID)
			if err != nil {
				return zero, repoErr("find_field_device", err)
			}
			switch {
			case dev == nil || dev.Status != DeviceActive:
				return zero, usecase.Authorization(CodeUnknownDevice, "this device is not enrolled")
			case dev.Mode != ModePersonal:
				return zero, usecase.Authorization(CodeSharedDevice, "unlock keys are not allowed on a shared device")
			case dev.JKT == cmd.JKT:
				return zero, usecase.Validation("INVALID_PUBLIC_KEY", "the unlock key must not be the device key")
			}
			// The client's policy lock first: a policy change waits for this
			// registration, or this registration for it, so the policy read below
			// is the one in force when the key is stored.
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				return o.R.UnlockKeys.LockClientPolicy(ctx, tx, dev.ClientID)
			}); err != nil {
				return zero, repoErr("lock_field_policy", err)
			}
			pol, err := o.R.Policies.Effective(ctx, dev.ClientID)
			if err != nil {
				return zero, repoErr("find_field_policy", err)
			}
			if !pol.UnlockKeysEnabled {
				return zero, usecase.Authorization(CodeUnlockKeysDisabled, "unlock keys are switched off for this client")
			}
			// Serialise registrations of the worker, and against a suspension or a
			// PIN reset of the worker (which revoke its keys).
			// (lock_order.go: the policy lock, then the worker.)
			if err := o.lockWorkers(ctx, s, cmd.WorkerID); err != nil {
				return zero, err
			}
			ident, err := o.R.Identities.FindByID(ctx, cmd.WorkerID)
			if err != nil {
				return zero, repoErr("find_field_worker", err)
			}
			if ident == nil || ident.ClientID != dev.ClientID || ident.Status != StatusActive {
				return zero, usecase.Authorization(CodeWorkerUnavailable, "the worker cannot register an unlock key")
			}
			if ident.HardLocked {
				return zero, usecase.Authorization(CodeWorkerLocked, "this worker is locked; ask a supervisor to unlock them")
			}
			if ident.LockedUntil != nil && ident.LockedUntil.After(now) {
				return zero, usecase.Authorization(CodeWorkerLockedTemp, "too many failed attempts; try again later").
					WithDetails(map[string]any{"lockedUntil": *ident.LockedUntil})
			}
			if ident.PINChangeRequired {
				return zero, usecase.Authorization(CodePINSignInRequired, "the worker must set their own PIN first")
			}
			sess, err := o.R.Sessions.FamilyHead(ctx, cmd.SessionID)
			if err != nil {
				return zero, repoErr("find_field_session", err)
			}
			deadline, canRegister := RegistrationDeadline(sess, dev.ID)
			if sess == nil || sess.IdentityID != ident.ID || !canRegister || now.After(deadline) {
				return zero, usecase.Authorization(CodePINSignInRequired,
					"an unlock key can be registered only just after signing in with the PIN on this device")
			}
			live, err := o.R.UnlockKeys.LiveByDevice(ctx, dev.ID)
			if err != nil {
				return zero, repoErr("find_field_unlock_key", err)
			}
			replaces := ""
			if live != nil {
				if live.IdentityID != ident.ID {
					return zero, usecase.Conflict(CodeUnlockKeyOtherWorker, "another worker holds this device's unlock key")
				}
				replaces = live.ID
			}
			if taken, err := o.R.UnlockKeys.ExistsByJKT(ctx, cmd.JKT); err != nil {
				return zero, repoErr("find_field_unlock_key", err)
			} else if taken {
				return zero, usecase.Conflict(CodeUnlockKeyExists, "this unlock key is already registered")
			}
			summary := attestation.Evaluate(cmd.AttestationChain, cmd.PublicKey,
				attestation.Challenge(sess.FamilyID, ident.ID, dev.ID), o.attestationOptions(now))
			verified := summary.MeetsRequirement()
			if pol.UnlockKeyAttestation == UnlockAttestationRequired && !verified {
				if summary.Revocation == attestation.RevocationUnavailable {
					checked := summary
					checked.Revocation = attestation.RevocationClear
					if checked.MeetsRequirement() {
						return zero, usecase.Internal(CodeAttestationStatusUnavailable,
							"the attestation key status list is not available, so an attested key cannot be registered now; sign in with the PIN", nil)
					}
				}
				return zero, usecase.Validation(CodeAttestationRequired,
					"this client requires a trusted hardware attestation of the unlock key: "+summary.Shortfall())
			}
			chain := ""
			if len(cmd.AttestationChain) > 0 {
				raw, err := json.Marshal(cmd.AttestationChain)
				if err != nil {
					return zero, usecase.Internal("ATTESTATION_ENCODE", "could not store the attestation chain", err)
				}
				chain = string(raw)
			}
			key := NewUnlockKey(ident, dev, cmd.JKT, cmd.PublicKeyJWK, chain, summary, verified,
				sess.AuthTime, now.Truncate(time.Microsecond), replaces)
			data := map[string]any{
				"unlockKeyId": key.ID, "workerId": ident.ID, "clientId": ident.ClientID, "workerCode": ident.WorkerCode,
				"deviceId": dev.ID, "attestationVerdict": string(summary.Verdict),
			}
			if replaces != "" {
				data["replacedUnlockKeyId"] = replaces
			}
			if summary.SecurityLevel != "" {
				data["securityLevel"] = string(summary.SecurityLevel)
			}
			ev := NewEvent(ec, UnlockKeyRegisteredType, "field-unlock-key", key.ID, data)
			if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, key, o.R.UnlockKeys, ev, cmd)); err != nil {
				switch {
				case errors.Is(err, errUnlockKeyTaken):
					return zero, usecase.Conflict(CodeUnlockKeyOtherWorker, "another worker holds this device's unlock key")
				case errors.Is(err, errUnlockKeyRegistered):
					return zero, usecase.Conflict(CodeUnlockKeyExists, "this unlock key is already registered")
				}
				return zero, err
			}
			return RegisteredUnlockKey{Key: key, Replaced: replaces, Policy: pol}, nil
		},
	}
}
