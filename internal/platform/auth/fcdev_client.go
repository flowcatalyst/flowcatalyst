package auth

import "time"

// The built-in first-party OAuth client behind `fcdev login`: a PUBLIC
// client (no secret) that lets a person sign in to a remote platform from the
// CLI with the normal browser login and then act as themselves.
//
// It is seeded at every start (internal/platform/seed) and a few of its
// rules live in code rather than in the editable row, because they are
// security properties of THIS client and must not drift with console edits:
//
//   - redirect matching: the registered URIs carry no port, and only for this
//     client_id does /oauth/authorize accept any loopback port on them
//     (oauthapi.MatchLoopbackRedirectURI, RFC 8252 section 7.3).
//   - refresh lifetime: FcdevCLIRefreshTTL, an ABSOLUTE cap per family
//     (rotation never extends it), independent of OIDC_REFRESH_TOKEN_TTL.
const FcdevCLIClientID = "fcdev-cli"

// FcdevCLIClientName is the display name of the seeded client.
const FcdevCLIClientName = "fcdev CLI (browser login)"

// FcdevCLIRedirectURIs are the registered loopback redirects. They are
// port-less on purpose; the port is chosen by fcdev per login.
var FcdevCLIRedirectURIs = []string{
	"http://127.0.0.1/callback",
	"http://[::1]/callback",
}

// FcdevCLIGrantTypes are the only grants the client may use.
var FcdevCLIGrantTypes = []string{"authorization_code", "refresh_token"}

// DefaultFcdevCLIRefreshTTL is how long an fcdev login lasts before the person
// signs in again, counted from the original sign-in (absolute, not sliding).
// Configurable with FC_FCDEV_LOGIN_REFRESH_TTL_SECS.
const DefaultFcdevCLIRefreshTTL = 7 * 24 * time.Hour
