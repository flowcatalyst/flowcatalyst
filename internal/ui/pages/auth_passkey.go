package pages

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	wa "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/operations"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// Passkeys. The sign-in page (/ui/auth/passkey) and the "add a passkey" form of
// the security page run the browser's ceremony with fc-webauthn.js, which talks
// to the platform's WebAuthn endpoints (/auth/webauthn/*): they verify the
// attestation or assertion, apply the sign-in backoff, record the attempt and
// set the session cookie, exactly as for the current UI. A passkey sign-in
// does not meet a second factor (it is one), as on those endpoints. This file
// holds the page and the list and removal the security page does itself.

// passkeyCredential is a stored passkey (its raw credential never leaves the
// repository: the pages read the name and the dates).
type passkeyCredential = wa.Credential

// PasskeyStore lists a user's passkeys (GET /auth/webauthn/credentials).
type PasskeyStore interface {
	FindByPrincipal(ctx context.Context, principalID string) ([]passkeyCredential, error)
}

// PasskeyCommands revokes a passkey whose owner the caller has checked.
type PasskeyCommands interface {
	Revoke(ctx context.Context, id string) error
}

// PasskeyOps runs the revoke use case through the unit of work.
type PasskeyOps struct {
	Repo *wa.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Revoke deletes the credential (DELETE /auth/webauthn/credentials/{id}).
func (o PasskeyOps) Revoke(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.Revoke(o.Repo), operations.RevokeCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

const (
	authPasskeyPath = "/ui/auth/passkey"
	webauthnScript  = "fc-webauthn.js"
)

// AuthPasskeys is the "Sign in with a passkey" page.
type AuthPasskeys struct{}

// Mount registers the page; mount it in a group without the session guard.
func (h *AuthPasskeys) Mount(r chi.Router) {
	r.Get("/auth/passkey", h.page)
	// The sign-in page's "Sign in with a passkey" posts the typed address, so
	// it is in no URL.
	r.With(parseForm).Post("/auth/passkey", h.page)
}

type passkeySignInView struct {
	Next string
	// Email prefills the field (the sign-in page passes what was typed).
	Email string
}

func (h *AuthPasskeys) page(w http.ResponseWriter, r *http.Request) {
	v := passkeySignInView{Next: authNext(r.FormValue("next")), Email: strings.TrimSpace(r.FormValue("email"))}
	if len(v.Email) > 320 {
		v.Email = ""
	}
	renderAuthPage(w, r, http.StatusOK, "Sign in with a passkey", []string{webauthnScript}, passkeySignIn(v))
}
