package server

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
)

// The ECS task definitions set these deployment names; LoadEnv must honour
// them, with the FC_* name winning where one exists.
func TestLoadEnv_DeployedAliases(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		for _, k := range []string{
			"FC_API_PORT", "PORT",
			"FC_DISPATCH_PROCESSING_ENDPOINT", "DISPATCH_SCHEDULER_PROCESSING_ENDPOINT",
			"OIDC_SESSION_TTL", "FC_JWT_ACCESS_TOKEN_TTL_SECS", "OIDC_ACCESS_TOKEN_TTL", "OIDC_REFRESH_TOKEN_TTL",
		} {
			t.Setenv(k, "")
		}
		c := LoadEnv()
		if c.DispatchProcessingEndpoint != "http://localhost:8080/api/dispatch/process" {
			t.Errorf("DispatchProcessingEndpoint = %q", c.DispatchProcessingEndpoint)
		}
		if c.SessionTTLSecs != 86400 || c.AccessTokenTTLSecs != 3600 || c.RefreshTokenTTLSecs != 604800 {
			t.Errorf("TTLs = %d/%d/%d, want 86400/3600/604800", c.SessionTTLSecs, c.AccessTokenTTLSecs, c.RefreshTokenTTLSecs)
		}
	})

	t.Run("deployed names", func(t *testing.T) {
		t.Setenv("FC_DISPATCH_PROCESSING_ENDPOINT", "")
		t.Setenv("DISPATCH_SCHEDULER_PROCESSING_ENDPOINT", "http://fc-platform:8080/api/dispatch/process")
		t.Setenv("OIDC_SESSION_TTL", "28800")
		t.Setenv("FC_JWT_ACCESS_TOKEN_TTL_SECS", "")
		t.Setenv("OIDC_ACCESS_TOKEN_TTL", "1800")
		t.Setenv("OIDC_REFRESH_TOKEN_TTL", "2592000")
		c := LoadEnv()
		if c.DispatchProcessingEndpoint != "http://fc-platform:8080/api/dispatch/process" {
			t.Errorf("DispatchProcessingEndpoint = %q", c.DispatchProcessingEndpoint)
		}
		if c.SessionTTLSecs != 28800 || c.AccessTokenTTLSecs != 1800 || c.RefreshTokenTTLSecs != 2592000 {
			t.Errorf("TTLs = %d/%d/%d, want 28800/1800/2592000", c.SessionTTLSecs, c.AccessTokenTTLSecs, c.RefreshTokenTTLSecs)
		}
	})

	t.Run("FC name wins", func(t *testing.T) {
		t.Setenv("FC_DISPATCH_PROCESSING_ENDPOINT", "http://a/api/dispatch/process")
		t.Setenv("DISPATCH_SCHEDULER_PROCESSING_ENDPOINT", "http://b/api/dispatch/process")
		t.Setenv("FC_JWT_ACCESS_TOKEN_TTL_SECS", "600")
		t.Setenv("OIDC_ACCESS_TOKEN_TTL", "1800")
		c := LoadEnv()
		if c.DispatchProcessingEndpoint != "http://a/api/dispatch/process" {
			t.Errorf("DispatchProcessingEndpoint = %q", c.DispatchProcessingEndpoint)
		}
		if c.AccessTokenTTLSecs != 600 {
			t.Errorf("AccessTokenTTLSecs = %d, want 600", c.AccessTokenTTLSecs)
		}
	})

	t.Run("non-positive falls back", func(t *testing.T) {
		t.Setenv("OIDC_SESSION_TTL", "0")
		t.Setenv("FC_JWT_ACCESS_TOKEN_TTL_SECS", "-5")
		t.Setenv("OIDC_ACCESS_TOKEN_TTL", "")
		t.Setenv("OIDC_REFRESH_TOKEN_TTL", "nope")
		c := LoadEnv()
		if c.SessionTTLSecs != 86400 || c.AccessTokenTTLSecs != 3600 || c.RefreshTokenTTLSecs != 604800 {
			t.Errorf("TTLs = %d/%d/%d, want defaults", c.SessionTTLSecs, c.AccessTokenTTLSecs, c.RefreshTokenTTLSecs)
		}
	})
}

// The deployed router task sets NOTIFICATION_BATCH_INTERVAL and
// FLOWCATALYST_CONFIG_INTERVAL (in seconds); LoadEnv must honour both, with
// the FC_* name winning where set, and leave 0 (router default) when unset.
func TestLoadEnv_RouterIntervalAliases(t *testing.T) {
	t.Run("unset leaves router defaults", func(t *testing.T) {
		for _, k := range []string{
			"FC_NOTIFY_BATCH_INTERVAL_SECONDS", "NOTIFICATION_BATCH_INTERVAL",
			"FC_ROUTER_CONFIG_INTERVAL_SECONDS", "FLOWCATALYST_CONFIG_INTERVAL",
		} {
			t.Setenv(k, "")
		}
		c := LoadEnv()
		if c.RouterNotifyBatchIntervalSec != 0 || c.RouterConfigIntervalSec != 0 {
			t.Errorf("intervals = %d/%d, want 0/0", c.RouterNotifyBatchIntervalSec, c.RouterConfigIntervalSec)
		}
	})
	t.Run("deployed names", func(t *testing.T) {
		t.Setenv("FC_NOTIFY_BATCH_INTERVAL_SECONDS", "")
		t.Setenv("NOTIFICATION_BATCH_INTERVAL", "300")
		t.Setenv("FC_ROUTER_CONFIG_INTERVAL_SECONDS", "")
		t.Setenv("FLOWCATALYST_CONFIG_INTERVAL", "120")
		c := LoadEnv()
		if c.RouterNotifyBatchIntervalSec != 300 || c.RouterConfigIntervalSec != 120 {
			t.Errorf("intervals = %d/%d, want 300/120", c.RouterNotifyBatchIntervalSec, c.RouterConfigIntervalSec)
		}
	})
	t.Run("FC name wins", func(t *testing.T) {
		t.Setenv("FC_NOTIFY_BATCH_INTERVAL_SECONDS", "45")
		t.Setenv("NOTIFICATION_BATCH_INTERVAL", "300")
		t.Setenv("FC_ROUTER_CONFIG_INTERVAL_SECONDS", "60")
		t.Setenv("FLOWCATALYST_CONFIG_INTERVAL", "120")
		c := LoadEnv()
		if c.RouterNotifyBatchIntervalSec != 45 || c.RouterConfigIntervalSec != 60 {
			t.Errorf("intervals = %d/%d, want 45/60", c.RouterNotifyBatchIntervalSec, c.RouterConfigIntervalSec)
		}
	})
}

// The dispatch scheduler's engine sizing is overridable from the environment;
// unset, zero, negative or unparseable values keep the defaults.
func TestSchedulerConfig_EnvOverrides(t *testing.T) {
	keys := []string{"FC_SCHEDULER_BUFFER_CAPACITY", "FC_SCHEDULER_DISPATCHERS", "FC_SCHEDULER_BATCH_SIZE"}
	t.Run("defaults", func(t *testing.T) {
		for _, k := range keys {
			t.Setenv(k, "")
		}
		c := schedulerConfig(LoadEnv())
		if c.BufferCapacity != 1000 || c.Dispatchers != 10 || c.BatchSize != 500 || c.LaneBatch != 100 {
			t.Errorf("config = %d/%d/%d/%d, want 1000/10/500/100", c.BufferCapacity, c.Dispatchers, c.BatchSize, c.LaneBatch)
		}
	})
	t.Run("overrides", func(t *testing.T) {
		t.Setenv("FC_SCHEDULER_BUFFER_CAPACITY", "5000")
		t.Setenv("FC_SCHEDULER_DISPATCHERS", "24")
		t.Setenv("FC_SCHEDULER_BATCH_SIZE", "250")
		c := schedulerConfig(LoadEnv())
		if c.BufferCapacity != 5000 || c.Dispatchers != 24 || c.BatchSize != 250 {
			t.Errorf("config = %d/%d/%d, want 5000/24/250", c.BufferCapacity, c.Dispatchers, c.BatchSize)
		}
	})
	t.Run("bad values keep defaults", func(t *testing.T) {
		t.Setenv("FC_SCHEDULER_BUFFER_CAPACITY", "-1")
		t.Setenv("FC_SCHEDULER_DISPATCHERS", "0")
		t.Setenv("FC_SCHEDULER_BATCH_SIZE", "lots")
		c := schedulerConfig(LoadEnv())
		if c.BufferCapacity != 1000 || c.Dispatchers != 10 || c.BatchSize != 500 {
			t.Errorf("config = %d/%d/%d, want the defaults", c.BufferCapacity, c.Dispatchers, c.BatchSize)
		}
	})
}

func TestSchedulerPool_SizingAndOverride(t *testing.T) {
	scfg := scheduler.DefaultConfig()
	if got := schedulerPoolSize(EnvCfg{}, scfg); got != 12 {
		t.Errorf("default pool size = %d, want dispatchers+2 = 12", got)
	}
	scfg.Dispatchers = 24
	if got := schedulerPoolSize(EnvCfg{}, scfg); got != 26 {
		t.Errorf("pool size = %d, want 26", got)
	}
	if got := schedulerPoolSize(EnvCfg{SchedulerDBMaxConnections: 40}, scfg); got != 40 {
		t.Errorf("override pool size = %d, want 40", got)
	}
	t.Setenv("FC_SCHEDULER_DB_MAX_CONNECTIONS", "17")
	if got := schedulerPoolSize(LoadEnv(), scfg); got != 17 {
		t.Errorf("env override pool size = %d, want 17", got)
	}
	t.Setenv("FC_SCHEDULER_DB_MAX_CONNECTIONS", "-3")
	if got := schedulerPoolSize(LoadEnv(), scfg); got != 26 {
		t.Errorf("bad override pool size = %d, want the default 26", got)
	}
}

// The scheduler's pool copies the shared pool's connection settings and takes its
// own size; a disabled scheduler opens none. (pgxpool connects lazily, so no
// database is needed.)
func TestSchedulerPool_OwnPoolOnlyWhenEnabled(t *testing.T) {
	ctx := context.Background()
	shared, err := pgxpool.New(ctx, "postgres://u:p@127.0.0.1:1/db?pool_max_conns=50&pool_max_conn_lifetime=7m")
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()

	before := schedulerPoolsOpened.Load()
	var wg sync.WaitGroup
	if startSchedulerIfEnabled(ctx, &wg, shared, EnvCfg{SchedulerEnabled: false}, dispatch.Settings{}) {
		t.Fatal("a disabled scheduler must not start")
	}
	wg.Wait()
	if schedulerPoolsOpened.Load() != before {
		t.Error("a disabled scheduler opened a pool")
	}

	p, err := newSchedulerPool(ctx, shared, 12)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if p == shared {
		t.Fatal("the scheduler must get its own pool")
	}
	pc, sc := p.Config(), shared.Config()
	if pc.MaxConns != 12 || sc.MaxConns != 50 {
		t.Errorf("max conns scheduler/shared = %d/%d, want 12/50", pc.MaxConns, sc.MaxConns)
	}
	if pc.ConnConfig.Host != sc.ConnConfig.Host || pc.MaxConnLifetime != sc.MaxConnLifetime {
		t.Error("the scheduler pool must carry the shared pool's connection settings")
	}
	if p2, err := newSchedulerPool(ctx, nil, 12); p2 != nil || err != nil {
		t.Error("no shared pool (no database) means no scheduler pool")
	}
}
