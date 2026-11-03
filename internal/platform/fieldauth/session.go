package fieldauth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── login ────────────────────────────────────────────────────────────────

type loginRequest struct {
	AppCode    string `json:"appCode"`
	WorkerCode string `json:"workerCode"`
	PIN        string `json:"pin"`
}

func invalidCredentials(c *call) {
	fail(c.w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid worker code or PIN")
}

func noAppAccess(c *call) {
	fail(c.w, http.StatusForbidden, "NO_APP_ACCESS", "this worker cannot sign in to that application on this device")
}

// login is POST /field/auth/login. Order (spec §4.2): device ACTIVE (prelude)
// → device/client budget → worker exists → worker ACTIVE → hard lock → soft
// lock → PIN verify → (success only) grant, application, device and
// assignment checks. Unknown and suspended workers cost a dummy verify so
// the response time does not tell them apart from a wrong PIN.
func (s *State) login(c *call) {
	var req loginRequest
	if !c.decode(&req) {
		return
	}
	req.WorkerCode = strings.TrimSpace(req.WorkerCode)
	if req.WorkerCode == "" || req.PIN == "" || strings.TrimSpace(req.AppCode) == "" {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "appCode, workerCode and pin are required")
		return
	}
	ctx := c.r.Context()
	dev := c.device
	attempt := fieldidentity.LoginAttempt{WorkerCode: fieldidentity.NormalizeWorkerCode(req.WorkerCode), ApplicationID: ""}
	refuse := func(reason string) {
		attempt.Outcome, attempt.FailureReason = fieldidentity.OutcomeFailure, reason
		s.attempt(c, attempt)
	}

	if scope, secs, hit := s.throttled(c, dev.ClientID, dev.ID); hit {
		refuse("THROTTLED_" + scope)
		retryAfter(c.w, secs)
		fail(c.w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "too many failed sign-ins; try again later")
		return
	}
	if len(req.PIN) > maxPINLen {
		s.Ops.Hasher.Dummy(req.PIN[:maxPINLen])
		refuse("PIN_INVALID")
		invalidCredentials(c)
		return
	}

	pol, err := s.Ops.R.Policies.Effective(ctx, dev.ClientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	tx, err := s.Ops.R.Sessions.BeginAuth(ctx)
	if err != nil {
		s.internal(c, "begin", err)
		return
	}
	defer tx.Rollback(ctx)
	ident, err := tx.LockIdentityByCode(ctx, dev.ClientID, req.WorkerCode)
	if err != nil {
		s.internal(c, "lock identity", err)
		return
	}
	if ident == nil {
		tx.Rollback(ctx)
		s.Ops.Hasher.Dummy(req.PIN)
		s.noteFailure(c, dev.ClientID, dev.ID)
		refuse("UNKNOWN_WORKER")
		invalidCredentials(c)
		return
	}
	attempt.IdentityID = ident.ID
	if ident.Status != fieldidentity.StatusActive || !ident.HasPIN() {
		tx.Rollback(ctx)
		s.Ops.Hasher.Dummy(req.PIN)
		if ident.Status != fieldidentity.StatusActive {
			refuse("WORKER_SUSPENDED")
		} else {
			refuse("NO_PIN")
		}
		invalidCredentials(c)
		return
	}
	now := time.Now().UTC()
	if ident.HardLocked {
		tx.Rollback(ctx)
		refuse("WORKER_LOCKED")
		fail(c.w, http.StatusLocked, "WORKER_LOCKED", "this worker is locked; ask a supervisor to unlock them")
		return
	}
	if ident.LockedUntil != nil && ident.LockedUntil.After(now) {
		tx.Rollback(ctx)
		refuse("WORKER_SOFT_LOCKED")
		retryAfter(c.w, uint32(ident.LockedUntil.Sub(now).Seconds())+1)
		fail(c.w, http.StatusTooManyRequests, "WORKER_LOCKED_TEMPORARILY", "too many wrong PINs; try again later")
		return
	}

	ok, rehash, verr := s.Ops.Hasher.Verify(ident.ClientID, ident.ID, req.PIN, *ident.PINHash, ident.PINScheme)
	if verr != nil {
		tx.Rollback(ctx)
		if errors.Is(verr, pin.ErrUnconfigured) {
			fail(c.w, http.StatusServiceUnavailable, fieldidentity.CodeUnconfigured, "field identity is unavailable")
			return
		}
		s.internal(c, "verify", verr)
		return
	}
	if !ok {
		if err := tx.RecordFailure(ctx, ident, pol, now); err != nil {
			s.internal(c, "record failure", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			s.internal(c, "commit failure", err)
			return
		}
		s.noteFailure(c, dev.ClientID, dev.ID)
		refuse("PIN_INVALID")
		invalidCredentials(c)
		return
	}

	// The PIN is right. Only now do access checks run, and every refusal
	// answers the same 403 (the reason is recorded, not returned).
	app, err := s.findApp(c, req.AppCode)
	if err != nil {
		s.internal(c, "application", err)
		return
	}
	reason := ""
	switch {
	case app == nil || !app.Active:
		reason = "APP_UNAVAILABLE"
	case !ident.HasApp(string(app.ID)):
		reason = "NO_GRANT"
	case !dev.HasApp(string(app.ID)):
		reason = "DEVICE_APP_NOT_ENROLLED"
	case dev.Mode == fieldidentity.ModePersonal && dev.AssignedIdentityID != nil && *dev.AssignedIdentityID != ident.ID:
		reason = "DEVICE_ASSIGNED_TO_OTHER"
	}
	cl, err := s.activeClient(c, dev.ClientID)
	if err != nil {
		s.internal(c, "client", err)
		return
	}
	if reason == "" && cl == nil {
		reason = "CLIENT_INACTIVE"
	}
	if reason != "" {
		tx.Rollback(ctx)
		if app != nil {
			attempt.ApplicationID = string(app.ID)
		}
		refuse(reason)
		noAppAccess(c)
		return
	}
	attempt.ApplicationID = string(app.ID)

	var newHash *string
	if rehash {
		h, err := s.Ops.Hasher.Hash(ident.ClientID, ident.ID, req.PIN)
		if err != nil {
			s.internal(c, "rehash", err)
			return
		}
		newHash = &h
	}
	if err := tx.RecordSuccess(ctx, ident, newHash, now); err != nil {
		s.internal(c, "record success", err)
		return
	}
	token, hash, err := newRefreshToken()
	if err != nil {
		s.internal(c, "refresh token", err)
		return
	}
	id := fieldidentity.NewSessionID()
	sess := &fieldidentity.Session{
		ID: id, FamilyID: id, IdentityID: ident.ID, DeviceID: dev.ID, ApplicationID: string(app.ID),
		RefreshHash: hash, AMR: []string{"pin"}, AuthTime: now.Truncate(time.Microsecond),
		IssuedAt:          now.Truncate(time.Microsecond),
		IdleExpiresAt:     now.Add(time.Duration(idleSecs(pol, dev.Mode)) * time.Second),
		AbsoluteExpiresAt: now.Add(time.Duration(absoluteSecs(pol, dev.Mode)) * time.Second),
		PINChangeRequired: ident.PINChangeRequired,
	}
	if err := tx.InsertSession(ctx, sess); err != nil {
		s.internal(c, "insert session", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(c, "commit login", err)
		return
	}
	attempt.Outcome = fieldidentity.OutcomeSuccess
	s.attempt(c, attempt)
	s.issue(c, ident, cl, app, sess, token)
}

// ── refresh ──────────────────────────────────────────────────────────────

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func invalidGrant(c *call) {
	fail(c.w, http.StatusUnauthorized, "INVALID_GRANT", "the refresh token is invalid, used or revoked")
}

// refresh is POST /field/auth/refresh: rotate the refresh token. A rotated
// token presented again within RetryGrace by the session's own device is a
// lost-response retry (the live child is rotated and a fresh one issued);
// otherwise it is reuse and the whole family is revoked.
func (s *State) refresh(c *call) {
	var req refreshRequest
	if !c.decode(&req) {
		return
	}
	if !strings.HasPrefix(req.RefreshToken, "fwr_") {
		invalidGrant(c)
		return
	}
	ctx := c.r.Context()
	hash := fieldidentity.HashToken(req.RefreshToken)
	probe, err := s.Ops.R.Sessions.FindByRefreshHash(ctx, hash)
	if err != nil {
		s.internal(c, "find session", err)
		return
	}
	if probe == nil {
		invalidGrant(c)
		return
	}
	dev := c.device

	// Lock order is identity, then session (the PIN-change use case takes the
	// same order), so a concurrent PIN change cannot deadlock a refresh.
	tx, err := s.Ops.R.Sessions.BeginAuth(ctx)
	if err != nil {
		s.internal(c, "begin", err)
		return
	}
	defer tx.Rollback(ctx)
	ident, err := tx.LockIdentityByID(ctx, probe.IdentityID)
	if err != nil {
		s.internal(c, "lock identity", err)
		return
	}
	sess, err := tx.LockSessionByHash(ctx, hash)
	if err != nil {
		s.internal(c, "lock session", err)
		return
	}
	if sess == nil || ident == nil {
		invalidGrant(c)
		return
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	reuse := func() {
		tx.Rollback(ctx)
		if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.RevokeSessionFamilyOnReuse(), fieldidentity.RevokeFamilyCommand{
			ClientID: dev.ClientID, WorkerID: sess.IdentityID, DeviceID: sess.DeviceID, FamilyID: sess.FamilyID,
		}, usecase.NewExecutionContext(sess.IdentityID)); err != nil {
			s.internal(c, "revoke family", err)
			return
		}
		invalidGrant(c)
	}

	// A token of another device, live or rotated, is theft: kill the family.
	if sess.DeviceID != dev.ID {
		reuse()
		return
	}
	cur := sess
	if sess.RevokedAt != nil {
		if sess.RevokedReason != fieldidentity.ReasonRotated {
			invalidGrant(c)
			return
		}
		if sess.RotatedAt == nil || now.Sub(*sess.RotatedAt) > RetryGrace {
			reuse()
			return
		}
		head, err := tx.LockFamilyHead(ctx, sess.FamilyID)
		if err != nil {
			s.internal(c, "family head", err)
			return
		}
		if head == nil {
			invalidGrant(c) // the family is already over
			return
		}
		cur = head
	}
	if !now.Before(cur.IdleExpiresAt) || !now.Before(cur.AbsoluteExpiresAt) {
		fail(c.w, http.StatusUnauthorized, "SESSION_EXPIRED", "the session has expired; sign in again")
		return
	}
	if ident.Status != fieldidentity.StatusActive {
		invalidGrant(c)
		return
	}
	app, err := s.Ops.Apps.FindByID(ctx, cur.ApplicationID)
	if err != nil {
		s.internal(c, "application", err)
		return
	}
	cl, err := s.activeClient(c, dev.ClientID)
	if err != nil {
		s.internal(c, "client", err)
		return
	}
	if app == nil || !app.Active || cl == nil || !ident.HasApp(string(app.ID)) || !dev.HasApp(string(app.ID)) {
		noAppAccess(c)
		return
	}
	pol, err := s.Ops.R.Policies.Effective(ctx, dev.ClientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}

	token, newHash, err := newRefreshToken()
	if err != nil {
		s.internal(c, "refresh token", err)
		return
	}
	if err := tx.Rotate(ctx, cur.ID, now); err != nil {
		s.internal(c, "rotate", err)
		return
	}
	child := &fieldidentity.Session{
		ID: fieldidentity.NewSessionID(), FamilyID: cur.FamilyID, IdentityID: cur.IdentityID, DeviceID: cur.DeviceID,
		ApplicationID: cur.ApplicationID, RefreshHash: newHash, AMR: cur.AMR, AuthTime: cur.AuthTime,
		IssuedAt: now, IdleExpiresAt: now.Add(time.Duration(idleSecs(pol, dev.Mode)) * time.Second),
		AbsoluteExpiresAt: cur.AbsoluteExpiresAt,
		// The worker's current flag is the truth: a PIN change clears it.
		PINChangeRequired: ident.PINChangeRequired,
	}
	if err := tx.InsertSession(ctx, child); err != nil {
		s.internal(c, "insert session", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(c, "commit refresh", err)
		return
	}
	s.issue(c, ident, cl, app, child, token)
}

// ── PIN change ───────────────────────────────────────────────────────────

type pinRequest struct {
	CurrentPIN string `json:"currentPin"`
	NewPIN     string `json:"newPin"`
}

// changePin is POST /field/auth/pin (bearer). The current PIN goes through the
// same lockout accounting as a sign-in; the new PIN is policy-checked and set
// by the ChangeOwnPin use case, which revokes the worker's other sessions and
// keeps this one.
func (s *State) changePin(c *call) {
	var req pinRequest
	if !c.decode(&req) {
		return
	}
	if req.CurrentPIN == "" || req.NewPIN == "" || len(req.CurrentPIN) > maxPINLen || len(req.NewPIN) > maxPINLen {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "currentPin and newPin are required")
		return
	}
	ctx := c.r.Context()
	dev := c.device
	attempt := fieldidentity.LoginAttempt{WorkerCode: c.claims.WorkerCode, IdentityID: c.claims.Subject, AuthMethod: "PIN_CHANGE"}
	refuse := func(reason string) {
		attempt.Outcome, attempt.FailureReason = fieldidentity.OutcomeFailure, reason
		s.attempt(c, attempt)
	}
	if scope, secs, hit := s.throttled(c, dev.ClientID, dev.ID); hit {
		refuse("THROTTLED_" + scope)
		retryAfter(c.w, secs)
		fail(c.w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "too many failed PIN attempts; try again later")
		return
	}
	pol, err := s.Ops.R.Policies.Effective(ctx, dev.ClientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	tx, err := s.Ops.R.Sessions.BeginAuth(ctx)
	if err != nil {
		s.internal(c, "begin", err)
		return
	}
	defer tx.Rollback(ctx)
	ident, err := tx.LockIdentityByID(ctx, c.claims.Subject)
	if err != nil {
		s.internal(c, "lock identity", err)
		return
	}
	if ident == nil || ident.Status != fieldidentity.StatusActive || !ident.HasPIN() {
		tx.Rollback(ctx)
		s.Ops.Hasher.Dummy(req.CurrentPIN)
		refuse("WORKER_UNAVAILABLE")
		invalidCredentials(c)
		return
	}
	now := time.Now().UTC()
	if ident.HardLocked {
		tx.Rollback(ctx)
		refuse("WORKER_LOCKED")
		fail(c.w, http.StatusLocked, "WORKER_LOCKED", "this worker is locked; ask a supervisor to unlock them")
		return
	}
	if ident.LockedUntil != nil && ident.LockedUntil.After(now) {
		tx.Rollback(ctx)
		refuse("WORKER_SOFT_LOCKED")
		retryAfter(c.w, uint32(ident.LockedUntil.Sub(now).Seconds())+1)
		fail(c.w, http.StatusTooManyRequests, "WORKER_LOCKED_TEMPORARILY", "too many wrong PINs; try again later")
		return
	}
	ok, _, verr := s.Ops.Hasher.Verify(ident.ClientID, ident.ID, req.CurrentPIN, *ident.PINHash, ident.PINScheme)
	if verr != nil {
		tx.Rollback(ctx)
		s.internal(c, "verify", verr)
		return
	}
	if !ok {
		if err := tx.RecordFailure(ctx, ident, pol, now); err != nil {
			s.internal(c, "record failure", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			s.internal(c, "commit failure", err)
			return
		}
		s.noteFailure(c, dev.ClientID, dev.ID)
		refuse("PIN_INVALID")
		invalidCredentials(c)
		return
	}
	if err := tx.ResetFailures(ctx, ident, now); err != nil {
		s.internal(c, "reset failures", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(c, "commit pin check", err)
		return
	}
	if req.NewPIN == req.CurrentPIN {
		failWith(c.w, http.StatusUnprocessableEntity, "PIN_POLICY", "the new PIN must differ from the current PIN",
			map[string]any{"violations": []string{"SAME_AS_CURRENT"}})
		return
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.ChangeOwnPin(), fieldidentity.ChangeOwnPinCommand{
		WorkerID: ident.ID, FamilyID: c.claims.SessionID, NewPIN: req.NewPIN,
	}, usecase.NewExecutionContext(ident.ID)); err != nil {
		useCaseError(c.w, err)
		return
	}
	c.w.WriteHeader(http.StatusNoContent)
}

// ── logout ───────────────────────────────────────────────────────────────

type logoutRequest struct {
	RefreshToken string `json:"refreshToken"`
	Scope        string `json:"scope"`
}

// logout is POST /field/auth/logout. Idempotent: an unknown, already-revoked
// or foreign token is a 204 that changes nothing. Scope "session" ends the
// token's login family; "device-all" ends every session on this device (a
// SHARED device hand-over).
func (s *State) logout(c *call) {
	var req logoutRequest
	if !c.decode(&req) {
		return
	}
	scope := req.Scope
	if scope == "" {
		scope = "session"
	}
	if scope != "session" && scope != "device-all" {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", `scope must be "session" or "device-all"`)
		return
	}
	ctx := c.r.Context()
	sess, err := s.Ops.R.Sessions.FindByRefreshHash(ctx, fieldidentity.HashToken(req.RefreshToken))
	if err != nil {
		s.internal(c, "find session", err)
		return
	}
	if sess == nil || sess.DeviceID != c.device.ID {
		c.w.WriteHeader(http.StatusNoContent)
		return
	}
	tx, err := s.Ops.R.Sessions.BeginAuth(ctx)
	if err != nil {
		s.internal(c, "begin", err)
		return
	}
	defer tx.Rollback(ctx)
	now := time.Now().UTC()
	if scope == "device-all" {
		_, err = tx.RevokeDeviceSessions(ctx, c.device.ID, fieldidentity.ReasonLogout, now)
	} else {
		_, err = tx.RevokeFamilySessions(ctx, sess.FamilyID, fieldidentity.ReasonLogout, now)
	}
	if err != nil {
		s.internal(c, "revoke", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(c, "commit logout", err)
		return
	}
	c.w.WriteHeader(http.StatusNoContent)
}
