package server

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// oauthRouteRegistrars are the OAuth/OIDC provider surface's route-registration
// methods on oauthapi.State. Every one of them must be mounted OUTSIDE the
// bearer-token middleware.
var oauthRouteRegistrars = []string{
	"RegisterAuthorizeRoutes",
	"RegisterTokenRoutes",
	"RegisterIntrospectRoutes",
	"RegisterRevokeRoutes",
	"RegisterUserinfoRoutes",
	"RegisterDiscoveryRoutes",
}

// TestOAuthProviderRoutesMountedOutsideAuthGroup guards the invariant that the
// OAuth/OIDC endpoints are not mounted behind platformmw.Authenticator.
//
// The middleware is not a gate — a credential-less request passes straight
// through — but it DOES hard-fail an explicit Authorization: Bearer it won't
// accept, before the handler runs. Two consequences make the auth group the
// wrong home for these routes:
//
//   - /oauth/userinfo is invoked WITH the access token as a bearer, and an
//     ordinary (non-APIAccess) OIDC client holds a token_use=identity token,
//     which the middleware rejects outright. Mounting it inside the group 401s
//     the canonical authorization_code → access_token → userinfo flow.
//   - /oauth/token, /.well-known/*, introspect and revoke get spurious 401s
//     from any caller that leaves a stale bearer attached (e.g. a
//     refresh_token call carrying the expired access token).
//
// Each endpoint authenticates its own caller, or is public by design, so the
// middleware adds no protection to offset that. Asserting on source placement
// rather than behaviour keeps the guard cheap: registerPlatformAPI needs a live
// pgxpool to run.
func TestOAuthProviderRoutesMountedOutsideAuthGroup(t *testing.T) {
	authGroupSrc := readServerFile(t, "wire_routes.go")
	publicSrc := readServerFile(t, "wire_public.go")

	var misplaced, missing []string
	for _, name := range oauthRouteRegistrars {
		re := regexp.MustCompile(`\.` + regexp.QuoteMeta(name) + `\(`)
		if re.Match(authGroupSrc) {
			misplaced = append(misplaced, name)
		}
		if !re.Match(publicSrc) {
			missing = append(missing, name)
		}
	}
	sort.Strings(misplaced)
	sort.Strings(missing)

	if len(misplaced) > 0 {
		t.Errorf("these OAuth routes are registered in wire_routes.go, inside the Authenticator group: %v\n"+
			"Mount them in registerPublicRoutes (wire_public.go) instead — the middleware 401s the bearer "+
			"shapes these endpoints are required to accept.", misplaced)
	}
	// A registrar that appears in neither file has been renamed or dropped;
	// without this the misplacement check above would pass vacuously.
	if len(missing) > 0 {
		t.Errorf("these OAuth routes are not registered in wire_public.go: %v\n"+
			"Either they moved (fix the wiring) or the method was renamed (update oauthRouteRegistrars).", missing)
	}
}

func readServerFile(t *testing.T, name string) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(".", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return src
}
