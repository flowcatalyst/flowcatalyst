package oauthapi

import (
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// portalSubjectPrefix identifies portal-identity subjects on authorization
// codes (the ptu_ TSID prefix).
var portalSubjectPrefix = tsid.PortalUser.Prefix() + "_"

// redeemPortalCode completes the authorization_code grant for a PORTAL-plane
// subject. The identity must still exist and be ACTIVE (a suspension or
// offboarding between code issuance and redemption bites here). Token
// shapes: identity-only access token (authority-free, as every interactive
// login), id_token minted from the portal identity with an EMPTY roles claim
// (portal roles are portal-side data), and never a refresh token.
func (s *State) redeemPortalCode(w http.ResponseWriter, r *http.Request, code *grantstore.AuthorizationCode, client *auth.OAuthClient) {
	if s.PortalIdentities == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Portal subjects are not supported")
		return
	}
	ident, err := s.PortalIdentities.FindByID(r.Context(), code.PrincipalID)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	if ident == nil || ident.Status != portalidentity.StatusActive {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Portal identity not found or suspended")
		return
	}

	// A transient principal-shaped view of the identity: the token
	// generators only read ID/Name/email/roles, and this synthetic value
	// never touches the principal store. sub = the ptu_ id.
	synth := &principal.Principal{
		ID:   ident.ID,
		Type: principal.TypeUser,
		Name: ident.Name,
		UserIdentity: &principal.UserIdentity{
			Email: ident.Email,
		},
	}
	accessToken, err := s.Auth.GenerateIdentityAccessToken(synth)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	scope := ""
	if code.Scope != nil {
		scope = *code.Scope
	}
	var idToken *string
	if scopeHas(scope, "openid") {
		t, terr := s.Auth.GenerateIDTokenWithRoles(synth, code.ClientID, code.Nonce, []string{}, code.AuthTime)
		if terr != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "")
			return
		}
		idToken = &t
	}
	_ = client // client identity was already bound to the code by the caller

	writeToken(w, tokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   s.Auth.AccessTokenTTLSecs(),
		IDToken:     idToken,
		Scope:       code.Scope,
	})
}
