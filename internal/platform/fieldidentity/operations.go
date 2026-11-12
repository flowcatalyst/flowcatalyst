package fieldidentity

import (
	"context"
	"crypto/rand"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// CodeUnconfigured is the error code of every operation that needs the PIN
// pepper when FLOWCATALYST_APP_KEY is not set. The admin API answers it 503.
const CodeUnconfigured = "FIELD_IDENTITY_UNCONFIGURED"

// EnrolmentCodeTTL is how long an enrolment code stays valid.
const EnrolmentCodeTTL = 10 * time.Minute

// enrolmentAlphabet is unambiguous: no 0/O, 1/I/L.
const enrolmentAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// EnrolmentCodeLength is the number of characters in an enrolment code.
const EnrolmentCodeLength = 10

// NewEnrolmentCodeValue returns a fresh random enrolment code.
func NewEnrolmentCodeValue() (string, error) {
	n := big.NewInt(int64(len(enrolmentAlphabet)))
	b := make([]byte, EnrolmentCodeLength)
	for i := range b {
		d, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", err
		}
		b[i] = enrolmentAlphabet[d.Int64()]
	}
	return string(b), nil
}

// Ops builds the field-identity domain operations over their dependencies.
// Every operation is client-boxed in its Authorize phase (creates) or right
// after loading its target (by-id operations); application-confined callers
// (service accounts holding an explicit application list) are limited to
// their own applications. The coarse permission check stays in the
// controller, per the locked authorization model.
type Ops struct {
	R       *Repos
	Clients *client.Repository
	Apps    *application.Repository
	Hasher  *pin.Hasher
	// Principals resolves the platform user a worker is linked to.
	Principals *principal.Repository
}

// ── shared helpers ───────────────────────────────────────────────────────

func unconfigured() error {
	return usecase.Internal(CodeUnconfigured, "field PINs are unavailable: FLOWCATALYST_APP_KEY is not set", pin.ErrUnconfigured)
}

func pinPolicyError(violations []string) error {
	return usecase.Validation("PIN_POLICY", "the PIN does not meet the client's PIN policy").
		WithDetails(map[string]any{"violations": violations})
}

func repoErr(what string, err error) error {
	return usecase.Internal("REPO", what+" failed", err)
}

// resolvedApp is an application resolved by code.
type resolvedApp struct{ ID, Code string }

// resolveApps resolves codes to applications. Unknown (or, with requireActive,
// inactive) applications are 400 UNKNOWN_APPLICATION; an application the
// caller is not confined to is 403 APPLICATION_FORBIDDEN.
func (o *Ops) resolveApps(ctx context.Context, codes []string, requireActive bool) ([]resolvedApp, error) {
	ac := auth.FromContext(ctx)
	out := make([]resolvedApp, 0, len(codes))
	for _, raw := range codes {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		a, err := o.Apps.FindByCode(ctx, code)
		if err == nil && a == nil && strings.ToLower(code) != code {
			a, err = o.Apps.FindByCode(ctx, strings.ToLower(code))
		}
		if err != nil {
			return nil, repoErr("find_application", err)
		}
		if a == nil || (requireActive && !a.Active) {
			return nil, usecase.Validation("UNKNOWN_APPLICATION", "unknown or inactive application: "+code)
		}
		if !ac.CanAccessApplication(string(a.ID)) {
			return nil, usecase.Authorization("APPLICATION_FORBIDDEN", "no access to application: "+a.Code)
		}
		if !slices.ContainsFunc(out, func(r resolvedApp) bool { return r.ID == string(a.ID) }) {
			out = append(out, resolvedApp{ID: string(a.ID), Code: a.Code})
		}
	}
	return out, nil
}

// canActOnWorker enforces application confinement on an existing worker: an
// unconfined caller always may; a confined one needs the worker to hold a
// grant to one of its applications.
func canActOnWorker(ac *auth.AuthContext, i *Identity) error {
	if !ac.IsApplicationScoped() {
		return nil
	}
	if slices.ContainsFunc(i.Apps, func(g AppGrant) bool { return ac.CanAccessApplication(g.AppID) }) {
		return nil
	}
	return usecase.Authorization("APPLICATION_FORBIDDEN", "the worker has no access to any of the caller's applications")
}

func canActOnDevice(ac *auth.AuthContext, d *Device) error {
	if !ac.IsApplicationScoped() {
		return nil
	}
	if slices.ContainsFunc(d.AppIDs, ac.CanAccessApplication) {
		return nil
	}
	return usecase.Authorization("APPLICATION_FORBIDDEN", "the device is not enrolled for any of the caller's applications")
}

// loadWorker loads a worker and applies the resource-level checks.
func (o *Ops) loadWorker(ctx context.Context, id, clientID string) (*Identity, error) {
	ident, err := o.R.Identities.FindByID(ctx, id)
	if err != nil {
		return nil, repoErr("find_field_worker", err)
	}
	if ident == nil || (clientID != "" && ident.ClientID != clientID) {
		return nil, httperror.NotFound("FieldWorker", id)
	}
	ac := auth.FromContext(ctx)
	if err := auth.CheckScopeAccess(ac, &ident.ClientID); err != nil {
		return nil, err
	}
	if err := canActOnWorker(ac, ident); err != nil {
		return nil, err
	}
	return ident, nil
}

func (o *Ops) loadDevice(ctx context.Context, id, clientID string) (*Device, error) {
	d, err := o.R.Devices.FindByID(ctx, id)
	if err != nil {
		return nil, repoErr("find_field_device", err)
	}
	if d == nil || (clientID != "" && d.ClientID != clientID) {
		return nil, httperror.NotFound("FieldDevice", id)
	}
	ac := auth.FromContext(ctx)
	if err := auth.CheckScopeAccess(ac, &d.ClientID); err != nil {
		return nil, err
	}
	if err := canActOnDevice(ac, d); err != nil {
		return nil, err
	}
	return d, nil
}

func appIDs(apps []resolvedApp) []string {
	ids := make([]string, len(apps))
	for n, a := range apps {
		ids[n] = a.ID
	}
	return ids
}

// actorCond selects the sessions a worker holds open as an actor (a supervisor
// acting as another worker).
const actorCond = `(actor_id = $1 AND actor_type = '` + ActorFieldWorker + `')`

// countLive counts live sessions matching cond ($1 = owner id) inside the open
// transaction.
func countLive(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cond, ownerID, appID string) (int, error) {
	n := 0
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		q := `SELECT COUNT(*) FROM field_sessions WHERE (` + cond + `) AND revoked_at IS NULL`
		args := []any{ownerID}
		if appID != "" {
			q += ` AND application_id = $2`
			args = append(args, appID)
		}
		return tx.QueryRow(ctx, q, args...).Scan(&n)
	})
	return n, err
}

func sessionsRevokedEvent(ec usecase.ExecutionContext, clientID, reason string, count int, workerID, deviceID, appID string) Event {
	data := map[string]any{"clientId": clientID, "reason": reason, "count": count}
	id := workerID
	if workerID != "" {
		data["workerId"] = workerID
	}
	if deviceID != "" {
		data["deviceId"] = deviceID
		if id == "" {
			id = deviceID
		}
	}
	if appID != "" {
		data["applicationId"] = appID
	}
	return NewEvent(ec, SessionRevokedType, "field-session", id, data)
}

func requireClient(ctx context.Context, clients *client.Repository, clientID string) error {
	c, err := clients.FindByID(ctx, clientID)
	if err != nil {
		return repoErr("find_client", err)
	}
	if c == nil {
		return httperror.NotFound("Client", clientID)
	}
	return nil
}

// ── ensure worker ────────────────────────────────────────────────────────

// EnsureWorkerCommand creates the (client, workerCode) worker or, if it
// exists, adds the named application grants. PIN is never audited.
type EnsureWorkerCommand struct {
	ClientID   string   `json:"clientId"`
	WorkerCode string   `json:"workerCode"`
	Name       string   `json:"name"`
	Email      string   `json:"email,omitempty"`
	Phone      string   `json:"phone,omitempty"`
	AppCodes   []string `json:"appCodes,omitempty"`
	// PIN is a supervisor-entered PIN for a NEW worker; it must pass the
	// client's policy and is stored with pinChangeRequired.
	PIN string `json:"-"`
	// CanImpersonate and IsTest apply to a NEW worker only (an existing worker
	// keeps every field).
	CanImpersonate bool `json:"canImpersonate,omitempty"`
	IsTest         bool `json:"isTest,omitempty"`
}

// EnsureWorker idempotently creates a worker and grants applications. An
// existing worker keeps every field (including PIN); only missing grants are
// added.
func (o *Ops) EnsureWorker() usecaseop.Operation[EnsureWorkerCommand, Event] {
	return usecaseop.Operation[EnsureWorkerCommand, Event]{
		Name: "EnsureFieldWorker",
		Validate: func(_ context.Context, cmd EnsureWorkerCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			if NormalizeWorkerCode(cmd.WorkerCode) == "" {
				return usecase.Validation("WORKER_CODE_REQUIRED", "workerCode is required")
			}
			if len(NormalizeWorkerCode(cmd.WorkerCode)) > 64 {
				return usecase.Validation("WORKER_CODE_INVALID", "workerCode is at most 64 characters")
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
			existing, err := o.R.Identities.FindByClientAndCode(ctx, cmd.ClientID, cmd.WorkerCode)
			if err != nil {
				return nil, repoErr("find_field_worker", err)
			}
			created := existing == nil
			ident := existing
			if created {
				ident = NewIdentity(cmd.ClientID, cmd.WorkerCode, cmd.Name)
				ident.Email = strings.TrimSpace(cmd.Email)
				ident.Phone = strings.TrimSpace(cmd.Phone)
				ident.CanImpersonate, ident.IsTest = cmd.CanImpersonate, cmd.IsTest
				if cmd.PIN != "" {
					if !o.Hasher.Configured() {
						return nil, unconfigured()
					}
					pol, err := o.R.Policies.Effective(ctx, cmd.ClientID)
					if err != nil {
						return nil, repoErr("find_field_policy", err)
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
			}
			granted := []string{}
			for _, a := range apps {
				if ident.Grant(a.ID, SourceApp) {
					granted = append(granted, a.ID)
				}
			}
			if created {
				data := workerData(ident, map[string]any{"name": ident.Name, "isTest": ident.IsTest})
				if len(granted) > 0 {
					data["applicationIds"] = granted
				}
				return usecaseop.Save(ident, o.R.Identities, NewEvent(ec, WorkerCreatedType, "field-worker", ident.ID, data)), nil
			}
			data := workerData(ident, map[string]any{"isTest": ident.IsTest})
			if len(granted) > 0 {
				data["applicationIds"] = granted
			}
			return usecaseop.Save(ident, o.R.Identities, NewEvent(ec, WorkerEnsuredType, "field-worker", ident.ID, data)), nil
		},
	}
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

// ── application grants ───────────────────────────────────────────────────

// WorkerAppCommand grants or revokes one application.
type WorkerAppCommand struct {
	WorkerRef
	AppCode string `json:"appCode"`
}

func validateWorkerApp(ctx context.Context, cmd WorkerAppCommand) error {
	if err := validateRef(ctx, cmd.WorkerRef); err != nil {
		return err
	}
	if strings.TrimSpace(cmd.AppCode) == "" {
		return usecase.Validation("APP_CODE_REQUIRED", "appCode is required")
	}
	return nil
}

// GrantWorkerApp lets a worker sign in to an (active) application. Idempotent.
func (o *Ops) GrantWorkerApp() usecaseop.Operation[WorkerAppCommand, Event] {
	return usecaseop.Operation[WorkerAppCommand, Event]{
		Name:      "GrantFieldWorkerApp",
		Validate:  validateWorkerApp,
		Authorize: usecaseop.Public[WorkerAppCommand],
		Execute: func(ctx context.Context, cmd WorkerAppCommand, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			apps, err := o.resolveApps(ctx, []string{cmd.AppCode}, true)
			if err != nil {
				return nil, err
			}
			// An application-confined caller may grant its own application to
			// a worker it cannot yet see (that is how workers join an app), so
			// the worker is loaded with the client box only.
			ident, err := o.R.Identities.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, repoErr("find_field_worker", err)
			}
			if ident == nil || (cmd.ClientID != "" && ident.ClientID != cmd.ClientID) {
				return nil, httperror.NotFound("FieldWorker", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), &ident.ClientID); err != nil {
				return nil, err
			}
			ident.Grant(apps[0].ID, SourceAdmin)
			ev := NewEvent(ec, WorkerAppGrantedType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"applicationId": apps[0].ID, "applicationCode": apps[0].Code}))
			return usecaseop.Save(ident, o.R.Identities, ev), nil
		},
	}
}

// RevokeWorkerApp removes a worker's access to an application and revokes
// that application's sessions for them. Idempotent.
func (o *Ops) RevokeWorkerApp() usecaseop.TxOperation[WorkerAppCommand, Event] {
	return usecaseop.TxOperation[WorkerAppCommand, Event]{
		Name:      "RevokeFieldWorkerApp",
		Validate:  validateWorkerApp,
		Authorize: usecaseop.Public[WorkerAppCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd WorkerAppCommand, ec usecase.ExecutionContext) (Event, error) {
			apps, err := o.resolveApps(ctx, []string{cmd.AppCode}, false)
			if err != nil {
				return Event{}, err
			}
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			live, err := countLive(ctx, s, `identity_id = $1`, ident.ID, apps[0].ID)
			if err != nil {
				return Event{}, repoErr("count_sessions", err)
			}
			ident.Revoke(apps[0].ID)
			ident.RevokeSessions(ReasonAdmin, apps[0].ID)
			ev := NewEvent(ec, WorkerAppRevokedType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"applicationId": apps[0].ID, "applicationCode": apps[0].Code}))
			out, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, ident, o.R.Identities, ev, cmd))
			if err != nil {
				return Event{}, err
			}
			if live > 0 {
				if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
					sessionsRevokedEvent(ec, ident.ClientID, ReasonAdmin, live, ident.ID, "", apps[0].ID), cmd)); err != nil {
					return Event{}, err
				}
			}
			return out, nil
		},
	}
}

// ── platform user link ───────────────────────────────────────────────────

// CodeInvalidLink is the error code of a link target that cannot be linked;
// the admin API answers it 422.
const CodeInvalidLink = "INVALID_LINK"

// LinkWorkerCommand links a worker to one platform user.
type LinkWorkerCommand struct {
	WorkerRef
	PrincipalID string `json:"principalId"`
}

// LinkWorker links a worker to a platform user, for audit correlation only (it
// grants nothing on either side). The principal must exist, be a USER (not a
// service account), be ACTIVE and be able to access the worker's client
// (anchor, the client itself, or a granted/partner client): 422 INVALID_LINK
// otherwise. A user already linked to another worker of the client is 409
// LINK_EXISTS. Re-linking the same user to the same worker changes nothing but
// still records the event; linking a different user replaces the link.
func (o *Ops) LinkWorker() usecaseop.Operation[LinkWorkerCommand, Event] {
	return usecaseop.Operation[LinkWorkerCommand, Event]{
		Name: "LinkFieldWorker",
		Validate: func(ctx context.Context, cmd LinkWorkerCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if strings.TrimSpace(cmd.PrincipalID) == "" {
				return usecase.Validation("PRINCIPAL_ID_REQUIRED", "principalId is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[LinkWorkerCommand],
		Execute: func(ctx context.Context, cmd LinkWorkerCommand, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return nil, err
			}
			principalID := strings.TrimSpace(cmd.PrincipalID)
			p, err := o.Principals.FindByID(ctx, ids.PrincipalID(principalID))
			if err != nil {
				return nil, repoErr("find_principal", err)
			}
			if err := linkable(p, ident.ClientID); err != nil {
				return nil, err
			}
			other, err := o.R.Identities.FindByClientAndPrincipal(ctx, ident.ClientID, principalID)
			if err != nil {
				return nil, repoErr("find_field_worker", err)
			}
			if other != nil && other.ID != ident.ID {
				return nil, usecase.Conflict("LINK_EXISTS", "that user is already linked to another field worker of this client")
			}
			ident.LinkedPrincipalID = principalID
			ev := NewEvent(ec, WorkerLinkedType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"principalId": principalID}))
			return usecaseop.Save(ident, o.R.Identities, ev), nil
		},
	}
}

// linkable is the rule for a link target: a USER principal that is ACTIVE and
// reaches the client. Its reasons are deliberately coarse.
func linkable(p *principal.Principal, clientID string) error {
	switch {
	case p == nil:
		return usecase.BusinessRule(CodeInvalidLink, "the platform user does not exist")
	case !p.IsUser():
		return usecase.BusinessRule(CodeInvalidLink, "only a platform user (not a service account) can be linked")
	case !p.Active:
		return usecase.BusinessRule(CodeInvalidLink, "the platform user is not active")
	}
	var home *string
	if p.ClientID != nil {
		h := string(*p.ClientID)
		home = &h
	}
	assigned := make([]string, len(p.AssignedClients))
	for n, c := range p.AssignedClients {
		assigned[n] = string(c)
	}
	if !p.Scope.CanAccessClient(clientID, home, assigned) {
		return usecase.BusinessRule(CodeInvalidLink, "the platform user cannot access the worker's client")
	}
	return nil
}

// UnlinkWorker removes a worker's platform user link. Idempotent: an unlinked
// worker stays unlinked (the event is still recorded, with an empty principalId).
func (o *Ops) UnlinkWorker() usecaseop.Operation[WorkerRef, Event] {
	return usecaseop.Operation[WorkerRef, Event]{
		Name:      "UnlinkFieldWorker",
		Validate:  validateRef,
		Authorize: usecaseop.Public[WorkerRef],
		Execute: func(ctx context.Context, cmd WorkerRef, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return nil, err
			}
			previous := ident.LinkedPrincipalID
			ident.LinkedPrincipalID = ""
			ev := NewEvent(ec, WorkerUnlinkedType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"principalId": previous}))
			return usecaseop.Save(ident, o.R.Identities, ev), nil
		},
	}
}

// ── enrolment codes ──────────────────────────────────────────────────────

// CreateEnrolmentCodeCommand mints a single-use device enrolment code. Code is
// the plaintext, generated by the caller with [NewEnrolmentCodeValue]; it is
// never audited and only its hash is stored.
type CreateEnrolmentCodeCommand struct {
	ClientID         string   `json:"clientId"`
	Mode             string   `json:"mode"`
	AppCodes         []string `json:"appCodes"`
	AssignedWorkerID string   `json:"assignedWorkerId,omitempty"`
	Label            string   `json:"label,omitempty"`
	Code             string   `json:"-"`
}

// CreateEnrolmentCodeResult is what the admin needs back.
type CreateEnrolmentCodeResult struct {
	CodeID    string
	ExpiresAt time.Time
}

// CreateEnrolmentCode creates an enrolment code for a mode, applications and,
// for a PERSONAL device, optionally the worker it is assigned to. A SHARED
// code cannot name a worker.
func (o *Ops) CreateEnrolmentCode() usecaseop.TxOperation[CreateEnrolmentCodeCommand, CreateEnrolmentCodeResult] {
	return usecaseop.TxOperation[CreateEnrolmentCodeCommand, CreateEnrolmentCodeResult]{
		Name: "CreateFieldEnrolmentCode",
		Validate: func(_ context.Context, cmd CreateEnrolmentCodeCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			mode := Mode(cmd.Mode)
			if !mode.Valid() {
				return usecase.Validation("MODE_INVALID", "mode must be PERSONAL or SHARED")
			}
			if mode == ModeShared && strings.TrimSpace(cmd.AssignedWorkerID) != "" {
				return usecase.Validation("SHARED_DEVICE_ASSIGNED", "a SHARED device cannot be assigned to a worker")
			}
			if len(cmd.AppCodes) == 0 {
				return usecase.Validation("APP_CODES_REQUIRED", "appCodes must name at least one application")
			}
			if cmd.Code == "" {
				return usecase.Internal("CODE_MISSING", "enrolment code was not generated", nil)
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd CreateEnrolmentCodeCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), &cmd.ClientID)
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd CreateEnrolmentCodeCommand, ec usecase.ExecutionContext) (CreateEnrolmentCodeResult, error) {
			var zero CreateEnrolmentCodeResult
			if err := requireClient(ctx, o.Clients, cmd.ClientID); err != nil {
				return zero, err
			}
			apps, err := o.resolveApps(ctx, cmd.AppCodes, true)
			if err != nil {
				return zero, err
			}
			var assigned *string
			if id := strings.TrimSpace(cmd.AssignedWorkerID); id != "" {
				ident, err := o.loadWorker(ctx, id, cmd.ClientID)
				if err != nil {
					return zero, err
				}
				assigned = &ident.ID
			}
			now := time.Now().UTC()
			code := &EnrolmentCode{
				ID:                 tsid.Generate(tsid.FieldEnrolmentCode),
				ClientID:           cmd.ClientID,
				CodeHash:           HashToken(cmd.Code),
				Mode:               Mode(cmd.Mode),
				AppIDs:             appIDs(apps),
				AssignedIdentityID: assigned,
				Label:              strings.TrimSpace(cmd.Label),
				ExpiresAt:          now.Add(EnrolmentCodeTTL),
				CreatedBy:          ec.PrincipalID,
				CreatedAt:          now,
			}
			data := map[string]any{
				"codeId": code.ID, "clientId": code.ClientID, "mode": cmd.Mode,
				"applicationIds": code.AppIDs, "expiresAt": code.ExpiresAt.Format(time.RFC3339),
			}
			if assigned != nil {
				data["assignedWorkerId"] = *assigned
			}
			ev := NewEvent(ec, EnrolmentCodeCreatedType, "field-device", code.ID, data)
			if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, code, o.R.Codes, ev, cmd)); err != nil {
				return zero, err
			}
			return CreateEnrolmentCodeResult{CodeID: code.ID, ExpiresAt: code.ExpiresAt}, nil
		},
	}
}

// ── devices ──────────────────────────────────────────────────────────────

// DeviceRef addresses a device by id.
type DeviceRef struct {
	ClientID string `json:"clientId,omitempty"`
	ID       string `json:"id"`
}

func validateDeviceRef(_ context.Context, cmd DeviceRef) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

// SuspendDevice pauses a device (resumable) and revokes its sessions. A
// revoked device cannot be suspended.
func (o *Ops) SuspendDevice() usecaseop.TxOperation[DeviceRef, Event] {
	return usecaseop.TxOperation[DeviceRef, Event]{
		Name:      "SuspendFieldDevice",
		Validate:  validateDeviceRef,
		Authorize: usecaseop.Public[DeviceRef],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DeviceRef, ec usecase.ExecutionContext) (Event, error) {
			d, err := o.loadDevice(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			if d.Status == DeviceRevoked {
				return Event{}, usecase.Conflict("DEVICE_REVOKED", "the device is revoked")
			}
			return o.endDeviceSessions(ctx, s, d, DeviceSuspended, DeviceSuspendedType, ReasonAdmin, ec, cmd)
		},
	}
}

// RevokeDevice permanently revokes a device and its sessions (terminal).
// Revoking an already-revoked device is a no-op.
func (o *Ops) RevokeDevice() usecaseop.TxOperation[DeviceRef, Event] {
	return usecaseop.TxOperation[DeviceRef, Event]{
		Name:      "RevokeFieldDevice",
		Validate:  validateDeviceRef,
		Authorize: usecaseop.Public[DeviceRef],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DeviceRef, ec usecase.ExecutionContext) (Event, error) {
			d, err := o.loadDevice(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			if d.Status == DeviceRevoked {
				return Event{}, nil
			}
			return o.endDeviceSessions(ctx, s, d, DeviceRevoked, DeviceRevokedType, ReasonDeviceRevoked, ec, cmd)
		},
	}
}

func (o *Ops) endDeviceSessions(
	ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, d *Device,
	status DeviceStatus, eventType, reason string, ec usecase.ExecutionContext, cmd DeviceRef,
) (Event, error) {
	live, err := countLive(ctx, s, `device_id = $1`, d.ID, "")
	if err != nil {
		return Event{}, repoErr("count_sessions", err)
	}
	d.Status = status
	d.RevokeSessions(reason)
	ev := NewEvent(ec, eventType, "field-device", d.ID, deviceData(d, nil))
	out, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, d, o.R.Devices, ev, cmd))
	if err != nil {
		return Event{}, err
	}
	if live > 0 {
		if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
			sessionsRevokedEvent(ec, d.ClientID, reason, live, "", d.ID, ""), cmd)); err != nil {
			return Event{}, err
		}
	}
	return out, nil
}

// ActivateDevice resumes a suspended device. A revoked device stays revoked.
func (o *Ops) ActivateDevice() usecaseop.Operation[DeviceRef, Event] {
	return usecaseop.Operation[DeviceRef, Event]{
		Name:      "ActivateFieldDevice",
		Validate:  validateDeviceRef,
		Authorize: usecaseop.Public[DeviceRef],
		Execute: func(ctx context.Context, cmd DeviceRef, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			d, err := o.loadDevice(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return nil, err
			}
			if d.Status == DeviceRevoked {
				return nil, usecase.Conflict("DEVICE_REVOKED", "the device is revoked")
			}
			d.Status = DeviceActive
			ev := NewEvent(ec, DeviceActivatedType, "field-device", d.ID, deviceData(d, nil))
			return usecaseop.Save(d, o.R.Devices, ev), nil
		},
	}
}

// ── policy ───────────────────────────────────────────────────────────────

// PutPolicyCommand replaces a client's field policy. Omitted (nil) numeric
// fields take the defaults; when the minimum PIN length is below the default
// the hard-lock threshold defaults to 5.
type PutPolicyCommand struct {
	ClientID           string `json:"clientId"`
	PINMinLength       *int   `json:"pinMinLength,omitempty"`
	SoftLockThreshold  *int   `json:"softLockThreshold,omitempty"`
	SoftLockSecs       *int   `json:"softLockSecs,omitempty"`
	HardLockThreshold  *int   `json:"hardLockThreshold,omitempty"`
	SharedIdleSecs     *int   `json:"sharedIdleSecs,omitempty"`
	PersonalIdleSecs   *int   `json:"personalIdleSecs,omitempty"`
	ShiftMaxSecs       *int   `json:"shiftMaxSecs,omitempty"`
	RequireAttestation bool   `json:"requireAttestation"`
	PINReauthDays      *int   `json:"pinReauthDays,omitempty"`
	// RequireEnrolledDevice and BadgeOnlyLogin are plain flags (false when
	// omitted: a PUT replaces the policy); the two numbers take their
	// defaults (900, 14400) when omitted.
	RequireEnrolledDevice bool `json:"requireEnrolledDevice"`
	UnenrolledIdleSecs    *int `json:"unenrolledIdleSecs,omitempty"`
	BadgeOnlyLogin        bool `json:"badgeOnlyLogin"`
	ImpersonationMaxSecs  *int `json:"impersonationMaxSecs,omitempty"`
}

func pick(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// Build resolves the command against the defaults and validates ranges.
func (c PutPolicyCommand) Build() (Policy, error) {
	def := DefaultPolicy(c.ClientID)
	p := Policy{
		ClientID:           c.ClientID,
		PINMinLength:       pick(c.PINMinLength, def.PINMinLength),
		SoftLockThreshold:  pick(c.SoftLockThreshold, def.SoftLockThreshold),
		SoftLockSecs:       pick(c.SoftLockSecs, def.SoftLockSecs),
		HardLockThreshold:  pick(c.HardLockThreshold, def.HardLockThreshold),
		SharedIdleSecs:     pick(c.SharedIdleSecs, def.SharedIdleSecs),
		PersonalIdleSecs:   pick(c.PersonalIdleSecs, def.PersonalIdleSecs),
		ShiftMaxSecs:       pick(c.ShiftMaxSecs, def.ShiftMaxSecs),
		RequireAttestation: c.RequireAttestation,
		PINReauthDays:      pick(c.PINReauthDays, def.PINReauthDays),

		RequireEnrolledDevice: c.RequireEnrolledDevice,
		UnenrolledIdleSecs:    pick(c.UnenrolledIdleSecs, def.UnenrolledIdleSecs),
		BadgeOnlyLogin:        c.BadgeOnlyLogin,
		ImpersonationMaxSecs:  pick(c.ImpersonationMaxSecs, def.ImpersonationMaxSecs),
	}
	if c.HardLockThreshold == nil && p.PINMinLength < pin.DefaultMinLength {
		p.HardLockThreshold = 5
	}
	bad := func(field, rng string) error {
		return usecase.Validation("POLICY_INVALID", field+" must be "+rng).
			WithDetails(map[string]any{"field": field})
	}
	switch {
	case !pin.ValidMinLength(p.PINMinLength):
		return p, bad("pinMinLength", "between 4 and 12")
	case p.SoftLockThreshold < 1 || p.SoftLockThreshold > 20:
		return p, bad("softLockThreshold", "between 1 and 20")
	case p.SoftLockSecs < 30 || p.SoftLockSecs > 86400:
		return p, bad("softLockSecs", "between 30 and 86400")
	case p.HardLockThreshold < p.SoftLockThreshold || p.HardLockThreshold > 50:
		return p, bad("hardLockThreshold", "at least softLockThreshold and at most 50")
	case p.PINMinLength < pin.DefaultMinLength && p.HardLockThreshold > 5:
		return p, bad("hardLockThreshold", "at most 5 when pinMinLength is below 6")
	case p.SharedIdleSecs < 60 || p.SharedIdleSecs > 86400:
		return p, bad("sharedIdleSecs", "between 60 and 86400")
	case p.PersonalIdleSecs < 60 || p.PersonalIdleSecs > 86400:
		return p, bad("personalIdleSecs", "between 60 and 86400")
	case p.ShiftMaxSecs < 3600 || p.ShiftMaxSecs > 86400:
		return p, bad("shiftMaxSecs", "between 3600 and 86400")
	case p.PINReauthDays < 1 || p.PINReauthDays > 90:
		return p, bad("pinReauthDays", "between 1 and 90")
	case p.UnenrolledIdleSecs < 60 || p.UnenrolledIdleSecs > 86400:
		return p, bad("unenrolledIdleSecs", "between 60 and 86400")
	case p.ImpersonationMaxSecs < 300 || p.ImpersonationMaxSecs > 43200:
		return p, bad("impersonationMaxSecs", "between 300 and 43200")
	}
	return p, nil
}

// PutPolicy sets the client's field policy. Policy is a client-wide setting,
// so an application-confined caller may not change it.
func (o *Ops) PutPolicy() usecaseop.Operation[PutPolicyCommand, Event] {
	return usecaseop.Operation[PutPolicyCommand, Event]{
		Name: "PutFieldPolicy",
		Validate: func(_ context.Context, cmd PutPolicyCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			_, err := cmd.Build()
			return err
		},
		Authorize: func(ctx context.Context, cmd PutPolicyCommand) error {
			ac := auth.FromContext(ctx)
			if err := auth.CheckScopeAccess(ac, &cmd.ClientID); err != nil {
				return err
			}
			if ac.IsApplicationScoped() {
				return usecase.Authorization("APPLICATION_FORBIDDEN", "an application-confined caller cannot change the client's field policy")
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd PutPolicyCommand, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			if err := requireClient(ctx, o.Clients, cmd.ClientID); err != nil {
				return nil, err
			}
			p, err := cmd.Build()
			if err != nil {
				return nil, err
			}
			ev := NewEvent(ec, PolicyUpdatedType, "field-policy", p.ClientID, map[string]any{
				"clientId": p.ClientID, "requireEnrolledDevice": p.RequireEnrolledDevice,
				"unenrolledIdleSecs": p.UnenrolledIdleSecs, "badgeOnlyLogin": p.BadgeOnlyLogin,
				"impersonationMaxSecs": p.ImpersonationMaxSecs,
			})
			return usecaseop.Save(&p, o.R.Policies, ev), nil
		},
	}
}

// validPhone accepts an empty number, or digits with spaces, '-', '.', '(' and
// ')' and at most one leading '+'. It needs 7 to 15 digits (E.164 allows 15)
// and at most 32 characters. The number is stored as entered.
func validPhone(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if len(s) > 32 {
		return false
	}
	digits := 0
	for i, c := range s {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == ' ' || c == '-' || c == '.' || c == '(' || c == ')':
		case c == '+' && i == 0:
		default:
			return false
		}
	}
	return digits >= 7 && digits <= 15
}
