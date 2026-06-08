// Package publicapi serves the small set of endpoints that must be
// reachable without an authenticated session:
//
//	GET /api/public/platform     — feature flags shown on the login page
//	GET /api/public/login-theme  — branded login-page theme (logo, colours, …)
//
// Mirrors crates/fc-platform/src/shared/public_api.rs. Both endpoints
// are read-only and intentionally low-privilege — the SPA hits them
// before the user signs in.
package publicapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/branding"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
)

// Endpoint bundles the deps for the public API.
type Endpoint struct {
	configs *platformconfig.Repository
}

// New wires an Endpoint.
func New(configs *platformconfig.Repository) *Endpoint {
	return &Endpoint{configs: configs}
}

// RegisterRoutes mounts /api/public/platform and /api/public/login-theme
// on r. Callers MUST mount r outside any bearer-auth middleware.
func (e *Endpoint) RegisterRoutes(r chi.Router) {
	r.Get("/api/public/platform", e.handlePlatform)
	r.Get("/api/public/login-theme", e.handleLoginTheme)
	// SPA bootstrap path — same payload as /api/public/platform but at
	// the path the embedded frontend's platformConfig store fetches.
	// Mirrors Rust's `/api/config/platform` (platform_config_router).
	r.Get("/api/config/platform", e.handlePlatform)
}

// platformResponse mirrors Rust's PlatformInfoResponse. Static today —
// future expansion adds env-driven flags.
type platformResponse struct {
	Features featuresResponse `json:"features"`
	// PlatformName is the configurable brand name, defaulting to "Flowcatalyst".
	// The SPA uses it for the document title and as the fallback brand.
	PlatformName string `json:"platformName"`
}

type featuresResponse struct {
	MessagingEnabled bool `json:"messagingEnabled"`
}

func (e *Endpoint) handlePlatform(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, platformResponse{
		Features:     featuresResponse{MessagingEnabled: true},
		PlatformName: branding.PlatformName(r.Context(), e.configs),
	})
}

// loginThemeResponse mirrors Rust's LoginThemeResponse. All fields
// optional; the SPA falls back to its own defaults when absent.
type loginThemeResponse struct {
	BrandName          *string `json:"brandName,omitempty"`
	BrandSubtitle      *string `json:"brandSubtitle,omitempty"`
	LogoURL            *string `json:"logoUrl,omitempty"`
	LogoSVG            *string `json:"logoSvg,omitempty"`
	LogoHeight         *uint32 `json:"logoHeight,omitempty"`
	PrimaryColor       *string `json:"primaryColor,omitempty"`
	AccentColor        *string `json:"accentColor,omitempty"`
	BackgroundColor    *string `json:"backgroundColor,omitempty"`
	BackgroundGradient *string `json:"backgroundGradient,omitempty"`
	FooterText         *string `json:"footerText,omitempty"`
	CustomCSS          *string `json:"customCss,omitempty"`
}

func (e *Endpoint) handleLoginTheme(w http.ResponseWriter, r *http.Request) {
	theme := loadLoginTheme(r.Context(), e.configs)
	writeJSON(w, http.StatusOK, theme)
}

// loadLoginTheme reads app_platform_configs at
// (app_code="platform", section="login", property="theme", scope="GLOBAL").
// Returns an empty theme on miss or parse error so the SPA always gets
// a 200 + JSON object (matching the Rust behaviour).
func loadLoginTheme(ctx context.Context, configs *platformconfig.Repository) loginThemeResponse {
	if configs == nil {
		return loginThemeResponse{}
	}
	cfg, err := configs.FindByCoordinate(ctx, "platform", "login", "theme", platformconfig.ScopeGlobal, nil)
	if err != nil {
		slog.Warn("public login-theme: lookup failed", "err", err)
		return loginThemeResponse{}
	}
	if cfg == nil || cfg.Value == "" {
		return loginThemeResponse{}
	}
	var out loginThemeResponse
	if err := json.Unmarshal([]byte(cfg.Value), &out); err != nil {
		slog.Warn("public login-theme: stored value is not valid JSON", "err", err)
		return loginThemeResponse{}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
