// Package api wires the WebAuthn HTTP routes under /auth/webauthn/*.
//
// Six endpoints mirror fc-platform/src/webauthn/api.rs:
//
//   POST /auth/webauthn/register/begin          — issue registration challenge
//   POST /auth/webauthn/register/complete       — verify attestation, persist
//   POST /auth/webauthn/authenticate/begin      — issue authentication challenge
//   POST /auth/webauthn/authenticate/complete   — verify assertion, set session
//   GET  /auth/webauthn/credentials             — list user's passkeys
//   DELETE /auth/webauthn/credentials/{id}      — revoke a passkey
//
// Drop-in parity caveats:
//   - The enumeration-defence fake_authentication_challenge for unknown /
//     federated / no-credentials emails ships in a focused follow-up; for
//     now those cases return 200 with a synthetic state_id whose /complete
//     fails with INVALID_CREDENTIALS — same observable behaviour modulo
//     the deterministic-fake allowCredentials list.
//   - Session-cookie write on authenticate/complete is delegated to the
//     SessionWriter callback the platform server installs (see
//     bridge/login_endpoint.go for the same pattern).
package api

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/operations"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Service      *webauthn.Service
	Principals   *principal.Repository
	RegisterUC   *operations.RegisterUseCase
	AuthenticateUC *operations.AuthenticateUseCase
	RevokeUC     *operations.RevokeUseCase

	// SessionWriter is invoked on successful authenticate/complete. The
	// platform server installs a function that sets the session cookie
	// and returns 200 with the principal payload.
	SessionWriter func(w http.ResponseWriter, r *http.Request, principalID string)
}

// RegisterRoutes mounts the WebAuthn endpoints.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/auth/webauthn", func(r chi.Router) {
		r.Post("/register/begin", s.registerBegin)
		r.Post("/register/complete", s.registerComplete)
		r.Post("/authenticate/begin", s.authenticateBegin)
		r.Post("/authenticate/complete", s.authenticateComplete)
		r.Get("/credentials", s.listCredentials)
		r.Delete("/credentials/{id}", s.deleteCredential)
	})
}

// ── register ─────────────────────────────────────────────────────────────

type registerBeginRequest struct {
	DisplayName *string `json:"displayName,omitempty"`
}

type registerBeginResponse struct {
	StateID string `json:"stateId"`
	Options any    `json:"options"`
}

func (s *State) registerBegin(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil || ac.PrincipalID == "" {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	p, err := s.Principals.FindByID(r.Context(), ac.PrincipalID)
	if err != nil || p == nil {
		httperror.Write(w, httperror.NotFound("Principal", ac.PrincipalID))
		return
	}
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}

	var body registerBeginRequest
	_ = json.NewDecoder(r.Body).Decode(&body) // body is optional
	displayName := p.Name
	if body.DisplayName != nil && *body.DisplayName != "" {
		displayName = *body.DisplayName
	}

	existing, err := s.Service.Credentials().LibraryCredentialsByPrincipal(r.Context(), p.ID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list credentials failed", err))
		return
	}
	user := &webauthn.PrincipalUser{
		PrincipalID: p.ID,
		DisplayName: displayName,
		Username:    email,
		Credentials: existing,
	}
	options, sessionData, err := s.Service.WebAuthn().BeginRegistration(user,
		gowebauthn.WithExclusions(credentialDescriptorsFor(existing)))
	if err != nil {
		httperror.Write(w, usecase.Internal("WEBAUTHN", "begin registration failed", err))
		return
	}
	stateID := newUUID()
	if err := s.Service.Ceremonies().StoreRegistration(r.Context(), stateID, p.ID, sessionData, &displayName); err != nil {
		httperror.Write(w, usecase.Internal("REPO", "store ceremony failed", err))
		return
	}
	writeJSON(w, http.StatusOK, registerBeginResponse{StateID: stateID, Options: options})
}

type registerCompleteRequest struct {
	StateID    string          `json:"stateId"`
	Name       *string         `json:"name,omitempty"`
	Credential json.RawMessage `json:"credential"`
}

type registerCompleteResponse struct {
	CredentialID string `json:"credentialId"`
}

func (s *State) registerComplete(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil || ac.PrincipalID == "" {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}

	var body registerCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}

	consumed, err := s.Service.Ceremonies().ConsumeRegistration(r.Context(), body.StateID)
	if err != nil || consumed == nil {
		httperror.Write(w, httperror.BadRequest("STATE_NOT_FOUND",
			"registration ceremony state not found or expired"))
		return
	}
	if consumed.PrincipalID != ac.PrincipalID {
		httperror.Write(w, httperror.Forbidden("registration ceremony belongs to a different principal"))
		return
	}

	p, err := s.Principals.FindByID(r.Context(), consumed.PrincipalID)
	if err != nil || p == nil {
		httperror.Write(w, httperror.NotFound("Principal", consumed.PrincipalID))
		return
	}
	user := &webauthn.PrincipalUser{PrincipalID: p.ID, DisplayName: p.Name}

	// Parse the browser's attestation response and call CreateCredential.
	parsed, err := protocol.ParseCredentialCreationResponseBody(io.NopCloser(bytes.NewReader(body.Credential)))
	if err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_CREDENTIAL", err.Error()))
		return
	}
	cred, err := s.Service.WebAuthn().CreateCredential(user, consumed.Session, parsed)
	if err != nil {
		httperror.Write(w, httperror.BadRequest("ATTESTATION_INVALID", err.Error()))
		return
	}

	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.RegisterUC,
		operations.RegisterCommand{StateID: body.StateID, Response: *cred, Name: body.Name}, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, registerCompleteResponse{CredentialID: event.CredentialID})
}

// ── authenticate ─────────────────────────────────────────────────────────

type authenticateBeginRequest struct {
	Email string `json:"email"`
}

type authenticateBeginResponse struct {
	StateID string `json:"stateId"`
	Options any    `json:"options"`
}

func (s *State) authenticateBegin(w http.ResponseWriter, r *http.Request) {
	var body authenticateBeginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	if body.Email == "" {
		httperror.Write(w, httperror.BadRequest("EMAIL_REQUIRED", "email is required"))
		return
	}

	// Resolve the real credentials for this email — but on any failure
	// (unknown email, federated user, inactive, no credentials) fall
	// through to the enumeration-defence path. TODO(webauthn-runtime):
	// replace synthetic state-id with deterministic-fake allowCredentials
	// keyed by HMAC(email) to match the Rust impl's response shape.
	p, _ := s.Principals.FindByEmail(r.Context(), body.Email)
	if p == nil || !p.Active {
		writeJSON(w, http.StatusOK, authenticateBeginResponse{StateID: newUUID(), Options: emptyChallenge()})
		return
	}
	creds, err := s.Service.Credentials().LibraryCredentialsByPrincipal(r.Context(), p.ID)
	if err != nil || len(creds) == 0 {
		writeJSON(w, http.StatusOK, authenticateBeginResponse{StateID: newUUID(), Options: emptyChallenge()})
		return
	}
	user := &webauthn.PrincipalUser{
		PrincipalID: p.ID,
		DisplayName: p.Name,
		Credentials: creds,
	}
	options, sessionData, err := s.Service.WebAuthn().BeginLogin(user)
	if err != nil {
		httperror.Write(w, usecase.Internal("WEBAUTHN", "begin login failed", err))
		return
	}
	stateID := newUUID()
	if err := s.Service.Ceremonies().StoreAuthentication(r.Context(), stateID, &p.ID, sessionData); err != nil {
		httperror.Write(w, usecase.Internal("REPO", "store ceremony failed", err))
		return
	}
	writeJSON(w, http.StatusOK, authenticateBeginResponse{StateID: stateID, Options: options})
}

type authenticateCompleteRequest struct {
	StateID    string          `json:"stateId"`
	Credential json.RawMessage `json:"credential"`
}

func (s *State) authenticateComplete(w http.ResponseWriter, r *http.Request) {
	var body authenticateCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		invalidCredentials(w)
		return
	}

	consumed, err := s.Service.Ceremonies().ConsumeAuthentication(r.Context(), body.StateID)
	if err != nil || consumed == nil || consumed.PrincipalID == nil {
		invalidCredentials(w)
		return
	}
	p, err := s.Principals.FindByID(r.Context(), *consumed.PrincipalID)
	if err != nil || p == nil || !p.Active {
		invalidCredentials(w)
		return
	}
	creds, err := s.Service.Credentials().LibraryCredentialsByPrincipal(r.Context(), p.ID)
	if err != nil || len(creds) == 0 {
		invalidCredentials(w)
		return
	}
	user := &webauthn.PrincipalUser{
		PrincipalID: p.ID,
		DisplayName: p.Name,
		Credentials: creds,
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(io.NopCloser(bytes.NewReader(body.Credential)))
	if err != nil {
		invalidCredentials(w)
		return
	}
	cred, err := s.Service.WebAuthn().ValidateLogin(user, consumed.Session, parsed)
	if err != nil {
		invalidCredentials(w)
		return
	}

	// Persist the updated counter/backup-state via the AuthenticateUseCase.
	ec := usecase.NewExecutionContext(p.ID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.AuthenticateUC,
		operations.AuthenticateCommand{StateID: body.StateID, UpdatedCredential: *cred}, ec)); err != nil {
		// The credential validated but persistence failed — still issue
		// the session (the counter update is a defence-in-depth, not a
		// gate). Log via the use-case layer.
	}

	if s.SessionWriter != nil {
		s.SessionWriter(w, r, p.ID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"principalId": p.ID})
}

// ── credentials list / delete ────────────────────────────────────────────

func (s *State) listCredentials(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil || ac.PrincipalID == "" {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	rows, err := s.Service.Credentials().FindByPrincipal(r.Context(), ac.PrincipalID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list credentials failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) deleteCredential(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil || ac.PrincipalID == "" {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.RevokeUC,
		operations.RevokeCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ──────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func invalidCredentials(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             "INVALID_CREDENTIALS",
		"error_description": "Invalid credentials.",
	})
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// UUID v4-ish; not RFC 4122 strict but unique-by-randomness.
	return base64.RawURLEncoding.EncodeToString(b)
}

// emptyChallenge returns a minimal "options" payload for the
// enumeration-defence path. The shape mirrors WebAuthn's
// PublicKeyCredentialRequestOptions so the browser SDK doesn't crash.
func emptyChallenge() map[string]any {
	chal := make([]byte, 32)
	_, _ = rand.Read(chal)
	return map[string]any{
		"publicKey": map[string]any{
			"challenge":        base64.RawURLEncoding.EncodeToString(chal),
			"timeout":          60000,
			"rpId":             "",
			"allowCredentials": []any{},
			"userVerification": "preferred",
		},
	}
}

// credentialDescriptorsFor produces the exclude-list for register/begin.
func credentialDescriptorsFor(creds []gowebauthn.Credential) []protocol.CredentialDescriptor {
	out := make([]protocol.CredentialDescriptor, 0, len(creds))
	for _, c := range creds {
		out = append(out, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: c.ID,
		})
	}
	return out
}
