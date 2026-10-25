package pages

import (
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
	confined.Applications = []string{"app_acme"}
	all := fnsUser(fnPermView, fnPermManage, appSvcView)
	all.AllApplications = true
	admin := fnsUser(fnPermView, fnPermManage, adminView)

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
