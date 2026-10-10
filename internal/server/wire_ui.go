package server

import (
	"context"
	"log/slog"
	"net/url"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// registerUI mounts the server-rendered UI under /ui (internal/ui,
// docs/ui/README.md) beside the SPA, which stays the default. It reuses the
// API group's authentication exactly (the same Authenticator configuration,
// so the fc_session cookie and, in dev, the test headers behave the same) and
// adds the UI's own CSRF, session and role guards. Handlers read the
// repositories directly, with the API handlers' permission checks.
func registerUI(r chi.Router, cfg EnvCfg, pool *pgxpool.Pool, uow *usecasepgx.UnitOfWork, repos *repoSet, svcs *serviceSet) error {
	sjInstances := scheduledjob.NewInstanceRepository(pool)
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
			// The principal's name (what /auth/login returns), then the
			// identity's first/last name (which falls back to the email).
			if p.Name != "" {
				return p.Name
			}
			if p.UserIdentity != nil {
				return p.UserIdentity.DisplayName()
			}
			return ""
		},
		DispatchPools:    repos.dispatchPoolRepo,
		DispatchPoolOps:  pages.PoolOps{Repo: repos.dispatchPoolRepo, UoW: uow},
		Clients:          repos.clientRepo,
		Connections:      repos.connectionRepo,
		ConnectionOps:    pages.ConnectionOps{Repo: repos.connectionRepo, Apps: repos.applicationRepo, Subs: repos.subscriptionRepo, UoW: uow},
		ServiceAccounts:  repos.serviceAccountRepo,
		EventTypes:       repos.eventTypeRepo,
		EventTypeOps:     pages.EventTypeOps{Repo: repos.eventTypeRepo, UoW: uow},
		Applications:     repos.applicationRepo,
		Subscriptions:    repos.subscriptionRepo,
		SubscriptionOps:  pages.SubscriptionOps{Repo: repos.subscriptionRepo, UoW: uow},
		ApplicationStore: repos.applicationRepo,
		ApplicationOps: pages.ApplicationOps{
			Repo: repos.applicationRepo, ServiceAccounts: repos.serviceAccountRepo, Principals: repos.principalRepo,
			OAuthClients: repos.authRepo.OAuthClients, UoW: uow,
		},
		LoginClients: repos.authRepo.OAuthClients,
		AppDocs:      appdocs.NewRepository(pool),
		ClientStore:  repos.clientRepo,
		ClientOps: pages.ClientOps{
			Repo: repos.clientRepo, Apps: repos.applicationRepo, Configs: repos.applicationClientConfigRepo, UoW: uow,
		},
		ClientConfigs:         repos.applicationClientConfigRepo,
		ScheduledJobs:         repos.scheduledJobRepo,
		ScheduledJobInstances: sjInstances,
		ScheduledJobOps:       pages.ScheduledJobOps{Repo: repos.scheduledJobRepo, Instances: sjInstances, UoW: uow},
		DispatchJobs:          repos.dispatchJobRepo,
		DispatchJobOps:        pages.DispatchJobOps{Repo: repos.dispatchJobRepo, UoW: uow, Planner: dispatchPlanner(repos)},
		ClientIdentifier:      client.NewCachedIdentifierResolver(repos.clientRepo, time.Minute),
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
