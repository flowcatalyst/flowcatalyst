package branding

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html"
	"log/slog"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
)

// The login/email theme lives at
// (app_code="platform", section="login", property="theme", scope=GLOBAL) —
// the same row the public /api/public/login-theme endpoint reads. Stored as a
// JSON object; every field is optional and falls back to the SPA default, so
// the platform stays branded out-of-the-box and white-labels per business when
// a theme is configured.
const (
	themeSection  = "login"
	themeProperty = "theme"

	// Defaults mirror the SPA's loginTheme.ts so emails match the login page.
	DefaultPrimaryColor = "#102a43"
	DefaultAccentColor  = "#0967d2"
)

// colorPattern allows only hex (#rgb..#rrggbbaa) and rgb()/rgba() values, so a
// configured colour can't break out of the inline style attribute it's placed
// in. Anything else falls back to the default.
var colorPattern = regexp.MustCompile(`(?i)^#[0-9a-f]{3,8}$|^rgba?\([0-9.,%\s]+\)$`)

// Theme is the resolved branding used in transactional emails. Values come
// from the platform login-theme config; absent fields use the defaults, so
// callers stay unconditional.
type Theme struct {
	BrandName    string
	PrimaryColor string
	AccentColor  string
	LogoURL      string // hosted logo image URL; "" when unset
	LogoSVG      string // raw SVG markup; "" when unset
	FooterText   string
}

// rawTheme is the stored JSON shape (subset of the SPA's LoginThemeResponse).
type rawTheme struct {
	BrandName    *string `json:"brandName"`
	PrimaryColor *string `json:"primaryColor"`
	AccentColor  *string `json:"accentColor"`
	LogoURL      *string `json:"logoUrl"`
	LogoSVG      *string `json:"logoSvg"`
	FooterText   *string `json:"footerText"`
}

// LoadTheme resolves the email/login theme, layering the stored config over the
// defaults. Brand name reuses the platform-name resolver. Safe with a nil
// reader (returns all defaults).
func LoadTheme(ctx context.Context, r Reader) Theme {
	t := Theme{
		BrandName:    PlatformName(ctx, r),
		PrimaryColor: DefaultPrimaryColor,
		AccentColor:  DefaultAccentColor,
	}
	if r == nil {
		return t
	}

	cfg, err := r.FindByCoordinate(ctx, App, themeSection, themeProperty, platformconfig.ScopeGlobal, nil)
	if err != nil {
		slog.Warn("branding: login-theme lookup failed", "err", err)
		return t
	}
	if cfg == nil || strings.TrimSpace(cfg.Value) == "" {
		return t
	}

	var raw rawTheme
	if err := json.Unmarshal([]byte(cfg.Value), &raw); err != nil {
		slog.Warn("branding: login-theme is not valid JSON", "err", err)
		return t
	}

	if raw.BrandName != nil && strings.TrimSpace(*raw.BrandName) != "" {
		t.BrandName = strings.TrimSpace(*raw.BrandName)
	}
	if raw.PrimaryColor != nil {
		t.PrimaryColor = safeColor(*raw.PrimaryColor, t.PrimaryColor)
	}
	if raw.AccentColor != nil {
		t.AccentColor = safeColor(*raw.AccentColor, t.AccentColor)
	}
	if raw.LogoURL != nil {
		t.LogoURL = strings.TrimSpace(*raw.LogoURL)
	}
	if raw.LogoSVG != nil {
		t.LogoSVG = strings.TrimSpace(*raw.LogoSVG)
	}
	if raw.FooterText != nil {
		t.FooterText = strings.TrimSpace(*raw.FooterText)
	}
	return t
}

// LogoSrc resolves the value for the banner <img src>. A hosted logoUrl wins
// (PNG/JPG render reliably in Outlook/Gmail, which often block inline SVG);
// otherwise the SVG is converted to a base64 data: URI on the fly so the email
// stays self-contained. Returns "" when neither is set or the URL scheme is
// not http(s)/data.
func (t Theme) LogoSrc() string {
	if u := strings.TrimSpace(t.LogoURL); u != "" {
		lower := strings.ToLower(u)
		if strings.HasPrefix(lower, "https://") ||
			strings.HasPrefix(lower, "http://") ||
			strings.HasPrefix(lower, "data:image/") {
			return u
		}
	}
	return t.LogoDataURI()
}

// LogoDataURI returns the theme's SVG logo as a base64 data: URI for an
// <img src>, or "" when no logo SVG is configured.
func (t Theme) LogoDataURI() string {
	svg := strings.TrimSpace(t.LogoSVG)
	if svg == "" {
		return ""
	}
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func safeColor(v, fallback string) string {
	v = strings.TrimSpace(v)
	if colorPattern.MatchString(v) {
		return v
	}
	return fallback
}

// EmailContent is the body of a branded transactional email.
type EmailContent struct {
	Heading     string
	Intro       string
	ButtonLabel string
	ButtonURL   string
	AfterButton []string
	Footer      string // small-print; defaults to an automated-message note
}

// RenderEmail wraps content in a table-based, inline-styled HTML layout branded
// with the theme: a header banner (logo data-URI when set, else the brand name
// in white), an accent-coloured primary button with white text, a plain-text
// fallback link, and a footer. All text is HTML-escaped; colours are validated
// in LoadTheme.
func (t Theme) RenderEmail(c EmailContent) string {
	header := `<span style="color:#ffffff;font-size:20px;font-weight:700;">` +
		html.EscapeString(t.BrandName) + `</span>`
	if src := t.LogoSrc(); src != "" {
		header = `<img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(t.BrandName) +
			`" height="40" style="height:40px;max-height:40px;display:block;margin:0 auto;border:0;outline:none;text-decoration:none;" />`
	}

	var body strings.Builder
	if c.Heading != "" {
		body.WriteString(`<h1 style="margin:0 0 16px;font-size:22px;font-weight:700;color:` +
			t.PrimaryColor + `;">` + html.EscapeString(c.Heading) + `</h1>`)
	}
	if c.Intro != "" {
		body.WriteString(`<p style="margin:0 0 24px;font-size:15px;line-height:1.6;color:#33475b;">` +
			html.EscapeString(c.Intro) + `</p>`)
	}
	if c.ButtonLabel != "" && c.ButtonURL != "" {
		url := html.EscapeString(c.ButtonURL)
		body.WriteString(
			`<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 24px;"><tr>` +
				`<td style="border-radius:6px;background-color:` + t.AccentColor + `;">` +
				`<a href="` + url + `" style="display:inline-block;padding:12px 28px;font-size:15px;` +
				`font-weight:600;color:#ffffff;text-decoration:none;border-radius:6px;">` +
				html.EscapeString(c.ButtonLabel) + `</a></td></tr></table>` +
				`<p style="margin:0 0 24px;font-size:13px;line-height:1.6;color:#62748b;">` +
				`Or paste this link into your browser:<br>` +
				`<a href="` + url + `" style="color:` + t.AccentColor + `;word-break:break-all;">` +
				url + `</a></p>`)
	}
	for _, p := range c.AfterButton {
		if strings.TrimSpace(p) == "" {
			continue
		}
		body.WriteString(`<p style="margin:0 0 16px;font-size:14px;line-height:1.6;color:#33475b;">` +
			html.EscapeString(p) + `</p>`)
	}

	footer := strings.TrimSpace(c.Footer)
	if footer == "" {
		footer = "This is an automated message from " + t.BrandName + ". Please do not reply to this email."
	}

	return `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1"></head>` +
		`<body style="margin:0;padding:0;background-color:#f4f6f8;">` +
		`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" ` +
		`style="background-color:#f4f6f8;padding:24px 0;"><tr><td align="center">` +
		`<table role="presentation" width="600" cellpadding="0" cellspacing="0" ` +
		`style="width:600px;max-width:600px;background-color:#ffffff;border-radius:10px;` +
		`overflow:hidden;border:1px solid #e3e8ee;">` +
		`<tr><td style="background-color:` + t.PrimaryColor + `;padding:24px 32px;text-align:center;">` +
		header + `</td></tr>` +
		`<tr><td style="padding:32px;font-family:Arial,Helvetica,sans-serif;">` + body.String() + `</td></tr>` +
		`<tr><td style="padding:20px 32px;background-color:#f4f6f8;border-top:1px solid #e3e8ee;` +
		`font-family:Arial,Helvetica,sans-serif;font-size:12px;line-height:1.5;color:#8a94a6;text-align:center;">` +
		html.EscapeString(footer) + `</td></tr>` +
		`</table></td></tr></table></body></html>`
}
