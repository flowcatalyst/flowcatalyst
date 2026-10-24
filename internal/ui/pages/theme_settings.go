package pages

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The login theme is the platform config row (platform, login, theme): a JSON
// object, the platform-wide value or one client's override. The settings page
// edits the platform-wide one and the client's Login Branding page the
// override; both read and write it through the platform-config service, so
// they obey the rules of PUT /api/config/platform/login/theme.

const (
	themeApp      = "platform"
	themeSection  = "login"
	themeProperty = "theme"

	thmPath = "/ui/platform/settings/theme"

	// Limits for the fields the sign-in pages render. The SPA sets none; they
	// keep one row from growing without bound and keep a logo a small image.
	thmMaxText = 500
	thmMaxBlob = 64 << 10

	thmMinLogoHeight, thmMaxLogoHeight = 20, 120
)

// themeValues is the theme as the form holds it.
type themeValues struct {
	BrandName, BrandSubtitle, FooterText       string
	LogoURL, LogoSVG, LogoHeight               string
	PrimaryColor, AccentColor, BackgroundColor string
	BackgroundGradient, CustomCSS              string
}

// defaultTheme is the built-in platform look (the SPA's defaultLoginTheme).
func defaultTheme() themeValues {
	return themeValues{
		BrandName:          "FlowCatalyst",
		BrandSubtitle:      "Platform Administration",
		LogoHeight:         "40",
		PrimaryColor:       "#102a43",
		AccentColor:        "#0967d2",
		BackgroundColor:    "#0a1929",
		BackgroundGradient: "linear-gradient(135deg, #102a43 0%, #0a1929 100%)",
		FooterText:         "Secure access to your FlowCatalyst platform",
	}
}

// overlay lays a stored theme over v the way the SPA's `{...base, ...stored}`
// does: a stored key replaces the value, a null clears it. A value that is not
// valid JSON leaves v as it is.
func (v themeValues) overlay(raw string) themeValues {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return v
	}
	text := func(key string, dst *string) {
		switch x := m[key].(type) {
		case string:
			*dst = x
		case float64:
			*dst = strconv.FormatFloat(x, 'f', -1, 64)
		case nil:
			if _, set := m[key]; set {
				*dst = ""
			}
		}
	}
	text("brandName", &v.BrandName)
	text("brandSubtitle", &v.BrandSubtitle)
	text("footerText", &v.FooterText)
	text("logoUrl", &v.LogoURL)
	text("logoSvg", &v.LogoSVG)
	text("logoHeight", &v.LogoHeight)
	text("primaryColor", &v.PrimaryColor)
	text("accentColor", &v.AccentColor)
	text("backgroundColor", &v.BackgroundColor)
	text("backgroundGradient", &v.BackgroundGradient)
	text("customCss", &v.CustomCSS)
	return v
}

// json is the stored value: the SPA's JSON.stringify(theme), blank optional
// fields as null.
func (v themeValues) json() string {
	orNull := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	m := map[string]any{
		"brandName": v.BrandName, "brandSubtitle": v.BrandSubtitle, "footerText": v.FooterText,
		"logoUrl": orNull(v.LogoURL), "logoSvg": orNull(v.LogoSVG),
		"primaryColor": v.PrimaryColor, "accentColor": v.AccentColor, "backgroundColor": v.BackgroundColor,
		"backgroundGradient": orNull(v.BackgroundGradient), "customCss": orNull(v.CustomCSS),
	}
	if n, err := strconv.Atoi(v.LogoHeight); err == nil {
		m["logoHeight"] = n
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func submittedTheme(r *http.Request) themeValues {
	f := func(key string) string { return strings.TrimSpace(r.PostFormValue(key)) }
	return themeValues{
		BrandName: f("brandName"), BrandSubtitle: f("brandSubtitle"), FooterText: f("footerText"),
		LogoURL: f("logoUrl"), LogoSVG: f("logoSvg"), LogoHeight: f("logoHeight"),
		PrimaryColor: f("primaryColor"), AccentColor: f("accentColor"), BackgroundColor: f("backgroundColor"),
		BackgroundGradient: f("backgroundGradient"), CustomCSS: r.PostFormValue("customCss"),
	}
}

var (
	// thmColorPattern is the platform's colour rule (branding.colorPattern,
	// which the emails use): hex or rgb()/rgba(), so a colour cannot leave the
	// style attribute it is placed in.
	thmColorPattern = regexp.MustCompile(`(?i)^#[0-9a-f]{3,8}$|^rgba?\([0-9.,%\s]+\)$`)
	// thmGradientPattern is a gradient made of colours, stops and angles only.
	thmGradientPattern = regexp.MustCompile(`(?i)^(?:repeating-)?(?:linear|radial|conic)-gradient\([#0-9a-z%,.()\s-]{1,400}\)$`)
	// thmSVGUnsafe finds what makes an inline logo executable on the sign-in page.
	thmSVGUnsafe = regexp.MustCompile(`(?i)<\s*(?:script|foreignObject|iframe|embed|object)|\son[a-z]+\s*=|javascript:`)
)

// validate checks the form the way the sign-in pages will use it. The
// colours must be colours (the picker only ever produces hex), the logo must
// be an http(s) URL or an inline SVG that cannot run script, the height the
// range the field offers.
func (v themeValues) validate(errs *formErrors) {
	for _, c := range []struct{ key, label, val string }{
		{"primaryColor", "Primary color", v.PrimaryColor},
		{"accentColor", "Accent color", v.AccentColor},
		{"backgroundColor", "Background color", v.BackgroundColor},
	} {
		if !thmColorPattern.MatchString(c.val) {
			errs.field(c.key, c.label+" must be a hex color such as #0967d2 (or rgb()).")
		}
	}
	if v.BackgroundGradient != "" && !thmGradientPattern.MatchString(v.BackgroundGradient) {
		errs.field("backgroundGradient", "Enter a CSS gradient such as linear-gradient(135deg, #102a43 0%, #0a1929 100%).")
	}
	for _, t := range []struct{ key, val string }{{"brandName", v.BrandName}, {"brandSubtitle", v.BrandSubtitle}, {"footerText", v.FooterText}, {"logoUrl", v.LogoURL}} {
		if len(t.val) > thmMaxText {
			errs.field(t.key, fmt.Sprintf("Use at most %d characters.", thmMaxText))
		}
	}
	if v.LogoURL != "" && !thmLogoURLOK(v.LogoURL) {
		errs.field("logoUrl", "Enter an http(s) address, such as https://example.com/logo.png.")
	}
	switch {
	case len(v.LogoSVG) > thmMaxBlob:
		errs.field("logoSvg", "The SVG is too large (64 kB at most).")
	case v.LogoSVG != "" && (!strings.HasPrefix(strings.ToLower(v.LogoSVG), "<svg") || thmSVGUnsafe.MatchString(v.LogoSVG)):
		errs.field("logoSvg", "Paste an <svg> element without scripts or event handlers.")
	}
	if v.LogoHeight != "" {
		if n, err := strconv.Atoi(v.LogoHeight); err != nil || n < thmMinLogoHeight || n > thmMaxLogoHeight {
			errs.field("logoHeight", fmt.Sprintf("Enter a whole number from %d to %d.", thmMinLogoHeight, thmMaxLogoHeight))
		}
	}
	if len(v.CustomCSS) > thmMaxBlob {
		errs.field("customCss", "The CSS is too large (64 kB at most).")
	}
}

func thmLogoURLOK(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && !strings.ContainsAny(s, " \t\r\n")
}

// ── preview ────────────────────────────────────────────────────────────

// thmPreview is what the live preview draws. Nothing the admin typed reaches
// the preview as markup or style unchecked: colours and the gradient are
// validated (a bad one falls back to the built-in look), text is escaped, an
// inline SVG is shown as an image (an image cannot run script), and the
// custom CSS is not applied at all (the admin page must not wear the sign-in
// page's styles, nor run them).
type thmPreview struct {
	Background, Accent  string
	BrandName, Subtitle string
	Footer              string
	// LogoSrc is a data: image to show; LogoURL an external logo the
	// admin page's policy (img-src 'self' data:) cannot load, shown as text.
	LogoSrc, LogoURL string
}

func (v themeValues) preview() thmPreview {
	def := defaultTheme()
	colour := func(c, fallback string) string {
		if thmColorPattern.MatchString(c) {
			return c
		}
		return fallback
	}
	p := thmPreview{
		Accent:    colour(v.AccentColor, def.AccentColor),
		BrandName: v.BrandName, Subtitle: v.BrandSubtitle, Footer: v.FooterText,
	}
	// The gradient wins over the colour when it is set (the SPA's rule).
	switch {
	case thmGradientPattern.MatchString(v.BackgroundGradient):
		p.Background = v.BackgroundGradient
	default:
		p.Background = colour(v.BackgroundColor, def.BackgroundColor)
	}
	switch {
	case v.LogoSVG != "" && !thmSVGUnsafe.MatchString(v.LogoSVG) && strings.HasPrefix(strings.ToLower(v.LogoSVG), "<svg") && len(v.LogoSVG) <= thmMaxBlob:
		p.LogoSrc = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(v.LogoSVG))
	case v.LogoURL != "" && thmLogoURLOK(v.LogoURL):
		p.LogoURL = v.LogoURL
	}
	return p
}

// ── the form's view ────────────────────────────────────────────────────

// thmView is the editor: the form, its preview and, on a client's page, the
// switch that turns the override on.
type thmView struct {
	Values themeValues
	Errs   formErrors
	// Action is where the form posts, PreviewAction where the live preview asks.
	Action, PreviewAction string
	// ReadOnly shows the values without the controls to change them.
	ReadOnly bool
	// Client is set on a client's page.
	Client *thmClient
}

type thmClient struct {
	ID, Name, Identifier string
	// Enabled is whether the client has an override.
	Enabled bool
}

func (v thmView) id(key string) string         { return fieldID("thm", key) }
func (v thmView) fieldError(key string) string { return v.Errs.Fields[key] }

func (v thmView) firstInvalid() string {
	for _, k := range thmKeys {
		if v.fieldError(k) != "" {
			return v.id(k)
		}
	}
	return ""
}

var thmKeys = []string{"brandName", "brandSubtitle", "footerText", "logoUrl", "logoSvg", "logoHeight", "primaryColor", "accentColor", "backgroundColor", "backgroundGradient", "customCss"}

// ── loading ────────────────────────────────────────────────────────────

// storedTheme is the stored value of the platform-wide theme (clientID "") or
// of one client's override, "" when there is none. List applies the API's read
// rule, so a caller who may not read the platform's config gets its refusal.
func storedTheme(ctx context.Context, cfg ConfigService, ac *auth.AuthContext, clientID string) (string, error) {
	rows, err := cfg.List(ctx, ac, themeApp)
	if err != nil {
		return "", err
	}
	for i := range rows {
		if rows[i].Section == themeSection && rows[i].Property == themeProperty && deref(ids.StringPtr(rows[i].ClientID)) == clientID {
			return rows[i].Value, nil
		}
	}
	return "", nil
}

// setThemeCommand is PUT /api/config/platform/login/theme (with the client's
// id for an override), as the SPA sends it.
func setThemeCommand(v themeValues, clientID string) operations.SetPropertyCommand {
	desc, plain := "Login page theme configuration", "PLAIN"
	cmd := operations.SetPropertyCommand{ApplicationCode: themeApp, Section: themeSection, Property: themeProperty, Value: v.json(), ValueType: &plain}
	if clientID != "" {
		desc = "Client login page theme configuration"
		cmd.ClientID = &clientID
	}
	cmd.Description = &desc
	return cmd
}

// ── the platform-wide theme (/ui/platform/settings/theme) ──────────────

// ThemeSettings is the Theme settings page, mirroring the SPA's
// LoginThemeSettingsPage: the platform-wide login theme with a live preview.
type ThemeSettings struct {
	Config ConfigService
	Ops    ConfigCommands
	Shell  *shell.Shell
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *ThemeSettings) Mount(r chi.Router) {
	r.Get("/platform/settings/theme", h.page)
	post := r.With(parseForm)
	post.Post("/platform/settings/theme", h.save)
	post.Post("/platform/settings/theme/preview", h.preview)
}

func (h *ThemeSettings) view(values themeValues, readOnly bool) thmView {
	return thmView{Values: values, ReadOnly: readOnly, Action: thmPath, PreviewAction: thmPath + "/preview"}
}

func (h *ThemeSettings) page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	raw, err := storedTheme(ctx, h.Config, ac, "")
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	values := defaultTheme()
	// "Reset to Defaults" shows the built-in look; it is saved with Save.
	if r.URL.Query().Get("defaults") == "" {
		values = values.overlay(raw)
	}
	canEdit := h.Config.AuthorizeWrite(ctx, ac, themeApp) == nil
	h.Shell.Render(w, r, shell.Page{Title: "Theme Settings", Path: thmPath}, thmSettingsPage(h.view(values, !canEdit)))
}

func (h *ThemeSettings) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// PUT /api/config/...: the confinement and update gate first; the use case
	// decides for callers below anchor tier by their access grants.
	if err := h.Config.AuthorizeUpdate(ctx, ac, themeApp); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view := h.view(submittedTheme(r), false)
	view.Values.validate(&view.Errs)
	if !view.Errs.any() {
		if err := h.Ops.Set(ctx, setThemeCommand(view.Values, "")); err != nil {
			view.Errs.apply(r, err, nil)
		}
	}
	if view.Errs.any() {
		themeFailed(w, r, h.Shell, "Theme Settings", view, thmSettingsPage)
		return
	}
	themeSaved(w, r, h.Shell, thmPath, flashThemeSaved)
}

func (h *ThemeSettings) preview(w http.ResponseWriter, r *http.Request) {
	if err := h.Config.AuthorizeUpdate(r.Context(), auth.FromContext(r.Context()), themeApp); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	patchThemePreview(w, r, submittedTheme(r))
}

// patchThemePreview answers the live preview's ask: the preview of the
// submitted values, and nothing else (nothing is saved or read).
func patchThemePreview(w http.ResponseWriter, r *http.Request, v themeValues) {
	if !ds.IsDatastar(r) {
		http.Error(w, "The preview is for the page's own requests.", http.StatusBadRequest)
		return
	}
	if err := ds.NewSSE(w, r).Patch(thmPreviewPanel(v.preview())); err != nil {
		slog.DebugContext(r.Context(), "ui: theme preview not delivered", "err", err)
	}
}

// themeFailed answers a rejected save: the editor again with the values the
// admin typed and the errors, a native post as the whole page (422).
func themeFailed(w http.ResponseWriter, r *http.Request, sh *shell.Shell, title string, view thmView, page func(thmView) templ.Component) {
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		errs := []error{sse.Patch(thmEditor(view))}
		if id := view.firstInvalid(); id != "" {
			errs = append(errs, sse.Focus(id))
		}
		if err := errors.Join(errs...); err != nil {
			slog.DebugContext(r.Context(), "ui: theme failure patch not delivered", "err", err)
		}
		return
	}
	status := view.Errs.Status
	if status == 0 {
		status = http.StatusUnprocessableEntity
	}
	sh.RenderStatus(w, r, status, shell.Page{Title: title, Path: r.URL.Path}, page(view))
}

// themeSaved answers a successful save: a flash after a Datastar post, a
// redirect carrying the flash key after a native one.
func themeSaved(w http.ResponseWriter, r *http.Request, sh *shell.Shell, back, flash string) {
	if ds.IsDatastar(r) {
		if err := flashKey(ds.NewSSE(w, r), flash); err != nil {
			slog.DebugContext(r.Context(), "ui: theme flash not delivered", "err", err)
		}
		return
	}
	sh.SetFlash(w, flash)
	http.Redirect(w, r, back, http.StatusSeeOther)
}
