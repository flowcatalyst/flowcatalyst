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
	"net/netip"
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
	// TrustedProxies may set X-Forwarded-For on the public entry.
	TrustedProxies []netip.Prefix
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
	routes    routeTable
	revision  int64

	prepareSem chan struct{}
	prepares   sync.WaitGroup // running prepare goroutines; awaited before the engine closes
	retryBase  time.Duration  // first retry delay for a failed version; doubles per attempt
	ready      atomic.Bool
	draining   atomic.Bool // shutdown has begun: not ready, new calls are refused
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
		retryBase:  defaultRetryBase,
		wake:       make(chan struct{}, 1),
	}
	r.jwks = &jwksVerifier{fallback: cfg.IssuerFallback}
	r.tokens = r.jwks
	if cfg.Tokens != nil {
		r.tokens = cfg.Tokens
	}
	return r, nil
}

// Retry policy for a version that failed to prepare (a transient artifact
// fetch or compile failure must not be permanent).
const (
	defaultRetryBase = 5 * time.Second
	maxRetryDelay    = 5 * time.Minute
)

// Drain begins shutdown: the runner reports not ready and answers new calls
// 503, while calls already running finish. Call it before stopping the
// listeners; Run's own shutdown then unloads once its context ends.
func (r *Runner) Drain() {
	r.draining.Store(true)
	r.ready.Store(false)
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
	r.Drain()
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
	// Every prepare has stopped (versions were told to close, and their
	// context has ended) before the engine they compile on goes away.
	r.prepares.Wait()
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
				// A failed version is never pinned: this apply retries it
				// once its backoff has passed.
				if v.tryRetry(time.Now()) {
					toPrepare = append(toPrepare, v)
				}
				continue
			}
			nv, err := newVersion(fn, dv)
			if err != nil {
				nv = &version{fn: fn, fnID: df.ID, number: dv.Number, digest: dv.Digest, roles: dv.Roles}
				nv.setStateLocked(stateFailed, err.Error()) // the document itself is bad: retrying cannot help
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
	r.routes = buildRoutes(d.Routes)
	r.revision = d.Revision
	r.mu.Unlock()

	for _, v := range toClose {
		// #nosec G118 -- draining must finish even after reconcile's context ends.
		go v.close(r.cfg.DrainGrace)
	}
	for _, v := range toPrepare {
		r.spawnPrepare(ctx, v)
	}
	r.ready.Store(true)
	if r.draining.Load() {
		r.ready.Store(false)
	}
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
	v := &version{fn: fn, fnID: fn.id, number: dv.Number, digest: dv.Digest, runtime: dv.Runtime, describe: d, router: rt, roles: dv.Roles}
	v.setStateLocked(statePreparing, "")
	v.work.Add(1) // the prepare that follows; Add before the version is visible, so close's Wait cannot race it
	return v, nil
}

// spawnPrepare starts v's prepare (v.work was already incremented).
func (r *Runner) spawnPrepare(ctx context.Context, v *version) {
	r.prepares.Add(1)
	go func() {
		defer r.prepares.Done()
		defer v.work.Done()
		r.prepare(ctx, v)
	}()
}

// tryRetry moves a failed version back to preparing when its backoff has
// passed, and reports whether the caller must now spawn its prepare.
func (v *version) tryRetry(now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	at := v.retryAt.Load()
	if v.closing || versionState(v.state.Load()) != stateFailed || at == 0 || now.UnixNano() < at {
		return false
	}
	v.setStateLocked(statePreparing, "")
	v.work.Add(1)
	return true
}

// retryFailed retries every failed version whose backoff has passed (called
// on the maintenance tick).
func (r *Runner) retryFailed(ctx context.Context) {
	now := time.Now()
	var retry []*version
	for _, v := range r.allVersions() {
		if v.tryRetry(now) {
			retry = append(retry, v)
		}
	}
	for _, v := range retry {
		r.spawnPrepare(ctx, v)
	}
}

// allVersions snapshots every version the runner holds.
func (r *Runner) allVersions() []*version {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var vs []*version
	for _, fn := range r.functions {
		for _, v := range fn.versions {
			vs = append(vs, v)
		}
	}
	return vs
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
// instantiating it once; warm live versions keep that instance. Nothing here
// holds v.mu across I/O: it compiles first, then installs the result under a
// short lock that also checks the version was not closed meanwhile.
func (r *Runner) prepare(ctx context.Context, v *version) {
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	v.mu.Lock()
	if v.closing {
		v.mu.Unlock()
		return
	}
	v.cancelPrep = cancel
	v.mu.Unlock()

	select {
	case r.prepareSem <- struct{}{}:
		defer func() { <-r.prepareSem }()
	case <-pctx.Done():
		return
	}
	r.mu.RLock()
	warm := v.fn.snapshot().Warm && slices.Contains(v.roles, control.RoleLive)
	r.mu.RUnlock()

	err := r.load(pctx, v, warm)
	if err != nil && (errors.Is(err, errVersionClosed) || pctx.Err() != nil) {
		// Closed or interrupted (shutdown): not a load failure.
		return
	}
	if err != nil {
		v.mu.Lock()
		if !v.closing {
			v.evictIfLoadedLocked(pctx)
			n := v.attempts.Add(1)
			delay := min(r.retryBase<<min(n-1, 20), maxRetryDelay)
			v.retryAt.Store(time.Now().Add(delay).UnixNano())
			v.setStateLocked(stateFailed, reasonOf(err))
			r.log.Warn("function version failed to load", "fn.address", v.fn.address, "fn.version", v.number, "reason", reasonOf(err), "attempt", n, "retry_in", delay)
		}
		v.mu.Unlock()
	}

	r.mu.Lock()
	r.promoteLocked(v.fn)
	r.mu.Unlock()
	r.poke()
}

// errVersionClosed marks a prepare that stopped because its version was closed.
var errVersionClosed = errors.New("the version was closed while it was preparing")

// load compiles v, gives it a pool, proves it by instantiating once, and marks
// it ready. On errVersionClosed everything it built has been released.
func (r *Runner) load(ctx context.Context, v *version, warm bool) error {
	p, pl, err := r.compile(ctx, v, warm)
	if err != nil {
		return err
	}
	v.mu.Lock()
	ok := v.installLocked(p, pl, statePreparing)
	v.mu.Unlock()
	if !ok {
		p.Close(context.Background())
		return errVersionClosed
	}
	err = pl.prewarm(ctx)
	if err == nil && !warm {
		pl.trim(time.Now().Add(2 * idleInstanceTTL))
	}
	if errors.Is(err, engine.ErrNoMemory) {
		err = nil // loadable; there is simply no room for an idle instance now
	}
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closing {
		return errVersionClosed // close tears down what was installed
	}
	v.setStateLocked(stateReady, "")
	v.retryAt.Store(0)
	v.attempts.Store(0)
	v.touch()
	return nil
}

// evictIfLoadedLocked releases what a failed prepare had installed.
func (v *version) evictIfLoadedLocked(ctx context.Context) {
	if v.pool != nil || v.prepared != nil {
		v.evictLocked(ctx)
	}
}

// compile fetches and compiles v's module and builds its pool. It takes no
// locks and installs nothing: the caller installs the result (or closes it).
func (r *Runner) compile(ctx context.Context, v *version, warm bool) (*runtimes.Prepared, *pool, error) {
	artifact, err := r.artifacts.get(ctx, v.digest)
	if err != nil {
		return nil, nil, fmt.Errorf("ARTIFACT: %w", err)
	}
	p, err := r.loader.Prepare(ctx, v.runtime, artifact)
	if err != nil {
		return nil, nil, err
	}
	lim := limitsOf(v.fn.snapshot())
	pl := newPool(p.Module, p.InstanceConfig(engine.InstanceConfig{
		MemoryCapBytes: uint64(lim.MemoryMB) << 20,
		Stdout:         &guestWriter{log: r.log, fn: v.fn.address, ver: v.number, level: slog.LevelInfo},
		Stderr:         &guestWriter{log: r.log, fn: v.fn.address, ver: v.number, level: slog.LevelWarn},
	}), warm)
	return p, pl, nil
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
		r.retryFailed(ctx)
		var candidates []*version
		// Snapshot under the runner lock, then work each version on its own
		// short v.mu hold: nothing here waits on a version while holding r.mu.
		r.mu.RLock()
		type held struct {
			v      *version
			pinned bool
		}
		var all []held
		for _, fn := range r.functions {
			warm := fn.snapshot().Warm
			for n, v := range fn.versions {
				all = append(all, held{v, warm && n == fn.serving})
			}
		}
		r.mu.RUnlock()
		for _, h := range all {
			v := h.v
			v.mu.Lock()
			if v.pool != nil {
				v.pool.trim(now)
			}
			idle := now.Sub(time.Unix(0, v.lastUsed.Load()))
			if versionState(v.state.Load()) == stateReady && !h.pinned {
				if idle > r.cfg.IdleEvict {
					v.evictLocked(ctx)
					r.evictions.Add(1)
				} else {
					candidates = append(candidates, v)
				}
			}
			v.mu.Unlock()
		}
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
		if versionState(v.state.Load()) == stateReady {
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
	// Versions is always a list, never null: the platform validates the body.
	hb := control.Heartbeat{RunnerID: r.id, Pool: r.cfg.Pool, StartedAt: r.started, Versions: []control.VersionReport{}}
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
