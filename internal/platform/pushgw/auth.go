package pushgw

import (
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/httpfw"
	"github.com/flowcatalyst/framework/go/push"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldauth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
)

// PermAllClients lets a platform credential listen to every client's pushes: a stream for the
// platform client (_platform). It is never implied (spec 4, ruling 5). Four segments like every
// platform permission, so platform:*:*:* (super-admin) holds it.
const PermAllClients = "platform:push:stream:all-clients"

// Codes the authenticator answers besides the module's.
const (
	codeClientForbidden = "CLIENT_FORBIDDEN"
	codeInvalidToken    = "invalid_token"
	codeInvalidProof    = "invalid_dpop_proof"
)

// Authenticator maps the platform's credentials to the push module's push.Credentials (spec 5):
//
//   - a bearer access token, or the session cookie with the console's CSRF rule, for platform users
//     and service accounts;
//   - a field access token (Authorization: DPoP) with its proof, for field workers.
//
// A request with none of them is not refused: it may still subscribe with tickets. Bad credentials
// are 401 even when good tickets come with them. A stale session cookie counts as none, as it does
// for the platform's own authenticator (the browser replays it everywhere).
type Authenticator struct {
	// Provider validates bearer and session tokens and resolves a session's claims.
	Provider *provider.Provider
	// Auth validates field access tokens.
	Auth *authservice.AuthService
	// Field answers whether a field token's login family, worker and device are still live.
	Field *fieldidentity.Ops
	// CSRF is the console's cookie-session rule (middleware.SessionCSRFChecker).
	CSRF func(*http.Request) *platformmw.CSRFRefusal
	// BaseURL is the platform's public base URL: a field proof's htu is BaseURL plus the request
	// path, never the Host header (as for the field endpoints).
	BaseURL string
	// Replay refuses a field DPoP proof presented twice (spec section 5). Nil disables the check;
	// the server always wires one (a [PGReplayGuard], shared by every gateway task).
	Replay ReplayGuard
	// Now is the clock (default time.Now).
	Now func() time.Time
}

var _ push.ClientAuthenticator = (*Authenticator)(nil)

// Authenticate implements push.Authenticator: a request naming no client.
func (a *Authenticator) Authenticate(r *http.Request) (push.Credentials, bool, error) {
	return a.AuthenticateClient(r, "")
}

// AuthenticateClient implements push.ClientAuthenticator. client is the subscribe body's client
// member, "" when absent.
func (a *Authenticator) AuthenticateClient(r *http.Request, client string) (push.Credentials, bool, error) {
	if scheme, token := authorizationScheme(r); strings.EqualFold(scheme, "DPoP") {
		creds, err := a.field(r, token, client)
		return creds, true, err
	}
	cred, present, err := platformmw.AuthenticateRequest(r.Context(), a.Provider, r)
	if err != nil {
		return push.Credentials{}, true, httpfw.Unauthenticated("the bearer token is invalid or expired")
	}
	if !present {
		return push.Credentials{}, false, nil
	}
	if cred.FromCookie && a.CSRF != nil {
		if ref := a.CSRF(r); ref != nil {
			return push.Credentials{}, true, httpfw.NewError(ref.Status, ref.Code, ref.Message)
		}
	}
	creds, err := a.platform(cred, client)
	return creds, true, err
}

// authorizationScheme splits the Authorization header into scheme and credentials.
func authorizationScheme(r *http.Request) (scheme, token string) {
	scheme, token, _ = strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	return scheme, strings.TrimSpace(token)
}

// platform maps a user's or service account's credential to push.Credentials.
func (a *Authenticator) platform(cred platformmw.Credential, named string) (push.Credentials, error) {
	ac := cred.Auth
	client, err := chooseClient(ac, named)
	if err != nil {
		return push.Credentials{}, err
	}
	cid, err := fw.ParseClientID(client)
	if err != nil {
		return push.Credentials{}, httpfw.Forbidden(codeClientForbidden, "the client is not a usable client id")
	}
	pid, err := fw.ParsePrincipalID(ac.PrincipalID)
	if err != nil {
		return push.Credentials{}, httpfw.Unauthenticated("the credential names no principal")
	}
	kind := fw.PrincipalUser
	if ac.PrincipalType == auth.PrincipalTypeService {
		kind = fw.PrincipalService
	}
	cx := fw.ContextFor(cid).WithPrincipalOfKind(pid, kind)
	perms := make([]fw.Permission, 0, len(ac.Permissions))
	for _, p := range ac.Permissions {
		if perm, err := fw.ParsePermission(p); err == nil {
			perms = append(perms, perm)
		}
	}
	// a platform credential is for every application's rules (AppAccessPrefix)
	perms = append(perms, fw.MustPermission(AppAccessPermission("*")))
	cx = cx.WithPermissions(perms...)
	return push.Credentials{Context: cx, ExpiresAt: cred.ExpiresAt}, nil
}

// chooseClient is the stream's client for a platform credential (spec 4). A named client must be
// one the credential can access (AuthContext.CanAccessClient), else 403 CLIENT_FORBIDDEN. The
// platform client (_platform, a stream that hears every client) needs PermAllClients. Without
// one named, the credential's only client is used; a credential with several (or an anchor,
// which reaches all) is 400 VALIDATION, and no stream is ever platform-wide by default.
func chooseClient(ac *auth.AuthContext, named string) (string, error) {
	switch {
	case named == fw.PlatformClient.String():
		if !ac.HasPermission(PermAllClients) {
			return "", httpfw.Forbidden(codeClientForbidden, "a platform-wide stream needs the permission "+PermAllClients)
		}
		return named, nil
	case named != "":
		if !ac.CanAccessClient(named) {
			return "", httpfw.Forbidden(codeClientForbidden, "the credential cannot use that client")
		}
		return named, nil
	}
	var own []string
	for _, c := range ac.Clients {
		if c != "*" && c != "" {
			own = append(own, c)
		}
	}
	switch {
	case ac.IsAnchor() || len(own) > 1:
		return "", httpfw.Validation("the credential can act for several clients; name one in client")
	case len(own) == 0:
		return "", httpfw.Forbidden(codeClientForbidden, "the credential has no client")
	}
	return own[0], nil
}

// field maps a field access token and its DPoP proof to push.Credentials: the proof's signature,
// method, URL, access-token hash and key (the token's cnf.jkt), then the live state of the login
// family, worker and device. The proof key, device and login family go to the module for tickets
// bound to a key and for revocation.
func (a *Authenticator) field(r *http.Request, token, named string) (push.Credentials, error) {
	now := a.now()
	invalidToken := func(msg string) error {
		return httpfw.NewError(http.StatusUnauthorized, codeInvalidToken, msg)
	}
	invalidProof := func(msg string) error {
		return httpfw.NewError(http.StatusUnauthorized, codeInvalidProof, msg)
	}
	if token == "" {
		return push.Credentials{}, invalidToken("an Authorization: DPoP access token is required")
	}
	header := r.Header.Get("DPoP")
	if header == "" {
		return push.Credentials{}, invalidProof("the DPoP header is missing")
	}
	proof, err := fieldauth.ParseProof(header, now)
	if err != nil {
		return push.Credentials{}, invalidProof(strings.TrimPrefix(err.Error(), fieldauth.ErrProof.Error()+": "))
	}
	if proof.HTM != r.Method {
		return push.Credentials{}, invalidProof("htm does not match the request method")
	}
	if proof.HTU != strings.TrimRight(a.BaseURL, "/")+r.URL.Path {
		return push.Credentials{}, invalidProof("htu does not match the request URL")
	}
	claims, err := fieldauth.TokenVerifier{Auth: a.Auth}.VerifyFieldAccessToken(r.Context(), token)
	if err != nil {
		return push.Credentials{}, invalidToken("the access token is invalid or expired")
	}
	// the token is bound to the key the proof is signed with, and the proof names this token
	if claims.JKT == "" || claims.JKT != proof.JKT {
		return push.Credentials{}, invalidToken("the access token is not bound to this key")
	}
	if proof.ATH != fieldauth.AccessTokenHash(token) {
		return push.Credentials{}, invalidProof("ath does not match the access token")
	}
	live, err := a.Field.TokenActive(r.Context(), claims, now)
	if err != nil {
		return push.Credentials{}, err
	}
	if !live {
		return push.Credentials{}, invalidToken("the session has ended")
	}
	if named != "" && named != claims.ClientID {
		return push.Credentials{}, httpfw.Forbidden(codeClientForbidden, "the credential cannot use that client")
	}
	cid, err := fw.ParseClientID(claims.ClientID)
	if err != nil {
		return push.Credentials{}, invalidToken("the access token names no usable client")
	}
	pid, err := fw.ParsePrincipalID(claims.Subject)
	if err != nil {
		return push.Credentials{}, invalidToken("the access token names no principal")
	}
	// the worker's rules grant only the application the token was issued for (AppAccessPrefix);
	// fieldScope narrows the tickets the same way
	app, err := fw.ParsePermission(AppAccessPermission(claims.AppCode))
	if err != nil || claims.AppCode == "" {
		return push.Credentials{}, invalidToken("the access token names no application")
	}
	// a proof is accepted once: a replayed one is refused, after everything else checked out so
	// that only a valid proof takes a place in the store
	if a.Replay != nil {
		until := time.Unix(proof.IAT, 0).Add(fieldauth.ProofClockSkew + replayMargin)
		replayed, err := a.Replay.Seen(r.Context(), proof.JKT, proof.JTI, until, now)
		if err != nil {
			return push.Credentials{}, err
		}
		if replayed {
			return push.Credentials{}, invalidProof("the proof was already used; sign a new one")
		}
	}
	cx := fw.ContextFor(cid).WithPrincipalOfKind(pid, fw.PrincipalFieldWorker).WithPermissions(app)
	return push.Credentials{
		Context: cx, ExpiresAt: claims.Expires,
		ProofKey: proof.JKT, Device: claims.DeviceID, Session: claims.SessionID,
	}, nil
}

func (a *Authenticator) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}
