// Package runner is the function runner (docs/function-runner-plan.md §6):
// it long-polls the platform for its pool's desired state, prepares the
// versions it names (download, verify, compile), routes HTTP invocations to
// them with per-endpoint auth and per-function permits, serves their host
// calls, and reports back in heartbeats.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// ControlPlane is the platform as the runner sees it (control.Client in
// production, a fake in tests).
type ControlPlane interface {
	Desired(ctx context.Context, pool, etag string, wait time.Duration) (*control.Desired, string, bool, error)
	Heartbeat(ctx context.Context, hb control.Heartbeat) error
	Artifact(ctx context.Context, digest string) (io.ReadCloser, error)
	Emit(ctx context.Context, r control.EmitRequest) (*control.EmitResponse, *abi.Error)
}

// Config configures a Runner.
type Config struct {
	Pool         string
	ControlPlane ControlPlane
	Budget       *budget.Budget
	// CacheDir holds downloaded artifacts and compiled code.
	CacheDir string
	// IdleEvict drops a lazy version's compiled code after this long unused.
	IdleEvict time.Duration
	// HeartbeatEvery is the heartbeat period.
	HeartbeatEvery time.Duration
	// DrainGrace bounds how long an unloading version waits for in-flight calls.
	DrainGrace time.Duration
	// MaxDBPools bounds the database pools shared across functions.
	MaxDBPools int
	// IssuerFallback verifies platform tokens when Desired carries no issuer.
	IssuerFallback string
	// Tokens overrides platform-token verification (tests).
	Tokens TokenVerifier
	Logger *slog.Logger
}

// Runner serves one pool's functions.
type Runner struct {
	cfg        Config
	cp         ControlPlane
	eng        *engine.Engine
	loader     *runtimes.Loader
	budget     *budget.Budget
	artifacts  *artifactCache
	tokens     TokenVerifier
	jwks       *jwksVerifier
	db         *dbPools
	httpClient *http.Client
	metrics    *metrics
	log        *slog.Logger
	id         string
	started    time.Time

	mu        sync.RWMutex
	functions map[string]*function // by address
	revision  int64

	prepareSem chan struct{}
	ready      atomic.Bool
	evictions  atomic.Int64
	wake       chan struct{} // heartbeat soon
}

// New builds a runner. Run starts it.
func New(ctx context.Context, cfg Config) (*Runner, error) {
	if cfg.Pool == "" || cfg.ControlPlane == nil || cfg.Budget == nil {
		return nil, errors.New("runner: pool, control plane and budget are required")
	}
	if cfg.IdleEvict <= 0 {
		cfg.IdleEvict = 30 * time.Minute
	}
	if cfg.HeartbeatEvery <= 0 {
		cfg.HeartbeatEvery = 15 * time.Second
	}
	if cfg.DrainGrace <= 0 {
		cfg.DrainGrace = 30 * time.Second
	}
	if cfg.MaxDBPools <= 0 {
		cfg.MaxDBPools = 32
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	log = log.With("subsystem", "functions", "pool", cfg.Pool)
	compileDir := ""
	if cfg.CacheDir != "" {
		compileDir = cfg.CacheDir + "/compiled"
	}
	eng, err := engine.New(ctx, engine.Config{CacheDir: compileDir, Budget: cfg.Budget, Logger: log})
	if err != nil {
		return nil, err
	}
	arts, err := newArtifactCache(cfg.CacheDir, cfg.ControlPlane)
	if err != nil {
		_ = eng.Close(ctx)
		return nil, err
	}
	r := &Runner{
		cfg:        cfg,
		cp:         cfg.ControlPlane,
		eng:        eng,
		loader:     runtimes.NewLoader(eng),
		budget:     cfg.Budget,
		artifacts:  arts,
		db:         newDBPools(cfg.MaxDBPools, 4),
		httpClient: newHTTPClient(),
		metrics:    newMetrics(),
		log:        log,
		id:         tsid.GenerateUntyped(),
		started:    time.Now().UTC(),
		functions:  map[string]*function{},
		prepareSem: make(chan struct{}, 4),
		wake:       make(chan struct{}, 1),
	}
	r.jwks = &jwksVerifier{fallback: cfg.IssuerFallback}
	r.tokens = r.jwks
	if cfg.Tokens != nil {
		r.tokens = cfg.Tokens
	}
	return r, nil
}

// Run reconciles until ctx ends, then unloads everything.
func (r *Runner) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Go(func() { r.reconcileLoop(ctx) })
	wg.Go(func() { r.heartbeatLoop(ctx) })
	wg.Go(func() { r.maintenanceLoop(ctx) })
	wg.Wait()
	r.shutdown()
	return nil
}

func (r *Runner) shutdown() {
	r.mu.Lock()
	var vers []*version
	for _, fn := range r.functions {
		for _, v := range fn.versions {
			vers = append(vers, v)
		}
	}
	r.functions = map[string]*function{}
	r.mu.Unlock()
	var wg sync.WaitGroup
	for _, v := range vers {
		wg.Go(func() { v.close(r.cfg.DrainGrace) })
	}
	wg.Wait()
	r.db.closeAll()
	_ = r.eng.Close(context.Background())
}

// Ready reports whether the runner has applied a desired document.
func (r *Runner) Ready() bool { return r.ready.Load() }

// MetricsHandler serves the runner's Prometheus metrics.
func (r *Runner) MetricsHandler() http.Handler { return r.metrics.handler(r) }

func (r *Runner) reconcileLoop(ctx context.Context) {
	etag := ""
	backoff := time.Second
	for ctx.Err() == nil {
		d, next, changed, err := r.cp.Desired(ctx, r.cfg.Pool, etag, 30*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.log.Warn("desired state unavailable; serving what is loaded", "err", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		if changed {
			r.apply(ctx, d)
			etag = next
		}
	}
}

// apply makes the runner's holdings match d. Preparation is asynchronous; a
// function keeps serving its current live version until the new one is ready.
func (r *Runner) apply(ctx context.Context, d *control.Desired) {
	r.jwks.configure(d.Auth)
	r.mu.Lock()
	seen := map[string]bool{}
	var toPrepare []*version
	var toClose []*version
	for i := range d.Functions {
		df := d.Functions[i]
		seen[df.Address] = true
		fn, ok := r.functions[df.Address]
		if !ok {
			fn = &function{id: df.ID, address: df.Address, versions: map[int]*version{}, aliases: map[string]int{}}
			r.functions[df.Address] = fn
		}
		fn.setConcurrency(limitsOf(&df).MaxConcurrency)
		fn.settings.Store(&df)

		want := map[int]bool{}
		fn.live = 0
		fn.aliases = map[string]int{}
		for _, dv := range d.Functions[i].Versions {
			want[dv.Number] = true
			for _, role := range dv.Roles {
				switch {
				case role == control.RoleLive:
					fn.live = dv.Number
				case len(role) > len(control.RoleAlias) && role[:len(control.RoleAlias)] == control.RoleAlias:
					fn.aliases[role[len(control.RoleAlias):]] = dv.Number
				}
			}
			v, have := fn.versions[dv.Number]
			if have && v.digest == dv.Digest {
				v.roles = dv.Roles
				continue
			}
			nv, err := newVersion(fn, dv)
			if err != nil {
				nv = &version{fn: fn, fnID: df.ID, number: dv.Number, digest: dv.Digest, roles: dv.Roles, state: stateFailed, reason: err.Error()}
			} else {
				toPrepare = append(toPrepare, nv)
			}
			if have {
				toClose = append(toClose, v)
			}
			fn.versions[dv.Number] = nv
		}
		// Versions no longer wanted: keep the one still serving live until
		// its successor is ready (new before old); unload the rest.
		for n, v := range fn.versions {
			if want[n] {
				continue
			}
			if n == fn.serving && fn.serving != fn.live {
				v.roles = nil // no longer named; unloaded once its successor serves
				continue
			}
			toClose = append(toClose, v)
			delete(fn.versions, n)
		}
		r.promoteLocked(fn)
	}
	for addr, fn := range r.functions {
		if seen[addr] {
			continue
		}
		for _, v := range fn.versions {
			toClose = append(toClose, v)
		}
		delete(r.functions, addr)
	}
	r.revision = d.Revision
	r.mu.Unlock()

	for _, v := range toClose {
		// #nosec G118 -- draining must finish even after reconcile's context ends.
		go v.close(r.cfg.DrainGrace)
	}
	for _, v := range toPrepare {
		go r.prepare(ctx, v)
	}
	r.ready.Store(true)
	r.poke()
}

func newVersion(fn *function, dv control.Version) (*version, error) {
	d, err := abi.ParseDescribe(dv.Describe)
	if err != nil {
		return nil, fmt.Errorf("DESCRIBE: %w", err)
	}
	rt, err := abi.NewRouter(d.Endpoints)
	if err != nil {
		return nil, fmt.Errorf("DESCRIBE: %w", err)
	}
	if !runtimes.Valid(dv.Runtime) {
		return nil, fmt.Errorf("RUNTIME: %q is not a runtime this runner has", dv.Runtime)
	}
	return &version{fn: fn, fnID: fn.id, number: dv.Number, digest: dv.Digest, runtime: dv.Runtime, describe: d, router: rt, roles: dv.Roles, state: statePreparing}, nil
}

// promoteLocked routes live to the desired live version once it is ready,
// and unloads the version it replaces. Caller holds r.mu.
func (r *Runner) promoteLocked(fn *function) {
	if fn.live == 0 || fn.serving == fn.live {
		return
	}
	v, ok := fn.versions[fn.live]
	if !ok {
		return
	}
	if st, _ := v.currentState(); st != stateReady && st != stateEvicted {
		if fn.serving != 0 {
			return // keep the old version serving until the new one is ready
		}
		fn.serving = fn.live // nothing to fall back on: route to it (503 until ready)
		return
	}
	old := fn.serving
	fn.serving = fn.live
	if old != 0 && old != fn.live {
		if ov, ok := fn.versions[old]; ok && !r.wantedLocked(fn, old) {
			delete(fn.versions, old)
			go ov.close(r.cfg.DrainGrace)
		}
	}
}

// wantedLocked reports whether version n is still named by desired state
// (live, an alias, or a candidate). Caller holds r.mu.
func (r *Runner) wantedLocked(fn *function, n int) bool {
	v, ok := fn.versions[n]
	return ok && len(v.roles) > 0
}

// prepare downloads, verifies and compiles a version, proving it loadable by
// instantiating it once; warm live versions keep that instance.
func (r *Runner) prepare(ctx context.Context, v *version) {
	r.prepareSem <- struct{}{}
	defer func() { <-r.prepareSem }()
	r.mu.RLock()
	warm := v.fn.snapshot().Warm && slices.Contains(v.roles, control.RoleLive)
	r.mu.RUnlock()

	v.mu.Lock()
	err := r.compileLocked(ctx, v)
	if err == nil {
		v.pool.warm = warm
		err = v.pool.prewarm(ctx)
		if err == nil && !warm {
			v.pool.trim(time.Now().Add(2 * idleInstanceTTL))
		}
		if errors.Is(err, engine.ErrNoMemory) {
			err = nil // loadable; there is simply no room for an idle instance now
		}
	}
	if err != nil {
		v.evictLocked(ctx)
		v.state, v.reason = stateFailed, reasonOf(err)
		r.log.Warn("function version failed to load", "fn.address", v.fn.address, "fn.version", v.number, "reason", v.reason)
	}
	v.mu.Unlock()

	r.mu.Lock()
	r.promoteLocked(v.fn)
	r.mu.Unlock()
	r.poke()
}

// compileLocked fetches and compiles v's module and gives it a pool.
// Caller holds v.mu (and must not hold r.mu).
func (r *Runner) compileLocked(ctx context.Context, v *version) error {
	artifact, err := r.artifacts.get(ctx, v.digest)
	if err != nil {
		return fmt.Errorf("ARTIFACT: %w", err)
	}
	p, err := r.loader.Prepare(ctx, v.runtime, artifact)
	if err != nil {
		return err
	}
	lim := limitsOf(v.fn.snapshot())
	v.prepared = p
	v.pool = newPool(p.Module, p.InstanceConfig(engine.InstanceConfig{
		MemoryCapBytes: uint64(lim.MemoryMB) << 20,
		Stdout:         &guestWriter{log: r.log, fn: v.fn.address, ver: v.number, level: slog.LevelInfo},
		Stderr:         &guestWriter{log: r.log, fn: v.fn.address, ver: v.number, level: slog.LevelWarn},
	}), false)
	v.state, v.reason = stateReady, ""
	v.touch()
	return nil
}

func reasonOf(err error) string {
	if le, ok := errors.AsType[*engine.LoadError](err); ok {
		return le.Error()
	}
	return err.Error()
}

// maintenanceLoop trims idle instances, evicts idle lazy versions and, when
// memory runs high, evicts compiled code least-recently-used first.
func (r *Runner) maintenanceLoop(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := time.Now()
		var candidates []*version
		r.mu.RLock()
		for _, fn := range r.functions {
			warm := fn.snapshot().Warm
			for n, v := range fn.versions {
				v.mu.Lock()
				if v.pool != nil {
					v.pool.trim(now)
				}
				idle := now.Sub(time.Unix(0, v.lastUsed.Load()))
				pinned := warm && n == fn.serving
				if v.state == stateReady && !pinned {
					if idle > r.cfg.IdleEvict {
						v.evictLocked(ctx)
						r.evictions.Add(1)
					} else {
						candidates = append(candidates, v)
					}
				}
				v.mu.Unlock()
			}
		}
		r.mu.RUnlock()
		r.relieveMemory(ctx, candidates)
		r.db.closeIdle(10 * time.Minute)
	}
}

// relieveMemory evicts compiled code, oldest first, while process memory is
// above 90% of the limit (Linux; elsewhere usage is unknown and it is idle).
func (r *Runner) relieveMemory(ctx context.Context, candidates []*version) {
	used, ok := budget.CurrentUsage()
	high := r.budget.Limit() / 10 * 9
	if !ok || used < high {
		return
	}
	slices.SortFunc(candidates, func(a, b *version) int { return int(a.lastUsed.Load() - b.lastUsed.Load()) })
	for _, v := range candidates {
		v.mu.Lock()
		if v.state == stateReady {
			v.evictLocked(ctx)
			r.evictions.Add(1)
		}
		v.mu.Unlock()
		if used, ok = budget.CurrentUsage(); !ok || used < high {
			return
		}
	}
}

func (r *Runner) poke() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runner) heartbeatLoop(ctx context.Context) {
	t := time.NewTicker(r.cfg.HeartbeatEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-r.wake:
		}
		hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := r.cp.Heartbeat(hctx, r.heartbeat()); err != nil && ctx.Err() == nil {
			r.log.Warn("heartbeat failed", "err", err)
		}
		cancel()
	}
}

func (r *Runner) heartbeat() control.Heartbeat {
	hb := control.Heartbeat{RunnerID: r.id, Pool: r.cfg.Pool, StartedAt: r.started}
	loaded := 0
	r.mu.RLock()
	hb.Revision = r.revision
	for _, fn := range r.functions {
		for _, v := range fn.versions {
			st, reason := v.currentState()
			rep := control.VersionReport{FunctionID: fn.id, Number: v.number}
			switch st {
			case stateReady, stateEvicted:
				rep.State = control.StateLoaded
				loaded++
			case stateFailed:
				rep.State, rep.Reason = control.StateFailed, reason
			default:
				continue // preparing or closed: nothing to report yet
			}
			hb.Versions = append(hb.Versions, rep)
		}
	}
	r.mu.RUnlock()
	hb.Budget = control.BudgetReport{Stats: r.budget.Stats(), Loaded: loaded, Evictions: r.evictions.Load()}
	if used, ok := budget.CurrentUsage(); ok {
		hb.Budget.RSSBytes = used
	}
	return hb
}

// guestWriter routes a guest's WASI stdout/stderr lines to the log.
type guestWriter struct {
	log   *slog.Logger
	fn    string
	ver   int
	level slog.Level
}

func (g *guestWriter) Write(p []byte) (int, error) {
	g.log.Log(context.Background(), g.level, string(trimNewline(p)), "fn.address", g.fn, "fn.version", g.ver, "stream", "guest")
	return len(p), nil
}

func trimNewline(p []byte) []byte {
	for len(p) > 0 && (p[len(p)-1] == '\n' || p[len(p)-1] == '\r') {
		p = p[:len(p)-1]
	}
	return p
}

func (r *Runner) guestLogger(fn *function, v *version) *slog.Logger {
	return r.log.With("fn.address", fn.address, "fn.version", v.number)
}
