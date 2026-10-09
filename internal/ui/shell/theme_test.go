package shell

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

func TestResolveTheme(t *testing.T) {
	for _, tc := range []struct {
		name          string
		theme, scheme string
		query         string
		want          pageTheme
	}{
		{name: "nothing stored: system, light until the page reports", want: pageTheme{choice: kit.ThemeSystem}},
		{name: "system, last seen dark", scheme: "dark", want: pageTheme{choice: kit.ThemeSystem, dark: true}},
		{name: "explicit dark", theme: "dark", want: pageTheme{choice: kit.ThemeDark, dark: true}},
		{name: "explicit light wins over a dark system", theme: "light", scheme: "dark", want: pageTheme{choice: kit.ThemeLight}},
		{name: "query overrides the cookie for one page", theme: "light", query: "theme=dark", want: pageTheme{choice: kit.ThemeDark, dark: true}},
		{name: "unknown values are ignored", theme: "<b>", scheme: "purple", query: "theme=x", want: pageTheme{choice: kit.ThemeSystem}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/ui/x?"+tc.query, nil)
			if tc.theme != "" {
				r.AddCookie(&http.Cookie{Name: kit.ThemeCookie, Value: tc.theme})
			}
			if tc.scheme != "" {
				r.AddCookie(&http.Cookie{Name: kit.SchemeCookie, Value: tc.scheme})
			}
			if got := resolveTheme(r); got != tc.want {
				t.Errorf("resolveTheme = %+v, want %+v", got, tc.want)
			}
		})
	}
}

var htmlTag = regexp.MustCompile(`<html[^>]*>`)

// The server renders the theme on <html> (no flash of light before Datastar
// runs) and the user menu's picker with the choice pressed.
func TestPageRendersTheThemeAndTheMobileShell(t *testing.T) {
	render := func(cookies ...*http.Cookie) string {
		r := request(false)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		(&Shell{}).Render(rec, r, Page{Title: "T", Path: "/ui/x"}, kit.EmptyState(kit.EmptyStateProps{Message: "x"}))
		return rec.Body.String()
	}
	dark := render(&http.Cookie{Name: kit.ThemeCookie, Value: "dark"})
	tag := htmlTag.FindString(dark)
	for _, want := range []string{
		`class="dark"`, `data-signals:dark="true"`, `data-signals:theme="&#39;dark&#39;"`, `data-class:dark="$dark"`,
		`data-signals:sidebar-mobile-open="false"`, `data-class:fc-mobile-open="$sidebarMobileOpen"`,
	} {
		if !strings.Contains(tag, want) {
			t.Errorf("<html> lacks %s: %s", want, tag)
		}
	}
	for _, want := range []string{
		`<button type="button" class="fc-theme-option" aria-pressed="true" data-attr:aria-pressed="$theme === &#39;dark&#39; ? &#39;true&#39; : &#39;false&#39;"`,
		`<header class="fc-topbar">`, `id="fc-topbar-menu"`, `aria-controls="fc-sidebar"`, `<aside id="fc-sidebar" class="fc-sidebar">`,
		`id="fc-sidebar-close"`, `class="fc-sidebar-backdrop"`,
	} {
		if !strings.Contains(dark, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	light := htmlTag.FindString(render())
	if strings.Contains(light, `class="dark"`) || !strings.Contains(light, `data-signals:theme="&#39;system&#39;"`) {
		t.Errorf("default page: %s", light)
	}
}
