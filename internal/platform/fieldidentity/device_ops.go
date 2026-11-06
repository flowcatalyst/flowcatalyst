package fieldidentity

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Error codes the device protocol (fieldauth) maps to wire responses.
const (
	CodeInvalidEnrolmentCode = "INVALID_ENROLMENT_CODE"
	CodeDeviceKeyExists      = "DEVICE_KEY_EXISTS"
	CodeAttestationRequired  = "ATTESTATION_REQUIRED"
	CodeClientInactive       = "CLIENT_INACTIVE"
	CodeWorkerUnavailable    = "WORKER_UNAVAILABLE"
	CodeInvalidCredentials   = "INVALID_CREDENTIALS"
)

// PrincipalEnrolment is the execution-context principal of an enrolment: the
// caller is a device holding a code, not a platform principal.
const PrincipalEnrolment = "field:enrolment"

// ── enrol a device ───────────────────────────────────────────────────────

// EnrolDeviceCommand turns an enrolment code and a device key into an enrolled
// device. Code and the raw attestation chain are never audited.
type EnrolDeviceCommand struct {
	Code             string `json:"-"`
	JKT              string `json:"jkt"`
	PublicKeyJWK     string `json:"-"`
	AttestationChain string `json:"-"`
	Platform         string `json:"platform"`
	Model            string `json:"model"`
	Label            string `json:"label,omitempty"`
}

// EnrolDeviceResult describes the new device.
type EnrolDeviceResult struct {
	DeviceID string
	ClientID string
	Mode     Mode
	AppCodes []string
}

// EnrolDevice consumes a single-use enrolment code and registers the device
// key against the code's client, mode and applications. The code, the device
// and the enrolled event commit together: a failure leaves the code unused.
func (o *Ops) EnrolDevice() usecaseop.TxOperation[EnrolDeviceCommand, EnrolDeviceResult] {
	return usecaseop.TxOperation[EnrolDeviceCommand, EnrolDeviceResult]{
		Name: "EnrolFieldDevice",
		Validate: func(_ context.Context, cmd EnrolDeviceCommand) error {
			if strings.TrimSpace(cmd.Code) == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			if cmd.JKT == "" || cmd.PublicKeyJWK == "" {
				return usecase.Validation("PUBLIC_KEY_REQUIRED", "publicKeyJwk is required")
			}
			return nil
		},
		// The enrolment code is the credential; there is no principal.
		Authorize: usecaseop.Public[EnrolDeviceCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd EnrolDeviceCommand, ec usecase.ExecutionContext) (EnrolDeviceResult, error) {
			var zero EnrolDeviceResult
			if existing, err := o.R.Devices.FindByJKT(ctx, cmd.JKT); err != nil {
				return zero, repoErr("find_field_device", err)
			} else if existing != nil {
				return zero, usecase.Conflict(CodeDeviceKeyExists, "this device key is already enrolled")
			}
			var code *EnrolmentCode
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				c, err := o.R.Codes.Consume(ctx, usecasepgx.WrapTxForBootstrap(tx), HashToken(strings.ToUpper(strings.TrimSpace(cmd.Code))))
				code = c
				return err
			}); err != nil {
				return zero, repoErr("consume_enrolment_code", err)
			}
			if code == nil {
				return zero, usecase.Authorization(CodeInvalidEnrolmentCode, "the enrolment code is invalid, used or expired")
			}
			cl, err := o.Clients.FindByID(ctx, code.ClientID)
			if err != nil {
				return zero, repoErr("find_client", err)
			}
			if cl == nil || cl.Status != client.StatusActive {
				return zero, usecase.Authorization(CodeClientInactive, "the client is not active")
			}
			pol, err := o.R.Policies.Effective(ctx, code.ClientID)
			if err != nil {
				return zero, repoErr("find_field_policy", err)
			}
			if pol.RequireAttestation {
				// P1 cannot verify an attestation chain, so a client that
				// requires one cannot enrol devices yet.
				return zero, usecase.Validation(CodeAttestationRequired, "this client requires device attestation, which is not supported yet")
			}
			level := "NONE"
			if strings.TrimSpace(cmd.AttestationChain) != "" {
				level = "SOFTWARE" // reported, unverified
			}
			now := time.Now().UTC()
			dev := &Device{
				ID: tsid.Generate(tsid.FieldDevice), ClientID: code.ClientID, Mode: code.Mode,
				Label: firstNonEmpty(cmd.Label, code.Label), Platform: cmd.Platform, Model: cmd.Model,
				JKT: cmd.JKT, PublicKeyJWK: cmd.PublicKeyJWK, AttestationLevel: level,
				AttestationChain: cmd.AttestationChain, AssignedIdentityID: code.AssignedIdentityID,
				Status: DeviceActive, EnrolledBy: code.CreatedBy, EnrolledAt: now, UpdatedAt: now,
				AppIDs: code.AppIDs,
			}
			codes := make([]string, 0, len(code.AppIDs))
			for _, id := range code.AppIDs {
				a, err := o.Apps.FindByID(ctx, id)
				if err != nil {
					return zero, repoErr("find_application", err)
				}
				if a != nil {
					codes = append(codes, a.Code)
				}
			}
			ev := NewEvent(ec, DeviceEnrolledType, "field-device", dev.ID,
				deviceData(dev, map[string]any{"applicationIds": dev.AppIDs}))
			if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, dev, o.R.Devices, ev, cmd)); err != nil {
				return zero, err
			}
			return EnrolDeviceResult{DeviceID: dev.ID, ClientID: dev.ClientID, Mode: dev.Mode, AppCodes: codes}, nil
		},
	}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return b
}

// ── change own PIN ───────────────────────────────────────────────────────

// ChangeOwnPinCommand sets a worker's own new PIN after the protocol layer has
// verified their current PIN (through the lockout accounting). NewPIN is never
// audited.
type ChangeOwnPinCommand struct {
	WorkerID string `json:"workerId"`
	// FamilyID is the login family that keeps its session.
	FamilyID string `json:"familyId"`
	NewPIN   string `json:"-"`
}

// ChangeOwnPin replaces the worker's PIN with one they chose (policy-checked),
// clears the forced-change flag on their current session family and revokes
// their other sessions (PIN_CHANGED).
func (o *Ops) ChangeOwnPin() usecaseop.TxOperation[ChangeOwnPinCommand, struct{}] {
	return usecaseop.TxOperation[ChangeOwnPinCommand, struct{}]{
		Name: "ChangeFieldWorkerOwnPin",
		Validate: func(_ context.Context, cmd ChangeOwnPinCommand) error {
			if cmd.WorkerID == "" || cmd.FamilyID == "" || cmd.NewPIN == "" {
				return usecase.Validation("PIN_CHANGE_INVALID", "workerId, familyId and newPin are required")
			}
			return nil
		},
		// The protocol layer authenticated the worker (bearer + current PIN).
		Authorize: usecaseop.Public[ChangeOwnPinCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd ChangeOwnPinCommand, ec usecase.ExecutionContext) (struct{}, error) {
			if !o.Hasher.Configured() {
				return struct{}{}, unconfigured()
			}
			ident, err := o.R.Identities.FindByID(ctx, cmd.WorkerID)
			if err != nil {
				return struct{}{}, repoErr("find_field_worker", err)
			}
			if ident == nil || ident.Status != StatusActive {
				return struct{}{}, usecase.Authorization(CodeWorkerUnavailable, "the worker cannot change their PIN")
			}
			pol, err := o.R.Policies.Effective(ctx, ident.ClientID)
			if err != nil {
				return struct{}{}, repoErr("find_field_policy", err)
			}
			if v := pin.Check(cmd.NewPIN, ident.WorkerCode, pol.PINMinLength); len(v) > 0 {
				return struct{}{}, pinPolicyError(v)
			}
			hash, err := o.Hasher.Hash(ident.ClientID, ident.ID, cmd.NewPIN)
			if err != nil {
				return struct{}{}, unconfigured()
			}
			ident.SetPIN(hash, false)
			ev := NewEvent(ec, WorkerPinChangedType, "field-worker", ident.ID, workerData(ident, map[string]any{"familyId": cmd.FamilyID}))
			if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, ident, o.R.Identities, ev, cmd)); err != nil {
				return struct{}{}, err
			}
			var revoked int64
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx,
					`UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = $3
					 WHERE identity_id = $1 AND family_id <> $2 AND revoked_at IS NULL`,
					ident.ID, cmd.FamilyID, ReasonPinChanged)
				if err != nil {
					return err
				}
				revoked = tag.RowsAffected()
				_, err = tx.Exec(ctx,
					`UPDATE field_sessions SET pin_change_required = FALSE WHERE family_id = $1 AND revoked_at IS NULL`,
					cmd.FamilyID)
				return err
			}); err != nil {
				return struct{}{}, repoErr("revoke_sessions", err)
			}
			if revoked > 0 {
				if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
					sessionsRevokedEvent(ec, ident.ClientID, ReasonPinChanged, int(revoked), ident.ID, "", ""), cmd)); err != nil {
					return struct{}{}, err
				}
			}
			return struct{}{}, nil
		},
	}
}

// ── refresh-token reuse ──────────────────────────────────────────────────

// RevokeFamilyCommand revokes a login family.
type RevokeFamilyCommand struct {
	ClientID string `json:"clientId"`
	WorkerID string `json:"workerId"`
	DeviceID string `json:"deviceId"`
	FamilyID string `json:"familyId"`
}

// RevokeSessionFamilyOnReuse kills a whole login family because a rotated
// refresh token was presented again outside the retry grace (a stolen or
// replayed token). It emits field-session.revoked.
func (o *Ops) RevokeSessionFamilyOnReuse() usecaseop.TxOperation[RevokeFamilyCommand, int64] {
	return usecaseop.TxOperation[RevokeFamilyCommand, int64]{
		Name: "RevokeFieldSessionFamilyOnReuse",
		Validate: func(_ context.Context, cmd RevokeFamilyCommand) error {
			if cmd.FamilyID == "" {
				return usecase.Validation("FAMILY_REQUIRED", "familyId is required")
			}
			return nil
		},
		// Triggered by the protocol layer on a detected replay, not by a caller.
		Authorize: usecaseop.Public[RevokeFamilyCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RevokeFamilyCommand, ec usecase.ExecutionContext) (int64, error) {
			var n int64
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx,
					`UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = $2
					 WHERE family_id = $1 AND revoked_at IS NULL`, cmd.FamilyID, ReasonReuse)
				n = tag.RowsAffected()
				return err
			}); err != nil {
				return 0, repoErr("revoke_family", err)
			}
			data := map[string]any{
				"clientId": cmd.ClientID, "reason": ReasonReuse, "count": n, "familyId": cmd.FamilyID,
				"workerId": cmd.WorkerID,
			}
			if cmd.DeviceID != "" {
				data["deviceId"] = cmd.DeviceID
			}
			ev := NewEvent(ec, SessionRevokedType, "field-session", cmd.WorkerID, data)
			if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, cmd)); err != nil {
				return 0, err
			}
			return n, nil
		},
	}
}

// ── throttle notice ──────────────────────────────────────────────────────

// ThrottleCommand records that a PIN-failure bucket tripped.
type ThrottleCommand struct {
	// Scope is "DEVICE", "CLIENT" or "IP" (an unenrolled key's bucket).
	Scope     string `json:"scope"`
	ClientID  string `json:"clientId"`
	DeviceID  string `json:"deviceId,omitempty"`
	IPAddress string `json:"ipAddress,omitempty"`
}

// NoteThrottled emits field-device.throttled once per tripped bucket window.
func (o *Ops) NoteThrottled() usecaseop.TxOperation[ThrottleCommand, struct{}] {
	return usecaseop.TxOperation[ThrottleCommand, struct{}]{
		Name:      "NoteFieldThrottled",
		Authorize: usecaseop.Public[ThrottleCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd ThrottleCommand, ec usecase.ExecutionContext) (struct{}, error) {
			data := map[string]any{"scope": cmd.Scope, "clientId": cmd.ClientID}
			id := cmd.ClientID
			if cmd.DeviceID != "" {
				data["deviceId"] = cmd.DeviceID
				id = cmd.DeviceID
			}
			if cmd.IPAddress != "" {
				data["ipAddress"] = cmd.IPAddress
				if id == "" {
					id = cmd.IPAddress
				}
			}
			ev := NewEvent(ec, DeviceThrottledType, "field-device", id, data)
			_, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, cmd))
			return struct{}{}, err
		},
	}
}
