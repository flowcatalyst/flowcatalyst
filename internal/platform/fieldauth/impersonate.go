package fieldauth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Impersonation (docs/field-worker-sign-in-options-spec.md section 4): a
// supervisor acts as a worker on a device (act-as, with a step-up PIN), and a
// console sign-in code signs a worker in on whatever device redeems it. Both
// end in an ordinary-looking session for the WORKER whose token carries an act
// claim for whoever is acting.

func impersonating(c *call) {
	fail(c.w, http.StatusForbidden, "IMPERSONATING", "this session is already signed in as another worker")
}

func impersonationForbidden(c *call) {
	fail(c.w, http.StatusForbidden, "IMPERSONATION_FORBIDDEN", "this worker may not act as another worker")
}

// ── act-as ───────────────────────────────────────────────────────────────

type actAsRequest struct {
	// WorkerCode or WorkerID, exactly one.
	WorkerCode string `json:"workerCode"`
	WorkerID   string `json:"workerId"`
	PIN        string `json:"pin"`
}

// maySupervise reports whether the (row-locked) supervisor may act as
// another worker at now: ACTIVE, allowed to, with a PIN, not locked.
func maySupervise(sup *fieldidentity.Identity, now time.Time) bool {
	return sup != nil && sup.Status == fieldidentity.StatusActive && sup.CanImpersonate && sup.HasPIN() && !sup.IsLocked(now)
}

// actAs is POST /field/auth/act-as (bearer = the supervisor's own field token).
// Order (spec §4.1): the bearer's session is not itself impersonated → the
// supervisor is ACTIVE, canImpersonate and not locked (403, no PIN check) →
// the supervisor's PIN verifies, with the full lockout accounting (a wrong PIN
// counts towards the supervisor's lock and the subject/client buckets) → the
// target is in the same client, ACTIVE, not a supervisor, not the supervisor
// and granted the token's application (any failure is the same 403, the
// reason is recorded) → the device checks of a login. The supervisor's own
// session is untouched.
func (s *State) actAs(c *call) {
	var req actAsRequest
	if !c.decode(&req) {
		return
	}
	claims := c.claims
	if claims.Act != nil {
		impersonating(c)
		return
	}
	if claims.PCR {
		c.w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		fail(c.w, http.StatusUnauthorized, "invalid_token", "a PIN-change-only token cannot act as another worker")
		return
	}
	req.WorkerCode, req.WorkerID = strings.TrimSpace(req.WorkerCode), strings.TrimSpace(req.WorkerID)
	if (req.WorkerCode == "") == (req.WorkerID == "") || req.PIN == "" || len(req.PIN) > maxPINLen {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "exactly one of workerCode and workerId, and pin, are required")
		return
	}
	ctx := c.r.Context()
	clientID := claims.ClientID
	attempt := fieldidentity.LoginAttempt{
		AuthMethod: fieldidentity.MethodActAs, ClientID: clientID, ActorID: claims.Subject,
		WorkerCode: fieldidentity.NormalizeWorkerCode(req.WorkerCode),
	}
	refuse := func(reason string) {
		attempt.Outcome, attempt.FailureReason = fieldidentity.OutcomeFailure, reason
		s.attempt(c, attempt)
	}
	// The token's application's effective policy (its override of the
	// client's); the application's own refusals come later.
	app, err := s.findApp(c, claims.Aud)
	if err != nil {
		s.internal(c, "application", err)
		return
	}
	// Provisional: decided again under both locks for the worker acted as.
	pol, err := s.policyFor(c, clientID, app, nil)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	if c.device == nil && pol.RequireEnrolledDevice {
		refuse("UNKNOWN_DEVICE")
		unknownDevice(c)
		return
	}
	if s.refuseThrottled(c, clientID, "too many failed sign-ins; try again later", refuse) {
		return
	}

	// The target is resolved first, without a lock, and only within the
	// supervisor's client: a worker id of another client resolves to nothing,
	// so its row is never locked (fieldidentity lock_order.go).
	var targetID string
	otherClient := false
	if req.WorkerID != "" {
		t, err := s.Ops.R.Identities.FindByID(ctx, req.WorkerID)
		if err != nil {
			s.internal(c, "find target", err)
			return
		}
		switch {
		case t == nil: // unknown: refused after the PIN, as before
		case t.ClientID == clientID:
			targetID = t.ID
		default:
			otherClient = true
		}
	} else {
		t, err := s.Ops.R.Identities.FindByClientAndCode(ctx, clientID, req.WorkerCode)
		if err != nil {
			s.internal(c, "find target", err)
			return
		}
		if t != nil {
			targetID = t.ID
		}
	}

	// Step 1, under the supervisor's own lock only: the supervisor may act and
	// their PIN verifies (a wrong PIN is counted and committed here). The
	// target is not locked while the PIN is hashed.
	tx, err := s.Ops.R.Sessions.BeginAuth(ctx)
	if err != nil {
		s.internal(c, "begin", err)
		return
	}
	defer tx.Rollback(ctx)
	sup, err := tx.LockIdentityByID(ctx, claims.Subject)
	if err != nil {
		s.internal(c, "lock supervisor", err)
		return
	}
	now := time.Now().UTC()
	if !maySupervise(sup, now) {
		tx.Rollback(ctx)
		refuse("SUPERVISOR_NOT_PERMITTED")
		impersonationForbidden(c)
		return
	}

	// Step-up: an unattended signed-in handheld must not let anyone act as
	// anyone. The attempt is the supervisor's until a target is found.
	attempt.IdentityID, attempt.WorkerCode = sup.ID, sup.WorkerCode
	verified := sup.StoredPIN()
	ok, _, verr := s.Ops.Hasher.Verify(sup.ClientID, sup.ID, req.PIN, verified)
	if verr != nil {
		tx.Rollback(ctx)
		if errors.Is(verr, pin.ErrUnconfigured) {
			fail(c.w, http.StatusServiceUnavailable, fieldidentity.CodeUnconfigured, "field identity is unavailable")
			return
		}
		if errors.Is(verr, pin.ErrPepperRetired) {
			refuse("PIN_RESET_REQUIRED")
			pinResetRequired(c, sup.ID)
			return
		}
		s.internal(c, "verify", verr)
		return
	}
	if !ok {
		if err := tx.RecordFailure(ctx, sup, pol, now); err != nil {
			s.internal(c, "record failure", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			s.internal(c, "commit failure", err)
			return
		}
		s.noteFailure(c, clientID)
		s.noteLockout(c, tx, sup)
		refuse("PIN_INVALID")
		invalidCredentials(c)
		return
	}
	// The PIN is right. A target outside the client (or none) is refused as
	// before, after the PIN and without ever locking it.
	if targetID == "" {
		tx.Rollback(ctx)
		if otherClient {
			refuse("TARGET_OTHER_CLIENT")
		} else {
			refuse("TARGET_NOT_FOUND")
		}
		noAppAccess(c)
		return
	}
	tx.Rollback(ctx) // nothing of step 1 is kept: the reset of the counter is step 2's

	// Step 2: the supervisor and the target in id order, then everything step
	// 1 decided on, again under the locks: the supervisor may still act and
	// their PIN is the one that verified.
	tx, err = s.Ops.R.Sessions.BeginAuth(ctx)
	if err != nil {
		s.internal(c, "begin", err)
		return
	}
	defer tx.Rollback(ctx)
	if err := tx.LockWorkers(ctx, sup.ID, targetID); err != nil {
		s.internal(c, "lock workers", err)
		return
	}
	sup, err = tx.LockIdentityByID(ctx, claims.Subject)
	if err != nil {
		s.internal(c, "lock supervisor", err)
		return
	}
	now = time.Now().UTC()
	if !maySupervise(sup, now) || sup.StoredPIN() != verified {
		tx.Rollback(ctx)
		refuse("SUPERVISOR_NOT_PERMITTED")
		impersonationForbidden(c)
		return
	}
	if err := tx.ResetFailures(ctx, sup, now); err != nil {
		s.internal(c, "reset failures", err)
		return
	}

	target, err := tx.LockIdentityByID(ctx, targetID)
	if err != nil {
		s.internal(c, "lock target", err)
		return
	}
	// The policy for the worker acted as, under both locks: their
	// application's override applies only if they hold the application.
	if target != nil {
		if pol, err = s.policyFor(c, clientID, app, target); err != nil {
			s.internal(c, "policy", err)
			return
		}
		if c.device == nil && pol.RequireEnrolledDevice {
			tx.Rollback(ctx)
			refuse("UNKNOWN_DEVICE")
			unknownDevice(c)
			return
		}
	}
	dev := c.device
	reason := ""
	switch {
	case target == nil:
		reason = "TARGET_NOT_FOUND"
	case target.ClientID != sup.ClientID:
		reason = "TARGET_OTHER_CLIENT"
	case target.ID == sup.ID:
		reason = "TARGET_IS_SELF"
	case target.Status != fieldidentity.StatusActive:
		reason = "TARGET_INACTIVE"
	case target.CanImpersonate:
		reason = "TARGET_IS_SUPERVISOR"
	case app == nil || !app.Active:
		reason = "APP_UNAVAILABLE"
	case !target.HasApp(string(app.ID)):
		reason = "NO_GRANT"
	case dev != nil && !dev.HasApp(string(app.ID)):
		reason = "DEVICE_APP_NOT_ENROLLED"
	case dev != nil && dev.Mode == fieldidentity.ModePersonal && dev.AssignedIdentityID != nil &&
		*dev.AssignedIdentityID != target.ID && *dev.AssignedIdentityID != sup.ID:
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
	if reason != "" {
		tx.Rollback(ctx)
		if target != nil && target.ClientID == sup.ClientID {
			attempt.IdentityID, attempt.WorkerCode = target.ID, target.WorkerCode
		}
		if app != nil {
			attempt.ApplicationID = string(app.ID)
		}
		refuse(reason)
		noAppAccess(c)
		return
	}
	attempt.IdentityID, attempt.WorkerCode, attempt.ApplicationID = target.ID, target.WorkerCode, string(app.ID)

	// The impersonated session ends no later than the supervisor's own session
	// or the policy's impersonation limit.
	head, err := tx.LockFamilyHead(ctx, claims.SessionID)
	if err != nil {
		s.internal(c, "supervisor session", err)
		return
	}
	if head == nil {
		tx.Rollback(ctx)
		c.w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		fail(c.w, http.StatusUnauthorized, "invalid_token", "the session has ended")
		return
	}
	token, hash, err := newRefreshToken()
	if err != nil {
		s.internal(c, "refresh token", err)
		return
	}
	at := now.Truncate(time.Microsecond)
	sess := fieldidentity.NewSession(at, target, string(app.ID), dev, c.proof.JKT, hash, pol)
	sess.AMR = []string{"pin", "imp"}
	sess.ActorType, sess.ActorID = fieldidentity.ActorFieldWorker, sup.ID
	sess.CapAbsolute(at.Add(time.Duration(pol.ImpersonationMaxSecs) * time.Second))
	sess.CapAbsolute(head.AbsoluteExpiresAt)
	if err := tx.InsertSession(ctx, sess); err != nil {
		s.internal(c, "insert session", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(c, "commit act-as", err)
		return
	}
	attempt.Outcome = fieldidentity.OutcomeSuccess
	s.attempt(c, attempt)
	s.noteImpersonated(c, fieldidentity.ImpersonationInfo{
		WorkerID: target.ID, ClientID: target.ClientID, AppCode: app.Code, ActorType: fieldidentity.ActorFieldWorker,
		ActorID: sup.ID, Method: fieldidentity.MethodActAs, DeviceID: sess.DeviceID, SessionID: sess.FamilyID,
	})
	s.issue(c, target, cl, app, sess, token, &actor{ID: sup.ID, Type: "field_worker", Name: sup.Name})
}

// noteImpersonated emits the impersonated event after the session committed.
// The session and the attempt row are the durable record; a failure here is
// logged, never an auth decision.
func (s *State) noteImpersonated(c *call, info fieldidentity.ImpersonationInfo) {
	if _, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.NoteImpersonated(), info,
		usecase.NewExecutionContext(info.ActorID)); err != nil {
		slog.Error("field impersonated event failed", "worker", info.WorkerID, "actor", info.ActorID, "err", err)
	}
}

// ── sign-in code ─────────────────────────────────────────────────────────

type signInCodeRequest struct {
	Code string `json:"code"`
}

// signInCode is POST /field/auth/sign-in-code: redeem a console sign-in code
// (single use) for a session as the code's worker in the code's application.
// An unknown, used or expired code, or one whose creator is no longer active,
// is 401 INVALID_CODE and counts against the subject (device or IP) bucket.
func (s *State) signInCode(c *call) {
	var req signInCodeRequest
	if !c.decode(&req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" || len(req.Code) > 64 {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "code is required")
		return
	}
	attempt := fieldidentity.LoginAttempt{AuthMethod: fieldidentity.MethodSignInCode}
	if c.device != nil {
		attempt.ClientID = c.device.ClientID
	}
	refuse := func(reason string) {
		attempt.Outcome, attempt.FailureReason = fieldidentity.OutcomeFailure, reason
		s.attempt(c, attempt)
	}
	// Before the code is known there is no client: the subject bucket alone.
	if s.refuseThrottled(c, attempt.ClientID, "too many failed sign-ins; try again later", refuse) {
		return
	}
	token, hash, err := newRefreshToken()
	if err != nil {
		s.internal(c, "refresh token", err)
		return
	}
	res, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.RedeemSignInCode(), fieldidentity.RedeemSignInCodeCommand{
		Code: req.Code, Device: c.device, JKT: c.proof.JKT, RefreshHash: hash,
	}, usecase.NewExecutionContext(fieldidentity.PrincipalSignInCode))
	if err != nil {
		if ue := usecase.AsError(err); ue != nil {
			switch ue.Code {
			case fieldidentity.CodeInvalidSignInCode:
				s.noteFailure(c, attempt.ClientID)
				refuse("INVALID_CODE")
			case fieldidentity.CodeUnknownDevice, fieldidentity.CodeNoAppAccess:
				refuse(ue.Code)
			}
		}
		useCaseError(c.w, err)
		return
	}
	attempt.Outcome = fieldidentity.OutcomeSuccess
	attempt.ClientID, attempt.IdentityID, attempt.WorkerCode = res.Worker.ClientID, res.Worker.ID, res.Worker.WorkerCode
	attempt.ApplicationID, attempt.ActorID = string(res.App.ID), res.Session.ActorID
	s.attempt(c, attempt)
	s.issue(c, res.Worker, res.Client, res.App, res.Session, token,
		&actor{ID: res.Session.ActorID, Type: "principal", Name: res.ActorName})
}
