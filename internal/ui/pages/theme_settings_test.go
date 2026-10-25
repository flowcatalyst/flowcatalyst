package pages

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
)

const (
	themeGlobalJSON   = `{"brandName":"Acme <Portal>","brandSubtitle":"Ops","primaryColor":"#112233","accentColor":"#0967d2","backgroundColor":"#0a1929","backgroundGradient":"linear-gradient(135deg, #112233 0%, #0a1929 100%)","footerText":"Acme footer","logoHeight":48,"customCss":".login-card{display:none}"}`
	themeOverrideJSON = `{"brandName":"Acme Client","accentColor":"#ff0000","logoUrl":null}`
)

// themeHarness serves the theme pages over the fake config store holding a
// platform-wide theme and an override for the client acme.
type themeHarness struct {
	cfg     *fakeConfig
	ops     *fakeConfigOps
	clients *fakeClientStore
}

func newThemeHarness() *themeHarness {
	h := &themeHarness{cfg: newFakeConfig(), ops: &fakeConfigOps{}, clients: &fakeClientStore{clients: []client.Client{cli("clt_acme", "acme", "Acme Corp", client.StatusActive)}}}
	global := cfgRowFixture("pfc_10", "login", "theme", platformconfig.ValuePlain, themeGlobalJSON)
	override := cfgRowFixture("pfc_11", "login", "theme", platformconfig.ValuePlain, themeOverrideJSON)
	acme := ids.ClientID("clt_acme")
	override.Scope, override.ClientID = platformconfig.ScopeClient, &acme
	h.cfg.rows = append(h.cfg.rows, global, override)
	return h
}

func (h *themeHarness) serve(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

func (h *themeHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	sh := shellForTest()
	return serveMounted(t, ac, q,
		&ThemeSettings{Config: h.cfg, Ops: h.ops, Shell: sh},
		&ClientTheme{Clients: h.clients, Config: h.cfg, Ops: h.ops, Shell: sh},
	)
}

// themeForm is a valid submitted theme.
func themeForm(kv ...string) url.Values {
	v := url.Values{
		"brandName": {"Acme"}, "brandSubtitle": {"Sub"}, "footerText": {"Foot"}, "logoHeight": {"40"},
		"primaryColor": {"#102a43"}, "accentColor": {"#0967d2"}, "backgroundColor": {"#0a1929"},
		"backgroundGradient": {"linear-gradient(135deg, #102a43 0%, #0a1929 100%)"},
	}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func TestThemeSettingsPage(t *testing.T) {
	h := newThemeHarness()
	t.Run("the stored theme fills the form and the preview", func(t *testing.T) {
		body := h.serve(t, cfgAdmin(), req{target: "/ui/platform/settings/theme"})
		mustContain(t, body, "Theme Settings", `name="brandName" value="Acme &lt;Portal&gt;"`, `name="primaryColor" value="#112233"`,
			`name="logoHeight"`, `value="48"`, `id="thm-preview"`, "Acme &lt;Portal&gt;", "Save Changes", "Reset to Defaults", "Login Branding")
		mustNotContain(t, body, "Acme <Portal>")
	})
	t.Run("reset to defaults shows the built-in look, unsaved", func(t *testing.T) {
		body := h.serve(t, cfgAdmin(), req{target: "/ui/platform/settings/theme?defaults=1"})
		mustContain(t, body, `name="brandName" value="FlowCatalyst"`, `name="accentColor" value="#0967d2"`)
		if len(h.ops.sets) != 0 {
			t.Fatal("a GET saved the defaults")
		}
	})
	t.Run("custom CSS is in the editor and never applied to the admin page", func(t *testing.T) {
		body := h.serve(t, cfgAdmin(), req{target: "/ui/platform/settings/theme"})
		mustContain(t, body, ".login-card{display:none}")
		if strings.Contains(strings.Replace(body, "<textarea", "", 1), "<style") && strings.Contains(body, "display:none}</style") {
			t.Error("custom CSS rendered in a style element")
		}
	})
}

func TestThemeSettingsPermissionMatrix(t *testing.T) {
	viewer := cfgUser(permCfgView)
	updater := cfgUser(permCfgView, permCfgUpdate)
	nobody := cfgUser()
	t.Run("a viewer sees the values but no controls to change them", func(t *testing.T) {
		h := newThemeHarness()
		body := h.serve(t, viewer, req{target: "/ui/platform/settings/theme"})
		mustContain(t, body, "Acme &lt;Portal&gt;", "disabled")
		mustNotContain(t, body, "Save Changes", "Reset to Defaults")
	})
	t.Run("an updater sees them", func(t *testing.T) {
		h := newThemeHarness()
		mustContain(t, h.serve(t, updater, req{target: "/ui/platform/settings/theme"}), "Save Changes")
	})
	t.Run("without the view permission the page is refused", func(t *testing.T) {
		h := newThemeHarness()
		if rec := h.rec(t, nobody, req{target: "/ui/platform/settings/theme"}); rec.Code != http.StatusForbidden {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("forged posts from a viewer or nobody change nothing", func(t *testing.T) {
		h := newThemeHarness()
		for _, ac := range []*auth.AuthContext{viewer, nobody} {
			for _, target := range []string{"/ui/platform/settings/theme", "/ui/clients/clt_acme/theme"} {
				form := themeForm("enabled", "on")
				if rec := h.rec(t, ac, req{method: http.MethodPost, target: target, form: form}); rec.Code != http.StatusForbidden {
					t.Errorf("%s: status %d", target, rec.Code)
				}
				body := h.serve(t, ac, req{method: http.MethodPost, target: target, form: form, datastar: true})
				mustContain(t, body, "fc-flash-error")
			}
			// The preview is for editors too.
			if rec := h.rec(t, ac, req{method: http.MethodPost, target: "/ui/platform/settings/theme/preview", form: themeForm(), datastar: true}); strings.Contains(rec.Body.String(), "thm-preview") {
				t.Error("a viewer got a preview")
			}
		}
		if len(h.ops.sets) != 0 || len(h.cfg.deleted) != 0 {
			t.Fatalf("forged posts changed things: %v %v", h.ops.sets, h.cfg.deleted)
		}
	})
}

func TestThemeSettingsSave(t *testing.T) {
	t.Run("the SPA's PUT body: the theme as JSON, PLAIN, the global description", func(t *testing.T) {
		h := newThemeHarness()
		body := h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/theme", datastar: true, multipart: true, form: themeForm("logoUrl", "https://example.com/logo.png", "customCss", ".a{b:c}")})
		if len(h.ops.sets) != 1 {
			t.Fatalf("sets %d: %s", len(h.ops.sets), body)
		}
		cmd := h.ops.sets[0]
		if cmd.ApplicationCode != "platform" || cmd.Section != "login" || cmd.Property != "theme" || cmd.ClientID != nil ||
			deref(cmd.ValueType) != "PLAIN" || deref(cmd.Description) != "Login page theme configuration" {
			t.Errorf("command = %+v", cmd)
		}
		for _, want := range []string{`"brandName":"Acme"`, `"logoUrl":"https://example.com/logo.png"`, `"logoSvg":null`, `"logoHeight":40`, `"customCss":".a{b:c}"`, `"backgroundGradient":"linear-gradient(135deg, #102a43 0%, #0a1929 100%)"`} {
			if !strings.Contains(cmd.Value, want) {
				t.Errorf("value lacks %s: %s", want, cmd.Value)
			}
		}
		mustContain(t, body, "Theme saved successfully")
	})
	t.Run("native post: redirect with a flash", func(t *testing.T) {
		h := newThemeHarness()
		rec := h.rec(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/theme", form: themeForm()})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/platform/settings/theme" || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=theme-saved") {
			t.Errorf("status %d, location %q, cookie %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
		}
	})
	t.Run("what the sign-in pages cannot use is refused on its field", func(t *testing.T) {
		for name, tc := range map[string]struct{ key, val, want string }{
			"colour":         {"accentColor", "red; background:url(x)", "Accent color must be a hex color"},
			"blank colour":   {"primaryColor", "", "Primary color must be a hex color"},
			"gradient":       {"backgroundGradient", "red; x:y", "Enter a CSS gradient"},
			"logo scheme":    {"logoUrl", "javascript:alert(1)", "Enter an http(s) address"},
			"logo data url":  {"logoUrl", "data:text/html,<script>alert(1)</script>", "Enter an http(s) address"},
			"svg script":     {"logoSvg", `<svg><script>alert(1)</script></svg>`, "without scripts or event handlers"},
			"svg handler":    {"logoSvg", `<svg onload="alert(1)"></svg>`, "without scripts or event handlers"},
			"svg not an svg": {"logoSvg", `<img src=x onerror=alert(1)>`, "Paste an &lt;svg&gt; element"},
			// What the old pattern check let through.
			"svg slash onload":   {"logoSvg", `<svg/onload=alert(1)>`, "without scripts or event handlers"},
			"svg image onerror":  {"logoSvg", `<svg><image/href=x/onerror=alert(1)></svg>`, "without scripts or event handlers"},
			"svg entity js url":  {"logoSvg", `<svg><a href="javascript&#58;alert(1)"><text>x</text></a></svg>`, "without scripts or event handlers"},
			"svg style breakout": {"logoSvg", `<svg><style><img src=x onerror=alert(1)></style></svg>`, "without scripts or event handlers"},
			"height low":         {"logoHeight", "5", "from 20 to 120"},
			"height high":        {"logoHeight", "500", "from 20 to 120"},
			"height text":        {"logoHeight", "tall", "from 20 to 120"},
			"css size":           {"customCss", strings.Repeat("a", thmMaxBlob+1), "too large"},
		} {
			t.Run(name, func(t *testing.T) {
				h := newThemeHarness()
				body := h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/theme", datastar: true, form: themeForm(tc.key, tc.val)})
				mustContain(t, body, tc.want, `id="thm-editor"`)
				if len(h.ops.sets) != 0 {
					t.Fatal("saved an invalid theme")
				}
			})
		}
	})
	t.Run("a failed save keeps what was typed", func(t *testing.T) {
		h := newThemeHarness()
		body := h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/theme", datastar: true, form: themeForm("brandName", "Typed <b>", "accentColor", "nope")})
		mustContain(t, body, `value="Typed &lt;b&gt;"`, `value="nope"`)
	})
	t.Run("a refusal of the use case is shown in the form", func(t *testing.T) {
		h := newThemeHarness()
		h.ops.err = httperror.Forbidden("not yours")
		body := h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/theme", datastar: true, form: themeForm()})
		mustContain(t, body, `id="thm-error"`, `role="alert"`, "Not yours")
	})
}

func TestThemePreview(t *testing.T) {
	h := newThemeHarness()
	post := func(kv ...string) string {
		t.Helper()
		return h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/theme/preview", datastar: true, form: themeForm(kv...)})
	}
	t.Run("text is escaped, colours are used", func(t *testing.T) {
		body := post("brandName", `<img src=x onerror=alert(1)>`, "accentColor", "#abcdef")
		mustContain(t, body, `id="thm-preview"`, "&lt;img src=x onerror=alert(1)&gt;", "background-color:#abcdef")
		mustNotContain(t, body, "<img src=x")
	})
	t.Run("custom CSS is not applied", func(t *testing.T) {
		body := post("customCss", `body{display:none} </style><script>alert(1)</script>`)
		mustNotContain(t, body, "display:none}", "<script", "<style", "alert(1)")
	})
	t.Run("an inline SVG is an image, never markup in the page", func(t *testing.T) {
		body := post("logoSvg", `<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`)
		mustContain(t, body, `src="data:image/svg+xml;base64,`)
		mustNotContain(t, body, "<circle", "<svg xmlns")
	})
	t.Run("an SVG with script is not shown at all", func(t *testing.T) {
		body := post("logoSvg", `<svg onload="alert(1)"><script>alert(1)</script></svg>`)
		mustNotContain(t, body, "data:image", "alert(1)", "<svg onload")
	})
	t.Run("an external logo is named, not loaded", func(t *testing.T) {
		body := post("logoUrl", "https://example.com/logo.png")
		mustContain(t, body, "Logo: https://example.com/logo.png")
		mustNotContain(t, body, `src="https://example.com`)
	})
	t.Run("an invalid colour or gradient falls back and cannot break out of the style", func(t *testing.T) {
		body := post("backgroundColor", `red;"onclick="x`, "backgroundGradient", `url(javascript:alert(1))`, "accentColor", `"><script>`)
		mustContain(t, body, "background:#0a1929", "background-color:#0967d2")
		mustNotContain(t, body, "javascript", "<script", "onclick")
	})
	t.Run("a gradient wins over the colour", func(t *testing.T) {
		mustContain(t, post(), "background:linear-gradient(135deg, #102a43 0%, #0a1929 100%)")
	})
	t.Run("it is the page's own request: a plain post is refused", func(t *testing.T) {
		rec := h.rec(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/settings/theme/preview", form: themeForm()})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d", rec.Code)
		}
	})
	if len(h.ops.sets) != 0 {
		t.Fatal("the preview saved something")
	}
}

func TestClientThemePage(t *testing.T) {
	t.Run("an override: the switch is on and the client's values lie over the platform's", func(t *testing.T) {
		h := newThemeHarness()
		body := h.serve(t, cfgAdmin(), req{target: "/ui/clients/clt_acme/theme"})
		mustContain(t, body, "Login Branding", "Acme Corp", `name="enabled"`, `checked`, `{thmEnabled: true}`,
			`name="brandName" value="Acme Client"`, `name="accentColor" value="#ff0000"`,
			// Not overridden: the platform's.
			`name="footerText" value="Acme footer"`, `name="primaryColor" value="#112233"`,
			"/oauth/authorize?…&amp;client=acme", `href="/ui/clients/clt_acme"`)
		mustNotContain(t, body, "Reset to Defaults")
	})
	t.Run("no override: the switch is off and the form starts from the platform theme", func(t *testing.T) {
		h := newThemeHarness()
		h.cfg.rows = h.cfg.rows[:len(h.cfg.rows)-1]
		body := h.serve(t, cfgAdmin(), req{target: "/ui/clients/clt_acme/theme"})
		mustContain(t, body, `{thmEnabled: false}`, `name="brandName" value="Acme &lt;Portal&gt;"`, `style="display:none"`)
	})
	t.Run("the client page needs the clients view; an unknown client is not found", func(t *testing.T) {
		h := newThemeHarness()
		if rec := h.rec(t, cfgUser(permCfgView), req{target: "/ui/clients/clt_acme/theme"}); rec.Code != http.StatusForbidden {
			t.Errorf("without clients:view: status %d", rec.Code)
		}
		if rec := h.rec(t, cfgAdmin(), req{target: "/ui/clients/clt_nobody/theme"}); rec.Code != http.StatusNotFound {
			t.Errorf("unknown client: status %d", rec.Code)
		}
	})
	t.Run("save with the switch on: the override is set for the client", func(t *testing.T) {
		h := newThemeHarness()
		body := h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/clients/clt_acme/theme", datastar: true, form: themeForm("enabled", "on", "brandName", "Acme Two")})
		if len(h.ops.sets) != 1 {
			t.Fatalf("sets %d: %s", len(h.ops.sets), body)
		}
		cmd := h.ops.sets[0]
		if deref(cmd.ClientID) != "clt_acme" || deref(cmd.Description) != "Client login page theme configuration" || !strings.Contains(cmd.Value, `"brandName":"Acme Two"`) {
			t.Errorf("command = %+v", cmd)
		}
		mustContain(t, body, "Login branding saved", `id="thm-editor"`)
	})
	t.Run("save with the switch off deletes the override and stores no copy", func(t *testing.T) {
		h := newThemeHarness()
		body := h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/clients/clt_acme/theme", datastar: true, form: themeForm()})
		if len(h.ops.sets) != 0 || strings.Join(h.cfg.deleted, ",") != "login/theme" {
			t.Fatalf("sets %v deleted %v", h.ops.sets, h.cfg.deleted)
		}
		mustContain(t, body, "Login branding reset to the platform theme")
	})
	t.Run("an invalid override is refused; with the switch off nothing is validated", func(t *testing.T) {
		h := newThemeHarness()
		body := h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/clients/clt_acme/theme", datastar: true, form: themeForm("enabled", "on", "accentColor", "nope")})
		mustContain(t, body, "Accent color must be a hex color")
		if len(h.ops.sets) != 0 {
			t.Fatal("saved an invalid override")
		}
		h.serve(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/clients/clt_acme/theme", datastar: true, form: themeForm("accentColor", "nope")})
		if len(h.cfg.deleted) != 1 {
			t.Fatal("did not clear the override")
		}
	})
	t.Run("an unknown client is refused by the post", func(t *testing.T) {
		h := newThemeHarness()
		if rec := h.rec(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/clients/clt_nobody/theme", form: themeForm("enabled", "on")}); rec.Code != http.StatusNotFound {
			t.Errorf("status %d", rec.Code)
		}
		if len(h.ops.sets) != 0 {
			t.Fatal("stored a theme for no client")
		}
	})
	t.Run("native post: redirect with a flash", func(t *testing.T) {
		h := newThemeHarness()
		rec := h.rec(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/clients/clt_acme/theme", form: themeForm("enabled", "on")})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/clients/clt_acme/theme" || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=client-theme-saved") {
			t.Errorf("status %d, location %q, cookie %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
		}
	})
}

func TestThemeOverlayAndJSON(t *testing.T) {
	t.Run("a stored null clears, a missing key keeps, bad JSON keeps everything", func(t *testing.T) {
		base := defaultTheme()
		base.LogoURL = "https://x/y.png"
		got := base.overlay(`{"logoUrl":null,"brandName":"B"}`)
		if got.LogoURL != "" || got.BrandName != "B" || got.AccentColor != base.AccentColor {
			t.Errorf("overlay = %+v", got)
		}
		if bad := base.overlay(`{nope`); bad != base {
			t.Errorf("bad JSON changed the theme: %+v", bad)
		}
	})
	t.Run("the stored value round-trips", func(t *testing.T) {
		v := defaultTheme()
		v.LogoSVG = `<svg/>`
		if got := defaultTheme().overlay(v.json()); got != v {
			t.Errorf("round trip: %+v != %+v", got, v)
		}
	})
}
