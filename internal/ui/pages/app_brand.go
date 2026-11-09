package pages

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/kit"
)

// appBrandTTL is how long AppBrand keeps what it read. A save on the Theme or
// Names settings page drops it at once on this instance; another instance
// shows the change within the TTL.
const appBrandTTL = 30 * time.Second

// AppBrand is the app shell's branding (shell.Branding) as the SPA's shell
// wears it: the sidebar logo of the platform-wide login theme (what GET
// /api/public/login-theme answers without a client, the SPA's appTheme
// store: logo URL, else logo SVG, else the bolt, at the logo height) and the
// platform name (GET /api/public/platform, the SPA's document title). Both are
// read through the platform's router, so the rules are the public endpoints'
// (the logo SVG check among them), and kept for appBrandTTL, so a page costs
// no read while they are fresh. The zero value with an Upstream is ready.
type AppBrand struct {
	// Upstream is the platform's router; nil is the default branding.
	Upstream http.Handler
	// Now is the clock (nil: time.Now); TTL the lifetime (0: appBrandTTL).
	Now func() time.Time
	TTL time.Duration

	mu     sync.Mutex
	cached shell.Branding
	at     time.Time
	fresh  bool
	gen    uint64
}

// Branding is the shell's resolver (shell.Shell.Branding).
func (b *AppBrand) Branding(r *http.Request) shell.Branding {
	now := b.now()
	b.mu.Lock()
	if b.fresh && now.Sub(b.at) < b.ttl() {
		v := b.cached
		b.mu.Unlock()
		return v
	}
	gen := b.gen
	b.mu.Unlock()

	v := b.load(r)

	b.mu.Lock()
	// A save while this read ran (Invalidate) makes what it read stale.
	if gen == b.gen {
		b.cached, b.at, b.fresh = v, now, true
	}
	b.mu.Unlock()
	return v
}

// Invalidate drops the kept branding: the next page reads it again. The
// settings pages call it after a save.
func (b *AppBrand) Invalidate() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.fresh = false
	b.gen++
	b.mu.Unlock()
}

func (b *AppBrand) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

func (b *AppBrand) ttl() time.Duration {
	if b.TTL > 0 {
		return b.TTL
	}
	return appBrandTTL
}

func (b *AppBrand) load(r *http.Request) shell.Branding {
	th := loadSigninTheme(b.Upstream, r, "")
	return shell.Branding{PlatformName: loadPlatformName(b.Upstream, r), Logo: th.shellBrand()}
}

// shellBrand is the sidebar logo of a theme: its logo URL (the kit refuses a
// data URL there, so an image data URL falls back to the bolt), else its SVG
// (the kit shows it as an image), else the bolt; at the theme's logo height,
// named by the brand name.
func (t signinTheme) shellBrand() kit.Brand {
	b := kit.Brand{Name: t.BrandName, LogoHeight: t.LogoHeight}
	switch {
	case t.LogoURL != "":
		if !strings.HasPrefix(t.LogoURL, "data:") {
			b.LogoURL = t.LogoURL
		}
	case t.LogoSVG != "":
		b.LogoSVG = t.LogoSVG
	}
	return b
}

// loadPlatformName reads the configurable platform name (GET
// /api/public/platform), "" when unavailable (the shell's default applies).
func loadPlatformName(upstream http.Handler, r *http.Request) string {
	if upstream == nil {
		return ""
	}
	resp := callUpstream(upstream, r, http.MethodGet, "/api/public/platform", nil)
	var body struct {
		PlatformName string `json:"platformName"`
	}
	if resp.Status != http.StatusOK || !resp.decode(&body) {
		return ""
	}
	if name := strings.TrimSpace(body.PlatformName); len(name) <= 200 {
		return name
	}
	return ""
}
