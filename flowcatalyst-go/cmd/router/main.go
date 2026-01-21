// FlowCatalyst Message Router
//
// Standalone message router binary for production deployments.
// Consumes messages from queue (NATS/SQS) and delivers via HTTP mediation.

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
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.flowcatalyst.tech/internal/common/health"
	"go.flowcatalyst.tech/internal/common/lifecycle"
	"go.flowcatalyst.tech/internal/config"
	"go.flowcatalyst.tech/internal/queue"
	natsqueue "go.flowcatalyst.tech/internal/queue/nats"
	sqsqueue "go.flowcatalyst.tech/internal/queue/sqs"
	"go.flowcatalyst.tech/internal/router/manager"
	"go.flowcatalyst.tech/internal/router/mediator"
	"go.flowcatalyst.tech/internal/router/standby"
	"go.flowcatalyst.tech/internal/router/warning"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// Configure logging
	setupLogging()

	slog.Info("Starting FlowCatalyst Message Router",
		"version", version,
		"build_time", buildTime,
		"component", "router")

	ctx := context.Background()

	// ========================================
	// 1. INFRASTRUCTURE INITIALIZATION
	// ========================================
	// Router doesn't need MongoDB, just config
	app, cleanup, err := lifecycle.Initialize(ctx, lifecycle.AppOptions{
		NeedsMongoDB: false,
	})
	if err != nil {
		slog.Error("Failed to initialize", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	// ========================================
	// 2. QUEUE SETUP
	// ========================================
	queueConsumer, queueHealthCheck, err := setupQueue(ctx, app)
	if err != nil {
		slog.Error("Failed to setup queue", "error", err)
		os.Exit(1)
	}

	// ========================================
	// 3. COMPONENT WIRING
	// ========================================
	// Create components by passing ready infrastructure

	// Health checker
	healthChecker := health.NewChecker()
	healthChecker.AddReadinessCheck(queueHealthCheck)

	// Message router
	mediatorCfg := mediator.DefaultHTTPMediatorConfig()
	messageRouter := manager.NewRouter(queueConsumer, mediatorCfg)
	routerService := manager.NewRouterService(messageRouter)

	// Standby service for leader election
	standbyService := setupStandbyService(app.Config, routerService)

	// Warning service
	warningService := warning.NewInMemoryService()
	warningHandler := warning.NewHandler(warningService)

	// HTTP Router
	httpRouter := setupHTTPRouter(healthChecker, standbyService, warningHandler)

	// HTTP Server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.Config.HTTP.Port),
		Handler:      httpRouter,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ========================================
	// 4. SERVICE STARTUP
	// ========================================
	// Build the service list based on configuration
	var services []lifecycle.Service

	// HTTP service (always runs)
	httpService := lifecycle.NewHTTPService("http-server", httpServer)
	services = append(services, httpService)

	// Standby service wraps router lifecycle when leader election is enabled
	if app.Config.Leader.Enabled {
		standbyServiceWrapper := newStandbyServiceWrapper(standbyService)
		services = append(services, standbyServiceWrapper)
	} else {
		// No leader election - run router directly
		services = append(services, routerService)
	}

	slog.Info("Router ready",
		"port", app.Config.HTTP.Port,
		"queueType", app.Config.Queue.Type,
		"leaderElection", app.Config.Leader.Enabled)

	// ========================================
	// 5. RUN UNTIL SHUTDOWN
	// ========================================
	if err := lifecycle.Run(ctx, services...); err != nil {
		slog.Error("Service error", "error", err)
		os.Exit(1)
	}

	slog.Info("FlowCatalyst Message Router stopped")
}

// setupLogging configures the slog default logger.
func setupLogging() {
	logLevel := slog.LevelInfo
	if os.Getenv("FLOWCATALYST_DEV") == "true" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
}

// setupQueue initializes the queue consumer based on configuration.
// Returns the consumer, a health check function, and any error.
func setupQueue(ctx context.Context, app *lifecycle.App) (queue.Consumer, health.CheckFunc, error) {
	cfg := app.Config

	switch cfg.Queue.Type {
	case "nats":
		return setupNATSQueue(ctx, app)
	case "sqs":
		return setupSQSQueue(ctx, app)
	default:
		return nil, nil, fmt.Errorf("unknown queue type: %s (use 'nats' or 'sqs')", cfg.Queue.Type)
	}
}

func setupNATSQueue(ctx context.Context, app *lifecycle.App) (queue.Consumer, health.CheckFunc, error) {
	cfg := app.Config

	slog.Info("Connecting to NATS server", "url", cfg.Queue.NATS.URL)

	natsClient, err := natsqueue.NewClient(&queue.NATSConfig{
		URL:        cfg.Queue.NATS.URL,
		StreamName: "DISPATCH",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Register cleanup
	app.AddCleanup(func() error {
		slog.Info("Disconnecting from NATS")
		return natsClient.Close()
	})

	consumer, err := natsClient.CreateConsumer(ctx, "router-consumer", "dispatch.>")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create NATS consumer: %w", err)
	}

	healthCheck := health.NATSCheck(func() bool {
		return true // NATS client doesn't expose connection state easily
	})

	slog.Info("Connected to NATS server")
	return consumer, healthCheck, nil
}

func setupSQSQueue(ctx context.Context, app *lifecycle.App) (queue.Consumer, health.CheckFunc, error) {
	cfg := app.Config

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
		return nil, nil, fmt.Errorf("failed to create SQS client: %w", err)
	}

	// Register cleanup
	app.AddCleanup(func() error {
		slog.Info("Disconnecting from SQS")
		return sqsClient.Close()
	})

	consumer, err := sqsClient.CreateConsumer(ctx, "router-consumer", "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create SQS consumer: %w", err)
	}

	healthCheck := health.SQSCheck(func() error {
		checkCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return sqsClient.HealthCheck(checkCtx)
	})

	slog.Info("Connected to AWS SQS")
	return consumer, healthCheck, nil
}

// setupStandbyService configures leader election.
func setupStandbyService(cfg *config.Config, routerService *manager.RouterService) *standby.Service {
	standbyCfg := &standby.Config{
		Enabled:         cfg.Leader.Enabled,
		InstanceID:      cfg.Leader.InstanceID,
		LockKey:         "flowcatalyst:router:leader",
		LockTTL:         cfg.Leader.TTL,
		RefreshInterval: cfg.Leader.RefreshInterval,
	}

	callbacks := &standby.Callbacks{
		OnBecomePrimary: func() {
			slog.Info("Became PRIMARY - starting message processing")
			routerService.Resume()
		},
		OnBecomeStandby: func() {
			slog.Info("Became STANDBY - stopping message processing")
			routerService.Pause()
		},
	}

	return standby.NewService(standbyCfg, callbacks)
}

// setupHTTPRouter creates the HTTP router with health/metrics endpoints.
func setupHTTPRouter(healthChecker *health.Checker, standbyService *standby.Service, warningHandler *warning.Handler) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Health endpoints
	r.Get("/q/health", healthChecker.HandleHealth)
	r.Get("/q/health/live", healthChecker.HandleLive)
	r.Get("/q/health/ready", healthChecker.HandleReady)

	// Prometheus metrics
	r.Handle("/metrics", promhttp.Handler())
	r.Handle("/q/metrics", promhttp.Handler())

	// Standby status endpoint
	r.Get("/router/status", func(w http.ResponseWriter, req *http.Request) {
		status := standbyService.GetStatus()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"role":"%s","instanceId":"%s","standbyEnabled":%v}`,
			standbyService.GetRole(), standbyService.GetInstanceID(), status.StandbyEnabled)
	})

	// Warning endpoints
	warningHandler.RegisterRoutes(r)

	return r
}

// standbyServiceWrapper wraps standby.Service to implement lifecycle.Service.
type standbyServiceWrapper struct {
	service *standby.Service
}

func newStandbyServiceWrapper(svc *standby.Service) *standbyServiceWrapper {
	return &standbyServiceWrapper{service: svc}
}

func (s *standbyServiceWrapper) Name() string { return "standby-service" }

func (s *standbyServiceWrapper) Start(ctx context.Context) error {
	if err := s.service.Start(); err != nil {
		return err
	}
	// Block until context cancelled
	<-ctx.Done()
	return nil
}

func (s *standbyServiceWrapper) Stop(ctx context.Context) error {
	s.service.Stop()
	return nil
}

func (s *standbyServiceWrapper) Health() error {
	return nil
}
