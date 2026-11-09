package pages

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/kit"
)

// brandUpstream serves the two public endpoints the shell's branding reads.
func brandUpstream(theme, platform string) *fakeUpstream {
	up := &fakeUpstream{}
	up.reply("GET /api/public/login-theme", 200, theme)
	up.reply("GET /api/public/platform", 200, platform)
	return up
}

func brandRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/ui/users", nil)
}

// The shell's branding is the platform-wide theme's logo, as the SPA's
// sidebar shows it (logo URL, else SVG, else the bolt; the logo height,
// default 40), and the platform name.
func TestAppBrandReadsThePlatformTheme(t *testing.T) {
	t.Run("logo URL, height and name", func(t *testing.T) {
		up := brandUpstream(`{"brandName":"Acme","logoUrl":"https://cdn.acme.example/l.png","logoSvg":"<svg/>","logoHeight":56}`, `{"platformName":"Acme Ops"}`)
		got := (&AppBrand{Upstream: up}).Branding(brandRequest())
		assert.Equal(t, shell.Branding{PlatformName: "Acme Ops", Logo: kit.Brand{Name: "Acme", LogoURL: "https://cdn.acme.example/l.png", LogoHeight: 56}}, got)
		require.Len(t, up.callsTo("GET /api/public/login-theme"), 1)
		assert.Empty(t, up.callsTo("GET /api/public/login-theme")[0].query, "the platform-wide theme: no client")
	})
	t.Run("the SVG when there is no URL", func(t *testing.T) {
		svg := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="4" height="4"/></svg>`
		theme, err := json.Marshal(map[string]string{"logoSvg": svg})
		require.NoError(t, err)
		got := (&AppBrand{Upstream: brandUpstream(string(theme), `{}`)}).Branding(brandRequest())
		assert.Equal(t, kit.Brand{Name: "FlowCatalyst", LogoSVG: svg, LogoHeight: 40}, got.Logo)
	})
	t.Run("nothing configured: the bolt at the SPA's default height", func(t *testing.T) {
		got := (&AppBrand{Upstream: brandUpstream(`{}`, `{"platformName":"FlowCatalyst"}`)}).Branding(brandRequest())
		assert.Equal(t, shell.Branding{PlatformName: "FlowCatalyst", Logo: kit.Brand{Name: "FlowCatalyst", LogoHeight: 40}}, got)
	})
	t.Run("the endpoints failing, or no upstream: the default", func(t *testing.T) {
		want := shell.Branding{Logo: kit.Brand{Name: "FlowCatalyst", LogoHeight: 40}}
		assert.Equal(t, want, (&AppBrand{Upstream: &fakeUpstream{}}).Branding(brandRequest()))
		assert.Equal(t, want, (&AppBrand{}).Branding(brandRequest()))
	})
	t.Run("refused values fall back", func(t *testing.T) {
		got := (&AppBrand{Upstream: brandUpstream(
			`{"logoUrl":"javascript:alert(1)","logoHeight":-3}`, `{"platformName":"`+strings.Repeat("x", 201)+`"}`,
		)}).Branding(brandRequest())
		assert.Equal(t, shell.Branding{Logo: kit.Brand{Name: "FlowCatalyst", LogoHeight: 40}}, got)
		// An image data URL is a valid sign-in logo, but the kit refuses data
		// URLs: the shell shows the bolt.
		got = (&AppBrand{Upstream: brandUpstream(`{"logoUrl":"data:image/png;base64,iVBORw0KGgo="}`, `{}`)}).Branding(brandRequest())
		assert.Empty(t, got.Logo.LogoURL)
		assert.Empty(t, got.Logo.LogoSVG)
	})
}

// It is read once per TTL, again after Invalidate (a save) or once the TTL
// has passed.
func TestAppBrandIsKeptForItsTTL(t *testing.T) {
	up := brandUpstream(`{"brandName":"Acme"}`, `{"platformName":"Acme"}`)
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	b := &AppBrand{Upstream: up, Now: func() time.Time { return now }}
	reads := func() int { return len(up.callsTo("GET /api/public/login-theme")) }

	b.Branding(brandRequest())
	b.Branding(brandRequest())
	assert.Equal(t, 1, reads(), "fresh: no second read")

	b.Invalidate()
	b.Branding(brandRequest())
	assert.Equal(t, 2, reads(), "a save drops it")

	now = now.Add(appBrandTTL - time.Second)
	b.Branding(brandRequest())
	assert.Equal(t, 2, reads())
	now = now.Add(2 * time.Second)
	b.Branding(brandRequest())
	assert.Equal(t, 3, reads(), "the TTL passed")

	var nilBrand *AppBrand
	nilBrand.Invalidate() // a page without branding saves safely
}

// Saving the theme or the platform name drops the shell's branding, so the
// next page wears the change.
func TestSettingsSavesInvalidateTheShellBrand(t *testing.T) {
	up := brandUpstream(`{}`, `{}`)
	brand := &AppBrand{Upstream: up}
	reads := func() int { return len(up.callsTo("GET /api/public/login-theme")) }
	brand.Branding(brandRequest())
	require.Equal(t, 1, reads())

	th := newThemeHarness()
	rec := serveMounted(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/theme", form: themeForm()},
		&ThemeSettings{Config: th.cfg, Ops: th.ops, Shell: shellForTest(), Brand: brand})
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	brand.Branding(brandRequest())
	assert.Equal(t, 2, reads(), "after a theme save")

	nh := newNamesHarness("")
	rec = serveMounted(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/names", form: cfgSaveForm("platformName", "Acme")},
		&NamesSettings{Config: nh.cfg, Ops: nh.ops, Shell: shellForTest(), Brand: brand})
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	brand.Branding(brandRequest())
	assert.Equal(t, 3, reads(), "after a name save")
}

// The sign-in pages show the SPA's default logo (the bolt on a tile) when the
// theme has none, the footer in the SPA's colour, and accent buttons that
// darken on hover; the styles are fixed rules, the colour a validated value.
func TestSignInDefaultLogoAndAccentStates(t *testing.T) {
	body := siReq{target: "/ui/auth/login"}.do(signInRouter(siThemed(), false)).Body.String()
	assert.Contains(t, body, `width:72px;height:72px;background:rgba(255,255,255,.1);border-radius:16px`)
	assert.Contains(t, body, `<svg style="width:40px;height:40px;color:#fff"`)
	assert.Contains(t, body, `color:#627d98;font-size:14px`)
	assert.Contains(t, body, signinAccentClass)
	assert.Contains(t, body, "<style>"+signinStyles+"</style>")
	assert.NotContains(t, body, "<img", "no logo configured")

	up := &fakeUpstream{}
	up.reply("GET /api/public/login-theme", 200, `{"logoUrl":"https://cdn.acme.example/l.png"}`)
	body = siReq{target: "/ui/auth/login"}.do(signInRouter(up, false)).Body.String()
	assert.Contains(t, body, `<img src="https://cdn.acme.example/l.png" alt="Logo"`)
	assert.NotContains(t, body, "width:72px;height:72px", "a configured logo replaces the tile")
}

// themedUpstream serves a login theme for the platform and one for acme.
func themedUpstream() *fakeUpstream {
	up := &fakeUpstream{}
	up.on("GET /api/public/login-theme", func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("client") == "acme" {
			_, _ = w.Write([]byte(`{"brandName":"Acme Client","accentColor":"#ff0000","customCss":".acme{color:red}"}`))
			return
		}
		_, _ = w.Write([]byte(`{"brandName":"Platform <Co>","backgroundGradient":"linear-gradient(135deg, #111111 0%, #222222 100%)","accentColor":"#00aa00","customCss":"input[value^=a]{background:url(https://evil.example/a)}"}`))
	})
	return up
}

// The second-factor pages are steps of the SPA's login page, so they wear the
// login theme too, without the tenant's stylesheet (they show a TOTP key,
// recovery codes and challenge state).
func TestTwoFactorPagesWearTheLoginTheme(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	h.build()
	h.page.Upstream = themedUpstream()
	body := h.serve(t, nil, req{target: "/ui/auth/2fa", cookie: pendingCk("prn_ann")}).Body.String()
	mustContain(t, body, "Two-factor verification", "Platform &lt;Co&gt;", "linear-gradient(135deg, #111111 0%, #222222 100%)", "--login-accent:#00aa00")
	mustNotContain(t, body, "evil.example", "input[value^=a]")
}

// The passkey page is the login page's passkey button: the client the
// sign-in form carries brands it, with the tenant's stylesheet as on the
// login page.
func TestPasskeyPageWearsTheClientTheme(t *testing.T) {
	up := themedUpstream()
	rec := serveMounted(t, nil, mfaPost("/ui/auth/passkey", url.Values{"next": {"/ui/users"}, "email": {"ann@x.test"}, "client": {"acme"}}, nil), &AuthPasskeys{Upstream: up})
	body := rec.Body.String()
	mustContain(t, body, "Acme Client", "--login-accent:#ff0000", ".acme{color:red}", "fc-webauthn")
	calls := up.callsTo("GET /api/public/login-theme")
	require.Len(t, calls, 1)
	assert.Equal(t, "client=acme", calls[0].query)
}
