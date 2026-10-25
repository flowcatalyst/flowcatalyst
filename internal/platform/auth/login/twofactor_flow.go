package login

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/loginbackoff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/mfatoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/twofa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/notify"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// TwoFactor is the second factor of a password sign-in and the self-service
// management of a user's factors: the ONE implementation of the 2FA rules. The
// JSON endpoints (/auth/2fa/*, twofactor.go and twofactor_selfservice.go) and
// the server-rendered pages (internal/ui/pages/auth_mfa*.go) both call it, and
// after a password it is the only code that issues a session.
//
// The rules:
//
//   - The usable factors are the user's confirmed ones, narrowed to the
//     domain's allowed set when the domain compels 2FA. A challenge is passed
//     only with a usable factor, and the sign-in email code is sent only when
//     the email factor is usable: a factor the domain no longer allows is dead.
//   - A recovery code is accepted only while the user has a usable
//     authenticator-app (TOTP) factor, and is generated only for one: recovery
//     codes back the authenticator app; email is its own recovery channel.
//   - An enroll token is not a session. It leads to one only by confirming a
//     new factor in that flow, and only while the account still owes one (has
//     no usable factor). Once any usable factor exists, set up in this flow or
//     anywhere else, the token enrols nothing and the user must pass the
//     challenge instead.
//   - A method the domain does not allow is not enrolled; a user whose domain
//     compels 2FA cannot remove their last usable factor.
//   - Remember-this-device is honoured only when the domain allows it.
//   - Wrong codes are failed USER_LOGIN attempts and feed the password step's
//     per-(email, IP) backoff; while it holds, no code is checked.
//   - The sign-in email code is sent at most once per resend interval
//     (mfa.Config.EmailPinResendInterval, one minute).
//
// The between-step tokens are stateless (mfatoken: HS256, no server-side
// record), so they are not single-use; what bounds a replay is the rule set
// above (a pending token needs a fresh factor every time, TOTP steps, email
// codes and recovery codes are single-use; an enroll token dies with the first
// usable factor) and their lifetimes.
type TwoFactor struct {
	MFA        MFAService
	Tokens     TokenIssuer
	Policy     DomainPolicy
	Principals PrincipalFinder
	// Sessions mints the session JWT; SessionTTL bounds it (zero: SessionTTL,
	// the package's, read at sign-in).
	Sessions   SessionMinter
	SessionTTL time.Duration
	// Attempts records USER_LOGIN outcomes and feeds the backoff (nil: neither).
	Attempts *loginattempt.Repository
	Backoff  loginbackoff.Policy
	// Notifier (security emails) and Audit (2FA state changes) are optional.
	Notifier *notify.Notifier
	Audit    *audit.Repository
	// CookieSecure is the Secure flag of the session and trusted-device cookies.
	CookieSecure bool
	// PendingTokenTTL and EnrollTokenTTL bound the between-step tokens (zero:
	// 10 and 30 minutes).
	PendingTokenTTL, EnrollTokenTTL time.Duration
}

// MFAService is the part of mfa.Service the flow uses.
type MFAService interface {
	ConfirmedMethods(ctx context.Context, principalID string) ([]mfa.MethodType, error)
	VerifyTOTP(ctx context.Context, principalID, code string) (bool, error)
	VerifyLoginEmailPin(ctx context.Context, principalID, code string) (bool, error)
	SendLoginEmailPin(ctx context.Context, principalID, emailAddr string) error
	VerifyRecoveryCode(ctx context.Context, principalID, code string) (bool, error)
	BeginTOTPEnrollment(ctx context.Context, principalID, accountName string) (*mfa.TOTPEnrollment, error)
	ConfirmTOTPEnrollment(ctx context.Context, principalID, code string) (bool, error)
	BeginEmailEnrollment(ctx context.Context, principalID, emailAddr string) error
	ConfirmEmailEnrollment(ctx context.Context, principalID, code string) (bool, error)
	RemoveMethod(ctx context.Context, principalID string, t mfa.MethodType) error
	GenerateRecoveryCodes(ctx context.Context, principalID string) ([]string, error)
	RemainingRecoveryCodes(ctx context.Context, principalID string) (int, error)
	IssueTrustedDevice(ctx context.Context, principalID string, label *string, ttl time.Duration) (string, error)
	VerifyTrustedDevice(ctx context.Context, principalID, raw string) (bool, error)
	ListTrustedDevices(ctx context.Context, principalID string) ([]mfa.TrustedDevice, error)
	RevokeTrustedDevice(ctx context.Context, principalID, id string) error
}

// TokenIssuer mints and parses the between-step tokens (mfatoken.Issuer).
type TokenIssuer interface {
	Mint(subject string, purpose mfatoken.Purpose, ttl time.Duration) (string, error)
	Parse(token string, want mfatoken.Purpose) (*mfatoken.Claims, error)
}

// DomainPolicy resolves a domain's 2FA stance (twofa.Policy).
type DomainPolicy interface {
	Evaluate(ctx context.Context, email string) twofa.Eval
}

// PrincipalFinder loads the principal a token or a session names.
type PrincipalFinder interface {
	FindByID(ctx context.Context, id ids.PrincipalID) (*principal.Principal, error)
}

// SessionMinter mints the session JWT (provider.Provider).
type SessionMinter interface {
	MintSessionToken(ctx context.Context, principalID string, ttl time.Duration) (string, error)
}

// MethodRecoveryCode is the challenge method of a recovery code.
const MethodRecoveryCode = "RECOVERY_CODE"

// allMethods is the menu offered when a domain doesn't restrict 2FA methods
// (voluntary enrollment).
var allMethods = []string{string(mfa.MethodTOTP), string(mfa.MethodEmailPin)}

// The flow's refusals. Callers map them to their answer (JSON or a page).
var (
	// ErrInvalidToken: the between-step token is missing, bad, expired, of the
	// other purpose, or names an unknown or inactive user.
	ErrInvalidToken = errors.New("login: invalid or expired two-factor token")
	// ErrInvalidMethod: not a method name the flow knows.
	ErrInvalidMethod = errors.New("login: unknown two-factor method")
	// ErrMethodNotUsable: a known method this user cannot challenge with (not
	// held, or not allowed by the domain; a recovery code without usable TOTP).
	ErrMethodNotUsable = errors.New("login: two-factor method not usable for this account")
	// ErrInvalidCode: the code did not verify.
	ErrInvalidCode = errors.New("login: invalid or expired code")
	// ErrNoEmail: the account has no email address.
	ErrNoEmail = errors.New("login: account has no email")
	// ErrEmailNotSent: the sign-in email code could not be sent.
	ErrEmailNotSent = errors.New("login: email code not sent")
	// ErrMethodNotAllowed: the domain does not allow enrolling this method.
	ErrMethodNotAllowed = errors.New("login: two-factor method not permitted for this domain")
	// ErrEnrollmentNotOwed: an enroll token for an account that already has a
	// usable factor; it must sign in again and pass the challenge.
	ErrEnrollmentNotOwed = errors.New("login: the account already has a usable second factor")
	// ErrLastFactor: removing the method would leave a 2FA-required user
	// without a usable factor.
	ErrLastFactor = errors.New("login: the domain requires a second factor")
	// ErrNoTOTP: recovery codes need a usable authenticator-app factor.
	ErrNoTOTP = errors.New("login: recovery codes apply to authenticator-app 2FA")
	// ErrSessionNotIssued: the factor passed but the session could not be
	// minted (a server fault).
	ErrSessionNotIssued = errors.New("login: session not issued")
)

// ThrottledError refuses a code while the sign-in backoff holds.
type ThrottledError struct{ RetryAfterSecs uint32 }

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("login: too many failed attempts; retry in %ds", e.RetryAfterSecs)
}

// ── the decision after the password ────────────────────────────────────────

// DecisionKind is what a just-verified password leads to.
type DecisionKind int

// The outcomes of Decide.
const (
	// NoSecondFactor: complete the sign-in.
	NoSecondFactor DecisionKind = iota
	// ChallengeOwed: pass a challenge with Token (a pending token).
	ChallengeOwed
	// EnrollmentOwed: enrol a factor with Token (an enroll token).
	EnrollmentOwed
)

// Decision is the second-factor requirement of a password sign-in.
type Decision struct {
	Kind  DecisionKind
	Token string
	// Methods are the usable factor types (ChallengeOwed); AllowedMethods the
	// ones the user may enrol (EnrollmentOwed).
	Methods, AllowedMethods []string
	RememberAllowed         bool
}

// Decide is the second-factor gate after p's password verified: a usable
// factor means a challenge (unless this browser is a remembered device), none
// on a domain that compels 2FA means enrolment, otherwise nothing. Federated
// users never carry a password, so 2FA does not apply to them. A passkey does
// NOT exempt the password path. An error means "could not decide": fail
// closed.
func (t *TwoFactor) Decide(r *http.Request, p *principal.Principal) (Decision, error) {
	if p.ExternalIdentity != nil {
		return Decision{}, nil
	}
	ctx := r.Context()
	usable, ev, err := t.usable(ctx, p)
	if err != nil {
		return Decision{}, err
	}
	if len(usable) > 0 {
		if ev.RememberEnabled() {
			if raw := t.readTrustedDeviceCookie(r); raw != "" {
				ok, verr := t.MFA.VerifyTrustedDevice(ctx, string(p.ID), raw)
				if verr != nil {
					return Decision{}, verr
				}
				if ok {
					return Decision{}, nil
				}
			}
		}
		tok, err := t.Tokens.Mint(string(p.ID), mfatoken.PurposePending, t.PendingTTL())
		if err != nil {
			return Decision{}, err
		}
		return Decision{Kind: ChallengeOwed, Token: tok, Methods: usable, RememberAllowed: ev.RememberEnabled()}, nil
	}
	if !ev.Requires2FA() {
		return Decision{}, nil
	}
	tok, err := t.Tokens.Mint(string(p.ID), mfatoken.PurposeEnroll, t.EnrollTTL())
	if err != nil {
		return Decision{}, err
	}
	return Decision{Kind: EnrollmentOwed, Token: tok, AllowedMethods: ev.AllowedMethods()}, nil
}

// ── the challenge (pending token) ──────────────────────────────────────────

// Challenge is a pending sign-in: its user and what can complete it.
type Challenge struct {
	Principal *principal.Principal
	// Methods are the usable factor types; RememberAllowed whether the domain
	// lets the browser be remembered.
	Methods         []string
	RememberAllowed bool
}

// Allows reports whether method may answer the challenge: a usable factor, or
// a recovery code while the authenticator app is usable.
func (c *Challenge) Allows(method string) bool {
	if method == MethodRecoveryCode {
		return slices.Contains(c.Methods, string(mfa.MethodTOTP))
	}
	return slices.Contains(c.Methods, method)
}

// PendingChallenge is the challenge a pending token stands for. Methods is
// empty when the user has no usable factor left (they were removed since the
// password step).
func (t *TwoFactor) PendingChallenge(ctx context.Context, token string) (*Challenge, error) {
	p, err := t.tokenPrincipal(ctx, token, mfatoken.PurposePending)
	if err != nil {
		return nil, err
	}
	usable, ev, err := t.usable(ctx, p)
	if err != nil {
		return nil, err
	}
	return &Challenge{Principal: p, Methods: usable, RememberAllowed: ev.RememberEnabled()}, nil
}

// Verify checks code for the pending token's user and, when it passes,
// completes the sign-in: the trusted-device cookie (when asked for and the
// domain allows it) and the session cookie are set on w and the success is
// recorded. The caller writes the answer.
func (t *TwoFactor) Verify(w http.ResponseWriter, r *http.Request, token, method, code string, rememberDevice bool) (*principal.Principal, error) {
	ctx := r.Context()
	c, err := t.PendingChallenge(ctx, token)
	if err != nil {
		return nil, err
	}
	p := c.Principal
	email, ip := emailOf(p), clientIP(r)
	if err := t.checkBackoff(ctx, email, ip); err != nil {
		return nil, err
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	var verify func(context.Context, string, string) (bool, error)
	switch method {
	case string(mfa.MethodTOTP):
		verify = t.MFA.VerifyTOTP
	case string(mfa.MethodEmailPin):
		verify = t.MFA.VerifyLoginEmailPin
	case MethodRecoveryCode:
		verify = t.MFA.VerifyRecoveryCode
	default:
		return nil, ErrInvalidMethod
	}
	if !c.Allows(method) {
		return nil, ErrMethodNotUsable
	}
	ok, err := verify(ctx, string(p.ID), code)
	if err != nil {
		return nil, fmt.Errorf("login: verify %s: %w", method, err)
	}
	if !ok {
		recordLoginAttempt(ctx, t.Attempts, loginattempt.OutcomeFailure, email, ids.StringPtr(&p.ID), ip, "Invalid 2FA code")
		return nil, ErrInvalidCode
	}
	// A recovery code just got burned: let the user know out-of-band.
	if method == MethodRecoveryCode {
		t.Notifier.RecoveryCodeUsed(ctx, email)
	}
	if rememberDevice {
		t.rememberDevice(w, r, p)
	}
	if err := t.issueSession(w, r, p); err != nil {
		return nil, err
	}
	return p, nil
}

// SendChallengeEmail emails a sign-in code to the pending token's user, when
// the email factor is usable for them. Within the resend interval it sends
// nothing and returns the service's *mfa.ResendError.
func (t *TwoFactor) SendChallengeEmail(ctx context.Context, token string) error {
	c, err := t.PendingChallenge(ctx, token)
	if err != nil {
		return err
	}
	if !c.Allows(string(mfa.MethodEmailPin)) {
		return ErrMethodNotUsable
	}
	email := emailOf(c.Principal)
	if email == "" {
		return ErrNoEmail
	}
	if err := t.MFA.SendLoginEmailPin(ctx, string(c.Principal.ID), email); err != nil {
		if errors.Is(err, mfa.ErrCodeRecentlySent) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrEmailNotSent, err)
	}
	return nil
}

// ── enrolment after the password (enroll token) ────────────────────────────

// Enrollment is a sign-in that owes a factor: its user and what they may enrol.
type Enrollment struct {
	Principal *principal.Principal
	Methods   []string
}

// PendingEnrollment is the enrolment an enroll token stands for, while the
// account still owes a factor: ErrEnrollmentNotOwed once it has a usable one.
func (t *TwoFactor) PendingEnrollment(ctx context.Context, token string) (*Enrollment, error) {
	p, err := t.tokenPrincipal(ctx, token, mfatoken.PurposeEnroll)
	if err != nil {
		return nil, err
	}
	usable, ev, err := t.usable(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(usable) > 0 {
		return nil, ErrEnrollmentNotOwed
	}
	return &Enrollment{Principal: p, Methods: enrollable(ev)}, nil
}

// BeginEnrollment starts enrolling method for the enroll token's user: the
// authenticator setup (TOTP), or the confirmation code emailed (EMAIL_PIN,
// nil setup).
func (t *TwoFactor) BeginEnrollment(ctx context.Context, token string, method mfa.MethodType) (*mfa.TOTPEnrollment, error) {
	en, err := t.PendingEnrollment(ctx, token)
	if err != nil {
		return nil, err
	}
	return t.begin(ctx, en.Principal, method)
}

// CompleteEnrollment confirms the factor begun with the enroll token and, when
// the code matches, completes the sign-in (the session cookie set on w, the
// success recorded). It returns the user and the first recovery codes, shown
// once (nil for an email factor).
func (t *TwoFactor) CompleteEnrollment(w http.ResponseWriter, r *http.Request, token string, method mfa.MethodType, code string) (*principal.Principal, []string, error) {
	ctx := r.Context()
	en, err := t.PendingEnrollment(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	if err := t.confirm(ctx, en.Principal, method, code); err != nil {
		return nil, nil, err
	}
	codes := t.ensureRecoveryCodes(ctx, en.Principal)
	if err := t.issueSession(w, r, en.Principal); err != nil {
		return nil, nil, err
	}
	return en.Principal, codes, nil
}

// ── the session ────────────────────────────────────────────────────────────

// issueSession sets the session cookie for p and records the successful
// sign-in.
func (t *TwoFactor) issueSession(w http.ResponseWriter, r *http.Request, p *principal.Principal) error {
	ttl := t.SessionTTL
	if ttl <= 0 {
		ttl = SessionTTL
	}
	return issueSession(w, r, t.Sessions, t.Attempts, t.CookieSecure, ttl, p)
}

// issueSession mints p's session JWT, sets it as the session cookie and
// records the successful USER_LOGIN attempt: the end of every password sign-in.
func issueSession(w http.ResponseWriter, r *http.Request, minter SessionMinter, attempts *loginattempt.Repository, secure bool, ttl time.Duration, p *principal.Principal) error {
	token, err := minter.MintSessionToken(r.Context(), string(p.ID), ttl)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSessionNotIssued, err)
	}
	http.SetCookie(w, platformmw.SessionCookie(token, secure, ttl, true))
	recordLoginAttempt(r.Context(), attempts, loginattempt.OutcomeSuccess, emailOf(p), ids.StringPtr(&p.ID), clientIP(r), "")
	return nil
}

// ── shared steps ───────────────────────────────────────────────────────────

// usable is p's usable factor types and the domain's evaluation.
func (t *TwoFactor) usable(ctx context.Context, p *principal.Principal) ([]string, twofa.Eval, error) {
	ev := t.Policy.Evaluate(ctx, emailOf(p))
	confirmed, err := t.MFA.ConfirmedMethods(ctx, string(p.ID))
	if err != nil {
		return nil, ev, err
	}
	return usableMethods(confirmed, ev), ev, nil
}

// usableMethods narrows the confirmed factors to the domain's allowed set when
// it compels 2FA.
func usableMethods(confirmed []mfa.MethodType, ev twofa.Eval) []string {
	out := methodStrings(confirmed)
	if ev.Requires2FA() {
		out = intersect(out, ev.AllowedMethods())
	}
	return out
}

// enrollable is the methods a user may enrol: the domain's allowed set when it
// compels 2FA, both otherwise.
func enrollable(ev twofa.Eval) []string {
	if ev.Requires2FA() {
		return ev.AllowedMethods()
	}
	return allMethods
}

// methodAllowed reports whether a user may enrol method m.
func methodAllowed(ev twofa.Eval, m mfa.MethodType) bool {
	return !ev.Requires2FA() || slices.Contains(ev.AllowedMethods(), string(m))
}

// begin starts enrolling method for p, after the domain gate.
func (t *TwoFactor) begin(ctx context.Context, p *principal.Principal, method mfa.MethodType) (*mfa.TOTPEnrollment, error) {
	ev := t.Policy.Evaluate(ctx, emailOf(p))
	switch method {
	case mfa.MethodTOTP, mfa.MethodEmailPin:
	default:
		return nil, ErrInvalidMethod
	}
	if !methodAllowed(ev, method) {
		return nil, ErrMethodNotAllowed
	}
	if method == mfa.MethodTOTP {
		return t.MFA.BeginTOTPEnrollment(ctx, string(p.ID), emailOf(p))
	}
	email := emailOf(p)
	if email == "" {
		return nil, ErrNoEmail
	}
	return nil, t.MFA.BeginEmailEnrollment(ctx, string(p.ID), email)
}

// confirm confirms the begun enrolment of method for p (the domain gate again:
// the policy may have changed since begin), then notifies and audits.
func (t *TwoFactor) confirm(ctx context.Context, p *principal.Principal, method mfa.MethodType, code string) error {
	ev := t.Policy.Evaluate(ctx, emailOf(p))
	var confirm func(context.Context, string, string) (bool, error)
	op := ""
	switch method {
	case mfa.MethodTOTP:
		confirm, op = t.MFA.ConfirmTOTPEnrollment, "2FA_TOTP_ENROLLED"
	case mfa.MethodEmailPin:
		confirm, op = t.MFA.ConfirmEmailEnrollment, "2FA_EMAIL_ENROLLED"
	default:
		return ErrInvalidMethod
	}
	if !methodAllowed(ev, method) {
		return ErrMethodNotAllowed
	}
	ok, err := confirm(ctx, string(p.ID), strings.TrimSpace(code))
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCode
	}
	t.Notifier.TwoFactorEnrolled(ctx, emailOf(p), string(method))
	t.auditMFA(ctx, string(p.ID), op)
	return nil
}

// ensureRecoveryCodes generates a first recovery-code set only when the user
// has a usable TOTP factor and no codes yet, returning the plaintext codes
// (shown once) or nil.
func (t *TwoFactor) ensureRecoveryCodes(ctx context.Context, p *principal.Principal) []string {
	usable, _, err := t.usable(ctx, p)
	if err != nil || !slices.Contains(usable, string(mfa.MethodTOTP)) {
		return nil
	}
	n, err := t.MFA.RemainingRecoveryCodes(ctx, string(p.ID))
	if err != nil || n > 0 {
		return nil
	}
	codes, err := t.MFA.GenerateRecoveryCodes(ctx, string(p.ID))
	if err != nil {
		slog.ErrorContext(ctx, "recovery code generation failed", "principal", p.ID, "err", err)
		return nil
	}
	t.Notifier.RecoveryCodesRegenerated(ctx, emailOf(p))
	return codes
}

// rememberDevice issues a trusted-device token and sets the cookie, when the
// domain allows it (a no-op otherwise).
func (t *TwoFactor) rememberDevice(w http.ResponseWriter, r *http.Request, p *principal.Principal) {
	ev := t.Policy.Evaluate(r.Context(), emailOf(p))
	if !ev.RememberEnabled() {
		return
	}
	ttl := time.Duration(ev.RememberDays()) * 24 * time.Hour
	label := userAgentLabel(r)
	raw, err := t.MFA.IssueTrustedDevice(r.Context(), string(p.ID), label, ttl)
	if err != nil {
		slog.ErrorContext(r.Context(), "issue trusted device failed", "principal", p.ID, "err", err)
		return
	}
	http.SetCookie(w, platformmw.NewCookie(platformmw.CookieSpec{
		Name: trustedDeviceCookieName(t.CookieSecure), Value: raw, Secure: t.CookieSecure,
		SameSite: http.SameSiteStrictMode, TTL: ttl, SetExpires: true,
	}))
	labelStr := ""
	if label != nil {
		labelStr = *label
	}
	t.Notifier.NewTrustedDevice(r.Context(), emailOf(p), labelStr)
}

func (t *TwoFactor) readTrustedDeviceCookie(r *http.Request) string {
	c, err := r.Cookie(trustedDeviceCookieName(t.CookieSecure))
	if err != nil {
		return ""
	}
	return c.Value
}

// checkBackoff refuses while the per-(email, IP) sign-in backoff holds.
func (t *TwoFactor) checkBackoff(ctx context.Context, email, ip string) error {
	if t.Attempts == nil {
		return nil
	}
	if d, err := loginbackoff.Check(ctx, t.Attempts, t.Backoff, email, ip); err == nil && !d.Allowed {
		return &ThrottledError{RetryAfterSecs: d.RetryAfterSecs}
	}
	return nil
}

// tokenPrincipal parses token for purpose and loads its active user.
func (t *TwoFactor) tokenPrincipal(ctx context.Context, token string, purpose mfatoken.Purpose) (*principal.Principal, error) {
	if token == "" || len(token) > 4096 {
		return nil, ErrInvalidToken
	}
	claims, err := t.Tokens.Parse(token, purpose)
	if err != nil {
		return nil, ErrInvalidToken
	}
	p, err := t.Principals.FindByID(ctx, ids.PrincipalID(claims.Subject))
	if err != nil || p == nil || !p.Active {
		return nil, ErrInvalidToken
	}
	return p, nil
}

// auditMFA records a 2FA state change to the audit trail (best effort).
func (t *TwoFactor) auditMFA(ctx context.Context, principalID, operation string) {
	if t.Audit == nil {
		return
	}
	pid := principalID
	_ = t.Audit.Insert(ctx, &audit.Log{
		ID:          tsid.Generate(tsid.AuditLog),
		EntityType:  "PRINCIPAL",
		EntityID:    principalID,
		Operation:   operation,
		PrincipalID: &pid,
		PerformedAt: time.Now().UTC(),
	})
}

// PendingTTL is the lifetime of a pending token.
func (t *TwoFactor) PendingTTL() time.Duration {
	if t.PendingTokenTTL > 0 {
		return t.PendingTokenTTL
	}
	return defaultPendingTokenTTL
}

// EnrollTTL is the lifetime of an enroll token.
func (t *TwoFactor) EnrollTTL() time.Duration {
	if t.EnrollTokenTTL > 0 {
		return t.EnrollTokenTTL
	}
	return defaultEnrollTokenTTL
}

// recordLoginAttempt best-effort logs a USER_LOGIN outcome (no-op without a
// repository). The identifier is normalised so records and backoff checks
// always agree whatever casing the caller passed.
func recordLoginAttempt(ctx context.Context, repo *loginattempt.Repository, outcome loginattempt.Outcome, email string, principalID *string, ip, failureReason string) {
	if repo == nil {
		return
	}
	email = strings.ToLower(strings.TrimSpace(email))
	a := loginattempt.New(loginattempt.AttemptUserLogin, outcome)
	a.Identifier = &email
	a.PrincipalID = principalID
	if ip != "" {
		a.IPAddress = &ip
	}
	if failureReason != "" {
		a.FailureReason = &failureReason
	}
	_ = repo.Record(ctx, a)
}
