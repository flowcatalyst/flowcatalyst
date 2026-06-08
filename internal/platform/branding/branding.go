// Package branding resolves the configurable platform name — the brand a user
// sees in emails, their authenticator app (TOTP issuer), passkey prompts, and
// the SPA. It reads a single platform-config row and falls back to the default
// ("Flowcatalyst") whenever the value is unset, blank, or unavailable, so every
// caller can stay unconditional.
package branding

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
)

// The config coordinate the platform name lives at:
// (app_code="platform", section="branding", property="platform-name", scope=GLOBAL).
const (
	DefaultPlatformName = "Flowcatalyst"
	App                 = "platform"
	Section             = "branding"
	Property            = "platform-name"
)

// Reader is the slice of the platform-config repository the lookup needs.
// *platformconfig.Repository satisfies it.
type Reader interface {
	FindByCoordinate(ctx context.Context, app, section, property string, scope platformconfig.Scope, clientID *string) (*platformconfig.Config, error)
}

// PlatformName returns the configured platform/brand name, or the default
// ("Flowcatalyst") when unset, blank, or unavailable. Safe with a nil reader.
func PlatformName(ctx context.Context, r Reader) string {
	if r == nil {
		return DefaultPlatformName
	}
	cfg, err := r.FindByCoordinate(ctx, App, Section, Property, platformconfig.ScopeGlobal, nil)
	if err != nil || cfg == nil {
		return DefaultPlatformName
	}
	if name := strings.TrimSpace(cfg.Value); name != "" {
		return name
	}
	return DefaultPlatformName
}

// Provider adapts a Reader into a context→name function, for injecting the
// resolver into services (mfa, notify) that shouldn't depend on platformconfig
// directly. Each call re-reads, so a name change takes effect without a restart.
func Provider(r Reader) func(context.Context) string {
	return func(ctx context.Context) string { return PlatformName(ctx, r) }
}
