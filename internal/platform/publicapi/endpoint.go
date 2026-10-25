// Package publicapi serves the small set of endpoints that must be
// reachable without an authenticated session:
//
//	GET /api/public/platform     — feature flags shown on the login page
//	GET /api/public/login-theme  — branded login-page theme (logo, colours, …)
//
// Both endpoints
// are read-only and intentionally low-privilege — the SPA hits them
// before the user signs in.
package publicapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/branding"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
)

// ClientIDResolver maps a tenant client identifier (the URL-safe slug on
// clients.identifier) to that client's ID. It returns "" when no such
// client exists. Modelled as a func rather than the client repository so
// this pre-auth package stays free of domain dependencies.
type ClientIDResolver func(ctx context.Context, identifier string) (string, error)

// Endpoint bundles the deps for the public API.
type Endpoint struct {
	// configs is branding.Reader rather than the concrete repository so
	// the theme layering can be exercised without a database.
	configs branding.Reader
	// resolveClient is optional; when nil, ?client= is ignored and every
	// caller receives the GLOBAL theme.
	resolveClient ClientIDResolver
	// ingestion is features.ingestion: whether the platform serves ingestions.
	ingestion bool
}

// New wires an Endpoint.
func New(configs branding.Reader) *Endpoint {
	return &Endpoint{configs: configs}
}

// WithIngestion reports whether the platform serves ingestions
// (FC_INGESTION_ENABLED), which the SPA reads as features.ingestion to show or
// hide its ingestion pages.
func (e *Endpoint) WithIngestion(enabled bool) *Endpoint {
	e.ingestion = enabled
	return e
}

// WithClientResolver enables per-client login branding: GET
// /api/public/login-theme?client=<identifier> then layers that client's
// CLIENT-scoped theme over the GLOBAL one.
func (e *Endpoint) WithClientResolver(resolve ClientIDResolver) *Endpoint {
	e.resolveClient = resolve
	return e
}

// RegisterRoutes mounts /api/public/platform and /api/public/login-theme
// on r. Callers MUST mount r outside any bearer-auth middleware.
func (e *Endpoint) RegisterRoutes(r chi.Router) {
	r.Get("/api/public/platform", e.handlePlatform)
	r.Get("/api/public/login-theme", e.handleLoginTheme)
	// SPA bootstrap path — same payload as /api/public/platform but at
	// the path the embedded frontend's platformConfig store fetches.
	r.Get("/api/config/platform", e.handlePlatform)
}

// platformResponse is the platform-info payload. Static today —
// future expansion adds env-driven flags.
type platformResponse struct {
	Features featuresResponse `json:"features"`
	// PlatformName is the configurable brand name, defaulting to "FlowCatalyst".
	// The SPA uses it for the document title and as the fallback brand.
	PlatformName string `json:"platformName"`
}

type featuresResponse struct {
	MessagingEnabled bool `json:"messagingEnabled"`
	Ingestion        bool `json:"ingestion"`
}

func (e *Endpoint) handlePlatform(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, platformResponse{
		Features:     featuresResponse{MessagingEnabled: true, Ingestion: e.ingestion},
		PlatformName: branding.PlatformName(r.Context(), e.configs),
	})
}

// loginThemeResponse is the login-theme payload. All fields
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

// handleLoginTheme serves the login-page theme, optionally branded for a
// tenant client. The client is named by ?client=<identifier> (the slug a
// relying party passes through /oauth/authorize) or, for the admin
// preview, by ?clientId=<id>. Both are cosmetic and unauthenticated: an
// unknown or malformed value silently yields the platform theme.
func (e *Endpoint) handleLoginTheme(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := strings.TrimSpace(q.Get("clientId"))
	if clientID == "" {
		if ident := strings.TrimSpace(q.Get("client")); ident != "" && e.resolveClient != nil {
			resolved, err := e.resolveClient(r.Context(), ident)
			if err != nil {
				// Branding must never take the login page down.
				slog.Warn("public login-theme: client lookup failed", "identifier", ident, "err", err)
			} else {
				clientID = resolved
			}
		}
	}
	writeJSON(w, http.StatusOK, e.loadLoginTheme(r.Context(), clientID))
}

// loadLoginTheme reads app_platform_configs at (app_code="platform",
// section="login", property="theme"), layering the CLIENT-scoped row for
// clientID (when non-empty) over the GLOBAL one.
//
// The layering is field-level and falls out of decoding both blobs into
// the same struct: a key the client blob omits keeps the platform value,
// while an explicit null clears it (the admin deleted that field). Every
// failure degrades to what has been decoded so far, so the SPA always
// gets a 200 + JSON object.
func (e *Endpoint) loadLoginTheme(ctx context.Context, clientID string) loginThemeResponse {
	var out loginThemeResponse
	if e.configs == nil {
		return out
	}
	e.decodeThemeInto(ctx, &out, platformconfig.ScopeGlobal, nil)
	if clientID != "" {
		e.decodeThemeInto(ctx, &out, platformconfig.ScopeClient, &clientID)
	}
	return out
}

// decodeThemeInto merges the theme row at the given scope into dst,
// leaving dst untouched on miss or malformed JSON.
func (e *Endpoint) decodeThemeInto(ctx context.Context, dst *loginThemeResponse, scope platformconfig.Scope, clientID *string) {
	cfg, err := e.configs.FindByCoordinate(ctx, "platform", "login", "theme", scope, clientID)
	if err != nil {
		slog.Warn("public login-theme: lookup failed", "scope", scope, "err", err)
		return
	}
	if cfg == nil || cfg.Value == "" {
		return
	}
	// Decode into a copy so a malformed blob can't leave dst holding the
	// half-applied fields json.Unmarshal had already written before failing.
	// The copy's pointers are fresh: json.Unmarshal writes through a non-nil
	// pointer, so a shallow copy would let the blob rewrite dst's values.
	merged := dst.clone()
	if err := json.Unmarshal([]byte(cfg.Value), &merged); err != nil {
		slog.Warn("public login-theme: stored value is not valid JSON", "scope", scope, "err", err)
		return
	}
	// The SPA shows logoSvg; a stored one the platform's rule refuses (it can be
	// written through the generic config API, which does not know the theme)
	// is never served.
	if merged.LogoSVG != nil && *merged.LogoSVG != "" {
		if err := branding.CheckLogoSVG(*merged.LogoSVG); err != nil {
			slog.Warn("public login-theme: stored logo SVG refused", "scope", scope, "err", err)
			merged.LogoSVG = dst.LogoSVG // what the wider scope had, if anything
		}
	}
	*dst = merged
}

// clone copies r with every field in a pointer of its own.
func (r loginThemeResponse) clone() loginThemeResponse {
	return loginThemeResponse{
		BrandName: clonePtr(r.BrandName), BrandSubtitle: clonePtr(r.BrandSubtitle),
		LogoURL: clonePtr(r.LogoURL), LogoSVG: clonePtr(r.LogoSVG), LogoHeight: clonePtr(r.LogoHeight),
		PrimaryColor: clonePtr(r.PrimaryColor), AccentColor: clonePtr(r.AccentColor),
		BackgroundColor: clonePtr(r.BackgroundColor), BackgroundGradient: clonePtr(r.BackgroundGradient),
		FooterText: clonePtr(r.FooterText), CustomCSS: clonePtr(r.CustomCSS),
	}
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
