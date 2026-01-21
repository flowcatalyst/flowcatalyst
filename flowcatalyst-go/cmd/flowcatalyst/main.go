// FlowCatalyst API
//
// High-performance event-driven message routing platform.
//
//	@title			FlowCatalyst API
//	@version		1.0
//	@description	Event-driven message routing platform with webhook delivery, FIFO ordering, and multi-tenant support.
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
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	_ "go.flowcatalyst.tech/docs" // Swagger docs

	"go.flowcatalyst.tech/internal/common/health"
	"go.flowcatalyst.tech/internal/config"
	"go.flowcatalyst.tech/internal/platform/api"
	"go.flowcatalyst.tech/internal/platform/auth"
	"go.flowcatalyst.tech/internal/platform/auth/federation"
	"go.flowcatalyst.tech/internal/platform/auth/jwt"
	"go.flowcatalyst.tech/internal/platform/auth/oidc"
	"go.flowcatalyst.tech/internal/platform/auth/session"
	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/principal"
	"go.flowcatalyst.tech/internal/queue"
	natsqueue "go.flowcatalyst.tech/internal/queue/nats"
	sqsqueue "go.flowcatalyst.tech/internal/queue/sqs"
	"go.flowcatalyst.tech/internal/router/manager"
	"go.flowcatalyst.tech/internal/router/mediator"
	"go.flowcatalyst.tech/internal/scheduler"
	"go.flowcatalyst.tech/internal/stream"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// Configure logging
	logLevel := slog.LevelInfo
	if os.Getenv("FLOWCATALYST_DEV") == "true" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	slog.Info("Starting FlowCatalyst",
		"version", version,
		"build_time", buildTime)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize health checker
	healthChecker := health.NewChecker()

	// Initialize MongoDB connection
	slog.Info("Connecting to MongoDB", "uri", maskURI(cfg.MongoDB.URI))
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDB.URI))
	if err != nil {
		slog.Error("Failed to connect to MongoDB", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := mongoClient.Disconnect(ctx); err != nil {
			slog.Error("Error disconnecting from MongoDB", "error", err)
		}
	}()

	// Ping MongoDB to verify connection
	if err := mongoClient.Ping(ctx, nil); err != nil {
		slog.Error("Failed to ping MongoDB", "error", err)
		os.Exit(1)
	}
	slog.Info("Connected to MongoDB", "database", cfg.MongoDB.Database)

	// Add MongoDB health check
	healthChecker.AddReadinessCheck(health.MongoDBCheck(func() error {
		return mongoClient.Ping(ctx, nil)
	}))

	// Initialize queue based on configuration
	var queuePublisher queue.Publisher
	var queueConsumer queue.Consumer
	var queueCloser func() error

	switch cfg.Queue.Type {
	case "embedded":
		slog.Info("Starting embedded NATS server")
		natsCfg := natsqueue.DefaultEmbeddedConfig()
		natsCfg.DataDir = cfg.Queue.NATS.DataDir
		if cfg.DataDir != "" {
			natsCfg.DataDir = cfg.DataDir + "/nats"
		}

		embeddedNATS, err := natsqueue.NewEmbeddedServer(natsCfg)
		if err != nil {
			slog.Error("Failed to start embedded NATS server", "error", err)
			os.Exit(1)
		}
		queueCloser = embeddedNATS.Close

		// Get publisher from embedded server
		queuePublisher = embeddedNATS.Publisher()

		// Create consumer
		consumer, err := embeddedNATS.CreateConsumer(ctx, "dispatch-consumer", "dispatch.>", nil)
		if err != nil {
			slog.Error("Failed to create NATS consumer", "error", err)
			os.Exit(1)
		}
		queueConsumer = consumer

		// Add NATS health check
		healthChecker.AddReadinessCheck(health.NATSCheck(func() bool {
			return embeddedNATS.Connection().IsConnected()
		}))

		slog.Info("Embedded NATS server started")

	case "nats":
		slog.Info("Connecting to external NATS server", "url", cfg.Queue.NATS.URL)
		natsClient, err := natsqueue.NewClient(&queue.NATSConfig{
			URL:        cfg.Queue.NATS.URL,
			StreamName: "DISPATCH",
		})
		if err != nil {
			slog.Error("Failed to connect to NATS server", "error", err)
			os.Exit(1)
		}
		queueCloser = natsClient.Close

		queuePublisher = natsClient.Publisher()

		consumer, err := natsClient.CreateConsumer(ctx, "dispatch-consumer", "dispatch.>")
		if err != nil {
			slog.Error("Failed to create NATS consumer", "error", err)
			os.Exit(1)
		}
		queueConsumer = consumer

		// Add NATS health check - external NATS is connected if consumer was created
		healthChecker.AddReadinessCheck(health.NATSCheck(func() bool {
			return true // Consumer creation would have failed if not connected
		}))

		slog.Info("Connected to external NATS server")

	case "sqs":
		slog.Info("Connecting to AWS SQS",
			"region", cfg.Queue.SQS.Region,
			"queueURL", cfg.Queue.SQS.QueueURL)

		sqsCfg := &queue.SQSConfig{
			QueueURL:            cfg.Queue.SQS.QueueURL,
			Region:              cfg.Queue.SQS.Region,
			WaitTimeSeconds:     int32(cfg.Queue.SQS.WaitTimeSeconds),
			VisibilityTimeout:   int32(cfg.Queue.SQS.VisibilityTimeout),
			MaxNumberOfMessages: 10,
		}

		sqsClient, err := sqsqueue.NewClient(ctx, sqsCfg)
		if err != nil {
			slog.Error("Failed to create SQS client", "error", err)
			os.Exit(1)
		}
		queueCloser = sqsClient.Close

		queuePublisher = sqsClient.Publisher()

		consumer, err := sqsClient.CreateConsumer(ctx, "dispatch-consumer", "")
		if err != nil {
			slog.Error("Failed to create SQS consumer", "error", err)
			os.Exit(1)
		}
		queueConsumer = consumer

		// Add SQS health check
		healthChecker.AddReadinessCheck(health.SQSCheck(func() error {
			checkCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return sqsClient.HealthCheck(checkCtx)
		}))

		slog.Info("Connected to AWS SQS")

	default:
		slog.Error("Unknown queue type", "type", cfg.Queue.Type)
		os.Exit(1)
	}

	// Ensure queue is closed on shutdown
	if queueCloser != nil {
		defer func() {
			if err := queueCloser(); err != nil {
				slog.Error("Error closing queue", "error", err)
			}
		}()
	}

	// Initialize database reference
	db := mongoClient.Database(cfg.MongoDB.Database)

	// Initialize stream processor
	streamCfg := stream.DefaultProcessorConfig()
	streamCfg.Database = cfg.MongoDB.Database
	streamProcessor := stream.NewProcessor(mongoClient, streamCfg)

	// Create indexes for projections
	if err := streamProcessor.EnsureIndexes(ctx); err != nil {
		slog.Warn("Failed to ensure projection indexes", "error", err)
	}

	// Start stream processor
	if err := streamProcessor.Start(); err != nil {
		slog.Error("Failed to start stream processor", "error", err)
		os.Exit(1)
	}
	defer streamProcessor.Stop()

	// Add stream processor health check
	healthChecker.AddReadinessCheck(streamProcessor.HealthCheck())

	// Initialize dispatch scheduler
	schedulerCfg := scheduler.DefaultSchedulerConfig()
	schedulerCfg.Database = cfg.MongoDB.Database
	dispatchScheduler := scheduler.NewScheduler(mongoClient, queuePublisher, schedulerCfg)
	dispatchScheduler.Start()
	defer dispatchScheduler.Stop()

	// Initialize message router
	mediatorCfg := mediator.DefaultHTTPMediatorConfig()
	messageRouter := manager.NewRouter(queueConsumer, mediatorCfg)
	messageRouter.Start()
	defer messageRouter.Stop()

	// Initialize API handlers
	apiHandlers := api.NewHandlers(mongoClient, db, cfg)

	// Initialize Auth Service
	keyManager := jwt.NewKeyManager()
	devKeyDir := cfg.DataDir
	if devKeyDir == "" {
		devKeyDir = "./data"
	}
	if err := keyManager.Initialize("", "", devKeyDir+"/keys"); err != nil {
		slog.Error("Failed to initialize key manager", "error", err)
		os.Exit(1)
	}

	tokenService := jwt.NewTokenService(keyManager, jwt.TokenServiceConfig{
		Issuer:             cfg.Auth.JWT.Issuer,
		AccessTokenExpiry:  1 * time.Hour,
		SessionTokenExpiry: 8 * time.Hour,
		RefreshTokenExpiry: 30 * 24 * time.Hour,
		AuthCodeExpiry:     10 * time.Minute,
	})

	sessionManager := session.NewManager(session.Config{
		CookieName: cfg.Auth.Session.CookieName,
		Path:       "/",
		Domain:     "",
		MaxAge:     8 * time.Hour,
		Secure:     cfg.Auth.Session.Secure,
		SameSite:   http.SameSiteStrictMode,
	})

	federationService := federation.NewService()

	principalRepo := principal.NewRepository(db)
	clientRepo := client.NewRepository(db)
	oidcRepo := oidc.NewRepository(db)

	authService := auth.NewAuthService(
		principalRepo,
		clientRepo,
		oidcRepo,
		tokenService,
		sessionManager,
		federationService,
		cfg.Auth.ExternalBase,
	)

	// Create OIDC discovery handler
	discoveryHandler := oidc.NewDiscoveryHandler(keyManager, cfg.Auth.JWT.Issuer, cfg.Auth.ExternalBase)

	slog.Info("Auth service initialized")

	// Set up HTTP router
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.HTTP.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health endpoints (Quarkus-compatible paths)
	r.Get("/q/health", healthChecker.HandleHealth)
	r.Get("/q/health/live", healthChecker.HandleLive)
	r.Get("/q/health/ready", healthChecker.HandleReady)

	// Swagger documentation
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Prometheus metrics endpoint
	r.Handle("/metrics", promhttp.Handler())
	r.Handle("/q/metrics", promhttp.Handler()) // Quarkus-compatible path

	// Mount API routes
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
			r.Get("/events", apiHandlers.BFFSearchEvents)
			r.Get("/events/filter-options", apiHandlers.BFFEventFilterOptions)
			r.Get("/events/{id}", apiHandlers.BFFGetEvent)

			r.Get("/dispatch-jobs", apiHandlers.BFFSearchDispatchJobs)
			r.Get("/dispatch-jobs/filter-options", apiHandlers.BFFDispatchJobFilterOptions)
			r.Get("/dispatch-jobs/{id}", apiHandlers.BFFGetDispatchJob)
		})
	})

	// Admin API routes
	r.Route("/api/admin/platform", func(r chi.Router) {
		// Clients
		r.Route("/clients", func(r chi.Router) {
			r.Get("/", apiHandlers.ListClients)
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
			r.Get("/{id}", apiHandlers.GetApplication)
			r.Put("/{id}", apiHandlers.UpdateApplication)
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
			r.Get("/{id}", apiHandlers.GetOAuthClient)
			r.Put("/{id}", apiHandlers.UpdateOAuthClient)
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

	// Auth endpoints (using real auth service)
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

	// OIDC discovery endpoints (using real handlers)
	r.Get("/.well-known/openid-configuration", discoveryHandler.HandleDiscovery)
	r.Get("/.well-known/jwks.json", discoveryHandler.HandleJWKS)

	// Start HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("HTTP server starting", "port", cfg.HTTP.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down gracefully...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server forced to shutdown", "error", err)
	}

	slog.Info("FlowCatalyst stopped")
}

// maskURI masks sensitive parts of a MongoDB URI for logging
func maskURI(uri string) string {
	// Simple masking - in production, use proper URI parsing
	if len(uri) > 20 {
		return uri[:20] + "..."
	}
	return uri
}
