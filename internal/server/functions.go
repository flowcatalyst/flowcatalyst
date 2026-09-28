package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runner"
	fcauth "github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/auth"
)

// FunctionRunnerConfig configures the function-runner subsystem
// (docs/function-runner-plan.md §3, §7). It is read from the environment by
// LoadFunctionRunnerEnv; fc-dev builds its own with in-process credentials.
type FunctionRunnerConfig struct {
	Enabled bool
	// Bind is the listen address for both entries.
	Bind string
	// Port is the private entry (/fn/…); PublicPort the public entry
	// (Host routing), 0 = off.
	Port, PublicPort int
	Pool             string
	// PlatformURL is where the control plane and token endpoint live.
	PlatformURL              string
	ClientID, ClientSecret   string
	MemoryLimitMB, ReserveMB int
	CacheDir                 string
	IdleEvict                time.Duration
	TrustedProxies           []netip.Prefix
	// SetMemoryLimit sets GOMEMLIMIT to the reserve: only when the runner is
	// the process's only subsystem (guest memory lives outside the Go heap).
	SetMemoryLimit bool
}

// DefaultFunctionRunnerPort is the private entry's default port.
const DefaultFunctionRunnerPort = 8095

// LoadFunctionRunnerEnv reads the runner's FC_FUNCTIONS_* variables. The
// platform URL defaults to this process's own API when the platform runs
// alongside.
func LoadFunctionRunnerEnv(cfg EnvCfg) FunctionRunnerConfig {
	c := FunctionRunnerConfig{
		Enabled:       envBool("FC_FUNCTIONS_ENABLED", false),
		Bind:          envOr("FC_FUNCTIONS_BIND", "0.0.0.0"),
		Port:          envInt("FC_FUNCTIONS_PORT", DefaultFunctionRunnerPort),
		PublicPort:    envInt("FC_FUNCTIONS_PUBLIC_PORT", 0),
		Pool:          envOr("FC_FUNCTIONS_POOL", "default"),
		PlatformURL:   os.Getenv("FC_FUNCTIONS_PLATFORM_URL"),
		ClientID:      os.Getenv("FC_FUNCTIONS_CLIENT_ID"),
		ClientSecret:  os.Getenv("FC_FUNCTIONS_CLIENT_SECRET"),
		MemoryLimitMB: envInt("FC_FUNCTIONS_MEMORY_LIMIT_MB", 0),
		ReserveMB:     envInt("FC_FUNCTIONS_MEMORY_RESERVE_MB", 0),
		CacheDir:      os.Getenv("FC_FUNCTIONS_CACHE_DIR"),
		IdleEvict:     time.Duration(envInt("FC_FUNCTIONS_IDLE_EVICT_SECONDS", 1800)) * time.Second,
	}
	if c.PlatformURL == "" && cfg.PlatformEnabled {
		c.PlatformURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.APIPort)
	}
	for p := range strings.SplitSeq(os.Getenv("FC_FUNCTIONS_TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			if addr, aerr := netip.ParseAddr(p); aerr == nil {
				prefix = netip.PrefixFrom(addr, addr.BitLen())
			} else {
				slog.Warn("FC_FUNCTIONS_TRUSTED_PROXIES: ignoring an entry that is not a CIDR or address", "entry", p)
				continue
			}
		}
		c.TrustedProxies = append(c.TrustedProxies, prefix)
	}
	c.SetMemoryLimit = !cfg.PlatformEnabled && !cfg.RouterEnabled && !cfg.SchedulerEnabled &&
		!cfg.ScheduledJobEnabled && !cfg.StreamEnabled && !cfg.OutboxEnabled && !cfg.MCPEnabled
	return c
}

// resolveBudget sizes the runner's memory budget (plan §7): an explicit
// limit, else the container's, and a reserve for the runner itself.
func (c FunctionRunnerConfig) resolveBudget() (*budget.Budget, error) {
	limit := int64(c.MemoryLimitMB) << 20
	if limit == 0 {
		detected, ok := budget.DetectLimit()
		if !ok {
			return nil, errors.New("cannot detect the memory limit; set FC_FUNCTIONS_MEMORY_LIMIT_MB")
		}
		limit = detected
	}
	reserve := int64(c.ReserveMB) << 20
	if reserve == 0 {
		reserve = budget.DefaultReserve(limit)
	}
	return budget.New(limit, reserve)
}

// StartFunctionRunner runs the function runner until ctx ends. Failures to
// start are logged, not fatal to the process — the same as other subsystems.
func StartFunctionRunner(ctx context.Context, c FunctionRunnerConfig) {
	log := slog.With("subsystem", "functions", "pool", c.Pool)
	if c.PlatformURL == "" {
		log.Error("function runner not started: FC_FUNCTIONS_PLATFORM_URL is required when the platform does not run in this process")
		return
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		log.Error("function runner not started: FC_FUNCTIONS_CLIENT_ID and FC_FUNCTIONS_CLIENT_SECRET are required (a service account holding platform:function-runner)")
		return
	}
	b, err := c.resolveBudget()
	if err != nil {
		log.Error("function runner not started", "err", err)
		return
	}
	if c.SetMemoryLimit {
		debug.SetMemoryLimit(b.Stats().ReserveBytes)
	}
	if c.CacheDir == "" {
		if dir, err := os.UserCacheDir(); err == nil {
			c.CacheDir = filepath.Join(dir, "flowcatalyst", "functions")
		}
	}

	tokens := fcauth.NewClientCredentialsProvider(fcauth.ClientCredentialsConfig{
		IssuerURL:    c.PlatformURL,
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
	})
	client := control.NewClient(c.PlatformURL, tokens.Token, nil)
	r, err := runner.New(ctx, runner.Config{
		Pool:           c.Pool,
		ControlPlane:   client,
		Budget:         b,
		CacheDir:       c.CacheDir,
		IdleEvict:      c.IdleEvict,
		IssuerFallback: c.PlatformURL,
		TrustedProxies: c.TrustedProxies,
		Logger:         slog.Default(),
	})
	if err != nil {
		log.Error("function runner not started", "err", err)
		return
	}

	private := http.NewServeMux()
	private.Handle("/", r.Handler())
	private.Handle("GET /metrics", r.MetricsHandler())
	servers := []*http.Server{newRunnerServer(net.JoinHostPort(c.Bind, fmt.Sprint(c.Port)), private)}
	if c.PublicPort > 0 {
		servers = append(servers, newRunnerServer(net.JoinHostPort(c.Bind, fmt.Sprint(c.PublicPort)), r.PublicHandler()))
	}
	for _, s := range servers {
		ln, err := net.Listen("tcp", s.Addr)
		if err != nil {
			log.Error("function runner listener failed", "addr", s.Addr, "err", err)
			return
		}
		go func() {
			if err := s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("function runner listener stopped", "addr", s.Addr, "err", err)
			}
		}()
		log.Info("function runner listening", "addr", s.Addr)
	}
	s := b.Stats()
	log.Info("function runner started", "budget_mb", s.BudgetBytes>>20, "reserve_mb", s.ReserveBytes>>20, "platform", c.PlatformURL, "cache_dir", c.CacheDir)

	_ = r.Run(ctx)
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		_ = s.Shutdown(shutdown)
	}
}

func newRunnerServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       75 * time.Second,
		Protocols:         cleartextHTTP2Protocols(),
	}
}

// cleartextHTTP2Protocols serves HTTP/1.1 and, on the same plain-TCP
// listener, HTTP/2 with prior knowledge (h2c). The router mediates to
// "http://" targets over h2c outside dev mode (router/mediator.go) — the
// platform's own /api/dispatch/process callback and the function runner are
// both such targets — so a listener that spoke only HTTP/1.1 failed every
// delivery with "http2: frame too large". HTTP/1.1 clients are unaffected.
func cleartextHTTP2Protocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}
