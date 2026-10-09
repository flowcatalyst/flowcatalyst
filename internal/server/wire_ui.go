package server

import (
	"context"
	"log/slog"
	"net/url"
	"runtime/debug"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui"
)

// registerUI mounts the server-rendered UI under /ui (internal/ui,
// docs/ui/README.md) beside the SPA, which stays the default. It reuses the
// API group's authentication exactly (the same Authenticator configuration,
// so the fc_session cookie and, in dev, the test headers behave the same) and
// adds the UI's own CSRF, session and role guards. Handlers read the
// repositories directly, with the API handlers' permission checks.
func registerUI(r chi.Router, cfg EnvCfg, repos *repoSet, svcs *serviceSet) error {
	return ui.Mount(r, ui.Config{
		Authenticate: platformmw.Authenticator(platformmw.AuthConfig{
			Provider:         svcs.authProvider,
			AllowTestHeaders: cfg.AuthAllowTestHeaders,
		}),
		// The session cookie's Secure flag follows the same switch at login.
		CookieSecure:   !cfg.AuthAllowTestHeaders,
		TrustedOrigins: publicOrigins(cfg.JWTIssuer),
		Version:        buildVersion(),
		DisplayName: func(ctx context.Context, principalID string) string {
			p, err := repos.principalRepo.FindByID(ctx, ids.PrincipalID(principalID))
			if err != nil || p == nil {
				return ""
			}
			if p.UserIdentity != nil {
				return p.UserIdentity.DisplayName()
			}
			return p.Name
		},
		DispatchPools: repos.dispatchPoolRepo,
	})
}

// buildVersion is the main module's version from the build info ("dev" for
// a local build).
func buildVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// publicOrigins trusts the configured public base URL (FC_JWT_ISSUER /
// FC_EXTERNAL_BASE_URL) for UI posts, for proxies that rewrite Host. A value
// that is not an http(s) URL is left out, with a warning, rather than
// failing the platform's boot over the UI.
func publicOrigins(base string) []string {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		slog.Warn("ui: public base URL is not an http(s) URL; /ui accepts same-origin posts only", "value", base)
		return nil
	}
	return []string{u.Scheme + "://" + u.Host}
}
