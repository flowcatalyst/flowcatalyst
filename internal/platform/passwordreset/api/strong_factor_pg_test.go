//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval"
	resetapprovalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/email"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	wops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/webauthntest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

const (
	sfRPID   = "localhost"
	sfOrigin = "http://localhost:8080"
	// sfPassword satisfies the password policy and contains no account name.
	sfPassword = "Tangerine-Lighthouse-42"
)

// mailbox is an email.Service that keeps what it was asked to send.
type mailbox struct {
	mu   sync.Mutex
	sent []email.Message
}

func (m *mailbox) Send(_ context.Context, msg email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

var tokenInLink = regexp.MustCompile(`token=([A-Za-z0-9_-]{43})`)

// lastToken is the raw reset token in the most recent email to addr.
func (m *mailbox) lastToken(t *testing.T, addr string) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range slices.Backward(m.sent) {
		if strings.EqualFold(msg.To, addr) {
			if mm := tokenInLink.FindStringSubmatch(msg.HTMLBody); mm != nil {
				return mm[1]
			}
		}
	}
	t.Fatalf("no reset link emailed to %s", addr)
	return ""
}

func (m *mailbox) count(addr string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, msg := range m.sent {
		if strings.EqualFold(msg.To, addr) && tokenInLink.MatchString(msg.HTMLBody) {
			n++
		}
	}
	return n
}

type sfRig struct {
	t         *testing.T
	ctx       context.Context
	s         *State
	mail      *mailbox
	mfa       *mfa.Service
	mfaRepo   *mfa.Repository
	wa        *webauthn.Service
	approvals *resetapproval.Repository
	approver  *resetapprovalapi.State
	clientID  string
	tag       string
}

func newSFRig(t *testing.T, tag string) *sfRig {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	principals := principal.NewRepository(pool)
	tokens := passwordreset.NewRepository(pool)

	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	mfaRepo := mfa.NewRepository(pool)
	mfaSvc := mfa.NewService(mfaRepo, enc, nil, mfa.DefaultConfig())

	wa, err := webauthn.NewService(webauthn.Config{RPDisplayName: "FC", RPID: sfRPID, RPOrigins: []string{sfOrigin}},
		webauthn.NewRepository(pool), webauthn.NewCeremonyRepository(pool))
	require.NoError(t, err)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(client.NewRepository(pool)),
		clientops.CreateCommand{Name: "Strong Factor " + tag, Identifier: "sf-" + tag}, testpg.TestEC())
	require.NoError(t, err)

	mail := &mailbox{}
	approvals := resetapproval.NewRepository(pool)
	s := &State{
		Principals: principals, Tokens: tokens, UoW: uow,
		ExternalBaseURL: "https://fc.test",
		Emailer:         NewEmailer(mail, nil),
		MFA:             mfaSvc,
		Passkeys:        WebAuthnFactor{Service: wa, UoW: uow},
		Approvals:       approvals, ClientAdmins: principals,
		RequireStrongFactorForReset: true,
		Go:                          func(fn func()) { fn() }, // the send is detached in production; run it inline here
	}
	return &sfRig{
		t: t, ctx: ctx, s: s, mail: mail, mfa: mfaSvc, mfaRepo: mfaRepo, wa: wa, approvals: approvals,
		approver: &resetapprovalapi.State{
			Approvals: approvals, Principals: principals,
			Sender: NewPrincipalEmailer(tokens, "https://fc.test", mail, nil),
		},
		clientID: clientEv.ClientID, tag: tag,
	}
}

// user creates an internal CLIENT user of the rig's client (or, with
// withClient false, an ANCHOR user with no home client) holding a password.
func (r *sfRig) user(local string, withClient bool) (string, string) {
	r.t.Helper()
	addr := local + "-" + r.tag + "@strongfactor.test"
	cmd := principalops.CreateCommand{Email: addr, Scope: "ANCHOR", Password: new("Initial-Password-77")}
	if withClient {
		cmd.Scope = "CLIENT"
		cmd.ClientID = &r.clientID
	}
	ev, err := usecaseop.Run(testpg.AnchorCtx(), r.s.UoW, principalops.CreateUser(r.s.Principals), cmd, testpg.TestEC())
	require.NoError(r.t, err)
	return string(ev.UserID), addr
}

// enrollTOTP confirms an authenticator for pid and returns its secret.
func (r *sfRig) enrollTOTP(pid, addr string) string {
	r.t.Helper()
	enr, err := r.mfa.BeginTOTPEnrollment(r.ctx, pid, addr)
	require.NoError(r.t, err)
	// Enrol with the previous step so the current step's code stays unused.
	code, err := totp.GenerateCode(enr.Secret, time.Now().Add(-30*time.Second))
	require.NoError(r.t, err)
	ok, err := r.mfa.ConfirmTOTPEnrollment(r.ctx, pid, code)
	require.NoError(r.t, err)
	require.True(r.t, ok)
	return enr.Secret
}

// enrollEmailPin confirms an email-PIN factor for pid directly in the store.
func (r *sfRig) enrollEmailPin(pid string) {
	r.t.Helper()
	m := mfa.NewMethod(pid, mfa.MethodEmailPin)
	require.NoError(r.t, r.mfaRepo.InsertMethod(r.ctx, m))
	require.NoError(r.t, r.mfaRepo.ConfirmMethod(r.ctx, m.ID, time.Now().UTC()))
}

// registerPasskey registers a software passkey for pid.
func (r *sfRig) registerPasskey(pid string) *webauthntest.Authenticator {
	r.t.Helper()
	a := webauthntest.New(r.t)
	user := &webauthn.PrincipalUser{PrincipalID: pid, DisplayName: pid}
	opts, sess, err := r.wa.WebAuthn().BeginRegistration(user)
	require.NoError(r.t, err)
	parsed, err := protocol.ParseCredentialCreationResponseBody(io.NopCloser(bytes.NewReader(
		a.Register(opts.Response.Challenge.String(), sfOrigin, sfRPID))))
	require.NoError(r.t, err)
	cred, err := r.wa.WebAuthn().CreateCredential(user, *sess, parsed)
	require.NoError(r.t, err)
	name := "key"
	_, err = usecaseop.Run(auth.WithContext(r.ctx, &auth.AuthContext{PrincipalID: pid}), r.s.UoW, wops.Register(r.wa.Credentials()),
		wops.RegisterCommand{StateID: "s", Response: *cred, Name: &name}, usecase.NewExecutionContext(pid))
	require.NoError(r.t, err)
	return a
}

func (r *sfRig) post(h http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	r.t.Helper()
	b, err := json.Marshal(body)
	require.NoError(r.t, err)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b)))
	return rec
}

func (r *sfRig) request(addr string) {
	r.t.Helper()
	rec := r.post(r.s.requestReset, "/auth/password-reset/request", map[string]any{"email": addr})
	require.Equal(r.t, http.StatusOK, rec.Code, rec.Body.String())
}

func (r *sfRig) validate(raw string) validateTokenResponse {
	r.t.Helper()
	rec := httptest.NewRecorder()
	r.s.validateToken(rec, httptest.NewRequest(http.MethodGet, "/auth/password-reset/validate?token="+url.QueryEscape(raw), nil))
	require.Equal(r.t, http.StatusOK, rec.Code)
	var out validateTokenResponse
	require.NoError(r.t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

func (r *sfRig) confirm(body map[string]any) *httptest.ResponseRecorder {
	r.t.Helper()
	return r.post(r.s.confirmReset, "/auth/password-reset/confirm", body)
}

func (r *sfRig) pending(pid string) bool {
	r.t.Helper()
	has, err := r.approvals.HasPending(r.ctx, pid)
	require.NoError(r.t, err)
	return has
}

func (r *sfRig) passwordHash(pid string) string {
	r.t.Helper()
	p, err := r.s.Principals.FindByID(r.ctx, ids.PrincipalID(pid))
	require.NoError(r.t, err)
	require.NotNil(r.t, p)
	require.NotNil(r.t, p.UserIdentity.PasswordHash)
	return *p.UserIdentity.PasswordHash
}

// passkeyBegin asks for the reset's passkey challenge.
func (r *sfRig) passkeyBegin(raw string) (string, string) {
	r.t.Helper()
	rec := r.post(r.s.passkeyBegin, "/auth/password-reset/passkey/begin", map[string]any{"token": raw})
	require.Equal(r.t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		StateID string `json:"stateId"`
		Options struct {
			PublicKey struct {
				Challenge string `json:"challenge"`
			} `json:"publicKey"`
		} `json:"options"`
	}
	require.NoError(r.t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(r.t, out.StateID)
	require.NotEmpty(r.t, out.Options.PublicKey.Challenge)
	return out.StateID, out.Options.PublicKey.Challenge
}

// TOTP user: no approval request; a factor-gated link that the link alone
// cannot use, confirmed with a valid authenticator code.
func TestStrongFactorReset_TOTPUserGetsFactorGatedLink(t *testing.T) {
	r := newSFRig(t, "totp")
	pid, addr := r.user("totp", true)
	secret := r.enrollTOTP(pid, addr)

	r.request(addr)
	assert.False(t, r.pending(pid), "a TOTP user is never queued for approval")
	raw := r.mail.lastToken(t, addr)
	v := r.validate(raw)
	assert.True(t, v.RequiresFactor)
	assert.Equal(t, []string{FactorTOTP}, v.FactorMethods)

	before := r.passwordHash(pid)
	rec := r.confirm(map[string]any{"token": raw, "password": sfPassword})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "the link alone does not reset: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "INVALID_FACTOR")
	rec = r.confirm(map[string]any{"token": raw, "password": sfPassword, "factorCode": "000000"})
	assert.Contains(t, rec.Body.String(), "INVALID_FACTOR")
	assert.Equal(t, before, r.passwordHash(pid))

	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	rec = r.confirm(map[string]any{"token": raw, "password": sfPassword, "factorCode": code})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotEqual(t, before, r.passwordHash(pid), "the password changed")
}

// Passkey user: a passkey is a strong factor — no approval request; a
// factor-gated link confirmed with a passkey assertion bound to the link's
// account. Another account's passkey, a bare link, and a TOTP code (it has
// none) are all refused.
func TestStrongFactorReset_PasskeyUserConfirmsWithPasskey(t *testing.T) {
	r := newSFRig(t, "pkey")
	pid, addr := r.user("pkey", true)
	a := r.registerPasskey(pid)
	otherPID, _ := r.user("other", true)
	other := r.registerPasskey(otherPID)

	r.request(addr)
	assert.False(t, r.pending(pid), "a passkey user is never queued for approval")
	raw := r.mail.lastToken(t, addr)
	v := r.validate(raw)
	assert.True(t, v.RequiresFactor, "a passkey makes the link factor-gated")
	assert.Equal(t, []string{FactorPasskey}, v.FactorMethods)

	before := r.passwordHash(pid)
	rec := r.confirm(map[string]any{"token": raw, "password": sfPassword})
	assert.Contains(t, rec.Body.String(), "INVALID_FACTOR", "the link alone does not reset")

	// An assertion by someone else's passkey, over this reset's challenge.
	state, chal := r.passkeyBegin(raw)
	rec = r.confirm(map[string]any{
		"token": raw, "password": sfPassword, "passkeyStateId": state,
		"passkeyCredential": json.RawMessage(other.Assert(chal, sfOrigin, sfRPID, []byte(otherPID))),
	})
	assert.Contains(t, rec.Body.String(), "INVALID_FACTOR")
	assert.Equal(t, before, r.passwordHash(pid))

	// The account's own passkey.
	state, chal = r.passkeyBegin(raw)
	rec = r.confirm(map[string]any{
		"token": raw, "password": sfPassword, "passkeyStateId": state,
		"passkeyCredential": json.RawMessage(a.Assert(chal, sfOrigin, sfRPID, []byte(pid))),
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotEqual(t, before, r.passwordHash(pid), "the password changed")

	// The ceremony is single use: replaying it fails (the token is gone too).
	rec = r.confirm(map[string]any{
		"token": raw, "password": sfPassword, "passkeyStateId": state,
		"passkeyCredential": json.RawMessage(a.Assert(chal, sfOrigin, sfRPID, []byte(pid))),
	})
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

// No strong factor (no 2FA at all, or only the email PIN) with the opt-in
// rule on: no link — a request for the client's administrators, which,
// approved by one holding user:update and user:reset-2fa, emails a link that
// resets the password and clears 2FA.
func TestStrongFactorReset_NoStrongFactorIsQueuedThenApproved(t *testing.T) {
	r := newSFRig(t, "queue")
	for _, emailPinOnly := range []bool{false, true} {
		local := map[bool]string{false: "nofactor", true: "emailpin"}[emailPinOnly]
		pid, addr := r.user(local, true)
		if emailPinOnly {
			r.enrollEmailPin(pid)
		}

		r.request(addr)
		assert.Zero(t, r.mail.count(addr), "%s: no link is emailed", local)
		require.True(t, r.pending(pid), "%s: a request awaits approval", local)
		r.request(addr) // idempotent while pending
		reqs, err := r.approvals.ListPending(r.ctx, []string{r.clientID})
		require.NoError(t, err)
		var reqID string
		for _, q := range reqs {
			if q.PrincipalID == pid {
				require.Empty(t, reqID, "one pending request per user")
				reqID = q.ID
			}
		}
		require.NotEmpty(t, reqID)

		admin := &auth.AuthContext{
			PrincipalID: pid, Scope: auth.ScopeClient, Clients: []string{r.clientID},
			Permissions: []string{"platform:iam:user:update"},
		}
		_, err = r.approver.Approve(auth.WithContext(r.ctx, admin), reqID)
		require.Error(t, err, "%s: approving without user:reset-2fa is refused", local)
		assert.True(t, r.pending(pid))

		admin.Permissions = append(admin.Permissions, "platform:iam:user:reset-2fa")
		_, err = r.approver.Approve(auth.WithContext(r.ctx, admin), reqID)
		require.NoError(t, err)
		assert.False(t, r.pending(pid))

		raw := r.mail.lastToken(t, addr)
		v := r.validate(raw)
		assert.False(t, v.RequiresFactor, "an approved link is authorised by the approval")
		before := r.passwordHash(pid)
		rec := r.confirm(map[string]any{"token": raw, "password": sfPassword})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.NotEqual(t, before, r.passwordHash(pid))
		enrolled, err := r.mfa.HasConfirmedMethod(r.ctx, pid)
		require.NoError(t, err)
		assert.False(t, enrolled, "%s: the approved reset clears 2FA", local)
	}
}

// A user with no strong factor and no home client (anchor/partner) gets no
// link and no request under the opt-in rule — recovery is an administrator's
// console reset.
func TestStrongFactorReset_RuleOnNoClientGetsNothing(t *testing.T) {
	r := newSFRig(t, "rule")
	pid, addr := r.user("anchor", false)
	r.request(addr)
	assert.Zero(t, r.mail.count(addr))
	assert.False(t, r.pending(pid))
}

// The default (owner ruling 2026-11-09: FC_PASSWORD_RESET_REQUIRE_STRONG_FACTOR
// off): a user with no strong factor — none at all, or only the email PIN —
// gets the plain emailed link, which alone resets; a user WITH one still gets
// the factor-gated link.
func TestStrongFactorReset_DefaultRuleOff(t *testing.T) {
	r := newSFRig(t, "off")
	r.s.RequireStrongFactorForReset = false
	for _, emailPinOnly := range []bool{false, true} {
		local := map[bool]string{false: "nofactor", true: "emailpin"}[emailPinOnly]
		pid, addr := r.user(local, true)
		if emailPinOnly {
			r.enrollEmailPin(pid)
		}
		r.request(addr)
		assert.False(t, r.pending(pid), "%s: never queued with the rule off", local)
		raw := r.mail.lastToken(t, addr)
		assert.False(t, r.validate(raw).RequiresFactor, "%s: the link alone authorises the reset", local)
		before := r.passwordHash(pid)
		rec := r.confirm(map[string]any{"token": raw, "password": sfPassword})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.NotEqual(t, before, r.passwordHash(pid))
	}

	pid, addr := r.user("strong", true)
	r.registerPasskey(pid)
	r.request(addr)
	raw := r.mail.lastToken(t, addr)
	v := r.validate(raw)
	assert.True(t, v.RequiresFactor, "a strong-factor user confirms with it, rule or no rule")
	assert.Equal(t, []string{FactorPasskey}, v.FactorMethods)
	rec := r.confirm(map[string]any{"token": raw, "password": sfPassword})
	assert.Contains(t, rec.Body.String(), "INVALID_FACTOR")
}
