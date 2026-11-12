package pages

import (
	"net/http"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// The application filter and the create form's application select offer only
// the applications the caller reaches. Listing every application to a caller
// confined to some leaked the others' names and codes.
func TestFunctionApplicationChoicesFollowReach(t *testing.T) {
	const (
		appSvcView = "platform:application-service:application:view"
		adminView  = "platform:admin:application:view"
		billing    = "Billing (billing)"
		acme       = "Acme &lt;app&gt; (acme)"
	)
	confined := fnsUser(fnPermView, fnPermManage, appSvcView)
	confined.AllApplications = false // fnsUser is all-applications by default
	confined.Applications = []string{"app_acme"}
	all := fnsUser(fnPermView, fnPermManage, appSvcView)
	all.AllApplications = true
	admin := fnsUser(fnPermView, fnPermManage, adminView)
	admin.AllApplications = false // the admin view alone, not application reach, shows every application

	for name, c := range map[string]struct {
		ac         *auth.AuthContext
		want, hide []string
	}{
		"confined to one application": {confined, []string{acme}, []string{billing}},
		"all applications":            {all, []string{acme, billing}, nil},
		"admin application view":      {admin, []string{acme, billing}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			h := fnsHarnessNew()
			for _, target := range []string{"/ui/functions", "/ui/functions/new"} {
				body := h.do(t, c.ac, req{target: target}).Body.String()
				mustContain(t, body, c.want...)
				mustNotContain(t, body, c.hide...)
			}
		})
	}
}

// The drawer of a function of an application the caller does not reach is
// refused, as the API's by-id read is; its own application's opens.
func TestFunctionDrawerFollowsApplicationReach(t *testing.T) {
	ac := fnsUser(fnPermView)
	ac.AllApplications = false
	ac.Applications = []string{"app_acme"}
	h := fnsHarnessNew()

	other := h.do(t, ac, req{target: "/ui/functions/fn_1", datastar: true})
	// A datastar request answers 200 with the refusal patched in; the drawer
	// itself must not open.
	mustNotContain(t, other.Body.String(), "billing.invoice-pdf", `"selFn":"fn_1"`)

	own := h.do(t, ac, req{target: "/ui/functions/fn_2", datastar: true})
	if own.Code != http.StatusOK {
		t.Errorf("its own application's function: status %d, want 200", own.Code)
	}
	mustContain(t, own.Body.String(), "acme.orders")
}
