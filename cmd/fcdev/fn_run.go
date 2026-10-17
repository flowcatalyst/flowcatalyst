package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runner"
)

// newFnRunCmd is `fcdev fn run`: one artifact on the real runner, no
// platform. Settings come from flags, emitted events print to stdout, any
// bearer token is a dev principal, and --watch redeploys on every rebuild.
func newFnRunCmd() *cobra.Command {
	var (
		port                         int
		address, runtime, whSecret   string
		configs, secrets, dbs, perms []string
		memoryMB                     int
		watch                        bool
	)
	cmd := &cobra.Command{
		Use:   "run <artifact>",
		Short: "Run one function locally on the real runner, without a platform",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if runtime == "" {
				runtime = runtimeFor(path)
			}
			fn, err := newLocalFunction(address, whSecret, configs, secrets, dbs)
			if err != nil {
				return err
			}
			cp := &localCP{fn: fn, path: path, runtime: runtime, changed: make(chan struct{}, 1)}
			if err := cp.load(cmd.Context()); err != nil {
				return err
			}
			ls, err := startLocalServer(cmd.Context(), cp, perms, memoryMB, port)
			if err != nil {
				return err
			}
			defer ls.Close()
			fmt.Printf("serving %s (%s) at http://%s/fn/%s/…\n", path, runtime, ls.ln.Addr(), address)
			fmt.Printf("webhook endpoints verify signatures made with secret %q; platform endpoints accept any bearer token\n", whSecret)
			if watch {
				go cp.watch(cmd.Context())
			}
			return ls.r.Run(cmd.Context())
		},
	}
	f := cmd.Flags()
	f.IntVar(&port, "port", 8099, "listen port (127.0.0.1)")
	f.StringVar(&address, "address", "local.fn", "the address to serve the function at")
	f.StringVar(&runtime, "runtime", "", "wasm or js (default: from the file extension)")
	f.StringArrayVar(&configs, "config", nil, "KEY=VALUE config setting (repeatable)")
	f.StringArrayVar(&secrets, "secret", nil, "KEY=VALUE secret (repeatable)")
	f.StringArrayVar(&dbs, "db", nil, "NAME=DSN database binding (repeatable)")
	f.StringArrayVar(&perms, "permission", nil, "permission the dev principal holds (repeatable)")
	f.StringVar(&whSecret, "webhook-secret", "dev-webhook-secret", "the signing secret webhook endpoints verify")
	f.IntVar(&memoryMB, "memory-mb", 256, "memory budget for the function")
	f.BoolVar(&watch, "watch", false, "redeploy whenever the artifact changes")
	return cmd
}

// newLocalFunction is the one function a local runner serves: dev limits and
// the settings given as KEY=VALUE flags.
func newLocalFunction(address, whSecret string, configs, secrets, dbs []string) (control.Function, error) {
	fn := control.Function{
		ID: "fnc_local", Address: address, ApplicationID: "app_local",
		Limits:        control.Limits{MemoryMB: 64, MaxConcurrency: 64, TimeoutMs: 30_000},
		WebhookSecret: whSecret,
	}
	var err error
	if fn.Config, err = keyValues(configs); err != nil {
		return fn, err
	}
	if fn.Secrets, err = keyValues(secrets); err != nil {
		return fn, err
	}
	if fn.DB, err = keyValues(dbs); err != nil {
		return fn, err
	}
	return fn, nil
}

// localServer is a runner plus its 127.0.0.1 listener.
type localServer struct {
	r   *runner.Runner
	ln  net.Listener
	srv *http.Server
}

// startLocalServer builds the runner over cp and starts serving it on
// 127.0.0.1:port (0 = any free port). The caller runs r.Run.
func startLocalServer(ctx context.Context, cp runner.ControlPlane, perms []string, memoryMB, port int) (*localServer, error) {
	b, err := budget.New(int64(memoryMB)<<20, 0)
	if err != nil {
		return nil, err
	}
	r, err := runner.New(ctx, runner.Config{
		Pool: "local", ControlPlane: cp, Budget: b,
		Tokens: devTokens{permissions: perms}, HeartbeatEvery: time.Hour,
	})
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: r.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return &localServer{r: r, ln: ln, srv: srv}, nil
}

// Close stops the listener.
func (s *localServer) Close() { _ = s.srv.Close() }

func keyValues(in []string) (map[string]string, error) {
	out := map[string]string{}
	for _, kv := range in {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%q is not KEY=VALUE", kv)
		}
		out[k] = v
	}
	return out, nil
}

// devTokens accepts any bearer token as an anchor dev principal.
type devTokens struct{ permissions []string }

func (d devTokens) Verify(_ context.Context, bearer string) (*abi.Caller, error) {
	if !strings.HasPrefix(bearer, "Bearer ") || len(bearer) <= len("Bearer ") {
		return nil, errors.New("a bearer token is required")
	}
	return &abi.Caller{
		Kind: abi.CallerPrincipal, ID: "prn_dev", Type: "USER", Tier: "ANCHOR", Clients: []string{"*"},
		AllApplications: true, Permissions: d.permissions,
	}, nil
}

// localCP is a control plane of one function read from a local file.
type localCP struct {
	mu      sync.Mutex
	fn      control.Function
	path    string
	runtime string
	data    []byte
	version int
	mtime   time.Time
	changed chan struct{}
}

// loadInfo describes a version just made live.
type loadInfo struct {
	Version  int
	Size     int
	Digest   string
	Describe *abi.Describe
}

// reload (re)reads the artifact and makes it the live version, without
// printing. A failure leaves the previous version in place.
func (c *localCP) reload(ctx context.Context) (*loadInfo, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, err
	}
	info, _ := os.Stat(c.path)
	d, doc, err := describeArtifact(ctx, c.path, c.runtime)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", c.path, err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	c.mu.Lock()
	c.version++
	c.data = data
	if info != nil {
		c.mtime = info.ModTime()
	}
	c.fn.Versions = []control.Version{{Number: c.version, Digest: digest, Runtime: c.runtime, ABI: abi.Version, Describe: doc, Roles: []string{control.RoleLive}}}
	n := c.version
	c.mu.Unlock()
	select {
	case c.changed <- struct{}{}:
	default:
	}
	return &loadInfo{Version: n, Size: len(data), Digest: digest, Describe: d}, nil
}

// load is reload plus the describe printout `fn run` shows.
func (c *localCP) load(ctx context.Context) error {
	li, err := c.reload(ctx)
	if err != nil {
		return err
	}
	printDescribe(fmt.Sprintf("v%d %s", li.Version, c.path), li.Describe)
	return nil
}

func (c *localCP) watch(ctx context.Context) {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		info, err := os.Stat(c.path)
		c.mu.Lock()
		stale := err == nil && info.ModTime().After(c.mtime)
		c.mu.Unlock()
		if stale {
			time.Sleep(200 * time.Millisecond) // let the build finish writing
			if err := c.load(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "not redeployed:", err)
				c.mu.Lock()
				c.mtime = info.ModTime()
				c.mu.Unlock()
			}
		}
	}
}

func (c *localCP) Desired(ctx context.Context, _, etag string, wait time.Duration) (*control.Desired, string, bool, error) {
	c.mu.Lock()
	cur := strconv.Itoa(c.version)
	fn := c.fn
	c.mu.Unlock()
	if cur != etag {
		return &control.Desired{Pool: "local", Revision: int64(c.version), Functions: []control.Function{fn}}, cur, true, nil
	}
	select {
	case <-ctx.Done():
		return nil, "", false, ctx.Err()
	case <-c.changed:
		return c.Desired(ctx, "", etag, 0)
	case <-time.After(wait):
		return nil, etag, false, nil
	}
}

func (c *localCP) Heartbeat(context.Context, control.Heartbeat) error { return nil }

func (c *localCP) Artifact(_ context.Context, _ string) (io.ReadCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return io.NopCloser(bytes.NewReader(c.data)), nil
}

func (c *localCP) Emit(_ context.Context, r control.EmitRequest) (*control.EmitResponse, *abi.Error) {
	ev, _ := json.Marshal(r.Event)
	slog.Info("function emitted an event", "event", string(ev), "data", string(r.Data))
	return &control.EmitResponse{EventID: "evt_local_" + strconv.FormatInt(time.Now().UnixNano(), 36)}, nil
}

// Dispatch logs the job a function would create; `fn run` has no platform to
// create it on.
func (c *localCP) Dispatch(_ context.Context, r control.DispatchRequest) (*control.DispatchResponse, *abi.Error) {
	d, _ := json.Marshal(r.Dispatch)
	slog.Info("function dispatched a job", "dispatch", string(d), "payload", string(r.Payload))
	return &control.DispatchResponse{JobID: "djob_local_" + strconv.FormatInt(time.Now().UnixNano(), 36)}, nil
}
