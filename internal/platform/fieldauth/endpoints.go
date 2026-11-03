// Package fieldauth is the field-worker device protocol: the public /field/*
// routes a mobile app speaks to enrol a device, sign a worker in with a PIN,
// refresh, change the PIN and sign out. Spec:
// docs/field-worker-identity-p1-spec.md §4; wire contract:
// docs/wire-contract.md "Field identity".
//
// Every request carries a DPoP-style proof (a JWS signed by the device key,
// RFC 9449 shape, ES256 only) with a single-use server nonce. After
// enrolment the proof key must be an ACTIVE device's key. The routes are
// mounted OUTSIDE the platform API middleware (they authenticate the device
// and worker themselves), behind the per-IP governor.
//
// Writes that change domain state (enrolment, PIN change, family revocation on
// reuse, throttle notices) go through the usecase envelope
// (fieldidentity.Ops). The login and refresh hot paths use fieldidentity's
// AuthTx directly: a sign-in must serialise on the worker row and commit its
// failure counters even when it is refused, which the envelope's
// commit-on-success shape does not express.
package fieldauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Timing constants of the protocol.
const (
	// NonceTTL is how long an issued nonce can be used (once).
	NonceTTL = 5 * time.Minute
	// RetryGrace is the window after a refresh rotation in which the same
	// (rotated) token presented with a valid device proof is treated as a
	// lost-response retry rather than theft.
	RetryGrace = 60 * time.Second
	// maxBody caps a request body.
	maxBody = 64 << 10
	// maxPINLen bounds the PIN accepted on the wire (policy allows 12).
	maxPINLen = 64
)

// State bundles the device protocol's dependencies.
type State struct {
	Ops  *fieldidentity.Ops
	UoW  *usecasepgx.UnitOfWork
	Auth *authservice.AuthService
	// BaseURL is the platform's public base URL; proof htu is compared
	// against BaseURL + request path, never against the Host header.
	BaseURL string
	// RateLimit backs the device and client PIN-failure buckets (nil
	// disables them).
	RateLimit ratelimit.Store
	Limits    ratelimit.Policies
}

// RegisterRoutes mounts the device protocol.
func (s *State) RegisterRoutes(r chi.Router) {
	r.Post("/field/auth/nonce", s.handle(routeNonce, s.nonce))
	r.Post("/field/devices/enrol", s.handle(routeEnrol, s.enrol))
	r.Post("/field/auth/login", s.handle(routeDevice, s.login))
	r.Post("/field/auth/refresh", s.handle(routeDevice, s.refresh))
	r.Post("/field/auth/pin", s.handle(routeBearer, s.changePin))
	r.Post("/field/auth/logout", s.handle(routeDevice, s.logout))
}

// route says which prelude checks a handler needs.
type route struct {
	nonce  bool // a fresh single-use nonce must be in the proof
	device bool // the proof key must be an ACTIVE enrolled device
	bearer bool // an access token (Authorization: DPoP) must accompany the proof
}

var (
	routeNonce  = route{}
	routeEnrol  = route{nonce: true}
	routeDevice = route{nonce: true, device: true}
	routeBearer = route{nonce: true, device: true, bearer: true}
)

// call is one request after the prelude.
type call struct {
	w      http.ResponseWriter
	r      *http.Request
	proof  *Proof
	device *fieldidentity.Device
	claims *authservice.FieldAccessTokenClaims
	// nonce is the fresh nonce issued on this response.
	nonce    string
	nonceExp time.Time
	ip       string
	now      time.Time
}

func (s *State) handle(rt route, h func(*call)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.Ops.Hasher.Configured() {
			fail(w, http.StatusServiceUnavailable, fieldidentity.CodeUnconfigured,
				"field identity is unavailable: FLOWCATALYST_APP_KEY is not set")
			return
		}
		c := &call{w: w, r: r, ip: ratelimit.ClientIP(r), now: time.Now().UTC()}
		nonce, exp, err := newNonce()
		if err != nil {
			fail(w, http.StatusInternalServerError, "INTERNAL", "could not issue a nonce")
			return
		}
		if err := s.Ops.R.Sessions.StoreNonce(r.Context(), nonce, exp); err != nil {
			slog.Error("field nonce store failed", "err", err)
			fail(w, http.StatusInternalServerError, "INTERNAL", "could not issue a nonce")
			return
		}
		c.nonce, c.nonceExp = nonce, exp
		// A fresh nonce rides on every response, success or error.
		w.Header().Set("DPoP-Nonce", nonce)
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)

		if !s.prelude(c, rt) {
			return
		}
		h(c)
	}
}

// prelude verifies the proof, nonce, device and (for bearer routes) the access
// token. It writes the failure response itself and returns false.
func (s *State) prelude(c *call, rt route) bool {
	ctx := c.r.Context()
	header := c.r.Header.Get("DPoP")
	if header == "" {
		c.invalidProof("the DPoP header is missing")
		return false
	}
	proof, err := ParseProof(header, c.now)
	if err != nil {
		c.invalidProof(strings.TrimPrefix(err.Error(), ErrProof.Error()+": "))
		return false
	}
	c.proof = proof
	if proof.HTM != c.r.Method {
		c.invalidProof("htm does not match the request method")
		return false
	}
	if proof.HTU != strings.TrimRight(s.BaseURL, "/")+c.r.URL.Path {
		c.invalidProof("htu does not match the request URL")
		return false
	}
	if rt.nonce {
		if proof.Nonce == "" {
			c.useNonce()
			return false
		}
		ok, err := s.Ops.R.Sessions.ConsumeNonce(ctx, proof.Nonce)
		if err != nil {
			slog.Error("field nonce consume failed", "err", err)
			fail(c.w, http.StatusInternalServerError, "INTERNAL", "could not check the nonce")
			return false
		}
		if !ok {
			c.useNonce()
			return false
		}
	}
	if !rt.device {
		return true
	}
	dev, err := s.Ops.R.Devices.FindByJKT(ctx, proof.JKT)
	if err != nil {
		slog.Error("field device lookup failed", "err", err)
		fail(c.w, http.StatusInternalServerError, "INTERNAL", "device lookup failed")
		return false
	}
	switch {
	case dev == nil:
		fail(c.w, http.StatusUnauthorized, "UNKNOWN_DEVICE", "this device is not enrolled")
		return false
	case dev.Status == fieldidentity.DeviceRevoked:
		fail(c.w, http.StatusForbidden, "DEVICE_REVOKED", "this device has been revoked")
		return false
	case dev.Status == fieldidentity.DeviceSuspended:
		fail(c.w, http.StatusForbidden, "DEVICE_SUSPENDED", "this device is suspended")
		return false
	}
	c.device = dev
	if err := s.Ops.R.Devices.TouchLastSeen(ctx, dev.ID); err != nil {
		slog.Warn("field device last_seen update failed", "err", err)
	}
	if !rt.bearer {
		return true
	}
	return s.bearer(c)
}

// bearer verifies the Authorization: DPoP access token against the proof and
// the live session/worker/device state. A pcr token is accepted: the only
// bearer route is the PIN change.
func (s *State) bearer(c *call) bool {
	scheme, token, _ := strings.Cut(strings.TrimSpace(c.r.Header.Get("Authorization")), " ")
	if !strings.EqualFold(scheme, "DPoP") || strings.TrimSpace(token) == "" {
		c.w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		fail(c.w, http.StatusUnauthorized, "invalid_token", "an Authorization: DPoP access token is required")
		return false
	}
	token = strings.TrimSpace(token)
	claims, err := s.Auth.ValidateFieldToken(token)
	if err != nil || claims.Cnf.JKT != c.proof.JKT || claims.DeviceID != c.device.ID {
		c.w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		fail(c.w, http.StatusUnauthorized, "invalid_token", "the access token is invalid, expired or not bound to this device")
		return false
	}
	if c.proof.ATH != AccessTokenHash(token) {
		c.invalidProof("ath does not match the access token")
		return false
	}
	live, err := s.Ops.TokenUsable(c.r.Context(), tokenClaims(claims), c.now)
	if err != nil {
		slog.Error("field token liveness check failed", "err", err)
		fail(c.w, http.StatusInternalServerError, "INTERNAL", "could not check the session")
		return false
	}
	if !live {
		c.w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		fail(c.w, http.StatusUnauthorized, "invalid_token", "the session has ended")
		return false
	}
	c.claims = claims
	return true
}

func tokenClaims(c *authservice.FieldAccessTokenClaims) *fieldidentity.TokenClaims {
	out := &fieldidentity.TokenClaims{
		Subject: c.Subject, ClientID: c.ClientID, WorkerCode: c.WorkerCode, DeviceID: c.DeviceID,
		AppCode: c.Aud, SessionID: c.SessionID, AMR: c.AMR, AuthTime: time.Unix(c.AuthTime, 0).UTC(),
		PINChangeRequired: c.PCR, JKT: c.Cnf.JKT,
	}
	if c.ExpiresAt != nil {
		out.Expires = c.ExpiresAt.Time
	}
	return out
}

// ── responses ────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// fail writes the {error, message} envelope.
func fail(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": code, "message": message})
}

func failWith(w http.ResponseWriter, status int, code, message string, extra map[string]any) {
	body := map[string]any{"error": code, "message": message}
	maps.Copy(body, extra)
	writeJSON(w, status, body)
}

func (c *call) invalidProof(msg string) {
	c.w.Header().Set("WWW-Authenticate", `DPoP error="invalid_dpop_proof", algs="ES256"`)
	fail(c.w, http.StatusUnauthorized, "invalid_dpop_proof", msg)
}

// useNonce answers the missing/used/expired nonce case. The fresh nonce is
// already on the response, so the client retries once.
func (c *call) useNonce() {
	c.w.Header().Set("WWW-Authenticate", `DPoP error="use_dpop_nonce"`)
	fail(c.w, http.StatusUnauthorized, "use_dpop_nonce", "the DPoP nonce is missing, used or expired; retry with the DPoP-Nonce header")
}

// decode reads a JSON body into v, writing 400 on failure.
func (c *call) decode(v any) bool {
	raw, err := io.ReadAll(c.r.Body)
	if err != nil {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "the request body could not be read")
		return false
	}
	if err := json.Unmarshal(raw, v); err != nil {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "the request body is not valid JSON")
		return false
	}
	return true
}

// useCaseError maps an envelope error onto the wire.
func useCaseError(w http.ResponseWriter, err error) {
	ue := usecase.AsError(err)
	if ue == nil {
		slog.Error("field use case failed", "err", err)
		fail(w, http.StatusInternalServerError, "INTERNAL", "internal error")
		return
	}
	switch ue.Code {
	case fieldidentity.CodeUnconfigured:
		fail(w, http.StatusServiceUnavailable, ue.Code, ue.Message)
		return
	case fieldidentity.CodeInvalidEnrolmentCode:
		fail(w, http.StatusUnauthorized, ue.Code, ue.Message)
		return
	case fieldidentity.CodeDeviceKeyExists:
		fail(w, http.StatusConflict, ue.Code, ue.Message)
		return
	case fieldidentity.CodeAttestationRequired:
		fail(w, http.StatusUnprocessableEntity, ue.Code, ue.Message)
		return
	case fieldidentity.CodeClientInactive, fieldidentity.CodeWorkerUnavailable:
		fail(w, http.StatusForbidden, ue.Code, ue.Message)
		return
	case "PIN_POLICY":
		failWith(w, http.StatusUnprocessableEntity, ue.Code, ue.Message, map[string]any{"violations": ue.Details["violations"]})
		return
	}
	switch ue.Kind {
	case usecase.KindValidation:
		fail(w, http.StatusBadRequest, ue.Code, ue.Message)
	case usecase.KindNotFound:
		fail(w, http.StatusNotFound, ue.Code, ue.Message)
	case usecase.KindConflict, usecase.KindBusinessRule:
		fail(w, http.StatusConflict, ue.Code, ue.Message)
	case usecase.KindAuthorization:
		fail(w, http.StatusForbidden, ue.Code, ue.Message)
	case usecase.KindInternal:
		slog.Error("field use case failed", "code", ue.Code, "err", ue.Cause)
		fail(w, http.StatusInternalServerError, "INTERNAL", "internal error")
	default:
		fail(w, http.StatusInternalServerError, "INTERNAL", "internal error")
	}
}

// ── nonce ────────────────────────────────────────────────────────────────

func newNonce() (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	return base64.RawURLEncoding.EncodeToString(b), time.Now().UTC().Add(NonceTTL), nil
}

func (s *State) nonce(c *call) {
	writeJSON(c.w, http.StatusOK, map[string]any{"nonce": c.nonce, "expiresAt": jsontime.New(c.nonceExp)})
}

// ── enrol ────────────────────────────────────────────────────────────────

type enrolRequest struct {
	Code             string          `json:"code"`
	PublicKeyJWK     json.RawMessage `json:"publicKeyJwk"`
	AttestationChain json.RawMessage `json:"attestationChain"`
	Platform         string          `json:"platform"`
	Model            string          `json:"model"`
	Label            string          `json:"label"`
}

func (s *State) enrol(c *call) {
	var req enrolRequest
	if !c.decode(&req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" || len(req.PublicKeyJWK) == 0 ||
		strings.TrimSpace(req.Platform) == "" || strings.TrimSpace(req.Model) == "" ||
		len(req.Platform) > 40 || len(req.Model) > 120 || len(req.Label) > 255 {
		fail(c.w, http.StatusBadRequest, "INVALID_REQUEST", "code, publicKeyJwk, platform and model are required")
		return
	}
	_, canonical, jkt, err := ParseECJWK(req.PublicKeyJWK)
	if err != nil {
		fail(c.w, http.StatusBadRequest, "INVALID_PUBLIC_KEY", strings.TrimPrefix(err.Error(), ErrProof.Error()+": "))
		return
	}
	if jkt != c.proof.JKT {
		c.invalidProof("the proof key does not match publicKeyJwk")
		return
	}
	chain := ""
	if len(req.AttestationChain) > 0 && string(req.AttestationChain) != "null" {
		chain = string(req.AttestationChain)
	}
	res, err := usecaseop.RunTx(c.r.Context(), s.UoW, s.Ops.EnrolDevice(), fieldidentity.EnrolDeviceCommand{
		Code: req.Code, JKT: jkt, PublicKeyJWK: canonical, AttestationChain: chain,
		Platform: strings.TrimSpace(req.Platform), Model: strings.TrimSpace(req.Model), Label: strings.TrimSpace(req.Label),
	}, usecase.NewExecutionContext(fieldidentity.PrincipalEnrolment))
	if err != nil {
		useCaseError(c.w, err)
		return
	}
	writeJSON(c.w, http.StatusCreated, map[string]any{
		"deviceId": res.DeviceID, "mode": res.Mode, "clientId": res.ClientID, "appCodes": res.AppCodes,
	})
}

// ── sessions and tokens ──────────────────────────────────────────────────

// tokenResponse is the FieldTokenResponse wire shape.
type tokenResponse struct {
	AccessToken       string        `json:"accessToken"`
	TokenType         string        `json:"tokenType"`
	ExpiresIn         int64         `json:"expiresIn"`
	RefreshToken      string        `json:"refreshToken"`
	RefreshExpiresAt  jsontime.Time `json:"refreshExpiresAt"`
	IdleExpiresAt     jsontime.Time `json:"idleExpiresAt"`
	Worker            workerRef     `json:"worker"`
	PINChangeRequired bool          `json:"pinChangeRequired"`
}

type workerRef struct {
	ID         string `json:"id"`
	WorkerCode string `json:"workerCode"`
	Name       string `json:"name"`
}

// newRefreshToken returns an opaque refresh token and its stored hash.
func newRefreshToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = "fwr_" + base64.RawURLEncoding.EncodeToString(b)
	return token, fieldidentity.HashToken(token), nil
}

func idleSecs(pol fieldidentity.Policy, mode fieldidentity.Mode) int {
	if mode == fieldidentity.ModeShared {
		return pol.SharedIdleSecs
	}
	return pol.PersonalIdleSecs
}

func absoluteSecs(pol fieldidentity.Policy, mode fieldidentity.Mode) int {
	if mode == fieldidentity.ModeShared {
		return fieldidentity.SharedShiftMaxSecs
	}
	return pol.ShiftMaxSecs
}

// issue mints the access token for a stored session and writes the response.
func (s *State) issue(c *call, ident *fieldidentity.Identity, cl *client.Client, app *application.Application,
	sess *fieldidentity.Session, refreshToken string,
) {
	access, err := s.Auth.GenerateFieldAccessToken(authservice.FieldSubject{
		WorkerID: ident.ID, ClientID: ident.ClientID, ClientIdentifier: cl.Identifier,
		WorkerCode: ident.WorkerCode, Name: ident.Name, AppCode: app.Code,
		DeviceID: c.device.ID, DeviceMode: string(c.device.Mode), SessionID: sess.FamilyID,
		AMR: sess.AMR, AuthTime: sess.AuthTime, JKT: c.device.JKT, PINChangeRequired: sess.PINChangeRequired,
	})
	if err != nil {
		slog.Error("field access token mint failed", "err", err)
		fail(c.w, http.StatusInternalServerError, "INTERNAL", "could not mint the access token")
		return
	}
	writeJSON(c.w, http.StatusOK, tokenResponse{
		AccessToken: access, TokenType: "DPoP", ExpiresIn: authservice.FieldAccessTokenTTLSecs(),
		RefreshToken: refreshToken, RefreshExpiresAt: jsontime.New(sess.AbsoluteExpiresAt),
		IdleExpiresAt:     jsontime.New(sess.IdleExpiresAt),
		Worker:            workerRef{ID: ident.ID, WorkerCode: ident.WorkerCode, Name: ident.Name},
		PINChangeRequired: sess.PINChangeRequired,
	})
}

func (s *State) findApp(c *call, code string) (*application.Application, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil
	}
	a, err := s.Ops.Apps.FindByCode(c.r.Context(), code)
	if err == nil && a == nil && strings.ToLower(code) != code {
		a, err = s.Ops.Apps.FindByCode(c.r.Context(), strings.ToLower(code))
	}
	return a, err
}

func (s *State) activeClient(c *call, clientID string) (*client.Client, error) {
	cl, err := s.Ops.Clients.FindByID(c.r.Context(), clientID)
	if err != nil || cl == nil || cl.Status != client.StatusActive {
		return nil, err
	}
	return cl, nil
}

func (s *State) internal(c *call, what string, err error) {
	slog.Error("field protocol failure", "step", what, "err", err)
	fail(c.w, http.StatusInternalServerError, "INTERNAL", "internal error")
}

// attempt records a sign-in attempt, best effort (the audit trail must never
// turn an auth decision into an error).
func (s *State) attempt(c *call, a fieldidentity.LoginAttempt) {
	a.IPAddress = c.ip
	if c.device != nil {
		a.DeviceID, a.ClientID = c.device.ID, c.device.ClientID
	}
	if err := s.Ops.R.Sessions.RecordLoginAttempt(c.r.Context(), a); err != nil {
		slog.Warn("field login attempt record failed", "err", err)
	}
}

// ── PIN-failure buckets ──────────────────────────────────────────────────

// throttled reports whether the device or client PIN-failure bucket is full,
// with the Retry-After to send.
func (s *State) throttled(c *call, clientID, deviceID string) (scope string, retry uint32, hit bool) {
	cnt, ok := s.RateLimit.(ratelimit.Counter)
	if !ok || s.RateLimit == nil {
		return "", 0, false
	}
	check := func(b ratelimit.Bucket, key string, p ratelimit.Policy) (uint32, bool) {
		n, retry, err := cnt.Count(c.r.Context(), b, key, p.Window)
		if err != nil {
			slog.Warn("field throttle check failed; failing open", "err", err)
			return 0, false
		}
		return retry, n >= p.Limit
	}
	if r, hit := check(ratelimit.BucketFieldPinDevice, deviceID, s.Limits.FieldPinDevice); hit {
		return "DEVICE", r, true
	}
	if r, hit := check(ratelimit.BucketFieldPinClient, clientID, s.Limits.FieldPinClient); hit {
		return "CLIENT", r, true
	}
	return "", 0, false
}

// noteFailure counts a PIN failure against the device and client buckets and,
// the first time a bucket is full inside its window, emits
// field-device.throttled.
func (s *State) noteFailure(c *call, clientID, deviceID string) {
	if s.RateLimit == nil {
		return
	}
	ctx := c.r.Context()
	cnt, isCounter := s.RateLimit.(ratelimit.Counter)
	for _, b := range []struct {
		bucket, notice ratelimit.Bucket
		key            string
		pol            ratelimit.Policy
		scope          string
		device         string
	}{
		{ratelimit.BucketFieldPinDevice, ratelimit.BucketFieldThrottleNoticeDev, deviceID, s.Limits.FieldPinDevice, "DEVICE", deviceID},
		{ratelimit.BucketFieldPinClient, ratelimit.BucketFieldThrottleNoticeCli, clientID, s.Limits.FieldPinClient, "CLIENT", ""},
	} {
		rej := ratelimit.Enforce(ctx, s.RateLimit, b.bucket, b.key, b.pol)
		full := rej != nil
		if isCounter {
			if n, _, err := cnt.Count(ctx, b.bucket, b.key, b.pol.Window); err == nil && n >= b.pol.Limit {
				full = true
			}
		}
		if !full {
			continue
		}
		// One notice per bucket per window: the marker bucket admits once.
		if ratelimit.Enforce(ctx, s.RateLimit, b.notice, b.key, ratelimit.Policy{Window: b.pol.Window, Limit: 1}) != nil {
			continue
		}
		if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.NoteThrottled(),
			fieldidentity.ThrottleCommand{Scope: b.scope, ClientID: clientID, DeviceID: b.device},
			usecase.NewExecutionContext("field:throttle")); err != nil {
			slog.Warn("field throttle event failed", "err", err)
		}
	}
}

func retryAfter(w http.ResponseWriter, secs uint32) {
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.FormatUint(uint64(secs), 10))
}
