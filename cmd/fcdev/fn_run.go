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
			fn := control.Function{
				ID: "fnc_local", Address: address, ApplicationID: "app_local",
				Limits:        control.Limits{MemoryMB: 64, MaxConcurrency: 64, TimeoutMs: 30_000},
				WebhookSecret: whSecret,
			}
			var err error
			if fn.Config, err = keyValues(configs); err != nil {
				return err
			}
			if fn.Secrets, err = keyValues(secrets); err != nil {
				return err
			}
			if fn.DB, err = keyValues(dbs); err != nil {
				return err
			}
			cp := &localCP{fn: fn, path: path, runtime: runtime, changed: make(chan struct{}, 1)}
			if err := cp.load(cmd.Context()); err != nil {
				return err
			}
			b, err := budget.New(int64(memoryMB)<<20, 0)
			if err != nil {
				return err
			}
			r, err := runner.New(cmd.Context(), runner.Config{
				Pool: "local", ControlPlane: cp, Budget: b,
				Tokens: devTokens{permissions: perms}, HeartbeatEvery: time.Hour,
			})
			if err != nil {
				return err
			}
			ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				return err
			}
			srv := &http.Server{Handler: r.Handler(), ReadHeaderTimeout: 10 * time.Second}
			go func() { _ = srv.Serve(ln) }()
			defer func() { _ = srv.Close() }()
			fmt.Printf("serving %s (%s) at http://%s/fn/%s/…\n", path, runtime, ln.Addr(), address)
			fmt.Printf("webhook endpoints verify signatures made with secret %q; platform endpoints accept any bearer token\n", whSecret)
			if watch {
				go cp.watch(cmd.Context())
			}
			return r.Run(cmd.Context())
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

// load (re)reads the artifact and makes it the live version.
func (c *localCP) load(ctx context.Context) error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	info, _ := os.Stat(c.path)
	d, doc, err := describeArtifact(ctx, c.path, c.runtime)
	if err != nil {
		return fmt.Errorf("%s: %w", c.path, err)
	}
	sum := sha256.Sum256(data)
	c.mu.Lock()
	c.version++
	c.data = data
	c.mtime = info.ModTime()
	c.fn.Versions = []control.Version{{Number: c.version, Digest: hex.EncodeToString(sum[:]), Runtime: c.runtime, ABI: abi.Version, Describe: doc, Roles: []string{control.RoleLive}}}
	c.mu.Unlock()
	printDescribe(fmt.Sprintf("v%d %s", c.version, c.path), d)
	select {
	case c.changed <- struct{}{}:
	default:
	}
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
