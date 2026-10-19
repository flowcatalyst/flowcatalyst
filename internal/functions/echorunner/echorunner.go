// Package echorunner is a tiny in-repo test runner that speaks the Go v1
// control protocol for a made-up runtime, "echo". It has no guest and runs no
// code: it exists to prove, end to end, the generic runner plumbing a real
// out-of-process runner (Java, Rust) relies on, without waiting for one:
//
//   - it long-polls desired state asking only for its runtime
//     (the `runtimes` filter) and applies a document only when its revision
//     moves (the revision fence);
//   - it reports LOADED / COMPILED / FAILED / RETRYING versions in heartbeats
//     carrying its Capabilities;
//   - it answers POST /fn/{address}/... with an echo of the request for any
//     function it holds a live echo version of.
//
// It is a test fixture and a conformance skeleton, not a product.
package echorunner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// Runtime is the one runtime the echo runner runs.
const Runtime = "echo"

// Kind is the runner kind it reports.
const Kind = "echo"

// Plane is the control plane as the echo runner sees it (control.Client).
type Plane interface {
	Desired(ctx context.Context, pool, etag string, wait time.Duration) (*control.Desired, string, bool, error)
	Heartbeat(ctx context.Context, hb control.Heartbeat) error
}

// Config configures a Runner.
type Config struct {
	Plane Plane
	Pool  string
	// RunnerID defaults to "echo-runner".
	RunnerID string
	// Version is reported in Capabilities.
	Version string
	// Wait is the long-poll hold (default 20s).
	Wait time.Duration
	// HeartbeatEvery is the heartbeat period (default 15s).
	HeartbeatEvery time.Duration
	// StateFor decides the state a held version is reported in. Default:
	// every echo version is LOADED. Tests use it to drive COMPILED, RETRYING
	// and FAILED reports.
	StateFor func(functionID string, v control.Version) (state, reason string)
	Logger   *slog.Logger
}

type held struct {
	fnID    string
	address string
	version control.Version
}

// Runner is an echo runner.
type Runner struct {
	cfg     Config
	log     *slog.Logger
	started time.Time

	mu       sync.RWMutex
	revision int64
	held     map[string][]held // address -> versions
	ignored  int               // versions of other runtimes seen (should stay 0)
	wake     chan struct{}
}

// New builds a runner.
func New(cfg Config) *Runner {
	if cfg.RunnerID == "" {
		cfg.RunnerID = "echo-runner"
	}
	if cfg.Wait <= 0 {
		cfg.Wait = 20 * time.Second
	}
	if cfg.HeartbeatEvery <= 0 {
		cfg.HeartbeatEvery = 15 * time.Second
	}
	if cfg.StateFor == nil {
		cfg.StateFor = func(string, control.Version) (string, string) { return control.StateLoaded, "" }
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Runner{cfg: cfg, log: log, started: time.Now().UTC(), held: map[string][]held{}, wake: make(chan struct{}, 1)}
}

// Capabilities is what the runner advertises.
func (r *Runner) Capabilities() *control.RunnerCapabilities {
	return &control.RunnerCapabilities{
		Protocol:   control.ProtocolV1,
		Runtimes:   []string{Runtime},
		RunnerKind: Kind,
		Version:    r.cfg.Version,
	}
}

// Run polls and heartbeats until ctx ends.
func (r *Runner) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Go(func() { r.heartbeatLoop(ctx) })
	r.pollLoop(ctx)
	wg.Wait()
	return ctx.Err()
}

func (r *Runner) pollLoop(ctx context.Context) {
	etag := ""
	backoff := 200 * time.Millisecond
	for ctx.Err() == nil {
		d, next, changed, err := r.cfg.Plane.Desired(ctx, r.cfg.Pool, etag, r.cfg.Wait)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.log.Warn("desired failed", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 5*time.Second)
			continue
		}
		backoff = 200 * time.Millisecond
		if changed {
			r.apply(d)
			etag = next
		}
	}
}

// apply replaces the held set. The revision fence: the heartbeat carries the
// revision of the document applied here, so the platform counts a FAILED
// report only from a runner that has seen the version's last re-queue.
func (r *Runner) apply(d *control.Desired) {
	next := map[string][]held{}
	ignored := 0
	for _, f := range d.Functions {
		for _, v := range f.Versions {
			if v.Runtime != Runtime {
				ignored++ // the platform must not send these: filtered by `runtimes`
				continue
			}
			next[f.Address] = append(next[f.Address], held{fnID: f.ID, address: f.Address, version: v})
		}
	}
	r.mu.Lock()
	r.held, r.revision = next, d.Revision
	r.ignored += ignored
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runner) heartbeatLoop(ctx context.Context) {
	t := time.NewTicker(r.cfg.HeartbeatEvery)
	defer t.Stop()
	for {
		hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := r.cfg.Plane.Heartbeat(hctx, r.Heartbeat()); err != nil && ctx.Err() == nil {
			r.log.Warn("heartbeat failed", "err", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-r.wake:
		}
	}
}

// Heartbeat builds the report for the current state.
func (r *Runner) Heartbeat() control.Heartbeat {
	hb := control.Heartbeat{
		RunnerID: r.cfg.RunnerID, Pool: r.cfg.Pool, StartedAt: r.started,
		Versions: []control.VersionReport{}, Capabilities: r.Capabilities(),
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	hb.Revision = r.revision
	loaded := 0
	for _, hs := range r.held {
		for _, h := range hs {
			state, reason := r.cfg.StateFor(h.fnID, h.version)
			if state == control.StateLoaded {
				loaded++
			}
			hb.Versions = append(hb.Versions, control.VersionReport{FunctionID: h.fnID, Number: h.version.Number, State: state, Reason: reason})
		}
	}
	hb.Budget.Loaded = loaded
	sort.Slice(hb.Versions, func(i, j int) bool {
		if hb.Versions[i].FunctionID != hb.Versions[j].FunctionID {
			return hb.Versions[i].FunctionID < hb.Versions[j].FunctionID
		}
		return hb.Versions[i].Number < hb.Versions[j].Number
	})
	return hb
}

// Held reports how many versions are held and the applied revision.
func (r *Runner) Held() (versions int, revision int64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, hs := range r.held {
		versions += len(hs)
	}
	return versions, r.revision
}

// Ignored counts versions of a foreign runtime the platform sent (a filter
// bug); it must stay zero.
func (r *Runner) Ignored() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ignored
}

// Handler serves GET /healthz and POST /fn/{address}/{path...}: the latter
// echoes {address, path, body} as JSON for a function with a held echo
// version, else 404.
func (r *Runner) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /fn/", func(w http.ResponseWriter, req *http.Request) {
		rest := strings.TrimPrefix(req.URL.Path, "/fn/")
		address, path, _ := strings.Cut(rest, "/")
		path = "/" + path
		r.mu.RLock()
		_, ok := r.held[address]
		r.mu.RUnlock()
		if !ok {
			http.Error(w, "no such function", http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
		if err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"address": address, "path": path, "body": string(body)})
	})
	return mux
}
