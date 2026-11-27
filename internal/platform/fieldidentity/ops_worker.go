package fieldidentity

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ── ensure worker ────────────────────────────────────────────────────────

// EnsureWorkerCommand creates the (client, workerCode) worker or, if it
// exists, adds the named application grants. With no WorkerCode the platform
// generates the code; IdempotencyKey then makes the creation safe to repeat
// (a repeat answers the worker the first one made). PIN is never audited.
type EnsureWorkerCommand struct {
	ClientID   string `json:"clientId"`
	WorkerCode string `json:"workerCode,omitempty"`
	// IdempotencyKey keys a creation without a WorkerCode within the client;
	// it may not be combined with a WorkerCode (the code is then the key).
	IdempotencyKey string   `json:"idempotencyKey,omitempty"`
	Name           string   `json:"name"`
	Email          string   `json:"email,omitempty"`
	Phone          string   `json:"phone,omitempty"`
	AppCodes       []string `json:"appCodes,omitempty"`
	// PIN is a supervisor-entered PIN for a NEW worker; it must pass the
	// client's policy and is stored with pinChangeRequired.
	PIN string `json:"-"`
	// CanImpersonate and IsTest apply to a NEW worker only (an existing worker
	// keeps every field).
	CanImpersonate bool `json:"canImpersonate,omitempty"`
	IsTest         bool `json:"isTest,omitempty"`
}

// CodeWorkerCodesExhausted is the error code of a creation without a code that
// found no free code for the client in workerCodeDraws draws.
const CodeWorkerCodesExhausted = "WORKER_CODES_EXHAUSTED"

// RunEnsureWorker idempotently creates a worker and grants applications, in
// one transaction per attempt. An existing worker keeps every field (including
// PIN); only missing grants are added. With a typed code the worker is keyed
// by (client, code); without one the platform generates the code (see
// [GenerateWorkerCode]) and, when the command carries an idempotency key, keys
// the creation by (client, key).
//
// The unique (client, code) key is the only uniqueness guarantee: an attempt
// whose generated code is already held inserts nothing and the creation draws
// again, at most [workerCodeDraws] times, then reports
// [CodeWorkerCodesExhausted]. A concurrent creation with the same idempotency
// key that commits first is found by the next attempt and answered as an
// ensure.
func (o *Ops) RunEnsureWorker(ctx context.Context, uow *usecasepgx.UnitOfWork, cmd EnsureWorkerCommand, ec usecase.ExecutionContext) (Event, error) {
	for range workerCodeDraws {
		ev, err := usecaseop.Run(ctx, uow, o.ensureWorker(), cmd, ec)
		if !errors.Is(err, errWorkerCodeTaken) {
			return ev, err
		}
	}
	return Event{}, usecase.BusinessRule(CodeWorkerCodesExhausted,
		"no free worker code was found for this client; raise the worker code digits in the field policy, or give the worker a code")
}

// ensureWorker is one attempt of [Ops.RunEnsureWorker]: with a generated code
// its plan fails with errWorkerCodeTaken when the drawn code (or the key) is
// already held, having written nothing.
func (o *Ops) ensureWorker() usecaseop.Operation[EnsureWorkerCommand, Event] {
	return usecaseop.Operation[EnsureWorkerCommand, Event]{
		Name: "EnsureFieldWorker",
		Validate: func(_ context.Context, cmd EnsureWorkerCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			code, key := NormalizeWorkerCode(cmd.WorkerCode), strings.TrimSpace(cmd.IdempotencyKey)
			if len(code) > MaxWorkerCodeLength {
				return usecase.Validation("WORKER_CODE_INVALID", "workerCode is at most 64 characters")
			}
			if key != "" && code != "" {
				return usecase.Validation("IDEMPOTENCY_KEY_WITH_WORKER_CODE",
					"idempotencyKey is for a worker created without a workerCode; with a workerCode the code is the key")
			}
			if len(key) > MaxIdempotencyKeyLength {
				return usecase.Validation("INVALID_IDEMPOTENCY_KEY", "idempotencyKey is at most 100 characters")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			if e := strings.TrimSpace(cmd.Email); e != "" {
				if at := strings.IndexByte(e, '@'); at <= 0 || at == len(e)-1 {
					return usecase.Validation("EMAIL_INVALID", "email is not valid")
				}
			}
			if !validPhone(cmd.Phone) {
				return usecase.Validation("PHONE_INVALID", "phone is not a valid phone number")
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd EnsureWorkerCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), &cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd EnsureWorkerCommand, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			if err := requireClient(ctx, o.Clients, cmd.ClientID); err != nil {
				return nil, err
			}
			ac := auth.FromContext(ctx)
			if ac.IsApplicationScoped() && len(cmd.AppCodes) == 0 {
				return nil, usecase.Authorization("APPLICATION_FORBIDDEN",
					"an application-confined caller must name its application in appCodes")
			}
			apps, err := o.resolveApps(ctx, cmd.AppCodes, true)
			if err != nil {
				return nil, err
			}
			code, key := NormalizeWorkerCode(cmd.WorkerCode), strings.TrimSpace(cmd.IdempotencyKey)
			var existing *Identity
			switch {
			case code != "":
				existing, err = o.R.Identities.FindByClientAndCode(ctx, cmd.ClientID, code)
			case key != "":
				existing, err = o.R.Identities.FindByClientAndIdempotencyKey(ctx, cmd.ClientID, key)
			}
			if err != nil {
				return nil, repoErr("find_field_worker", err)
			}
			if existing != nil {
				granted := grantAll(existing, apps)
				data := workerData(existing, map[string]any{"isTest": existing.IsTest})
				if len(granted) > 0 {
					data["applicationIds"] = granted
				}
				return usecaseop.Save(existing, o.R.Identities, NewEvent(ec, WorkerEnsuredType, "field-worker", existing.ID, data)), nil
			}

			pol, err := o.R.Policies.Effective(ctx, cmd.ClientID)
			if err != nil {
				return nil, repoErr("find_field_policy", err)
			}
			ident := NewIdentity(cmd.ClientID, code, cmd.Name)
			if code == "" {
				// Drawn, never looked up first: the insert claims the code or
				// inserts nothing (Persist's errWorkerCodeTaken).
				drawn, err := o.drawWorkerCode(pol.WorkerCodeDigits)
				if err != nil {
					return nil, usecase.Internal("WORKER_CODE_GENERATE", "could not generate a worker code", err)
				}
				ident.WorkerCode, ident.IdempotencyKey, ident.generated = NormalizeWorkerCode(drawn), key, true
			}
			ident.Email = strings.TrimSpace(cmd.Email)
			ident.Phone = strings.TrimSpace(cmd.Phone)
			ident.CanImpersonate, ident.IsTest = cmd.CanImpersonate, cmd.IsTest
			if cmd.PIN != "" {
				if !o.Hasher.Configured() {
					return nil, unconfigured()
				}
				if v := pin.Check(cmd.PIN, ident.WorkerCode, pol.PINMinLength); len(v) > 0 {
					return nil, pinPolicyError(v)
				}
				hash, err := o.Hasher.Hash(ident.ClientID, ident.ID, cmd.PIN)
				if err != nil {
					return nil, unconfigured()
				}
				ident.SetPIN(hash, true)
			}
			granted := grantAll(ident, apps)
			data := workerData(ident, map[string]any{"name": ident.Name, "isTest": ident.IsTest})
			if len(granted) > 0 {
				data["applicationIds"] = granted
			}
			if code == "" {
				data["workerCodeGenerated"] = true
			}
			return usecaseop.Save(ident, o.R.Identities, NewEvent(ec, WorkerCreatedType, "field-worker", ident.ID, data)), nil
		},
	}
}

// grantAll adds the application grants, returning the ids that were new.
func grantAll(ident *Identity, apps []resolvedApp) []string {
	granted := []string{}
	for _, a := range apps {
		if ident.Grant(a.ID, SourceApp) {
			granted = append(granted, a.ID)
		}
	}
	return granted
}

// ── update / delete ──────────────────────────────────────────────────────

// WorkerRef addresses a worker by id (ClientID, when set, must match).
type WorkerRef struct {
	ClientID string `json:"clientId,omitempty"`
	ID       string `json:"id"`
}

func validateRef(_ context.Context, cmd WorkerRef) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

// UpdateWorkerCommand changes a worker's name, email and phone.
type UpdateWorkerCommand struct {
	WorkerRef
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
	// Phone nil leaves the number unchanged; "" clears it.
	Phone *string `json:"phone,omitempty"`
	// CanImpersonate and IsTest, when non-nil, replace the flag. Clearing
	// CanImpersonate revokes the sessions the worker holds open as an actor.
	CanImpersonate *bool `json:"canImpersonate,omitempty"`
	IsTest         *bool `json:"isTest,omitempty"`
}

// UpdateWorker renames a worker / changes the contact email and phone and the
// canImpersonate / isTest flags.
func (o *Ops) UpdateWorker() usecaseop.TxOperation[UpdateWorkerCommand, Event] {
	return usecaseop.TxOperation[UpdateWorkerCommand, Event]{
		Name: "UpdateFieldWorker",
		Validate: func(ctx context.Context, cmd UpdateWorkerCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			if cmd.Email != nil {
				if e := strings.TrimSpace(*cmd.Email); e != "" {
					if at := strings.IndexByte(e, '@'); at <= 0 || at == len(e)-1 {
						return usecase.Validation("EMAIL_INVALID", "email is not valid")
					}
				}
			}
			if cmd.Phone != nil && !validPhone(*cmd.Phone) {
				return usecase.Validation("PHONE_INVALID", "phone is not a valid phone number")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateWorkerCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd UpdateWorkerCommand, ec usecase.ExecutionContext) (Event, error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			// The worker and those whose sessions they hold open as an actor
			// first (lock_order.go): the update may expire the worker's sign-in
			// codes and, taking away canImpersonate, end those sessions.
			if err := o.lockWorkerAndActedAs(ctx, s, ident.ID); err != nil {
				return Event{}, err
			}
			ident.Name = strings.TrimSpace(cmd.Name)
			if cmd.Email != nil {
				ident.Email = strings.TrimSpace(*cmd.Email)
			}
			if cmd.Phone != nil {
				ident.Phone = strings.TrimSpace(*cmd.Phone)
			}
			if cmd.IsTest != nil && *cmd.IsTest != ident.IsTest {
				if *cmd.IsTest {
					// Marking a real worker as a test identity would let a
					// manage-only caller mint a sign-in code under the looser
					// test rule: it needs what signing in as a real worker needs.
					if err := auth.CanSignInAsFieldWorker(auth.FromContext(ctx), ident.ClientID, false); err != nil {
						return Event{}, err
					}
				} else if err := s.WithTx(ctx, func(tx pgx.Tx) error {
					// Unmarking: a code minted under the test rule must not
					// outlive it. Expiring it is what Consume already refuses.
					_, err := tx.Exec(ctx,
						`UPDATE field_sign_in_codes SET expires_at = NOW()
						 WHERE identity_id = $1 AND used_at IS NULL AND expires_at > NOW()`, ident.ID)
					return err
				}); err != nil {
					return Event{}, repoErr("expire_sign_in_codes", err)
				}
				ident.IsTest = *cmd.IsTest
			}
			if cmd.CanImpersonate != nil {
				if ident.CanImpersonate && !*cmd.CanImpersonate {
					ident.RevokeActorSessions(ReasonActorSuspended)
				}
				ident.CanImpersonate = *cmd.CanImpersonate
			}
			ev := NewEvent(ec, WorkerUpdatedType, "field-worker", ident.ID, workerData(ident, map[string]any{
				"name": ident.Name, "isTest": ident.IsTest, "canImpersonate": ident.CanImpersonate,
			}))
			return usecase.Into(usecasepgx.CommitScoped(ctx, s, ident, o.R.Identities, ev, cmd))
		},
	}
}

// DeleteWorker hard-deletes a worker. Their sessions are revoked first (and
// removed with the row); a field-session.revoked event is emitted when any
// were live.
func (o *Ops) DeleteWorker() usecaseop.TxOperation[WorkerRef, Event] {
	return usecaseop.TxOperation[WorkerRef, Event]{
		Name:      "DeleteFieldWorker",
		Validate:  validateRef,
		Authorize: usecaseop.Public[WorkerRef],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd WorkerRef, ec usecase.ExecutionContext) (Event, error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			if err := o.lockWorkerAndActedAs(ctx, s, ident.ID); err != nil {
				return Event{}, err
			}
			live, err := countLive(ctx, s, `identity_id = $1 OR `+actorCond, ident.ID, "")
			if err != nil {
				return Event{}, repoErr("count_sessions", err)
			}
			ev := NewEvent(ec, WorkerDeletedType, "field-worker", ident.ID, workerData(ident, nil))
			out, err := usecase.Into(usecasepgx.CommitDeleteScoped(ctx, s, ident, o.R.Identities, ev, cmd))
			if err != nil {
				return Event{}, err
			}
			if live > 0 {
				if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
					sessionsRevokedEvent(ec, ident.ClientID, ReasonAdmin, live, ident.ID, "", ""), cmd)); err != nil {
					return Event{}, err
				}
			}
			return out, nil
		},
	}
}

// ── PIN reset ────────────────────────────────────────────────────────────

// ResetPinCommand sets a worker's PIN: the supervisor's own (PIN) or, when
// empty, a random policy-valid temporary one. PIN is never audited.
type ResetPinCommand struct {
	WorkerRef
	PIN string `json:"-"`
}

// ResetPinResult carries the outcome. TemporaryPIN is set only when the
// platform generated the PIN; it is shown to the supervisor once.
type ResetPinResult struct {
	WorkerID          string
	ClientID          string
	TemporaryPIN      string
	PinChangeRequired bool
}

// ResetWorkerPin replaces a worker's PIN, flags a forced change, revokes all
// their sessions (PIN_CHANGED) and clears the lock state.
func (o *Ops) ResetWorkerPin() usecaseop.TxOperation[ResetPinCommand, ResetPinResult] {
	return usecaseop.TxOperation[ResetPinCommand, ResetPinResult]{
		Name: "ResetFieldWorkerPin",
		Validate: func(ctx context.Context, cmd ResetPinCommand) error {
			return validateRef(ctx, cmd.WorkerRef)
		},
		Authorize: usecaseop.Public[ResetPinCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd ResetPinCommand, ec usecase.ExecutionContext) (ResetPinResult, error) {
			var zero ResetPinResult
			if !o.Hasher.Configured() {
				return zero, unconfigured()
			}
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return zero, err
			}
			pol, err := o.R.Policies.Effective(ctx, ident.ClientID)
			if err != nil {
				return zero, repoErr("find_field_policy", err)
			}
			newPIN, temporary := cmd.PIN, ""
			if newPIN == "" {
				if newPIN, err = pin.Generate(pol.PINMinLength, ident.WorkerCode); err != nil {
					return zero, usecase.Internal("PIN_GENERATE", "could not generate a PIN", err)
				}
				temporary = newPIN
			} else if v := pin.Check(newPIN, ident.WorkerCode, pol.PINMinLength); len(v) > 0 {
				return zero, pinPolicyError(v)
			}
			hash, err := o.Hasher.Hash(ident.ClientID, ident.ID, newPIN)
			if err != nil {
				return zero, unconfigured()
			}
			if err := o.lockWorkers(ctx, s, ident.ID); err != nil {
				return zero, err
			}
			live, err := countLive(ctx, s, `identity_id = $1`, ident.ID, "")
			if err != nil {
				return zero, repoErr("count_sessions", err)
			}
			ident.SetPIN(hash, true)
			ident.RevokeSessions(ReasonPinChanged, "")
			ev := NewEvent(ec, WorkerPinResetType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"pinChangeRequired": true}))
			if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, ident, o.R.Identities, ev, cmd)); err != nil {
				return zero, err
			}
			if live > 0 {
				if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
					sessionsRevokedEvent(ec, ident.ClientID, ReasonPinChanged, live, ident.ID, "", ""), cmd)); err != nil {
					return zero, err
				}
			}
			if _, err := o.revokeUnlockKeys(ctx, s, ec, UnlockKeyRevocation{
				Scope: ScopeWorker, ClientID: ident.ClientID, WorkerID: ident.ID, Reason: UnlockRevokedPINReset, By: ec.PrincipalID,
			}, cmd); err != nil {
				return zero, err
			}
			return ResetPinResult{WorkerID: ident.ID, ClientID: ident.ClientID, TemporaryPIN: temporary, PinChangeRequired: true}, nil
		},
	}
}

// UnlockWorker clears the soft and hard locks and the failure counter.
func (o *Ops) UnlockWorker() usecaseop.Operation[WorkerRef, Event] {
	return usecaseop.Operation[WorkerRef, Event]{
		Name:      "UnlockFieldWorker",
		Validate:  validateRef,
		Authorize: usecaseop.Public[WorkerRef],
		Execute: func(ctx context.Context, cmd WorkerRef, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return nil, err
			}
			ident.ClearLock()
			ev := NewEvent(ec, WorkerUnlockedType, "field-worker", ident.ID, workerData(ident, nil))
			return usecaseop.Save(ident, o.R.Identities, ev), nil
		},
	}
}

// ── suspend / activate ───────────────────────────────────────────────────

// SuspendWorker disables a worker and revokes all their sessions.
func (o *Ops) SuspendWorker() usecaseop.TxOperation[WorkerRef, Event] {
	return usecaseop.TxOperation[WorkerRef, Event]{
		Name:      "SuspendFieldWorker",
		Validate:  validateRef,
		Authorize: usecaseop.Public[WorkerRef],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd WorkerRef, ec usecase.ExecutionContext) (Event, error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			if err := o.lockWorkerAndActedAs(ctx, s, ident.ID); err != nil {
				return Event{}, err
			}
			live, err := countLive(ctx, s, `identity_id = $1`, ident.ID, "")
			if err != nil {
				return Event{}, repoErr("count_sessions", err)
			}
			actorLive, err := countLive(ctx, s, actorCond, ident.ID, "")
			if err != nil {
				return Event{}, repoErr("count_sessions", err)
			}
			ident.Status = StatusDisabled
			ident.RevokeSessions(ReasonWorkerSuspended, "")
			ident.RevokeActorSessions(ReasonActorSuspended)
			ev := NewEvent(ec, WorkerSuspendedType, "field-worker", ident.ID, workerData(ident, nil))
			out, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, ident, o.R.Identities, ev, cmd))
			if err != nil {
				return Event{}, err
			}
			if live > 0 {
				if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
					sessionsRevokedEvent(ec, ident.ClientID, ReasonWorkerSuspended, live, ident.ID, "", ""), cmd)); err != nil {
					return Event{}, err
				}
			}
			if actorLive > 0 {
				if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
					sessionsRevokedEvent(ec, ident.ClientID, ReasonActorSuspended, actorLive, ident.ID, "", ""), cmd)); err != nil {
					return Event{}, err
				}
			}
			if _, err := o.revokeUnlockKeys(ctx, s, ec, UnlockKeyRevocation{
				Scope: ScopeWorker, ClientID: ident.ClientID, WorkerID: ident.ID, Reason: UnlockRevokedWorkerSuspended, By: ec.PrincipalID,
			}, cmd); err != nil {
				return Event{}, err
			}
			return out, nil
		},
	}
}

// ActivateWorker re-enables a suspended worker.
func (o *Ops) ActivateWorker() usecaseop.Operation[WorkerRef, Event] {
	return usecaseop.Operation[WorkerRef, Event]{
		Name:      "ActivateFieldWorker",
		Validate:  validateRef,
		Authorize: usecaseop.Public[WorkerRef],
		Execute: func(ctx context.Context, cmd WorkerRef, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return nil, err
			}
			ident.Status = StatusActive
			ev := NewEvent(ec, WorkerActivatedType, "field-worker", ident.ID, workerData(ident, nil))
			return usecaseop.Save(ident, o.R.Identities, ev), nil
		},
	}
}
