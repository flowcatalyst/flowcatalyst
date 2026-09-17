package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// OidcLogin is the audit-only command backing the UserLoggedIn event-only
// plan (usecaseop.Emit): the login itself already succeeded — the session is
// about to be established — by the time this runs, so there is no aggregate
// to write, only the event + its audit row.
//
// platformsink.WriteAudit uses this struct's own Go type name (via
// reflect.TypeOf) as the aud_logs `operation` column, so it is deliberately
// named without a "Command" suffix — docs/spec/oidc-logged-in-event.md calls
// for the literal operation name "OidcLogin".
type OidcLogin struct {
	Email              string `json:"email"`
	IdentityProviderID string `json:"identityProviderId"`
}

// EmitOidcLogin wraps an already-built UserLoggedIn event (constructed by the
// OIDC bridge callback, which alone has the verified id-token claims, the
// decoded access-token claims, and the post-role-sync principal) as an
// event-only operation. Authorize is Public: the caller just completed a
// full OIDC handshake: there is no further resource-level check an operation
// could usefully add on top of that.
func EmitOidcLogin(event UserLoggedIn) usecaseop.Operation[OidcLogin, UserLoggedIn] {
	return usecaseop.Operation[OidcLogin, UserLoggedIn]{
		Name:      "OidcLogin",
		Authorize: usecaseop.Public[OidcLogin],
		Execute: func(_ context.Context, _ OidcLogin, _ usecase.ExecutionContext) (usecaseop.Plan[UserLoggedIn], error) {
			return usecaseop.Emit(event), nil
		},
	}
}
