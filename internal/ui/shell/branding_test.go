package shell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowcatalyst/framework/go/ui/kit"
)

const baseCSP = "default-src 'self'; img-src 'self' data:; font-src 'self'"

func renderBranded(t *testing.T, b func(*http.Request) Branding) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Security-Policy", baseCSP)
	(&Shell{Branding: b}).Render(rec, request(false), Page{Title: "Users", Path: "/ui/users"}, kit.EmptyState(kit.EmptyStateProps{Message: "x"}))
	return rec
}

// Without a resolver the shell is the FlowCatalyst one: the bolt at the kit's
// size, the default name in the title, the page policy untouched.
func TestShellWithoutBrandingIsTheDefault(t *testing.T) {
	rec := renderBranded(t, nil)
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Users · FlowCatalyst</title>",
		`class="fc-sidebar-logo" style="height: 32px"><span style="width: 32px; height: 32px"><svg`,
		`aria-label="FlowCatalyst"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != baseCSP {
		t.Errorf("CSP changed: %q", csp)
	}
}

// The theme's logo URL and height reach the sidebar, the platform name the
// title, and the policy admits the logo's origin and nothing more.
func TestShellWearsTheThemeLogoAndName(t *testing.T) {
	rec := renderBranded(t, func(*http.Request) Branding {
		return Branding{PlatformName: "Acme <Ops>", Logo: kit.Brand{Name: "Acme", LogoURL: "https://cdn.acme.example/logo.png", LogoHeight: 48}}
	})
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Users · Acme &lt;Ops&gt;</title>",
		`<a href="/ui/" class="fc-sidebar-logo" style="height: 48px"><img src="https://cdn.acme.example/logo.png" alt="Acme" style="height: auto; max-height: 48px"></a>`,
		`class="fc-topbar-logo fc-topbar-logo-image"><img src="https://cdn.acme.example/logo.png" alt="Acme">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "default-src 'self'; img-src 'self' data: https://cdn.acme.example; font-src 'self'" {
		t.Errorf("CSP = %q", csp)
	}
}

// An SVG logo is an image data URL (already admitted by img-src data:), never
// inline markup.
func TestShellSVGLogoIsAnImage(t *testing.T) {
	rec := renderBranded(t, func(*http.Request) Branding {
		return Branding{Logo: kit.Brand{LogoSVG: `<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`, LogoHeight: 40}}
	})
	body := rec.Body.String()
	if !strings.Contains(body, `class="fc-sidebar-logo" style="height: 40px"><img src="data:image/svg+xml;base64,`) {
		t.Errorf("no SVG image logo:\n%s", body)
	}
	if strings.Contains(body, "<circle") {
		t.Error("the SVG was written inline")
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != baseCSP {
		t.Errorf("CSP changed for a data URL logo: %q", csp)
	}
}

// Only a plain https origin with a host of letters, digits, dots and hyphens
// is written into the policy: nothing a logo URL holds can add a directive.
func TestLogoOrigin(t *testing.T) {
	for raw, want := range map[string]string{
		"https://cdn.acme.example/a/logo.png?v=1": "https://cdn.acme.example",
		"https://CDN.Acme.example:8443/l.svg":     "https://cdn.acme.example:8443",
		"/assets/logo.png":                        "",
		"http://cdn.acme.example/logo.png":        "",
		"data:image/png;base64,AAAA":              "",
		"https://evil.example;script-src *":       "",
		"https://a.example'/x":                    "",
		"https://user@cdn.acme.example/x":         "",
		"https://cdn acme.example/x":              "",
		"https:///nohost":                         "",
	} {
		if got := logoOrigin(raw); got != want {
			t.Errorf("logoOrigin(%q) = %q, want %q", raw, got, want)
		}
	}
}

// An error page before a session (role-less) carries the platform name too.
func TestBareDocumentTitleUsesThePlatformName(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/x", nil)
	r.Header.Set("Accept", "text/html")
	(&Shell{Branding: func(*http.Request) Branding { return Branding{PlatformName: "Acme"} }}).Deny(rec, r, http.StatusForbidden, "no")
	if !strings.Contains(rec.Body.String(), "<title>Access denied · Acme</title>") {
		t.Errorf("title:\n%s", rec.Body.String())
	}
}
