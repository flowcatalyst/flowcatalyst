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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	fcauth "github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/auth"
)

// fnCredentials is what `fcdev fn` authenticates with. In CI, flags or
// FC_PLATFORM_URL / FC_CLIENT_ID / FC_CLIENT_SECRET supply it; against a
// local fcdev, the file fcdev start writes (fnCLICredentialsPath).
type fnCredentials struct {
	PlatformURL  string `json:"platformUrl"`
	RunnerURL    string `json:"runnerUrl,omitempty"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// fnCLICredentialsPath is where fcdev start leaves the dev CLI credential.
func fnCLICredentialsPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "flowcatalyst-dev", "fn-cli.json"), nil
}

// writeFnCLICredentials records the dev CLI credential (mode 0600).
func writeFnCLICredentials(c fnCredentials) error {
	path, err := fnCLICredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

// fnPlatformFlags are the connection flags every platform command takes.
type fnPlatformFlags struct{ platform, runner, clientID, clientSecret string }

func (f *fnPlatformFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.platform, "platform", "", "platform URL (env FC_PLATFORM_URL; default: the local fcdev)")
	cmd.Flags().StringVar(&f.runner, "runner", "", "function runner URL for invoke (env FC_RUNNER_URL; default: the local fcdev's)")
	cmd.Flags().StringVar(&f.clientID, "client-id", "", "OAuth client id (env FC_CLIENT_ID)")
	cmd.Flags().StringVar(&f.clientSecret, "client-secret", "", "OAuth client secret (env FC_CLIENT_SECRET)")
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// resolve settles the credential: flags, then env, then the dev file.
func (f *fnPlatformFlags) resolve() (fnCredentials, error) {
	c := fnCredentials{
		PlatformURL:  firstNonEmpty(f.platform, os.Getenv("FC_PLATFORM_URL")),
		RunnerURL:    firstNonEmpty(f.runner, os.Getenv("FC_RUNNER_URL")),
		ClientID:     firstNonEmpty(f.clientID, os.Getenv("FC_CLIENT_ID")),
		ClientSecret: firstNonEmpty(f.clientSecret, os.Getenv("FC_CLIENT_SECRET")),
	}
	if c.ClientID == "" || c.PlatformURL == "" || c.RunnerURL == "" {
		if path, err := fnCLICredentialsPath(); err == nil {
			if b, err := os.ReadFile(path); err == nil {
				var dev fnCredentials
				if json.Unmarshal(b, &dev) == nil {
					c.PlatformURL = firstNonEmpty(c.PlatformURL, dev.PlatformURL)
					c.RunnerURL = firstNonEmpty(c.RunnerURL, dev.RunnerURL)
					if c.ClientID == "" {
						c.ClientID, c.ClientSecret = dev.ClientID, dev.ClientSecret
					}
				}
			}
		}
	}
	if c.PlatformURL == "" || c.ClientID == "" || c.ClientSecret == "" {
		return c, errors.New("no platform credential: start fcdev (it writes one), or pass --platform, --client-id and --client-secret")
	}
	c.PlatformURL = strings.TrimRight(c.PlatformURL, "/")
	c.RunnerURL = strings.TrimRight(c.RunnerURL, "/")
	return c, nil
}

// fnAPI is a minimal authenticated client for the /api/functions routes.
type fnAPI struct {
	c      fnCredentials
	tokens *fcauth.ClientCredentialsProvider
	http   *http.Client
}

func newFnAPI(f *fnPlatformFlags) (*fnAPI, error) {
	c, err := f.resolve()
	if err != nil {
		return nil, err
	}
	return &fnAPI{
		c: c,
		tokens: fcauth.NewClientCredentialsProvider(fcauth.ClientCredentialsConfig{
			IssuerURL: c.PlatformURL, ClientID: c.ClientID, ClientSecret: c.ClientSecret,
		}),
		http: &http.Client{Timeout: 2 * time.Minute},
	}, nil
}

// apiError is the platform's error body.
type apiError struct {
	Status  int
	Code    string `json:"error"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

func (e *apiError) Error() string {
	msg := firstNonEmpty(e.Message, e.Detail)
	return fmt.Sprintf("HTTP %d %s: %s", e.Status, e.Code, msg)
}

func (a *fnAPI) do(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, a.c.PlatformURL+path, body)
	if err != nil {
		return err
	}
	tok, err := a.tokens.Token(ctx)
	if err != nil {
		return fmt.Errorf("sign in to %s: %w", a.c.PlatformURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode/100 != 2 {
		e := &apiError{Status: resp.StatusCode}
		if json.Unmarshal(raw, e) != nil || (e.Code == "" && e.Message == "" && e.Detail == "") {
			e.Message = strings.TrimSpace(string(raw))
		}
		return e
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (a *fnAPI) json(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	return a.do(ctx, method, path, body, "application/json", out)
}

type fnFunction struct {
	ID       string  `json:"id"`
	Address  string  `json:"address"`
	ClientID *string `json:"clientId"`
	Pool     *string `json:"pool"`
	Warm     bool    `json:"warm"`
}

type fnVersion struct {
	Number   int32           `json:"number"`
	Digest   string          `json:"digest"`
	Runtime  string          `json:"runtime"`
	Status   string          `json:"status"`
	Failure  json.RawMessage `json:"failure"`
	Describe json.RawMessage `json:"describe"`
}

type fnAlias struct {
	Name    string `json:"name"`
	Version int32  `json:"version"`
}

// findFunction resolves an address to its function, or nil.
func (a *fnAPI) findFunction(ctx context.Context, address string) (*fnFunction, error) {
	var page struct {
		Data []fnFunction `json:"data"`
	}
	if err := a.json(ctx, http.MethodGet, "/api/functions?size=100&addressPrefix="+url.QueryEscape(address), nil, &page); err != nil {
		return nil, err
	}
	for i := range page.Data {
		if page.Data[i].Address == address {
			return &page.Data[i], nil
		}
	}
	return nil, nil
}

// createFunction creates the function an address names.
func (a *fnAPI) createFunction(ctx context.Context, address, clientID, pool string) (*fnFunction, error) {
	app, name, ok := strings.Cut(address, ".")
	if !ok || app == "" || name == "" || strings.Contains(name, ".") {
		return nil, fmt.Errorf("%q is not an address of the form <application>.<name>", address)
	}
	var application struct {
		ID string `json:"id"`
	}
	if err := a.json(ctx, http.MethodGet, "/api/applications/by-code/"+url.PathEscape(app), nil, &application); err != nil {
		return nil, fmt.Errorf("application %q: %w", app, err)
	}
	req := map[string]any{"applicationId": application.ID, "name": name}
	if clientID != "" {
		req["clientId"] = clientID
	}
	if pool != "" {
		req["pool"] = pool
	}
	if err := a.json(ctx, http.MethodPost, "/api/functions", req, nil); err != nil {
		return nil, err
	}
	fn, err := a.findFunction(ctx, address)
	if err == nil && fn == nil {
		err = fmt.Errorf("%s was created but cannot be found", address)
	}
	return fn, err
}

// mustFunction resolves an address, failing when it does not exist.
func (a *fnAPI) mustFunction(ctx context.Context, address string) (*fnFunction, error) {
	fn, err := a.findFunction(ctx, address)
	if err == nil && fn == nil {
		err = fmt.Errorf("no function %s (publish with --create to create it)", address)
	}
	return fn, err
}

// publish uploads an artifact and publishes it as a version.
func (a *fnAPI) publish(ctx context.Context, fn *fnFunction, path, runtime string) (*fnVersion, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ctype := "application/wasm"
	if runtime == "js" {
		ctype = "text/javascript"
	}
	if err := a.do(ctx, http.MethodPut, "/api/functions/"+fn.ID+"/artifacts/"+digest, bytes.NewReader(data), ctype, nil); err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	var v fnVersion
	if err := a.json(ctx, http.MethodPost, "/api/functions/"+fn.ID+"/versions", map[string]any{"digest": digest, "runtime": runtime}, &v); err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	return &v, nil
}

// waitReady polls until a runner has proved the version loadable.
func (a *fnAPI) waitReady(ctx context.Context, fn *fnFunction, number int32, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var v fnVersion
		if err := a.json(ctx, http.MethodGet, fmt.Sprintf("/api/functions/%s/versions/%d", fn.ID, number), nil, &v); err != nil {
			return err
		}
		switch v.Status {
		case "READY":
			return nil
		case "FAILED":
			return fmt.Errorf("version %d failed to load on a runner: %s", number, string(v.Failure))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("version %d was not READY after %s — is a runner serving the function's pool?", number, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (a *fnAPI) promote(ctx context.Context, fn *fnFunction, alias string, number int32) (json.RawMessage, error) {
	var out struct {
		Wiring json.RawMessage `json:"wiring"`
	}
	err := a.json(ctx, http.MethodPut, "/api/functions/"+fn.ID+"/aliases/"+url.PathEscape(alias), map[string]any{"version": number}, &out)
	return out.Wiring, err
}

// addFnPlatformCmds is the platform half of `fcdev fn`.
func init() {
	addFnPlatformCmds = func(cmd *cobra.Command) {
		cmd.AddCommand(newFnPublishCmd(), newFnPromoteCmd(), newFnDeployCmd(), newFnSetCmd(), newFnInvokeCmd(), newFnStatusCmd())
	}
}

func newFnPublishCmd() *cobra.Command {
	var pf fnPlatformFlags
	var runtime, clientID, pool string
	var create bool
	cmd := &cobra.Command{
		Use:   "publish <address> <artifact>",
		Short: "Upload an artifact and publish it as a new version (changes nothing that is running)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(&pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			fn, err := api.findFunction(ctx, args[0])
			if err != nil {
				return err
			}
			if fn == nil {
				if !create {
					return fmt.Errorf("no function %s; pass --create to create it", args[0])
				}
				if fn, err = api.createFunction(ctx, args[0], clientID, pool); err != nil {
					return err
				}
				fmt.Printf("created %s\n", fn.Address)
			}
			if runtime == "" {
				runtime = runtimeFor(args[1])
			}
			v, err := api.publish(ctx, fn, args[1], runtime)
			if err != nil {
				return err
			}
			fmt.Printf("published %s v%d (%s, %s)\n", fn.Address, v.Number, runtime, v.Digest[:12])
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().StringVar(&runtime, "runtime", "", "wasm or js (default: from the file extension)")
	cmd.Flags().BoolVar(&create, "create", false, "create the function if it does not exist")
	cmd.Flags().StringVar(&clientID, "client", "", "with --create: the owning client id (default: platform-owned)")
	cmd.Flags().StringVar(&pool, "pool", "", "with --create: the runner pool (default: default)")
	return cmd
}

func newFnPromoteCmd() *cobra.Command {
	var pf fnPlatformFlags
	var alias string
	cmd := &cobra.Command{
		Use:   "promote <address> <version>",
		Short: "Point an alias (live by default) at a READY version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var n int32
			if _, err := fmt.Sscan(strings.TrimPrefix(args[1], "v"), &n); err != nil {
				return fmt.Errorf("version %q is not a number", args[1])
			}
			api, err := newFnAPI(&pf)
			if err != nil {
				return err
			}
			fn, err := api.mustFunction(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			wiring, err := api.promote(cmd.Context(), fn, alias, n)
			if err != nil {
				return err
			}
			fmt.Printf("%s@%s → v%d\n", fn.Address, alias, n)
			if len(wiring) > 0 && string(wiring) != "{}" {
				fmt.Printf("wiring: %s\n", wiring)
			}
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().StringVar(&alias, "alias", "live", "the alias to move")
	return cmd
}

func newFnDeployCmd() *cobra.Command {
	var pf fnPlatformFlags
	var runtime, clientID, pool string
	var create bool
	var wait time.Duration
	cmd := &cobra.Command{
		Use:   "deploy <address> [dir|artifact]",
		Short: "Build (for a directory), publish, wait until a runner has loaded it, and promote to live",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			target := "."
			if len(args) == 2 {
				target = args[1]
			}
			artifact := target
			if info, err := os.Stat(target); err == nil && info.IsDir() {
				out, err := buildProject(ctx, target)
				if err != nil {
					return err
				}
				artifact = out
			}
			if runtime == "" {
				runtime = runtimeFor(artifact)
			}
			if _, _, err := describeArtifact(ctx, artifact, runtime); err != nil {
				return fmt.Errorf("%s is not publishable: %w", artifact, err)
			}
			api, err := newFnAPI(&pf)
			if err != nil {
				return err
			}
			fn, err := api.findFunction(ctx, args[0])
			if err != nil {
				return err
			}
			if fn == nil {
				if !create {
					return fmt.Errorf("no function %s; pass --create to create it", args[0])
				}
				if fn, err = api.createFunction(ctx, args[0], clientID, pool); err != nil {
					return err
				}
				fmt.Printf("created %s\n", fn.Address)
			}
			v, err := api.publish(ctx, fn, artifact, runtime)
			if err != nil {
				return err
			}
			fmt.Printf("published v%d; waiting for a runner to load it…\n", v.Number)
			if err := api.waitReady(ctx, fn, v.Number, wait); err != nil {
				return err
			}
			wiring, err := api.promote(ctx, fn, "live", v.Number)
			if err != nil {
				return err
			}
			fmt.Printf("%s@live → v%d\n", fn.Address, v.Number)
			if len(wiring) > 0 && string(wiring) != "{}" {
				fmt.Printf("wiring: %s\n", wiring)
			}
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().StringVar(&runtime, "runtime", "", "wasm or js (default: from the artifact)")
	cmd.Flags().BoolVar(&create, "create", false, "create the function if it does not exist")
	cmd.Flags().StringVar(&clientID, "client", "", "with --create: the owning client id")
	cmd.Flags().StringVar(&pool, "pool", "", "with --create: the runner pool")
	cmd.Flags().DurationVar(&wait, "wait", time.Minute, "how long to wait for a runner to load the version")
	return cmd
}

func newFnSetCmd() *cobra.Command {
	var pf fnPlatformFlags
	var configs, secrets, dbs []string
	cmd := &cobra.Command{
		Use:   "set <address>",
		Short: "Set config values, secrets and database DSNs on a function",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(&pf)
			if err != nil {
				return err
			}
			fn, err := api.mustFunction(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for kind, list := range map[string][]string{"config": configs, "secrets": secrets, "db": dbs} {
				kv, err := keyValues(list)
				if err != nil {
					return err
				}
				for k, v := range kv {
					if err := api.json(cmd.Context(), http.MethodPut, "/api/functions/"+fn.ID+"/"+kind+"/"+url.PathEscape(k), map[string]string{"value": v}, nil); err != nil {
						return fmt.Errorf("%s %s: %w", kind, k, err)
					}
					fmt.Printf("set %s %s\n", kind, k)
				}
			}
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().StringArrayVar(&configs, "config", nil, "KEY=VALUE (repeatable)")
	cmd.Flags().StringArrayVar(&secrets, "secret", nil, "KEY=VALUE (repeatable; write-only)")
	cmd.Flags().StringArrayVar(&dbs, "db", nil, "NAME=DSN (repeatable; write-only)")
	return cmd
}

func newFnInvokeCmd() *cobra.Command {
	var pf fnPlatformFlags
	var method, data string
	var headers []string
	var anonymous bool
	cmd := &cobra.Command{
		Use:   "invoke <address>[@alias|@vN] [path]",
		Short: "Call a function through a runner (with a platform token unless --anonymous)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := pf.resolve()
			if err != nil && !anonymous {
				return err
			}
			if c.RunnerURL == "" {
				c.RunnerURL = "http://127.0.0.1:8095"
			}
			path := "/"
			if len(args) == 2 {
				path = "/" + strings.TrimPrefix(args[1], "/")
			}
			var body io.Reader
			if strings.HasPrefix(data, "@") {
				b, err := os.ReadFile(data[1:])
				if err != nil {
					return err
				}
				body = bytes.NewReader(b)
			} else if data != "" {
				body = strings.NewReader(data)
			}
			if method == "" {
				method = http.MethodGet
				if body != nil {
					method = http.MethodPost
				}
			}
			req, err := http.NewRequestWithContext(cmd.Context(), method, c.RunnerURL+"/fn/"+args[0]+path, body)
			if err != nil {
				return err
			}
			for _, h := range headers {
				k, v, ok := strings.Cut(h, ":")
				if !ok {
					return fmt.Errorf("header %q is not Name: value", h)
				}
				req.Header.Add(strings.TrimSpace(k), strings.TrimSpace(v))
			}
			if !anonymous {
				tok, err := fcauth.NewClientCredentialsProvider(fcauth.ClientCredentialsConfig{
					IssuerURL: c.PlatformURL, ClientID: c.ClientID, ClientSecret: c.ClientSecret,
				}).Token(cmd.Context())
				if err != nil {
					return fmt.Errorf("sign in: %w", err)
				}
				req.Header.Set("Authorization", "Bearer "+tok)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			fmt.Fprintf(os.Stderr, "HTTP %d", resp.StatusCode)
			if id := resp.Header.Get("X-FlowCatalyst-Invocation"); id != "" {
				fmt.Fprintf(os.Stderr, " (invocation %s)", id)
			}
			fmt.Fprintln(os.Stderr)
			_, _ = io.Copy(os.Stdout, resp.Body)
			if resp.StatusCode >= 400 {
				return fmt.Errorf("the call failed with HTTP %d", resp.StatusCode)
			}
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().StringVarP(&method, "method", "X", "", "HTTP method (default GET, or POST with --data)")
	cmd.Flags().StringVarP(&data, "data", "d", "", "request body, or @file")
	cmd.Flags().StringArrayVarP(&headers, "header", "H", nil, "request header Name: value (repeatable)")
	cmd.Flags().BoolVar(&anonymous, "anonymous", false, "send no platform token")
	return cmd
}

func newFnStatusCmd() *cobra.Command {
	var pf fnPlatformFlags
	cmd := &cobra.Command{
		Use:   "status <address>",
		Short: "Show a function's versions and aliases",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(&pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			fn, err := api.mustFunction(ctx, args[0])
			if err != nil {
				return err
			}
			var versions []fnVersion
			if err := api.json(ctx, http.MethodGet, "/api/functions/"+fn.ID+"/versions", nil, &versions); err != nil {
				var wrapped struct {
					Data []fnVersion `json:"data"`
				}
				if err2 := api.json(ctx, http.MethodGet, "/api/functions/"+fn.ID+"/versions", nil, &wrapped); err2 != nil {
					return err
				}
				versions = wrapped.Data
			}
			var aliases []fnAlias
			_ = api.json(ctx, http.MethodGet, "/api/functions/"+fn.ID+"/aliases", nil, &aliases)
			pool := "default"
			if fn.Pool != nil && *fn.Pool != "" {
				pool = *fn.Pool
			}
			fmt.Printf("%s (%s)  runner pool %s  warm %v\n", fn.Address, fn.ID, pool, fn.Warm)
			named := map[int32][]string{}
			for _, a := range aliases {
				named[a.Version] = append(named[a.Version], a.Name)
			}
			for _, v := range versions {
				line := fmt.Sprintf("  v%-4d %-9s %-4s %s", v.Number, v.Status, v.Runtime, v.Digest[:12])
				if n := named[v.Number]; len(n) > 0 {
					line += "  ← " + strings.Join(n, ", ")
				}
				fmt.Println(line)
			}
			return nil
		},
	}
	pf.add(cmd)
	return cmd
}
