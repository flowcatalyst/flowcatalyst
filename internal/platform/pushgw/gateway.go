// Package pushgw is the platform's push gateway (docs/push-gateway-spec.md): the framework's
// go/push module hosted inside fc-server, configured for many applications. The module is the
// framework's (hub, relay, log, stream tokens, wire); this package supplies what is the
// platform's: the database-backed providers of rules, ticket keys and limits (Store), the
// authenticator that maps platform credentials to the module's (Authenticator), the CORS rule
// for the push routes, and the revocation channels that end streams.
package pushgw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/flowcatalyst/framework/go/database"
	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/locks"
	"github.com/flowcatalyst/framework/go/locks/dblock"
	"github.com/flowcatalyst/framework/go/push"
)

// BasePath is where fc-server mounts the gateway's routes: POST /api/push/subscriptions,
// GET /api/push/stream and GET /api/push/keys. A reader's baseUrl is "{platform}/api".
const BasePath = "/api"

// Mount is the path pattern the gateway's handler serves on a chi router.
const Mount = BasePath + "/push/*"

// applicationName names the embedded framework application (its log lines and metrics).
const applicationName = "push-gateway"

// Config is what Start needs.
type Config struct {
	// Pool is the platform's pool: the providers' tables, the revocation channels.
	Pool *pgxpool.Pool
	// DatabaseURL is the platform database the framework application opens its own pool on (the
	// outbox, the A1 log, the relay lock).
	DatabaseURL string
	// Env holds the module's settings in the framework's environment names (PUSH_STREAM_SECRET,
	// PUSH_DRIVER, ...; see [Environment]).
	Env map[string]string
	// RedisURL is the Redis (or Valkey) server of the A2 driver, used when PUSH_DRIVER is redis.
	RedisURL string
	// Authenticator maps platform credentials to the module's. Required.
	Authenticator *Authenticator
	// Origins lists the allowed CORS origins; nil answers no CORS headers.
	Origins OriginSource
	// Logger is the gateway's logger (default slog.Default()).
	Logger *slog.Logger
	// Clock is for tests (default the system clock).
	Clock fw.Clock
}

// Gateway is the running gateway.
type Gateway struct {
	run     *fw.Running
	pushes  *push.Pushes
	store   *Store
	handler http.Handler
}

// Start builds and starts the gateway: the embedded framework application (configuration from
// cfg.Env, the platform database, locks on it), the module with the multi-application providers
// read from the platform tables, and the loops and health of the module. It returns once the hub
// follows the log. Mount [Gateway.Handler] at [Mount]; call [Gateway.Drain] when the HTTP server
// begins to shut down and [Gateway.Shutdown] after it has.
func Start(ctx context.Context, cfg Config) (*Gateway, error) {
	if cfg.Pool == nil || cfg.DatabaseURL == "" || cfg.Authenticator == nil {
		return nil, errors.New("pushgw: Start needs Pool, DatabaseURL and Authenticator")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	opts := []fw.Option{
		fw.WithEnvironment(cfg.Env),
		// no configuration files: the settings come from the environment only
		fw.WithConfigDir(filepath.Join(os.TempDir(), "flowcatalyst-push-gateway-no-config")),
		fw.WithLogger(log.With("component", "push-gateway")),
		fw.WithOutput(io.Discard, os.Stderr),
	}
	if cfg.Clock != nil {
		opts = append(opts, fw.WithClock(cfg.Clock))
	}
	app := fw.New(applicationName, opts...)
	ld := locks.Declare(app)
	// the platform's migration 104 owns the tables (fw_push_*, fw_lock): the module and the lock
	// store declare no migration sets here
	pd := push.DeclareWith(app, push.ExternalMigrations(), push.OptionalTicketKey())

	g := &Gateway{store: NewStore(cfg.Pool, log)}
	run, err := app.Start(ctx, func(ctx context.Context, b *fw.Boot) (fw.Serve, error) {
		db, err := database.Open(ctx, "main", cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		b.OnShutdown("push-gateway:db", func(context.Context) error { db.Close(); return nil })
		lk, err := locks.Build(ctx, b, ld, locks.WithStore("main", func(context.Context, *fw.Boot) (locks.Store, error) {
			return dblock.NewStore(db), nil
		}))
		if err != nil {
			return nil, err
		}
		if err := g.store.Reload(ctx); err != nil {
			return nil, fmt.Errorf("load rules and keys: %w", err)
		}
		popts := []push.Option{
			push.Authenticate(cfg.Authenticator),
			push.BasePath(BasePath),
			push.MultiApplication(g.store, g.store, push.AppLimitsFrom(g.store)),
		}
		if cfg.Env["PUSH_DRIVER"] == string(push.DriverRedis) {
			rc, err := redisClient(cfg.RedisURL)
			if err != nil {
				return nil, err
			}
			b.OnShutdown("push-gateway:redis", func(context.Context) error { return rc.Close() })
			popts = append(popts, push.RedisLog(func(context.Context, *fw.Boot, string) (push.Log, error) {
				return push.NewRedisStreamLog(rc, ""), nil
			}))
		}
		g.pushes, err = pd.Build(ctx, b, db, lk, popts...)
		if err != nil {
			return nil, err
		}
		h, err := g.pushes.Handler()
		if err != nil {
			return nil, err
		}
		g.handler = h

		// the caches follow the tables, and the revocation channels end streams, for as long as the
		// gateway runs
		loopCtx, stopLoops := context.WithCancel(context.WithoutCancel(ctx))
		loopsDone := make(chan struct{}, 2)
		var loopsStarted atomic.Bool
		b.OnStarted("push-gateway:loops", func(context.Context) error {
			loopsStarted.Store(true)
			go func() { g.store.run(loopCtx); loopsDone <- struct{}{} }()
			go func() { runEnds(loopCtx, log, cfg.Pool, g.pushes); loopsDone <- struct{}{} }()
			return nil
		})
		b.OnShutdown("push-gateway:loops", func(context.Context) error {
			stopLoops()
			if loopsStarted.Load() { // a start that failed before this action never ran them
				<-loopsDone
				<-loopsDone
			}
			return nil
		})
		return fw.ServeFunc(func(ctx context.Context) error { <-ctx.Done(); return nil }), nil
	})
	if err != nil {
		return nil, fmt.Errorf("push gateway: %w", err)
	}
	g.run = run

	if cfg.Origins != nil {
		g.handler = NewCORS(cfg.Origins).Wrap(g.handler)
	}
	return g, nil
}

func redisClient(url string) (*redis.Client, error) {
	if url == "" {
		return nil, errors.New("PUSH_DRIVER is redis but FC_REDIS_URL is not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("FC_REDIS_URL: %w", err)
	}
	return redis.NewClient(opts), nil
}

// Handler serves the gateway's routes under [BasePath], behind the CORS rule: mount it at [Mount]
// without stripping the prefix.
func (g *Gateway) Handler() http.Handler { return g.handler }

// Pushes returns the module, for publishing (PublishBatchAlone), tickets and EndStreams.
func (g *Gateway) Pushes() *push.Pushes { return g.pushes }

// Store returns the providers' cache (tests and the sync endpoint reload it).
func (g *Gateway) Store() *Store { return g.store }

// Ready reports whether the gateway is started and not shutting down: the hub follows the log.
func (g *Gateway) Ready() bool { return g.run.Ready() }

// Health runs the module's health checks.
func (g *Gateway) Health(ctx context.Context) fw.HealthReport { return g.run.Health(ctx) }

// Drain begins the drain: every open stream ends at once (reason drain, the clients reconnect to
// another task) and a subscribe or a listen is 503. Call it when the HTTP server begins to shut
// down, before it waits for its connections: a stream is a connection that never finishes.
func (g *Gateway) Drain() { g.pushes.Drain() }

// Shutdown stops the loops and closes what the gateway opened. Call it after the HTTP server has
// stopped.
func (g *Gateway) Shutdown() error { return g.run.Shutdown() }
