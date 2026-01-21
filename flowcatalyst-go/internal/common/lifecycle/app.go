package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/config"
)

// App holds initialized infrastructure that is guaranteed to be connected.
// If you have an *App, you know the database is connected and ready.
//
// This is NOT a god object - it just holds the "dangerous" infrastructure
// that requires connection/retry logic. Application logic should NOT go here.
//
// Queue initialization is left to specific binaries since the configuration
// (publisher vs consumer, stream names, etc.) varies by use case.
type App struct {
	Config *config.Config

	// Database
	MongoClient *mongo.Client
	DB          *mongo.Database

	// Internal cleanup - call AddCleanup to register cleanup functions
	cleanupFuncs []func() error
}

// AppOptions configures which infrastructure to initialize.
type AppOptions struct {
	// NeedsMongoDB indicates MongoDB connection is required
	NeedsMongoDB bool
}

// Initialize creates an App with connected infrastructure.
// Returns an error if any required connection fails.
//
// Usage:
//
//	app, cleanup, err := lifecycle.Initialize(ctx, lifecycle.AppOptions{
//	    NeedsMongoDB: true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cleanup()
func Initialize(ctx context.Context, opts AppOptions) (*App, func(), error) {
	app := &App{}

	// Load configuration first
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}
	app.Config = cfg

	// Initialize MongoDB if needed
	if opts.NeedsMongoDB {
		if err := app.initMongoDB(ctx); err != nil {
			app.Cleanup()
			return nil, nil, err
		}
	}

	cleanup := func() {
		app.Cleanup()
	}

	return app, cleanup, nil
}

// AddCleanup registers a cleanup function to be called on shutdown.
// Functions are called in reverse order of registration.
func (app *App) AddCleanup(fn func() error) {
	app.cleanupFuncs = append(app.cleanupFuncs, fn)
}

// initMongoDB connects to MongoDB with retries.
func (app *App) initMongoDB(ctx context.Context) error {
	cfg := app.Config

	slog.Info("Connecting to MongoDB", "database", cfg.MongoDB.Database)

	clientOpts := options.Client().
		ApplyURI(cfg.MongoDB.URI).
		SetConnectTimeout(10 * time.Second).
		SetServerSelectionTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to verify connection
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		client.Disconnect(ctx)
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	app.MongoClient = client
	app.DB = client.Database(cfg.MongoDB.Database)

	app.AddCleanup(func() error {
		slog.Info("Disconnecting from MongoDB")
		return client.Disconnect(context.Background())
	})

	slog.Info("Connected to MongoDB", "database", cfg.MongoDB.Database)
	return nil
}

// Cleanup runs all cleanup functions in reverse order.
func (app *App) Cleanup() {
	for i := len(app.cleanupFuncs) - 1; i >= 0; i-- {
		if err := app.cleanupFuncs[i](); err != nil {
			slog.Error("Cleanup error", "error", err)
		}
	}
}
