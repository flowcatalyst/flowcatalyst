package fieldauth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── login ────────────────────────────────────────────────────────────────

type loginRequest struct {
	AppCode string `json:"appCode"`
	// ClientIdentifier names the client (the client_identifier claim). Required
	// from an unenrolled key with a workerCode; optional otherwise.
	ClientIdentifier string `json:"clientIdentifier"`
	// WorkerCode or BadgeToken, exactly one.
	WorkerCode string `json:"workerCode"`
	BadgeToken string `json:"badgeToken"`
	PIN        string `json:"pin"`
}

func invalidCredentials(c *call) {
	fail(c.w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid worker code or PIN")
}

func noAppAccess(c *call) {
	fail(c.w, http.StatusForbidden, "NO_APP_ACCESS", "this worker cannot sign in to that application on this device")
}

func unknownDevice(c *call) {
	fail(c.w, http.StatusUnauthorized, "UNKNOWN_DEVICE", "this device is not enrolled")
}

// refuseThrottled answers 429 when the request's subject (device, or IP for an
// unenrolled key) or client PIN-failure bucket is full, reporting whether it
// did. refuse records the attempt.
func (s *State) refuseThrottled(c *call, clientID, msg string, refuse func(string)) bool {
	scope, secs, hit := s.throttled(c, clientID)
	if !hit {
		return false
	}
	refuse("THROTTLED_" + scope)
	retryAfter(c.w, secs)
	fail(c.w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", msg)
	return true
}

// login is POST /field/auth/login. Order (spec §1.3, §3.3): the client is
// resolved (the device's, the badge's, or the named one) → the client's
// enrolled-device requirement → subject/client budget → worker exists →
// worker ACTIVE → hard lock → soft lock → PIN verify (or, for a badge on an
// enrolled device whose policy allows it, no PIN) → (success only) grant,
// application and device checks. Unknown and suspended workers, unknown
// clients and unknown badges cost a dummy verify so the response time does not
// tell them apart from a wrong PIN.
func (s *State) login(c *call) {
	var req loginRequest
	if !c.decode(&req) {
		return
	}
	req.WorkerCode = strings.TrimSpace(req.WorkerCode)
	req.ClientIdentifier = strings.TrimSpace(req.ClientIdentifier)
	req.BadgeToken = fieldidentity.NormalizeToken(req.BadgeToken)
	byBadge := req.BadgeToken != ""
	dev := c.device
	switch {
	case strings.TrimSpace(req.AppCode) == "":
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "appCode is required")
		return
	case (req.WorkerCode != "") == byBadge:
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "exactly one of workerCode and badgeToken is required")
		return
	case !byBadge && req.PIN == "":
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "pin is required")
		return
	case !byBadge && dev == nil && req.ClientIdentifier == "":
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "clientIdentifier is required from a device that is not enrolled")
		return
	}
	ctx := c.r.Context()
	method := fieldidentity.MethodPIN
	switch {
	case byBadge && req.PIN != "":
		method = fieldidentity.MethodBadgePIN
	case byBadge:
		method = fieldidentity.MethodBadge
	}
	attempt := fieldidentity.LoginAttempt{WorkerCode: fieldidentity.NormalizeWorkerCode(req.WorkerCode), AuthMethod: method}
	refuse := func(reason string) {
		attempt.Outcome, attempt.FailureReason = fieldidentity.OutcomeFailure, reason
		s.attempt(c, attempt)
	}
	// dummy burns one PIN verify so the refusal costs what a real one does.
	dummy := func() {
		p := req.PIN
		if len(p) > maxPINLen {
			p = p[:maxPINLen]
		}
		s.Ops.Hasher.Dummy(p)
	}

	// ── resolve the client ──────────────────────────────────────────────
	var clientID string
	var badge *fieldidentity.Badge
	if dev != nil {
		clientID = dev.ClientID
		attempt.ClientID = clientID
	}
	// The subject bucket first: an unknown client or badge must not buy a
	// dummy verify for a throttled caller.
	if s.refuseThrottled(c, clientID, "too many failed sign-ins; try again later", refuse) {
		return
	}
	if len(req.PIN) > maxPINLen {
		dummy()
		s.noteFailure(c, clientID)
		refuse("PIN_INVALID")
		invalidCredentials(c)
		return
	}
	if byBadge {
		var err error
		if fieldidentity.WellFormedToken(fieldidentity.BadgeTokenPrefix, req.BadgeToken) {
			badge, err = s.Ops.R.Badges.FindByHash(ctx, fieldidentity.HashToken(req.BadgeToken))
			if err != nil {
				s.internal(c, "find badge", err)
				return
			}
		}
		switch {
		case badge == nil:
			dummy()
			s.noteFailure(c, clientID)
			refuse("UNKNOWN_BADGE")
			invalidCredentials(c)
			return
		case badge.Status != fieldidentity.BadgeActive:
			dummy()
			s.noteFailure(c, badge.ClientID)
			attempt.ClientID, attempt.IdentityID = badge.ClientID, badge.IdentityID
			refuse("BADGE_REVOKED")
			invalidCredentials(c)
			return
		case dev != nil && badge.ClientID != dev.ClientID:
			// A badge of another client on an enrolled device: indistinguishable
			// from an unknown badge.
			dummy()
			s.noteFailure(c, dev.ClientID)
			refuse("BADGE_WRONG_CLIENT")
			invalidCredentials(c)
			return
		}
		clientID = badge.ClientID
		attempt.ClientID, attempt.IdentityID = clientID, badge.IdentityID
		if req.ClientIdentifier != "" {
			named, err := s.Ops.Clients.FindByIdentifier(ctx, req.ClientIdentifier)
			if err != nil {
				s.internal(c, "client", err)
				return
			}
			if named == nil || string(named.ID) != clientID {
				dummy()
				s.noteFailure(c, clientID)
				refuse("CLIENT_MISMATCH")
				invalidCredentials(c)
				return
			}
		}
	} else if req.ClientIdentifier != "" {
		named, err := s.Ops.Clients.FindByIdentifier(ctx, req.ClientIdentifier)
		if err != nil {
			s.internal(c, "client", err)
			return
		}
		switch {
		case dev != nil:
			if named == nil || string(named.ID) != dev.ClientID {
				fail(c.w, http.StatusBadRequest, "CLIENT_MISMATCH", "clientIdentifier names a different client than this device")
				return
			}
		case named == nil || named.Status != client.StatusActive:
			// No client enumeration: an unknown or inactive client is a wrong PIN.
			dummy()
			s.noteFailure(c, "")
			refuse("UNKNOWN_CLIENT")
			invalidCredentials(c)
			return
		default:
			clientID = string(named.ID)
			attempt.ClientID = clientID
		}
	}

	pol, err := s.Ops.R.Policies.Effective(ctx, clientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	if dev == nil && pol.RequireEnrolledDevice {
		refuse("UNKNOWN_DEVICE")
		unknownDevice(c)
		return
	}
	if dev == nil && s.refuseThrottled(c, clientID, "too many failed sign-ins; try again later", refuse) {
		return
	}

	// ── the worker ──────────────────────────────────────────────────────
	tx, err := s.Ops.R.Sessions.BeginAuth(ctx)
	if err != nil {
		s.internal(c, "begin", err)
		return
	}
	defer tx.Rollback(ctx)
	var ident *fieldidentity.Identity
	if byBadge {
		ident, err = tx.LockIdentityByID(ctx, badge.IdentityID)
	} else {
		ident, err = tx.LockIdentityByCode(ctx, clientID, req.WorkerCode)
	}
	if err != nil {
		s.internal(c, "lock identity", err)
		return
	}
	if ident == nil {
		tx.Rollback(ctx)
		dummy()
		s.noteFailure(c, clientID)
		if byBadge {
			refuse("UNKNOWN_BADGE")
		} else {
			refuse("UNKNOWN_WORKER")
		}
		invalidCredentials(c)
		return
	}
	attempt.IdentityID, attempt.WorkerCode = ident.ID, ident.WorkerCode
	pinProvided := req.PIN != ""
	if ident.Status != fieldidentity.StatusActive || (pinProvided && !ident.HasPIN()) {
		tx.Rollback(ctx)
		dummy()
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

	var rehash bool
	if pinProvided {
		var ok bool
		var verr error
		ok, rehash, verr = s.Ops.Hasher.Verify(ident.ClientID, ident.ID, req.PIN, ident.StoredPIN())
		if verr != nil {
			tx.Rollback(ctx)
			if errors.Is(verr, pin.ErrUnconfigured) {
				fail(c.w, http.StatusServiceUnavailable, fieldidentity.CodeUnconfigured, "field identity is unavailable")
				return
			}
			if errors.Is(verr, pin.ErrPepperRetired) {
				refuse("PIN_RESET_REQUIRED")
				pinResetRequired(c, ident.ID)
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
			s.noteFailure(c, clientID)
			refuse("PIN_INVALID")
			invalidCredentials(c)
			return
		}
	} else if !pol.BadgeOnlyLogin || dev == nil {
		// A badge alone signs in only on an enrolled device whose client allows
		// it: a photocopied badge must not work from any phone.
		tx.Rollback(ctx)
		refuse("PIN_REQUIRED")
		fail(c.w, http.StatusBadRequest, "PIN_REQUIRED", "a PIN is required with this badge")
		return
	}

	// The PIN (or the badge) is right. Only now do access checks run, and every
	// refusal answers the same 403 (the reason is recorded, not returned).
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
	case dev != nil && !dev.HasApp(string(app.ID)):
		reason = "DEVICE_APP_NOT_ENROLLED"
	case dev != nil && dev.Mode == fieldidentity.ModePersonal && dev.AssignedIdentityID != nil && *dev.AssignedIdentityID != ident.ID:
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
		if app != nil {
			attempt.ApplicationID = string(app.ID)
		}
		refuse(reason)
		noAppAccess(c)
		return
	}
	attempt.ApplicationID = string(app.ID)

	// A LEGACY hash, or one made under a previous app key's pepper, is
	// re-hashed under the current pepper in this transaction (lazy migration
	// after a key rotation).
	var newHash *pin.Digest
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
	sess := fieldidentity.NewSession(now.Truncate(time.Microsecond), ident, string(app.ID), dev, c.proof.JKT, hash, pol)
	sess.AMR = []string{"pin"}
	switch method {
	case fieldidentity.MethodBadgePIN:
		sess.AMR = []string{"badge", "pin"}
	case fieldidentity.MethodBadge:
		sess.AMR = []string{"badge"}
	}
	sess.PINChangeRequired = ident.PINChangeRequired
	if err := tx.InsertSession(ctx, sess); err != nil {
		s.internal(c, "insert session", err)
		return
	}
	if badge != nil {
		if err := tx.TouchBadge(ctx, badge.ID, now); err != nil {
			s.internal(c, "touch badge", err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(c, "commit login", err)
		return
	}
	attempt.Outcome = fieldidentity.OutcomeSuccess
	s.attempt(c, attempt)
	s.issue(c, ident, cl, app, sess, token, nil)
}

// ── refresh ──────────────────────────────────────────────────────────────

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func invalidGrant(c *call) {
	fail(c.w, http.StatusUnauthorized, "INVALID_GRANT", "the refresh token is invalid, used or revoked")
}

// refresh is POST /field/auth/refresh: rotate the refresh token. The proof key
// must be the key the session was signed in with (and, for a session on an
// enrolled device, that device); anything else is theft and revokes the
// family. A rotated token presented again within RetryGrace by the session's
// own key is a lost-response retry (the live child is rotated and a fresh one
// issued); otherwise it is reuse and the whole family is revoked.
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
			ClientID: ident.ClientID, WorkerID: sess.IdentityID, DeviceID: sess.DeviceID, FamilyID: sess.FamilyID,
		}, usecase.NewExecutionContext(sess.IdentityID)); err != nil {
			s.internal(c, "revoke family", err)
			return
		}
		invalidGrant(c)
	}

	// A token presented with another key, live or rotated, is theft: kill the
	// family. A session on an enrolled device also needs that device.
	if sess.JKT != c.proof.JKT || (sess.DeviceID != "" && (dev == nil || dev.ID != sess.DeviceID)) {
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
	pol, err := s.Ops.R.Policies.Effective(ctx, ident.ClientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	if cur.DeviceID == "" && pol.RequireEnrolledDevice {
		// The client tightened its policy after this unenrolled session began.
		unknownDevice(c)
		return
	}
	var act *actor
	if cur.Impersonated() {
		name, ok, err := s.Ops.ActorOf(ctx, cur)
		if err != nil {
			s.internal(c, "actor", err)
			return
		}
		if !ok {
			invalidGrant(c) // the actor was suspended, deactivated or lost the power
			return
		}
		act = &actor{ID: cur.ActorID, Type: actorClaimType(cur.ActorType), Name: name}
	}
	app, err := s.Ops.Apps.FindByID(ctx, cur.ApplicationID)
	if err != nil {
		s.internal(c, "application", err)
		return
	}
	cl, err := s.activeClient(c, ident.ClientID)
	if err != nil {
		s.internal(c, "client", err)
		return
	}
	if app == nil || !app.Active || cl == nil || !ident.HasApp(string(app.ID)) ||
		(cur.DeviceID != "" && !dev.HasApp(string(app.ID))) {
		noAppAccess(c)
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
	// The child keeps the session's enrolment: a key enrolled after the session
	// began does not upgrade it until the next sign-in.
	mode := fieldidentity.ModePersonal
	if cur.DeviceID != "" {
		mode = dev.Mode
	}
	idle, _ := pol.SessionLimits(mode, cur.DeviceID != "")
	child := &fieldidentity.Session{
		ID: fieldidentity.NewSessionID(), FamilyID: cur.FamilyID, IdentityID: cur.IdentityID, DeviceID: cur.DeviceID,
		ApplicationID: cur.ApplicationID, RefreshHash: newHash, AMR: cur.AMR, AuthTime: cur.AuthTime,
		IssuedAt: now, IdleExpiresAt: now.Add(idle),
		AbsoluteExpiresAt: cur.AbsoluteExpiresAt,
		// The worker's current flag is the truth: a PIN change clears it. An
		// impersonated session never carries the worker's pending change.
		PINChangeRequired: ident.PINChangeRequired && !cur.Impersonated(),
		JKT:               cur.JKT, ActorType: cur.ActorType, ActorID: cur.ActorID,
	}
	if err := tx.InsertSession(ctx, child); err != nil {
		s.internal(c, "insert session", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(c, "commit refresh", err)
		return
	}
	s.issue(c, ident, cl, app, child, token, act)
}

// actorClaimType is the act claim's typ for a session's actor_type.
func actorClaimType(actorType string) string {
	if actorType == fieldidentity.ActorPrincipal {
		return "principal"
	}
	return "field_worker"
}

// ── PIN change ───────────────────────────────────────────────────────────

type pinRequest struct {
	CurrentPIN string `json:"currentPin"`
	NewPIN     string `json:"newPin"`
}

// changePin is POST /field/auth/pin (bearer). The current PIN goes through the
// same lockout accounting as a sign-in; the new PIN is policy-checked and set
// by the ChangeOwnPin use case, which revokes the worker's other sessions and
// keeps this one. It works on an unenrolled key; an impersonated session may
// not change the worker's PIN.
func (s *State) changePin(c *call) {
	var req pinRequest
	if !c.decode(&req) {
		return
	}
	if c.claims.Act != nil {
		fail(c.w, http.StatusForbidden, "IMPERSONATING", "the PIN cannot be changed while signed in as another worker")
		return
	}
	if req.CurrentPIN == "" || req.NewPIN == "" || len(req.CurrentPIN) > maxPINLen || len(req.NewPIN) > maxPINLen {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "currentPin and newPin are required")
		return
	}
	ctx := c.r.Context()
	clientID := c.claims.ClientID
	attempt := fieldidentity.LoginAttempt{
		WorkerCode: c.claims.WorkerCode, IdentityID: c.claims.Subject, AuthMethod: "PIN_CHANGE", ClientID: clientID,
	}
	refuse := func(reason string) {
		attempt.Outcome, attempt.FailureReason = fieldidentity.OutcomeFailure, reason
		s.attempt(c, attempt)
	}
	pol, err := s.Ops.R.Policies.Effective(ctx, clientID)
	if err != nil {
		s.internal(c, "policy", err)
		return
	}
	if c.device == nil && pol.RequireEnrolledDevice {
		refuse("UNKNOWN_DEVICE")
		unknownDevice(c)
		return
	}
	if s.refuseThrottled(c, clientID, "too many failed PIN attempts; try again later", refuse) {
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
	ok, _, verr := s.Ops.Hasher.Verify(ident.ClientID, ident.ID, req.CurrentPIN, ident.StoredPIN())
	if verr != nil {
		tx.Rollback(ctx)
		if errors.Is(verr, pin.ErrPepperRetired) {
			refuse("PIN_RESET_REQUIRED")
			pinResetRequired(c, ident.ID)
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
		s.noteFailure(c, clientID)
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
// or foreign token (one not signed in with the proof key) is a 204 that changes
// nothing. Scope "session" ends the token's login family; "device-all" ends
// every session on this device (a SHARED device hand-over) and needs an
// enrolled device.
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
	if scope == "device-all" && c.device == nil {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "device-all needs an enrolled device")
		return
	}
	ctx := c.r.Context()
	sess, err := s.Ops.R.Sessions.FindByRefreshHash(ctx, fieldidentity.HashToken(req.RefreshToken))
	if err != nil {
		s.internal(c, "find session", err)
		return
	}
	if sess == nil || sess.JKT != c.proof.JKT {
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
