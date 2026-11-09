package server

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/framework/go/push"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/sdksync"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
)

// runningPushGateway is the gateway of this process, set by Run once it has started (the console
// is wired before it). Nil when the push component is not enabled here: the Push pages still read
// the database; only the test send needs it.
var runningPushGateway atomic.Pointer[pushgw.Gateway]

// gatewayPublisher is the Push pages' test send: through the gateway of this process.
type gatewayPublisher struct{}

func (gatewayPublisher) Available() bool { return runningPushGateway.Load() != nil }

func (gatewayPublisher) PublishTest(ctx context.Context, topic, clientID string) error {
	gw := runningPushGateway.Load()
	if gw == nil {
		return pushgw.ErrNoGateway
	}
	return gw.PublishTest(ctx, topic, clientID)
}

// pushConsole wires the Push pages (/ui/push/...). They read the gateway's tables of the platform
// database, so they work whether the gateway runs here, in another process, or not at all.
func pushConsole(pool *pgxpool.Pool, repos *repoSet) *pages.PushConsole {
	cfgRepo := pushgw.NewConfigRepo(pool)
	return &pages.PushConsole{
		Config:    cfgRepo,
		Settings:  sdksync.PushSettingsOps{Push: cfgRepo, Audit: repos.auditRepo},
		Apps:      repos.applicationRepo,
		Clients:   repos.clientRepo,
		Stats:     &pushgw.StatsRepo{Pool: pool},
		Log:       pushLogReader(pool, os.Getenv),
		Traces:    &pushgw.TraceRepo{Pool: pool},
		Publisher: gatewayPublisher{},
		Audit: func(ctx context.Context, ac *auth.AuthContext, appID, op string, detail map[string]any) {
			sdksync.AuditPushChange(ctx, repos.auditRepo, ac, appID, op, detail)
		},
	}
}

// pushLogReader reads the push log the way the gateway keeps it: the A1 table, or the A2 Redis
// stream when this process is configured with the gateway's driver and FC_REDIS_URL. Nil when the
// log is in Redis and this process cannot reach it.
func pushLogReader(pool *pgxpool.Pool, getenv func(string) string) pushgw.LogReader {
	if getenv("FC_PUSH_DRIVER") != string(push.DriverRedis) {
		return pushgw.PGLog{Pool: pool}
	}
	rc, err := pushgw.RedisClient(getenv("FC_REDIS_URL"))
	if err != nil {
		slog.Warn("push console: the push log is in Redis but cannot be reached from this process", "err", err)
		return nil
	}
	return pushgw.ModuleLog{Log: push.NewRedisStreamLog(rc, "")}
}
