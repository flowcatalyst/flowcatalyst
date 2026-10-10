// Package ui mounts the server-rendered UI under /ui: templ components from
// the framework's UI kit (internal/ui/kit), Datastar for interaction
// (internal/ui/ds), served next to the Vue SPA and switched over area by area
// (only with the owner's approval; until then the SPA stays the default).
// Architecture, conventions and the CSRF design: docs/ui/README.md.
package ui

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/assets"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// Config is what the UI needs from the platform wiring.
type Config struct {
	// Authenticate attaches the caller's AuthContext from the fc_session
	// cookie (the platform's Authenticator, configured as for /api).
	Authenticate func(http.Handler) http.Handler
	// CookieSecure must match the Secure flag the session cookie was set with
	// (sign-out clears it with the same attributes).
	CookieSecure bool
	// TrustedOrigins are extra origins allowed to post to /ui (the public base
	// URL when a proxy rewrites Host). Same-origin requests need nothing here.
	TrustedOrigins []string
	// Version is shown in the user menu.
	Version string
	// DisplayName resolves the signed-in principal's name (optional).
	DisplayName func(ctx context.Context, principalID string) string

	DispatchPools   pages.PoolStore
	DispatchPoolOps pages.PoolCommands
	// Clients backs the client select (search and resolution).
	Clients pages.ClientDirectory

	Connections     pages.ConnectionStore
	ConnectionOps   pages.ConnectionCommands
	ServiceAccounts pages.ServiceAccountDirectory

	EventTypes   pages.EventTypeStore
	EventTypeOps pages.EventTypeCommands
	Applications pages.EventTypeApps

	Subscriptions   pages.SubscriptionStore
	SubscriptionOps pages.SubscriptionCommands

	ApplicationStore pages.ApplicationStore
	ApplicationOps   pages.ApplicationCommands
	LoginClients     pages.LoginClientLookup
	AppDocs          pages.AppDocsLister

	ClientStore   pages.ClientStore
	ClientOps     pages.ClientCommands
	ClientConfigs pages.ClientConfigReader

	ScheduledJobs         pages.ScheduledJobStore
	ScheduledJobInstances pages.ScheduledJobInstanceStore
	ScheduledJobOps       pages.ScheduledJobCommands
}

// Mount registers /ui and everything under it on r.
func Mount(r chi.Router, cfg Config) error {
	sh := &shell.Shell{DisplayName: cfg.DisplayName, Version: cfg.Version, Flashes: pages.FlashMessages, CookieSecure: cfg.CookieSecure}
	csrf, err := guard.CSRF(cfg.TrustedOrigins, sh.Deny)
	if err != nil {
		return err
	}
	r.Route("/ui", func(r chi.Router) {
		// Static, public, long-cached: no session needed (the future login
		// page uses them too).
		r.Handle("/assets/*", assets.Handler())

		r.Group(func(r chi.Router) {
			r.Use(
				platformmw.CorrelationID,
				cfg.Authenticate,
				guard.SecurityHeaders,
				csrf,
				guard.RequireSession,
				guard.ProfileOnly(sh.Deny),
				withSprite,
			)
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/ui/dispatch-pools", http.StatusFound)
			})
			r.Post("/logout", signOut(cfg.CookieSecure))
			(&pages.DispatchPools{Pools: cfg.DispatchPools, Ops: cfg.DispatchPoolOps, Clients: cfg.Clients, Shell: sh}).Mount(r)
			(&pages.Connections{Connections: cfg.Connections, Ops: cfg.ConnectionOps, Clients: cfg.Clients, ServiceAccounts: cfg.ServiceAccounts, Shell: sh}).Mount(r)
			(&pages.EventTypes{Store: cfg.EventTypes, Ops: cfg.EventTypeOps, Apps: cfg.Applications, Shell: sh}).Mount(r)
			(&pages.Subscriptions{Subs: cfg.Subscriptions, Ops: cfg.SubscriptionOps, Pools: cfg.DispatchPools, Connections: cfg.Connections, EventTypes: cfg.EventTypes, Apps: cfg.Applications, Clients: cfg.Clients, Shell: sh}).Mount(r)
			(&pages.Applications{Apps: cfg.ApplicationStore, Ops: cfg.ApplicationOps, Logins: cfg.LoginClients, Docs: cfg.AppDocs, Shell: sh}).Mount(r)
			(&pages.ClientsPage{Clients: cfg.ClientStore, Ops: cfg.ClientOps, Apps: cfg.ApplicationStore, Configs: cfg.ClientConfigs, Shell: sh}).Mount(r)
			(&pages.ScheduledJobs{Jobs: cfg.ScheduledJobs, Instances: cfg.ScheduledJobInstances, Ops: cfg.ScheduledJobOps, Clients: cfg.ClientStore, Apps: cfg.ApplicationStore, Shell: sh}).Mount(r)
			(&pages.ClientOptions{Clients: cfg.Clients, Shell: sh}).Mount(r)
		})

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			guard.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sh.Deny(w, r, http.StatusNotFound, "There is no page here (yet). The current UI has everything that has not been ported.")
			})).ServeHTTP(w, r)
		})
	})
	return nil
}

// withSprite makes kit icons reference the served, content-hashed sprite.
func withSprite(next http.Handler) http.Handler {
	sprite := assets.URL("lucide-sprite.svg")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(kit.WithSpriteURL(r.Context(), sprite)))
	})
}

// signOut clears the session cookie (the same attributes /auth/logout uses)
// and sends the browser to the login page: a 303 after a native form post,
// a Datastar redirect after an action.
func signOut(secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     platformmw.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		if ds.IsDatastar(r) {
			_ = ds.NewSSE(w, r).Redirect(guard.LoginPath)
			return
		}
		http.Redirect(w, r, guard.LoginPath, http.StatusSeeOther)
	}
}
