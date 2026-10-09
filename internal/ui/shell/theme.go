package shell

import (
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// pageTheme is the theme a page renders with: the user's choice and whether
// the page is dark (app-shell.md, Theme).
type pageTheme struct {
	choice kit.ThemeChoice
	dark   bool
}

// resolveTheme reads the theme cookies (kit.ThemeCookie: an explicit
// choice; kit.SchemeCookie: the system scheme the page last observed) so the
// server renders <html class="dark"> and a dark page does not flash light.
// ?theme=light|dark overrides both for one page (screenshots, as the kit
// gallery does) without changing the stored choice. In system mode with no
// observed scheme yet (a browser's first page) the page renders light and the
// client corrects it once Datastar runs; it records the scheme for the next
// page. Only "light" and "dark" are honoured; anything else is system.
func resolveTheme(r *http.Request) pageTheme {
	t := pageTheme{choice: kit.ThemeSystem}
	if c, err := r.Cookie(kit.ThemeCookie); err == nil {
		switch kit.ThemeChoice(c.Value) {
		case kit.ThemeLight, kit.ThemeDark:
			t.choice = kit.ThemeChoice(c.Value)
		case kit.ThemeSystem:
		}
	}
	switch q := kit.ThemeChoice(r.URL.Query().Get("theme")); q {
	case kit.ThemeLight, kit.ThemeDark:
		t.choice = q
	case kit.ThemeSystem:
	}
	switch t.choice {
	case kit.ThemeDark:
		t.dark = true
	case kit.ThemeLight:
	case kit.ThemeSystem:
		if c, err := r.Cookie(kit.SchemeCookie); err == nil && c.Value == "dark" {
			t.dark = true
		}
	}
	return t
}
