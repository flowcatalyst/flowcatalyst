package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/operations"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// The strong-factor rule of a self-service password reset
// (docs/auth-hardening-plan.md, Phase 8): a reset link alone never resets the
// password of a user who holds a strong factor — an authenticator app (TOTP)
// or a passkey. Both arrive at the confirm step as proof: a TOTP code, or a
// passkey assertion made against a challenge minted for the reset token's
// principal. An email PIN is never strong (the PIN and the link reach the same
// inbox).

// Factor method names on the validate answer (factorMethods): which proofs the
// reset page may offer.
const (
	FactorTOTP    = "TOTP"
	FactorPasskey = "PASSKEY"
)

// PasskeyFactor is the passkey half of the strong-factor rule. WebAuthnFactor
// implements it over the platform's WebAuthn service.
type PasskeyFactor interface {
	// HasPasskey reports whether the principal has at least one registered
	// passkey.
	HasPasskey(ctx context.Context, principalID string) (bool, error)
	// BeginAssertion mints an assertion challenge for the principal's passkeys
	// and returns the ceremony state id plus the options the browser passes to
	// navigator.credentials.get().
	BeginAssertion(ctx context.Context, principalID string) (stateID string, options any, err error)
	// VerifyAssertion consumes the ceremony (single use) and reports whether
	// credential is a valid assertion, by one of principalID's passkeys, of the
	// challenge minted for principalID.
	VerifyAssertion(ctx context.Context, principalID, stateID string, credential []byte) (bool, error)
}

// errNoPasskey is BeginAssertion's answer for a principal with no passkey.
var errNoPasskey = errors.New("no passkey registered")

// WebAuthnFactor verifies passkey assertions for the reset confirm step with
// the same verifier, ceremony store and signature-counter bookkeeping as the
// passkey sign-in (/auth/webauthn/authenticate/*). Its ceremonies share the
// sign-in's ceremony kind: either one only ever proves possession of the
// principal's passkey, which is what both flows ask.
type WebAuthnFactor struct {
	Service *webauthn.Service
	UoW     *usecasepgx.UnitOfWork
}

// HasPasskey implements PasskeyFactor.
func (f WebAuthnFactor) HasPasskey(ctx context.Context, principalID string) (bool, error) {
	creds, err := f.Service.Credentials().LibraryCredentialsByPrincipal(ctx, principalID)
	if err != nil {
		return false, err
	}
	return len(creds) > 0, nil
}

// BeginAssertion implements PasskeyFactor.
func (f WebAuthnFactor) BeginAssertion(ctx context.Context, principalID string) (string, any, error) {
	creds, err := f.Service.Credentials().LibraryCredentialsByPrincipal(ctx, principalID)
	if err != nil {
		return "", nil, err
	}
	if len(creds) == 0 {
		return "", nil, errNoPasskey
	}
	user := &webauthn.PrincipalUser{PrincipalID: principalID, Credentials: creds}
	options, session, err := f.Service.WebAuthn().BeginLogin(user)
	if err != nil {
		return "", nil, err
	}
	stateID, err := randomStateID()
	if err != nil {
		return "", nil, err
	}
	pid := principalID
	if err := f.Service.Ceremonies().StoreAuthentication(ctx, stateID, &pid, session); err != nil {
		return "", nil, err
	}
	return stateID, options, nil
}

// VerifyAssertion implements PasskeyFactor. Every refusal is (false, nil): the
// caller counts it as a failed factor attempt; only a storage failure is an
// error.
func (f WebAuthnFactor) VerifyAssertion(ctx context.Context, principalID, stateID string, credential []byte) (bool, error) {
	consumed, err := f.Service.Ceremonies().ConsumeAuthentication(ctx, stateID)
	if err != nil {
		return false, err
	}
	// The challenge must have been minted for THIS token's principal: an
	// assertion by someone else's passkey never resets this password.
	if consumed == nil || consumed.PrincipalID == nil || *consumed.PrincipalID != principalID {
		return false, nil
	}
	creds, err := f.Service.Credentials().LibraryCredentialsByPrincipal(ctx, principalID)
	if err != nil {
		return false, err
	}
	if len(creds) == 0 {
		return false, nil
	}
	user := &webauthn.PrincipalUser{PrincipalID: principalID, Credentials: creds}
	parsed, err := protocol.ParseCredentialRequestResponseBody(io.NopCloser(bytes.NewReader(credential)))
	if err != nil {
		return false, nil
	}
	cred, err := f.Service.WebAuthn().ValidateLogin(user, consumed.Session, parsed)
	if err != nil {
		return false, nil
	}
	// A signature counter that did not advance is a cloned authenticator or a
	// replayed state (WebAuthn L3 §7.2 step 22), refused exactly as the
	// passkey sign-in refuses it.
	if cred.Authenticator.CloneWarning {
		slog.Warn("password reset: passkey signature counter regression; assertion refused", "principal", principalID)
		return false, nil
	}
	f.recordAssertion(ctx, principalID, stateID, cred)
	return true, nil
}

// recordAssertion stores the advanced signature counter and last_used_at on
// the asserted credential (best-effort, as on the sign-in path): without it
// counter regression could not be detected on the next assertion.
func (f WebAuthnFactor) recordAssertion(ctx context.Context, principalID, stateID string, updated *gowebauthn.Credential) {
	if f.UoW == nil {
		return
	}
	stored, err := f.Service.Credentials().FindByCredentialID(ctx, updated.ID)
	if err != nil || stored == nil || stored.PrincipalID != principalID {
		slog.Warn("password reset: could not locate the asserted passkey to record its counter", "principal", principalID, "err", err)
		return
	}
	if _, err := usecaseop.Run(ctx, f.UoW, operations.Authenticate(f.Service.Credentials()),
		operations.AuthenticateCommand{StateID: stateID, UpdatedCredential: *updated, PersistedCredentialID: stored.ID},
		usecase.NewExecutionContext(principalID)); err != nil {
		slog.Warn("password reset: persisting the passkey signature counter failed", "principal", principalID, "err", err)
	}
}

func randomStateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// strongFactors reports which strong factors the user holds: a confirmed
// authenticator app (TOTP) and/or a registered passkey. A lookup failure
// counts as "not held" and is logged; with RequireStrongFactorForReset on, the
// request then goes to the approval queue rather than to a link.
func (s *State) strongFactors(ctx context.Context, principalID string) (totp, passkey bool) {
	if s.MFA != nil {
		methods, err := s.MFA.ConfirmedMethods(ctx, principalID)
		if err != nil {
			slog.Warn("password reset: could not read the user's 2FA methods", "principal", principalID, "err", err)
		}
		for _, m := range methods {
			if m == mfa.MethodTOTP {
				totp = true
			}
		}
	}
	if s.Passkeys != nil {
		has, err := s.Passkeys.HasPasskey(ctx, principalID)
		if err != nil {
			slog.Warn("password reset: could not read the user's passkeys", "principal", principalID, "err", err)
		}
		passkey = has
	}
	return totp, passkey
}

// hasStrongFactor reports whether the user holds any strong factor (TOTP or
// a passkey) — the factors that authorise a self-service reset confirm.
func (s *State) hasStrongFactor(ctx context.Context, principalID string) bool {
	totp, passkey := s.strongFactors(ctx, principalID)
	return totp || passkey
}

// factorMethods lists the strong factors the reset page may offer.
func (s *State) factorMethods(ctx context.Context, principalID string) []string {
	totp, passkey := s.strongFactors(ctx, principalID)
	out := []string{}
	if totp {
		out = append(out, FactorTOTP)
	}
	if passkey {
		out = append(out, FactorPasskey)
	}
	return out
}

// ── POST /auth/password-reset/passkey/begin ─────────────────────────────────

type passkeyBeginResponse struct {
	StateID string `json:"stateId"`
	Options any    `json:"options"`
}

// passkeyBegin mints the passkey assertion challenge a factor-gated reset is
// confirmed with. It needs a live, factor-gated reset token for a user who has
// a passkey; the challenge is bound to that token's principal.
func (s *State) passkeyBegin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, usecase.Validation("INVALID_BODY", "malformed request body"))
		return
	}
	t, err := s.Tokens.FindByTokenHash(r.Context(), hashToken(body.Token))
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "token lookup failed", err))
		return
	}
	if t == nil || t.IsExpired() || strings.HasPrefix(t.PrincipalID, portalSubjectPrefix) {
		httperror.Write(w, usecase.Validation("INVALID_TOKEN", "Invalid or expired reset token."))
		return
	}
	if !t.RequiresFactor || s.Passkeys == nil {
		httperror.Write(w, usecase.Validation("PASSKEY_NOT_AVAILABLE", "This reset is not confirmed with a passkey."))
		return
	}
	stateID, options, err := s.Passkeys.BeginAssertion(r.Context(), t.PrincipalID)
	if errors.Is(err, errNoPasskey) {
		httperror.Write(w, usecase.Validation("PASSKEY_NOT_AVAILABLE", "This account has no passkey."))
		return
	}
	if err != nil {
		httperror.Write(w, usecase.Internal("WEBAUTHN", "could not start the passkey check", err))
		return
	}
	writeJSON(w, http.StatusOK, passkeyBeginResponse{StateID: stateID, Options: options})
}
