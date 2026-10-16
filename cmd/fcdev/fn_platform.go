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

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	fcauth "github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
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

// fnCLICredentialsEnv overrides the credential file's location, for both
// the writer (fcdev start) and the reader (fcdev fn ...). Isolated or test
// instances set it so they never overwrite the developer's real file.
const fnCLICredentialsEnv = "FC_FN_CLI_CREDENTIALS_PATH"

// fnCLICredentialsPath is where fcdev start leaves the dev CLI credential.
func fnCLICredentialsPath() (string, error) {
	if p := os.Getenv(fnCLICredentialsEnv); p != "" {
		return p, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	// fng-: the other platforms' fcdevs keep their own fn-cli.json in this
	// shared directory, in another format (the fng_ prefix decision).
	return filepath.Join(dir, "flowcatalyst-dev", "fng-cli.json"), nil
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
type fnPlatformFlags struct{ platform, runner, clientID, clientSecret, profile string }

func (f *fnPlatformFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.profile, "profile", "", "connection profile (env FC_PROFILE; see `fcdev profile add`); required for any non-loopback platform")
	cmd.Flags().StringVar(&f.platform, "platform", "", "loopback platform URL (env FC_PLATFORM_URL; default: the local fcdev); remote platforms need --profile")
	cmd.Flags().StringVar(&f.runner, "runner", "", "function runner URL for invoke (env FC_RUNNER_URL; default: the profile's, else the local fcdev's)")
	cmd.Flags().StringVar(&f.clientID, "client-id", "", "OAuth client id for a loopback platform (env FC_CLIENT_ID)")
	cmd.Flags().StringVar(&f.clientSecret, "client-secret", "", "OAuth client secret for a loopback platform (env FC_CLIENT_SECRET)")
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// readDevCredentials reads the dev CLI credential file.
func readDevCredentials(path string) (fnCredentials, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return fnCredentials{}, false
	}
	var dev fnCredentials
	if json.Unmarshal(b, &dev) != nil {
		return fnCredentials{}, false
	}
	return dev, true
}

// resolve settles the credential (see resolveTarget for the rules), with
// warnings discarded. Kept for callers that only need the credential.
func (f *fnPlatformFlags) resolve() (fnCredentials, error) {
	t, err := f.resolveTarget(context.Background(), io.Discard, true)
	if err != nil {
		return fnCredentials{}, err
	}
	return t.creds, nil
}

// fnAPI is a minimal authenticated client for the /api/functions routes.
type fnAPI struct {
	c      fnCredentials
	target *fnTarget
	prompt *fnPrompter
	tokens *fcauth.ClientCredentialsProvider
	http   *http.Client
}

// newFnAPI resolves the target, prints the safety banner and returns a client.
func newFnAPI(cmd *cobra.Command, f *fnPlatformFlags) (*fnAPI, error) {
	t, err := fnConnect(cmd, f, true)
	if err != nil {
		return nil, err
	}
	c := t.creds
	return &fnAPI{
		c: c, target: t, prompt: newFnPrompter(cmd),
		tokens: fcauth.NewClientCredentialsProvider(fcauth.ClientCredentialsConfig{
			IssuerURL: c.PlatformURL, ClientID: c.ClientID, ClientSecret: c.ClientSecret,
			HTTPClient: noRedirectClient(30 * time.Second),
		}),
		http: &http.Client{Timeout: 2 * time.Minute},
	}, nil
}

// confirm gates a state-changing step; see fnTarget.confirmMutation.
func (a *fnAPI) confirm(action string, yes bool) error {
	return a.target.confirmMutation(a.prompt, action, yes)
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
		return fmt.Errorf("sign in to %s: %w", redactURL(a.c.PlatformURL), scrubErr(err, a.c.ClientSecret))
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
	AppCode  string  `json:"applicationCode"`
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

// fnClient is a resolved owning client. Code is empty when the value was
// an id passed through without a lookup.
type fnClient struct{ ID, Code string }

func (c fnClient) String() string {
	if c.Code == "" {
		return c.ID
	}
	return c.Code + " (" + c.ID + ")"
}

// looksLikeClientID reports whether v has the shape of a client id: the
// clt_ prefix followed by a 13-character Crockford base32 TSID. Anything
// else is treated as a client code (identifier).
func looksLikeClientID(v string) bool {
	rest, ok := strings.CutPrefix(v, "clt_")
	if !ok {
		return false
	}
	_, ok = tsid.DecodeCrockford(rest)
	return ok
}

// resolveClient turns a --client value into a client id. A client id passes
// through untouched (no lookup); anything else is a client code, looked up
// with GET /api/clients/by-identifier/{code} (read-only).
func (a *fnAPI) resolveClient(ctx context.Context, value string) (fnClient, error) {
	if looksLikeClientID(value) {
		return fnClient{ID: value}, nil
	}
	var c struct {
		ID         string `json:"id"`
		Identifier string `json:"identifier"`
	}
	err := a.json(ctx, http.MethodGet, "/api/clients/by-identifier/"+url.PathEscape(value), nil, &c)
	var ae *apiError
	switch {
	case err == nil:
		if c.ID == "" {
			return fnClient{}, fmt.Errorf("client %q: the platform returned no id", value)
		}
		return fnClient{ID: c.ID, Code: firstNonEmpty(c.Identifier, value)}, nil
	case errors.As(err, &ae) && ae.Status == http.StatusNotFound:
		return fnClient{}, fmt.Errorf("no client with code %q on %s (pass the client's clt_ id instead if it is not a code)", value, a.target.banner())
	case errors.As(err, &ae) && ae.Status == http.StatusForbidden:
		return fnClient{}, fmt.Errorf("cannot look up client code %q: this credential may not read clients (%s); pass the client's clt_ id instead", value, firstNonEmpty(ae.Message, ae.Detail, ae.Code))
	}
	return fnClient{}, fmt.Errorf("look up client code %q: %w", value, err)
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
		cmd.AddCommand(newFnPublishCmd(), newFnPromoteCmd(), newFnDeployCmd(), newFnSetCmd(), newFnInvokeCmd(), newFnStatusCmd(), newFnDeleteCmd())
	}
}

func newFnPublishCmd() *cobra.Command {
	var pf fnPlatformFlags
	var runtime, clientID, pool string
	var create, yes bool
	cmd := &cobra.Command{
		Use:   "publish <address> <artifact>",
		Short: "Upload an artifact and publish it as a new version (changes nothing that is running)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			fn, err := api.findFunction(ctx, args[0])
			if err != nil {
				return err
			}
			if fn == nil && !create {
				return fmt.Errorf("no function %s; pass --create to create it", args[0])
			}
			var owner fnClient
			if fn == nil && clientID != "" {
				if owner, err = api.resolveClient(ctx, clientID); err != nil {
					return err
				}
				fmt.Printf("owning client: %s\n", owner)
			}
			if err := api.confirm("publish "+args[0], yes); err != nil {
				return err
			}
			if fn == nil {
				if fn, err = api.createFunction(ctx, args[0], owner.ID, pool); err != nil {
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
	cmd.Flags().StringVar(&clientID, "client", "", "with --create: the owning client, by code (e.g. bevco) or id (clt_...) (default: platform-owned)")
	cmd.Flags().StringVar(&pool, "pool", "", "with --create: the runner pool (default: default)")
	addYesFlag(cmd, &yes)
	return cmd
}

func newFnPromoteCmd() *cobra.Command {
	var pf fnPlatformFlags
	var alias string
	var yes bool
	cmd := &cobra.Command{
		Use:   "promote <address> <version>",
		Short: "Point an alias (live by default) at a READY version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var n int32
			if _, err := fmt.Sscan(strings.TrimPrefix(args[1], "v"), &n); err != nil {
				return fmt.Errorf("version %q is not a number", args[1])
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			fn, err := api.mustFunction(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := api.confirm(fmt.Sprintf("promote %s@%s to v%d", fn.Address, alias, n), yes); err != nil {
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
	addYesFlag(cmd, &yes)
	return cmd
}

func newFnDeployCmd() *cobra.Command {
	var pf fnPlatformFlags
	var runtime, clientID, pool string
	var create, yes, dryRun bool
	var wait time.Duration
	cmd := &cobra.Command{
		Use:   "deploy <address> [dir|artifact]",
		Short: "Build (for a directory), publish, wait until a runner has loaded it, and promote to live",
		Long: `Builds (for a directory), publishes, waits until a runner has loaded the
version, and promotes it to live.

--dry-run builds and validates the describe document, looks the function up
(read-only) and prints what would be published and promoted, without writing
anything to the platform.

--create --client <client> sets the owning client of a new function. <client>
is the client's code (identifier, e.g. bevco, the same in every environment)
or its clt_... id. A code is resolved with a read-only lookup of the client
(needs permission to read clients; otherwise pass the id); an id is used as-is.
--client is ignored for a function that already exists.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
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
			d, _, err := describeArtifact(ctx, artifact, runtime)
			if err != nil {
				return fmt.Errorf("%s is not publishable: %w", artifact, err)
			}
			fn, err := api.findFunction(ctx, args[0])
			if err != nil {
				return err
			}
			if fn == nil && !create {
				return fmt.Errorf("no function %s; pass --create to create it", args[0])
			}
			var owner fnClient
			if fn == nil && clientID != "" {
				if owner, err = api.resolveClient(ctx, clientID); err != nil {
					return err
				}
			}
			if dryRun {
				return deployDryRun(cmd.OutOrStdout(), api, fn, args[0], artifact, runtime, d, wait, owner)
			}
			if err := api.confirm("deploy "+args[0], yes); err != nil {
				return err
			}
			if fn == nil {
				if owner.ID != "" {
					fmt.Printf("owning client: %s\n", owner)
				}
				if fn, err = api.createFunction(ctx, args[0], owner.ID, pool); err != nil {
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
	cmd.Flags().StringVar(&clientID, "client", "", "with --create: the owning client, by code (e.g. bevco, stable across environments) or id (clt_...)")
	cmd.Flags().StringVar(&pool, "pool", "", "with --create: the runner pool")
	cmd.Flags().DurationVar(&wait, "wait", time.Minute, "how long to wait for a runner to load the version")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "build and validate, then print what would be published and promoted; writes nothing remotely")
	addYesFlag(cmd, &yes)
	return cmd
}

// addYesFlag registers --yes for a state-changing command.
func addYesFlag(cmd *cobra.Command, yes *bool) {
	cmd.Flags().BoolVar(yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
}

// deployDryRun reports what deploy would do. It performs no remote write.
func deployDryRun(out io.Writer, api *fnAPI, fn *fnFunction, address, artifact, runtime string, d *abi.Describe, wait time.Duration, owner fnClient) error {
	data, err := os.ReadFile(artifact)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	fmt.Fprintf(out, "dry run: nothing is written to %s\n", redactURL(api.c.PlatformURL))
	printDescribeTo(out, artifact, d)
	fmt.Fprintf(out, "artifact: %s (%d bytes, sha256 %s, runtime %s)\n", artifact, len(data), hex.EncodeToString(sum[:])[:12], runtime)
	if fn == nil {
		fmt.Fprintf(out, "would create function %s\n", address)
		if owner.ID != "" {
			fmt.Fprintf(out, "owning client: %s\n", owner)
		}
	} else {
		fmt.Fprintf(out, "function %s exists (%s)\n", fn.Address, fn.ID)
	}
	fmt.Fprintf(out, "would upload the artifact and publish a new version, wait up to %s for a runner to load it, then promote it to live\n", wait)
	return nil
}

func newFnSetCmd() *cobra.Command {
	var pf fnPlatformFlags
	var configs, secrets, dbs []string
	var yes bool
	cmd := &cobra.Command{
		Use:   "set <address>",
		Short: "Set config values, secrets and database DSNs on a function",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			fn, err := api.mustFunction(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if len(configs)+len(secrets)+len(dbs) > 0 {
				if err := api.confirm("change settings of "+fn.Address, yes); err != nil {
					return err
				}
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
	addYesFlag(cmd, &yes)
	return cmd
}

func newFnInvokeCmd() *cobra.Command {
	var pf fnPlatformFlags
	var method, data string
	var headers []string
	var anonymous, yes bool
	cmd := &cobra.Command{
		Use:   "invoke <address>[@alias|@vN] [path]",
		Short: "Call a function through a runner (with a platform token unless --anonymous)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := fnConnect(cmd, &pf, !anonymous)
			if err != nil {
				return err
			}
			c := t.creds
			if c.RunnerURL == "" {
				if t.profile != nil {
					return fmt.Errorf("profile %s has no runner URL: pass --runner, or add one to the profile", t.profile.Name)
				}
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
			// A call can run arbitrary function code, so anything but a plain
			// read counts as a change for the prod guard.
			if method != http.MethodGet && method != http.MethodHead {
				if err := t.confirmMutation(newFnPrompter(cmd), "invoke "+args[0]+" ("+method+")", yes); err != nil {
					return err
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
					HTTPClient: noRedirectClient(30 * time.Second),
				}).Token(cmd.Context())
				if err != nil {
					return fmt.Errorf("sign in: %w", scrubErr(err, c.ClientSecret))
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
	addYesFlag(cmd, &yes)
	return cmd
}

func newFnStatusCmd() *cobra.Command {
	var pf fnPlatformFlags
	cmd := &cobra.Command{
		Use:   "status <address>",
		Short: "Show a function's versions and aliases",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
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

// fnStdinIsTerminal reports whether stdin is an interactive terminal. A var so
// tests can stand in for both cases.
var fnStdinIsTerminal = func() bool {
	// Not ModeCharDevice: /dev/null is a character device but not a terminal.
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

func newFnDeleteCmd() *cobra.Command {
	var pf fnPlatformFlags
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <address>",
		Short: "Permanently delete a function with all its versions, aliases, wiring and artifacts",
		Long: `Deletes the function, every version and alias, its wiring (subscriptions and
scheduled jobs) and its artifacts. This cannot be undone.

Interactively you must type the function's address to confirm. In scripts pass
--yes; without --yes a non-terminal stdin is refused.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			address := args[0]
			if !yes && !fnStdinIsTerminal() {
				return errors.New("refusing to delete without a terminal to confirm on; pass --yes to delete non-interactively")
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			fn, err := api.findFunction(ctx, address)
			if err != nil {
				return err
			}
			if fn == nil {
				return fmt.Errorf("no function %s", address)
			}
			out := cmd.OutOrStdout()
			client := "(platform-wide)"
			if fn.ClientID != nil && *fn.ClientID != "" {
				client = *fn.ClientID
			}
			app := firstNonEmpty(fn.AppCode, strings.SplitN(fn.Address, ".", 2)[0])
			fmt.Fprintf(out, "About to PERMANENTLY delete %s\n  id          %s\n  application %s\n  client      %s\n", fn.Address, fn.ID, app, client)
			if vs, err := api.listVersions(ctx, fn.ID); err == nil {
				fmt.Fprintf(out, "  versions    %d version(s)", len(vs))
				var aliases []fnAlias
				if api.json(ctx, http.MethodGet, "/api/functions/"+fn.ID+"/aliases", nil, &aliases) == nil {
					for _, a := range aliases {
						if a.Name == "live" {
							fmt.Fprintf(out, ", live = v%d", a.Version)
						}
					}
				}
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, "  This also removes every alias, wiring (subscriptions and scheduled jobs) and artifact.")
			if err := api.confirm("PERMANENTLY delete "+fn.Address, yes); err != nil {
				return err
			}
			if !yes {
				fmt.Fprintf(out, "Type the address (%s) to confirm: ", fn.Address)
				line, _ := api.prompt.in.ReadString('\n')
				if strings.TrimSpace(line) != fn.Address {
					return errors.New("confirmation did not match; nothing deleted")
				}
			}
			if err := api.json(ctx, http.MethodDelete, "/api/functions/"+fn.ID, nil, nil); err != nil {
				return fmt.Errorf("delete %s: %w", fn.Address, err)
			}
			fmt.Fprintf(out, "deleted %s\n", fn.Address)
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the typed confirmation (required when stdin is not a terminal)")
	return cmd
}

// listVersions returns a function's versions (the platform answers either a
// bare array or a {data: [...]} page).
func (a *fnAPI) listVersions(ctx context.Context, id string) ([]fnVersion, error) {
	var raw json.RawMessage
	if err := a.json(ctx, http.MethodGet, "/api/functions/"+id+"/versions", nil, &raw); err != nil {
		return nil, err
	}
	var vs []fnVersion
	if json.Unmarshal(raw, &vs) == nil {
		return vs, nil
	}
	var page struct {
		Data []fnVersion `json:"data"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	return page.Data, nil
}
