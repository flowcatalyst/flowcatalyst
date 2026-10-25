package pages

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// A caller who may add or remove origins but not view them gets the flash
// after the change, never the re-rendered table (the other origins).
func TestCorsOriginsChangeWithoutViewShowsNoTable(t *testing.T) {
	who := func(perm string) *auth.AuthContext {
		return &auth.AuthContext{PrincipalID: "prn_9", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{perm}, PrincipalType: auth.PrincipalTypeUser}
	}
	h := newCorsHarness()
	added := h.rec(t, who(corsCreatePermTest), req{
		method: http.MethodPost, target: "/ui/cors-origins/new", datastar: true, multipart: true,
		form: url.Values{"origin": {"https://new.example.com"}}, referer: "/ui/cors-origins",
	}).Body.String()
	mustContain(t, added, "https://new.example.com")
	mustNotContain(t, added, "http://localhost:3000", "Production &lt;SPA&gt;")

	removed := h.rec(t, who(corsDeletePermTest), req{
		method: http.MethodPost, target: "/ui/cors-origins/cor_1/delete", datastar: true, multipart: true,
		referer: "/ui/cors-origins",
	}).Body.String()
	mustContain(t, removed, "removed")
	mustNotContain(t, removed, "http://localhost:3000")

	// With the view permission the table comes back.
	h = newCorsHarness()
	viewer := h.rec(t, who(corsViewPermTest), req{target: "/ui/cors-origins"}).Body.String()
	mustContain(t, viewer, "http://localhost:3000")
}
