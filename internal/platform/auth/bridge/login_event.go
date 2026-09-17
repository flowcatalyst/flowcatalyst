package bridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// emitUserLoggedIn emits platform:iam:user:logged-in for a successful OIDC
// login (docs/spec/oidc-logged-in-event.md). Best-effort: any failure here is
// logged at WARN and swallowed — the login has already succeeded (p is
// resolved, every trust check passed) and its response must never change
// because of this.
//
// rolesSynced marks whether the caller ran syncIdpRoles for this login
// (mapping-based logins only — provider-direct/portal logins never sync
// platform roles). When it did, p's in-memory Roles predate that write, so
// the principal is re-read here: the Rust reference's sync call returns the
// already-synced principal directly, but Go's IDP role sync is a separate
// usecaseop.Run, so this is the equivalent re-read the spec calls for
// ("re-read the principal; Rust uses the synced principal").
func (e *LoginEndpoint) emitUserLoggedIn(
	ctx context.Context,
	p *principal.Principal,
	rolesSynced bool,
	email string,
	idp *identityprovider.IdentityProvider,
	idToken *oidc.IDToken,
	tok *oauth2.Token,
) {
	warn := func(err error) {
		slog.Warn("failed to emit UserLoggedIn event (login still succeeded)",
			"principal", p.ID, "err", err)
	}

	current := p
	if rolesSynced {
		if fresh, err := e.principals.FindByID(ctx, p.ID); err == nil && fresh != nil {
			current = fresh
		}
		// A failed re-read falls back to the pre-sync principal rather than
		// aborting the emit entirely — a momentarily-stale roles list beats no
		// event at all for a best-effort emission.
	}

	roles := make([]string, 0, len(current.Roles))
	for _, ra := range current.Roles {
		roles = append(roles, ra.Role)
	}

	var clients []string
	if current.Scope.IsAnchor() {
		clients = []string{"*"}
	} else {
		clients = current.AssignedClients
		if clients == nil {
			clients = []string{}
		}
	}

	var idTokenClaims map[string]any
	if err := idToken.Claims(&idTokenClaims); err != nil {
		warn(err)
		return
	}
	if idTokenClaims == nil {
		idTokenClaims = map[string]any{}
	}
	// Strip OIDC protocol artefacts — never platform business data — before
	// they land in the event (docs/spec/oidc-logged-in-event.md).
	delete(idTokenClaims, "nonce")
	delete(idTokenClaims, "at_hash")
	delete(idTokenClaims, "c_hash")

	var idpCode *string
	if idp != nil {
		c := idp.Code
		idpCode = &c
	}

	ec := usecase.NewExecutionContext(p.ID)
	event := principalops.UserLoggedIn{
		Metadata:             usecase.NewEventMetadata(ec, principalops.UserLoggedInType, principalops.Source, principalops.UserLoggedInSubject(p.ID)),
		UserID:               p.ID,
		Email:                email,
		LoginMethod:          "OIDC",
		IdentityProviderCode: idpCode,
		FlowcatalystClaims: principalops.FlowcatalystClaims{
			Email:        email,
			Type:         "USER",
			Roles:        roles,
			Clients:      clients,
			Applications: applicationPrefixes(roles),
		},
		FederatedClaims: &principalops.FederatedClaims{
			IDToken:     idTokenClaims,
			AccessToken: decodeJWTPayloadUnverified(tok.AccessToken),
		},
	}

	cmd := principalops.OidcLogin{Email: email}
	if idp != nil {
		cmd.IdentityProviderID = idp.ID
	}
	if _, err := usecaseop.Run(ctx, e.uow, principalops.EmitOidcLogin(event), cmd, ec); err != nil {
		warn(err)
	}
}

// applicationPrefixes returns the distinct, sorted set of prefixes before the
// first ':' in each role name, skipping any role with no prefix (or an empty
// one, e.g. a role name starting with ':') — "ondemand:admin" contributes
// "ondemand".
func applicationPrefixes(roles []string) []string {
	set := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		if idx := strings.Index(r, ":"); idx > 0 {
			set[r[:idx]] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// decodeJWTPayloadUnverified extracts a JWT's middle segment (the payload)
// as a generic claim map WITHOUT verifying its signature — used only for the
// OIDC access token, whose signature this platform never checks (the code
// exchange already trusts the IdP that returned it, over TLS). A non-JWT
// (opaque) access token, or one whose payload isn't a JSON object, decodes to
// an empty map rather than failing.
func decodeJWTPayloadUnverified(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return map[string]any{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return map[string]any{}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil || claims == nil {
		return map[string]any{}
	}
	return claims
}
