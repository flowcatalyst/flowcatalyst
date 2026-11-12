package pages

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// operatorAdmin is an anchor administrator with all-applications access: an
// ordinary platform operator. (anchorAdmin is anchor but not all-applications,
// which is how an application's own service account looks.)
func operatorAdmin() *auth.AuthContext {
	ac := anchorAdmin()
	ac.AllApplications = true
	return ac
}

// The sign action returns a real signature of the job's body, so it is an
// anchor-tier action in the console as in the API (T7).
func TestDispatchJobSignRequiresAnchor(t *testing.T) {
	h := newDJHarness()
	clientTier := &auth.AuthContext{
		PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_2"}, AllApplications: true,
		Roles: []string{"r"}, Permissions: []string{djViewPerm, djRawPerm}, PrincipalType: auth.PrincipalTypeUser,
	}
	rec := h.rec(t, clientTier, req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_3/sign", datastar: true, form: url.Values{}})
	// A datastar request is answered with a 200 stream carrying the refusal.
	mustContain(t, rec.Body.String(), "anchor scope required to sign a delivery")
	if len(h.ops.planned) != 0 {
		t.Errorf("a delivery was planned for a client-tier caller: %v", h.ops.planned)
	}
	// Nor is the action offered to them.
	drawer := h.do(t, clientTier, req{target: "/ui/dispatch-jobs/djb_3", datastar: true})
	mustNotContain(t, drawer, "/sign")

	// An operator still signs.
	rec = h.rec(t, operatorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_3/sign", datastar: true, form: url.Values{}})
	if rec.Code != http.StatusOK || len(h.ops.planned) != 1 {
		t.Errorf("operator sign: %d, planned %v", rec.Code, h.ops.planned)
	}
}
