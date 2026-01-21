// FlowCatalyst Platform API
//
// Standalone platform API binary for production deployments.
// Provides control plane APIs, authentication, and admin functionality.
//
//	@title			FlowCatalyst Platform API
//	@version		1.0
//	@description	Control plane API for FlowCatalyst - multi-tenant event routing platform.
//
//	@contact.name	FlowCatalyst Support
//	@contact.url	https://flowcatalyst.tech/support
//	@contact.email	support@flowcatalyst.tech
//
//	@license.name	Proprietary
//	@license.url	https://flowcatalyst.tech/license
//
//	@host		localhost:8080
//	@BasePath	/api
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				JWT Bearer token. Format: "Bearer {token}"
//
//	@securityDefinitions.apikey	SessionCookie
//	@in							cookie
//	@name						FLOWCATALYST_SESSION
//	@description				Session cookie for browser-based authentication

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "go.flowcatalyst.tech/docs" // Swagger docs

	"go.flowcatalyst.tech/internal/common/health"
	"go.flowcatalyst.tech/internal/common/lifecycle"
	"go.flowcatalyst.tech/internal/platform/api"
	"go.flowcatalyst.tech/internal/platform/auth"
	"go.flowcatalyst.tech/internal/platform/auth/federation"
	"go.flowcatalyst.tech/internal/platform/auth/jwt"
	"go.flowcatalyst.tech/internal/platform/auth/oidc"
	"go.flowcatalyst.tech/internal/platform/auth/session"
	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/principal"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// Configure logging
	setupLogging()

	slog.Info("Starting FlowCatalyst Platform API",
		"version", version,
		"build_time", buildTime,
		"component", "platform")

	ctx := context.Background()

	// ========================================
	// 1. INFRASTRUCTURE INITIALIZATION
	// ========================================
	// Platform needs MongoDB for all storage
	app, cleanup, err := lifecycle.Initialize(ctx, lifecycle.AppOptions{
		NeedsMongoDB: true,
	})
	if err != nil {
		slog.Error("Failed to initialize", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	// ========================================
	// 2. COMPONENT WIRING
	// ========================================
	// Create components by passing ready infrastructure

	// Health checker
	healthChecker := health.NewChecker()
	healthChecker.AddReadinessCheck(health.MongoDBCheck(func() error {
		return app.MongoClient.Ping(ctx, nil)
	}))

	// API handlers
	apiHandlers := api.NewHandlers(app.MongoClient, app.DB, app.Config)

	// Auth services
	authService, discoveryHandler, err := setupAuthServices(app)
	if err != nil {
		slog.Error("Failed to initialize auth services", "error", err)
		os.Exit(1)
	}

	// HTTP Router
	httpRouter := setupHTTPRouter(app, healthChecker, apiHandlers, authService, discoveryHandler)

	// HTTP Server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.Config.HTTP.Port),
		Handler:      httpRouter,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ========================================
	// 3. SERVICE STARTUP
	// ========================================
	httpService := lifecycle.NewHTTPService("platform-api", httpServer)

	slog.Info("Platform API ready", "port", app.Config.HTTP.Port)

	// ========================================
	// 4. RUN UNTIL SHUTDOWN
	// ========================================
	if err := lifecycle.Run(ctx, httpService); err != nil {
		slog.Error("Service error", "error", err)
		os.Exit(1)
	}

	slog.Info("FlowCatalyst Platform API stopped")
}

// setupLogging configures the slog default logger.
func setupLogging() {
	logLevel := slog.LevelInfo
	if os.Getenv("FLOWCATALYST_DEV") == "true" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
}

// setupAuthServices initializes all authentication-related services.
func setupAuthServices(app *lifecycle.App) (*auth.AuthService, *oidc.DiscoveryHandler, error) {
	cfg := app.Config

	// Key manager
	keyManager := jwt.NewKeyManager()
	devKeyDir := cfg.DataDir
	if devKeyDir == "" {
		devKeyDir = "./data"
	}
	if err := keyManager.Initialize("", "", devKeyDir+"/keys"); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize key manager: %w", err)
	}

	// Token service
	tokenService := jwt.NewTokenService(keyManager, jwt.TokenServiceConfig{
		Issuer:             cfg.Auth.JWT.Issuer,
		AccessTokenExpiry:  cfg.Auth.JWT.AccessTokenExpiry,
		SessionTokenExpiry: cfg.Auth.JWT.SessionTokenExpiry,
		RefreshTokenExpiry: cfg.Auth.JWT.RefreshTokenExpiry,
		AuthCodeExpiry:     cfg.Auth.JWT.AuthorizationCodeExpiry,
	})

	// Session manager
	sessionManager := session.NewManager(session.Config{
		CookieName: cfg.Auth.Session.CookieName,
		Path:       "/",
		Domain:     "",
		MaxAge:     cfg.Auth.JWT.SessionTokenExpiry,
		Secure:     cfg.Auth.Session.Secure,
		SameSite:   http.SameSiteStrictMode,
	})

	// Federation service
	federationService := federation.NewService()

	// Repositories
	principalRepo := principal.NewRepository(app.DB)
	clientRepo := client.NewRepository(app.DB)
	oidcRepo := oidc.NewRepository(app.DB)

	// Auth service
	authService := auth.NewAuthService(
		principalRepo,
		clientRepo,
		oidcRepo,
		tokenService,
		sessionManager,
		federationService,
		cfg.Auth.ExternalBase,
	)

	// OIDC discovery handler
	discoveryHandler := oidc.NewDiscoveryHandler(keyManager, cfg.Auth.JWT.Issuer, cfg.Auth.ExternalBase)

	slog.Info("Auth service initialized")

	return authService, discoveryHandler, nil
}

// setupHTTPRouter creates the HTTP router with all routes and middleware.
func setupHTTPRouter(
	app *lifecycle.App,
	healthChecker *health.Checker,
	apiHandlers *api.Handlers,
	authService *auth.AuthService,
	discoveryHandler *oidc.DiscoveryHandler,
) http.Handler {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   app.Config.HTTP.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health endpoints
	r.Get("/q/health", healthChecker.HandleHealth)
	r.Get("/q/health/live", healthChecker.HandleLive)
	r.Get("/q/health/ready", healthChecker.HandleReady)

	// Swagger documentation
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Prometheus metrics
	r.Handle("/metrics", promhttp.Handler())
	r.Handle("/q/metrics", promhttp.Handler())

	// Mount API routes
	mountAPIRoutes(r, apiHandlers)

	// Mount Admin routes
	mountAdminRoutes(r, apiHandlers)

	// Auth endpoints
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", authService.HandleLogin)
		r.Post("/logout", authService.HandleLogout)
		r.Get("/me", authService.HandleMe)
		r.Post("/check-domain", authService.HandleCheckDomain)

		// OIDC Federation endpoints
		r.Get("/oidc/login", authService.HandleOIDCLogin)
		r.Get("/oidc/callback", authService.HandleOIDCCallback)
	})

	// OAuth/OIDC endpoints
	r.Get("/oauth/authorize", authService.HandleAuthorize)
	r.Post("/oauth/token", authService.HandleToken)

	// OIDC discovery endpoints
	r.Get("/.well-known/openid-configuration", discoveryHandler.HandleDiscovery)
	r.Get("/.well-known/jwks.json", discoveryHandler.HandleJWKS)

	return r
}

// mountAPIRoutes mounts the main API routes.
func mountAPIRoutes(r chi.Router, apiHandlers *api.Handlers) {
	r.Route("/api", func(r chi.Router) {
		// Events API
		r.Route("/events", func(r chi.Router) {
			r.Post("/", apiHandlers.CreateEvent)
			r.Post("/batch", apiHandlers.CreateEventBatch)
			r.Get("/{id}", apiHandlers.GetEvent)
		})

		// Event Types API
		r.Route("/event-types", func(r chi.Router) {
			r.Get("/", apiHandlers.ListEventTypes)
			r.Post("/", apiHandlers.CreateEventType)
			r.Get("/{id}", apiHandlers.GetEventType)
			r.Put("/{id}", apiHandlers.UpdateEventType)
			r.Delete("/{id}", apiHandlers.DeleteEventType)
		})

		// Subscriptions API
		r.Route("/subscriptions", func(r chi.Router) {
			r.Get("/", apiHandlers.ListSubscriptions)
			r.Post("/", apiHandlers.CreateSubscription)
			r.Get("/{id}", apiHandlers.GetSubscription)
			r.Put("/{id}", apiHandlers.UpdateSubscription)
			r.Delete("/{id}", apiHandlers.DeleteSubscription)
			r.Post("/{id}/pause", apiHandlers.PauseSubscription)
			r.Post("/{id}/resume", apiHandlers.ResumeSubscription)
		})

		// Dispatch Pools API
		r.Route("/dispatch-pools", func(r chi.Router) {
			r.Get("/", apiHandlers.ListDispatchPools)
			r.Post("/", apiHandlers.CreateDispatchPool)
			r.Get("/{id}", apiHandlers.GetDispatchPool)
			r.Put("/{id}", apiHandlers.UpdateDispatchPool)
			r.Delete("/{id}", apiHandlers.DeleteDispatchPool)
		})

		// Dispatch Jobs API
		r.Route("/dispatch/jobs", func(r chi.Router) {
			r.Post("/", apiHandlers.CreateDispatchJob)
			r.Post("/batch", apiHandlers.CreateDispatchJobBatch)
			r.Get("/", apiHandlers.SearchDispatchJobs)
			r.Get("/{id}", apiHandlers.GetDispatchJob)
			r.Get("/{id}/attempts", apiHandlers.GetDispatchJobAttempts)
		})

		// BFF APIs (read projections)
		r.Route("/bff", func(r chi.Router) {
			// Events BFF (read projections)
			r.Get("/events", apiHandlers.BFFSearchEvents)
			r.Get("/events/filter-options", apiHandlers.BFFEventFilterOptions)
			r.Get("/events/{id}", apiHandlers.BFFGetEvent)

			// Dispatch Jobs BFF (read projections)
			r.Get("/dispatch-jobs", apiHandlers.BFFSearchDispatchJobs)
			r.Get("/dispatch-jobs/filter-options", apiHandlers.BFFDispatchJobFilterOptions)
			r.Get("/dispatch-jobs/{id}", apiHandlers.BFFGetDispatchJob)

			// Event Types BFF
			r.Route("/event-types", func(r chi.Router) {
				r.Get("/", apiHandlers.BFFListEventTypes)
				r.Post("/", apiHandlers.BFFCreateEventType)
				r.Get("/filters/applications", apiHandlers.BFFEventTypeApplications)
				r.Get("/filters/subdomains", apiHandlers.BFFEventTypeSubdomains)
				r.Get("/filters/aggregates", apiHandlers.BFFEventTypeAggregates)
				r.Get("/{id}", apiHandlers.BFFGetEventType)
				r.Patch("/{id}", apiHandlers.BFFUpdateEventType)
				r.Post("/{id}/archive", apiHandlers.BFFArchiveEventType)
				r.Post("/{id}/schemas", apiHandlers.BFFAddEventTypeSchema)
				r.Post("/{id}/schemas/{version}/finalise", apiHandlers.BFFFinaliseEventTypeSchema)
				r.Post("/{id}/schemas/{version}/deprecate", apiHandlers.BFFDeprecateEventTypeSchema)
			})

			// Roles BFF
			r.Route("/roles", func(r chi.Router) {
				r.Get("/", apiHandlers.BFFListRoles)
				r.Post("/", apiHandlers.BFFCreateRole)
				r.Get("/filters/applications", apiHandlers.BFFRoleApplications)
				r.Get("/permissions", apiHandlers.BFFListPermissions)
				r.Get("/permissions/{code}", apiHandlers.BFFGetPermission)
				r.Get("/{id}", apiHandlers.BFFGetRole)
				r.Put("/{id}", apiHandlers.BFFUpdateRole)
				r.Delete("/{id}", apiHandlers.BFFDeleteRole)
			})

			// Debug endpoints (raw collections)
			r.Route("/debug", func(r chi.Router) {
				r.Get("/events", apiHandlers.BFFListRawEvents)
				r.Get("/events/{id}", apiHandlers.BFFGetRawEvent)
				r.Get("/dispatch-jobs", apiHandlers.BFFListRawDispatchJobs)
				r.Get("/dispatch-jobs/{id}", apiHandlers.BFFGetRawDispatchJob)
			})
		})
	})
}

// mountAdminRoutes mounts the admin API routes.
func mountAdminRoutes(r chi.Router, apiHandlers *api.Handlers) {
	r.Route("/api/admin/platform", func(r chi.Router) {
		// Clients
		r.Route("/clients", func(r chi.Router) {
			r.Get("/", apiHandlers.ListClients)
			r.Get("/search", apiHandlers.SearchClients)
			r.Get("/by-identifier/{identifier}", apiHandlers.GetClientByIdentifier)
			r.Post("/", apiHandlers.CreateClient)
			r.Get("/{id}", apiHandlers.GetClient)
			r.Put("/{id}", apiHandlers.UpdateClient)
			r.Post("/{id}/suspend", apiHandlers.SuspendClient)
			r.Post("/{id}/activate", apiHandlers.ActivateClient)
		})

		// Principals
		r.Route("/principals", func(r chi.Router) {
			r.Get("/", apiHandlers.ListPrincipals)
			r.Post("/", apiHandlers.CreatePrincipal)
			r.Get("/{id}", apiHandlers.GetPrincipal)
			r.Put("/{id}", apiHandlers.UpdatePrincipal)
			r.Post("/{id}/activate", apiHandlers.ActivatePrincipal)
			r.Post("/{id}/deactivate", apiHandlers.DeactivatePrincipal)
		})

		// Roles
		r.Route("/roles", func(r chi.Router) {
			r.Get("/", apiHandlers.ListRoles)
			r.Post("/", apiHandlers.CreateRole)
			// Nested permissions endpoints (matching Java reference API)
			r.Get("/permissions", apiHandlers.ListPermissions)
			r.Get("/permissions/{code}", apiHandlers.GetPermission)
			r.Get("/{id}", apiHandlers.GetRole)
			r.Put("/{id}", apiHandlers.UpdateRole)
			r.Delete("/{id}", apiHandlers.DeleteRole)
		})

		// Permissions
		r.Route("/permissions", func(r chi.Router) {
			r.Get("/", apiHandlers.ListPermissions)
		})

		// Applications
		r.Route("/applications", func(r chi.Router) {
			r.Get("/", apiHandlers.ListApplications)
			r.Post("/", apiHandlers.CreateApplication)
			r.Get("/by-code/{code}", apiHandlers.GetApplicationByCode)
			r.Get("/{id}", apiHandlers.GetApplication)
			r.Put("/{id}", apiHandlers.UpdateApplication)
			r.Post("/{id}/activate", apiHandlers.ActivateApplication)
			r.Post("/{id}/deactivate", apiHandlers.DeactivateApplication)
			r.Delete("/{id}", apiHandlers.DeleteApplication)
		})

		// Service Accounts
		r.Route("/service-accounts", func(r chi.Router) {
			r.Get("/", apiHandlers.ListServiceAccounts)
			r.Post("/", apiHandlers.CreateServiceAccount)
			r.Get("/{id}", apiHandlers.GetServiceAccount)
			r.Put("/{id}", apiHandlers.UpdateServiceAccount)
			r.Delete("/{id}", apiHandlers.DeleteServiceAccount)
			r.Post("/{id}/regenerate", apiHandlers.RegenerateServiceAccountCredentials)
		})

		// OAuth Clients
		r.Route("/oauth-clients", func(r chi.Router) {
			r.Get("/", apiHandlers.ListOAuthClients)
			r.Post("/", apiHandlers.CreateOAuthClient)
			r.Get("/by-client-id/{clientId}", apiHandlers.GetOAuthClientByClientID)
			r.Get("/{id}", apiHandlers.GetOAuthClient)
			r.Put("/{id}", apiHandlers.UpdateOAuthClient)
			r.Post("/{id}/rotate-secret", apiHandlers.RotateOAuthClientSecret)
			r.Post("/{id}/activate", apiHandlers.ActivateOAuthClient)
			r.Post("/{id}/deactivate", apiHandlers.DeactivateOAuthClient)
			r.Delete("/{id}", apiHandlers.DeleteOAuthClient)
		})

		// Audit Logs
		r.Route("/audit-logs", func(r chi.Router) {
			r.Get("/", apiHandlers.ListAuditLogs)
			r.Get("/entity-types", apiHandlers.GetAuditEntityTypes)
			r.Get("/operations", apiHandlers.GetAuditOperations)
			r.Get("/entity/{entityType}/{entityId}", apiHandlers.GetAuditLogsForEntity)
			r.Get("/{id}", apiHandlers.GetAuditLog)
		})

		// Auth Configs
		r.Route("/auth-configs", func(r chi.Router) {
			r.Get("/", apiHandlers.ListAuthConfigs)
			r.Post("/", apiHandlers.CreateAuthConfig)
			r.Get("/{id}", apiHandlers.GetAuthConfig)
			r.Put("/{id}", apiHandlers.UpdateAuthConfig)
			r.Delete("/{id}", apiHandlers.DeleteAuthConfig)
		})

		// Anchor Domains
		r.Route("/anchor-domains", func(r chi.Router) {
			r.Get("/", apiHandlers.ListAnchorDomains)
			r.Post("/", apiHandlers.CreateAnchorDomain)
			r.Delete("/{domain}", apiHandlers.DeleteAnchorDomain)
		})
	})
}
