package main

import (
	"bytes"
	"context"
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

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
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
	tokens tokenSource
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
		tokens: t.tokenSource(),
		http:   &http.Client{Timeout: 2 * time.Minute},
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
		return a.target.wrapAuthErr("sign in to "+redactURL(a.c.PlatformURL), err)
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
	// ServiceAccountCode / ServiceAccountID: the function's own webhook
	// verifier account; absent when it uses its application's.
	ServiceAccountID   *string `json:"serviceAccountId"`
	ServiceAccountCode *string `json:"serviceAccountCode"`
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
func (a *fnAPI) createFunction(ctx context.Context, address, clientID, pool, serviceAccount string) (*fnFunction, error) {
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
	if serviceAccount != "" {
		req["serviceAccountId"] = serviceAccount // the platform resolves a code or an id
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
		cmd.AddCommand(newFnPublishCmd(), newFnPromoteCmd(), newFnDeployCmd(), newFnSetCmd(), newFnInvokeCmd(), newFnStatusCmd(), newFnPruneCmd(), newFnDeleteCmd(), newFnTraceCmd())
	}
}

func newFnPublishCmd() *cobra.Command {
	var pf fnPlatformFlags
	var runtime, clientID, pool, serviceAccount string
	var create, yes, asJSON bool
	cmd := &cobra.Command{
		Use:   "publish <address> <artifact>",
		Short: "Upload an artifact and publish it as a new version (changes nothing that is running)",
		Long: `Uploads an artifact and publishes it as a version. It never promotes: no alias
moves and no subscription or schedule is wired.

Publishing is idempotent per digest (sha256 of the artifact). The function's
versions are read first; when one already has the digest nothing is uploaded or
written and no confirmation is asked:

  new: <address> vN (<runtime>, <digest12>)
  unchanged: <address> vN already has digest <digest12>
  requeued: <address> vN was FAILED (<reason>), retrying

A version a runner marked FAILED is never "unchanged": the failure is often
transient (an artifact-store blip), so publishing the same digest again
re-queues that version (same number, back to PUBLISHED) for a runner to retry.

--json prints ONE JSON object on stdout (everything else goes to stderr):
  {"address","functionId","version","digest","created","requeued"?,"runtime","client"?}
created is true when this run created the version; requeued (only when true)
means the digest's version was FAILED and this run re-queued it; digest is the
full sha256.

--create --client <client> sets the owning client of a new function. <client>
is the client's code (e.g. bevco) or its clt_... id; --client is ignored for a
function that already exists.
--create --service-account <account> sets the function's webhook verifier
identity: the service account (code or id) whose signing secret its
auth: webhook endpoints verify deliveries with. Set it when the deliveries are
signed by an account other than the application's (for example a connection's
own account); omitted, the application's service account is used. Change it
later with: fcdev fn set <address> --service-account <account>
Like --client it is ignored for a function that already exists.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			ctx := cmd.Context()
			address, artifact := args[0], args[1]
			data, err := os.ReadFile(artifact)
			if err != nil {
				return err
			}
			digest := digestHex(data)
			if runtime == "" {
				runtime = runtimeFor(artifact)
			}
			fn, err := api.findFunction(ctx, address)
			if err != nil {
				return err
			}
			if fn == nil && !create {
				return fmt.Errorf("no function %s; pass --create to create it", address)
			}
			var failed *fnVersion // the digest's version, when the platform latched it FAILED
			if fn != nil {
				existing, err := api.findDigest(ctx, fn, digest)
				if err != nil {
					return err
				}
				if versionFailed(existing) {
					failed = existing
				}
				if existing != nil && !versionFailed(existing) {
					rep.say("unchanged: %s v%d already has digest %s", fn.Address, existing.Number, short(digest))
					return rep.emit(fnPublishJSON{Address: fn.Address, FunctionID: fn.ID, Version: existing.Number, Digest: digest, Created: false, Runtime: firstNonEmpty(existing.Runtime, runtime), Client: ownerClient(fn, fnClient{})})
				}
			}
			var owner fnClient
			if fn == nil && clientID != "" {
				if owner, err = api.resolveClient(ctx, clientID); err != nil {
					return err
				}
				rep.say("owning client: %s", owner)
			}
			if err := api.confirm("publish "+address, yes); err != nil {
				return err
			}
			if fn == nil {
				if fn, err = api.createFunction(ctx, address, owner.ID, pool, serviceAccount); err != nil {
					return err
				}
				rep.say("created %s", fn.Address)
			}
			v, err := api.publishBytes(ctx, fn, data, digest, runtime)
			if err != nil {
				return err
			}
			if failed != nil {
				// The digest's version was FAILED: publishing it again
				// re-queued it (no new version).
				if err := checkRequeued(fn.Address, failed, v); err != nil {
					return err
				}
				rep.say("%s", requeuedLine(fn.Address, failed))
				return rep.emit(fnPublishJSON{Address: fn.Address, FunctionID: fn.ID, Version: v.Number, Digest: digest, Created: false, Requeued: true, Runtime: firstNonEmpty(v.Runtime, runtime), Client: ownerClient(fn, owner)})
			}
			rep.say("new: %s v%d (%s, %s)", fn.Address, v.Number, runtime, short(digest))
			return rep.emit(fnPublishJSON{Address: fn.Address, FunctionID: fn.ID, Version: v.Number, Digest: digest, Created: true, Runtime: runtime, Client: ownerClient(fn, owner)})
		},
	}
	pf.add(cmd)
	cmd.Flags().StringVar(&runtime, "runtime", "", "wasm or js (default: from the file extension)")
	cmd.Flags().BoolVar(&create, "create", false, "create the function if it does not exist")
	cmd.Flags().StringVar(&clientID, "client", "", "with --create: the owning client, by code (e.g. bevco) or id (clt_...) (default: platform-owned)")
	cmd.Flags().StringVar(&pool, "pool", "", "with --create: the runner pool (default: default)")
	cmd.Flags().StringVar(&serviceAccount, "service-account", "", "with --create: the webhook verifier service account, by code or id (default: the application's)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON object on stdout; human output goes to stderr")
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
	var runtime, clientID, pool, serviceAccount string
	var create, yes, dryRun, noPromote, skipUnchanged, asJSON bool
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
--client is ignored for a function that already exists.
--create --service-account <account> sets the function's webhook verifier
identity: the service account (code or id) whose signing secret its
auth: webhook endpoints verify deliveries with. Set it when the deliveries are
signed by an account other than the application's (for example a connection's
own account); omitted, the application's service account is used. Change it
later with: fcdev fn set <address> --service-account <account>
Like --client it is ignored for a function that already exists.

For pipelines (the artifact's sha256 digest is the identity of a version):

  --skip-unchanged  when the version the live alias points at already has this
                    digest, do nothing and exit 0, printing
                      unchanged: <address> live vN already has digest <digest12>
                    (no publish, no wait, no promote, no confirmation, no
                    remote write). When the digest exists as a version that is
                    not live, the upload is skipped but it is still promoted.
                    A version that is FAILED is never "unchanged": the digest is
                    published again, which re-queues that version (same number)
                    for a runner to retry, and the run waits and promotes as for
                    a new version, printing
                      requeued: <address> vN was FAILED (<reason>), retrying
  --no-promote      publish and wait until a runner has loaded the version
                    (READY), but leave live where it is; subscriptions and
                    schedules are only wired at promote.
  --json            print ONE JSON object on stdout, human output on stderr:
                    {"address","functionId","version","digest","created",
                     "requeued"?,"promoted","skipped","runtime","client"?}
                    created: this run created the version; requeued: the
                    digest's version was FAILED and this run re-queued it;
                    promoted: live was
                    moved by this run; skipped: --skip-unchanged did nothing.

A run that makes no remote write (skipped, or --no-promote of an existing
digest) needs no prod confirmation; any other write to a prod profile does.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			address := args[0]
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
			d, _, err := fnDescribeArtifact(ctx, artifact, runtime)
			if err != nil {
				return fmt.Errorf("%s is not publishable: %w", artifact, err)
			}
			data, err := os.ReadFile(artifact)
			if err != nil {
				return err
			}
			plan := &deployPlan{
				address: address, artifact: artifact, runtime: runtime, describe: d, digest: digestHex(data),
				size: len(data), wait: wait, noPromote: noPromote, skipUnchanged: skipUnchanged,
			}
			if plan.fn, err = api.findFunction(ctx, address); err != nil {
				return err
			}
			if plan.fn == nil && !create {
				return fmt.Errorf("no function %s; pass --create to create it", address)
			}
			if plan.fn != nil {
				if plan.existing, err = api.findDigest(ctx, plan.fn, plan.digest); err != nil {
					return err
				}
				if plan.existing != nil {
					if plan.live, err = api.liveVersion(ctx, plan.fn); err != nil {
						return err
					}
				}
			} else if clientID != "" {
				if plan.owner, err = api.resolveClient(ctx, clientID); err != nil {
					return err
				}
			}
			result := fnDeployJSON{Address: address, Digest: plan.digest, Runtime: runtime, Client: ownerClient(plan.fn, plan.owner)}
			if plan.fn != nil {
				result.FunctionID = plan.fn.ID
			}
			if plan.existing != nil {
				result.Version = plan.existing.Number
			}
			if dryRun {
				plan.dryRunReport(rep.human, api.c.PlatformURL)
				result.Skipped, result.DryRun = plan.skip(), true
				return rep.emit(result)
			}
			if plan.skip() {
				rep.say("%s", plan.unchangedLive())
				result.Skipped = true
				return rep.emit(result)
			}
			if plan.needsWrite() {
				action := "deploy " + address
				if noPromote {
					action = "publish " + address + " (without promoting)"
				}
				if err := api.confirm(action, yes); err != nil {
					return err
				}
			}
			fn := plan.fn
			if fn == nil {
				if plan.owner.ID != "" {
					rep.say("owning client: %s", plan.owner)
				}
				if fn, err = api.createFunction(ctx, address, plan.owner.ID, pool, serviceAccount); err != nil {
					return err
				}
				rep.say("created %s", fn.Address)
				result.FunctionID, result.Client = fn.ID, ownerClient(fn, plan.owner)
			}
			v := plan.existing
			switch {
			case v == nil:
				if v, err = api.publishBytes(ctx, fn, data, plan.digest, runtime); err != nil {
					return err
				}
				result.Created = true
				rep.say("new: %s v%d (%s, %s)", fn.Address, v.Number, runtime, short(plan.digest))
			case versionFailed(v):
				// FAILED is not "unchanged": publish the digest again to
				// re-queue it, then wait for a runner as for a new version.
				was := v
				if v, err = api.publishBytes(ctx, fn, data, plan.digest, runtime); err != nil {
					return err
				}
				if err := checkRequeued(fn.Address, was, v); err != nil {
					return err
				}
				result.Requeued = true
				rep.say("%s", requeuedLine(fn.Address, was))
			default:
				rep.say("unchanged: %s v%d already has digest %s", fn.Address, v.Number, short(plan.digest))
			}
			result.Version = v.Number
			if plan.existing == nil || plan.existing.Status != "READY" {
				rep.say("waiting for a runner to load v%d…", v.Number)
				if err := api.waitReady(ctx, fn, v.Number, wait); err != nil {
					return err
				}
			}
			if noPromote {
				rep.say("not promoted: %s v%d is READY; live was not moved (--no-promote)", fn.Address, v.Number)
				return rep.emit(result)
			}
			wiring, err := api.promote(ctx, fn, "live", v.Number)
			if err != nil {
				return err
			}
			result.Promoted = true
			rep.say("%s@live → v%d", fn.Address, v.Number)
			if len(wiring) > 0 && string(wiring) != "{}" {
				rep.say("wiring: %s", wiring)
			}
			return rep.emit(result)
		},
	}
	pf.add(cmd)
	cmd.Flags().StringVar(&runtime, "runtime", "", "wasm or js (default: from the artifact)")
	cmd.Flags().BoolVar(&create, "create", false, "create the function if it does not exist")
	cmd.Flags().StringVar(&clientID, "client", "", "with --create: the owning client, by code (e.g. bevco, stable across environments) or id (clt_...)")
	cmd.Flags().StringVar(&pool, "pool", "", "with --create: the runner pool")
	cmd.Flags().StringVar(&serviceAccount, "service-account", "", "with --create: the webhook verifier service account, by code or id (default: the application's)")
	cmd.Flags().DurationVar(&wait, "wait", time.Minute, "how long to wait for a runner to load the version")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "build and validate, then print what would be published and promoted; writes nothing remotely")
	cmd.Flags().BoolVar(&noPromote, "no-promote", false, "publish and wait until READY, but do not move live")
	cmd.Flags().BoolVar(&skipUnchanged, "skip-unchanged", false, "do nothing (exit 0) when the live version already has this artifact's digest")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON object on stdout; human output goes to stderr")
	addYesFlag(cmd, &yes)
	return cmd
}

// addYesFlag registers --yes for a state-changing command.
func addYesFlag(cmd *cobra.Command, yes *bool) {
	cmd.Flags().BoolVar(yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
}

func newFnSetCmd() *cobra.Command {
	var pf fnPlatformFlags
	var configs, secrets, dbs []string
	var serviceAccount string
	var clearServiceAccount, yes bool
	cmd := &cobra.Command{
		Use:   "set <address>",
		Short: "Set config values, secrets, database DSNs and the webhook service account on a function",
		Long: `Sets config values (--config), secrets (--secret) and database DSNs (--db) on a
function, and its webhook verifier identity.

--service-account <account> names the service account (by code or id) whose
signing secret the function's auth: webhook endpoints verify deliveries with.
The platform validates that it exists, is active and that you may use it. The
change reaches runners within about a minute (the platform caches service
account credentials for one minute). --clear-service-account (or
--service-account "") removes it, returning the function to its application's
service account.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			fn, err := api.mustFunction(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			saChanged := cmd.Flags().Changed("service-account") || clearServiceAccount
			if saChanged && clearServiceAccount && serviceAccount != "" {
				return fmt.Errorf("--service-account and --clear-service-account are mutually exclusive")
			}
			if len(configs)+len(secrets)+len(dbs) > 0 || saChanged {
				if err := api.confirm("change settings of "+fn.Address, yes); err != nil {
					return err
				}
			}
			if saChanged {
				req := map[string]any{}
				if clearServiceAccount || serviceAccount == "" {
					req["clearServiceAccount"] = true
				} else {
					req["serviceAccountId"] = serviceAccount // the platform resolves a code or an id
				}
				if err := api.json(cmd.Context(), http.MethodPatch, "/api/functions/"+fn.ID, req, nil); err != nil {
					return fmt.Errorf("service account: %w", err)
				}
				if clearServiceAccount || serviceAccount == "" {
					fmt.Printf("cleared service account on %s (back to the application's)\n", fn.Address)
				} else {
					fmt.Printf("set service account %s on %s\n", serviceAccount, fn.Address)
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
	cmd.Flags().StringVar(&serviceAccount, "service-account", "", "the webhook verifier service account, by code or id (empty string clears it)")
	cmd.Flags().BoolVar(&clearServiceAccount, "clear-service-account", false, "remove the function's own service account (back to the application's)")
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
				tok, err := t.tokenSource().Token(cmd.Context())
				if err != nil {
					return t.wrapAuthErr("sign in", err)
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
			if fn.ServiceAccountID != nil {
				fmt.Printf("  webhook service account %s\n", firstNonEmpty(derefStr(fn.ServiceAccountCode), *fn.ServiceAccountID))
			} else {
				fmt.Println("  webhook service account: the application's")
			}
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

func newFnPruneCmd() *cobra.Command {
	var pf fnPlatformFlags
	var keep int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "prune <address>",
		Short: "Delete a function's oldest untagged versions (and their artifacts)",
		Long: `Deletes a function's oldest UNTAGGED versions beyond the keep limit, with their
stored artifacts when no remaining version uses them. A version is tagged when
any alias (live or another) points at it: tagged versions are never pruned and
do not count. The newest version and a PUBLISHED version younger than an hour are
never pruned. Version numbers are not reused.

The platform prunes automatically after every publish (FC_FUNCTIONS_KEEP_VERSIONS,
default 20); this is for catching a function that is already over the limit up,
or for a different --keep. Use --dry-run to see what would go.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			fn, err := api.mustFunction(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			body := map[string]any{"dryRun": dryRun}
			if cmd.Flags().Changed("keep") {
				if keep < 1 {
					return errors.New("--keep must be >= 1")
				}
				body["keep"] = keep
			}
			var res struct {
				Keep             int `json:"keep"`
				Untagged         int `json:"untagged"`
				ArtifactsDeleted int `json:"artifactsDeleted"`
				Pruned           []struct {
					Number int32 `json:"number"`
				} `json:"pruned"`
			}
			if err := api.json(cmd.Context(), http.MethodPost, "/api/functions/"+fn.ID+"/versions/prune", body, &res); err != nil {
				return err
			}
			verb := "pruned"
			if dryRun {
				verb = "would prune"
			}
			nums := make([]string, len(res.Pruned))
			for i, p := range res.Pruned {
				nums[i] = fmt.Sprintf("v%d", p.Number)
			}
			fmt.Printf("%s: %d untagged version(s), keep %d, %s %d", fn.Address, res.Untagged, res.Keep, verb, len(res.Pruned))
			if len(nums) > 0 {
				fmt.Printf(" (%s)", strings.Join(nums, ", "))
			}
			if !dryRun && res.ArtifactsDeleted > 0 {
				fmt.Printf("; %d artifact(s) deleted", res.ArtifactsDeleted)
			}
			fmt.Println()
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().IntVar(&keep, "keep", 0, "untagged versions to keep (default: the platform's FC_FUNCTIONS_KEEP_VERSIONS)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be pruned; change nothing")
	return cmd
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

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
