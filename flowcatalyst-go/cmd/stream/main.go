// FlowCatalyst Stream Processor
//
// Standalone stream processor binary for production deployments.
// Watches MongoDB change streams and builds read-model projections.

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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/health"
	"go.flowcatalyst.tech/internal/config"
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

	slog.Info("Starting FlowCatalyst Stream Processor",
		"version", version,
		"build_time", buildTime,
		"component", "stream")

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

	slog.Info("Stream processor started")

	// Set up HTTP router for health/metrics only
	r := chi.NewRouter()
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

	// Stream processor status endpoint
	r.Get("/stream/status", func(w http.ResponseWriter, req *http.Request) {
		running := streamProcessor.IsRunning()
		watchers := streamProcessor.GetWatcherStatusMap()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"running":%v,"watchers":%d,"healthy":%v}`,
			running, len(watchers), running)
	})

	// Start HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

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

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server forced to shutdown", "error", err)
	}

	slog.Info("FlowCatalyst Stream Processor stopped")
}

// maskURI masks sensitive parts of a MongoDB URI for logging
func maskURI(uri string) string {
	if len(uri) > 20 {
		return uri[:20] + "..."
	}
	return uri
}
