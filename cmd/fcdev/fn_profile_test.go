package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// Safety net: no test in this package may reach the real OS keychain, the
// real profile file or the developer's real dev credential file. The
// sentinels point at directories that do not exist, so an accidental write
// fails instead of landing somewhere real, and the keychain is a fake that
// refuses to run unless a test installs its own.
func init() {
	fnKeychain = refusingKeychain{}
	_ = os.Setenv(fnProfilesEnv, "/nonexistent-fcdev-test-dir/profiles.toml")
	_ = os.Setenv(fnCLICredentialsEnv, "/nonexistent-fcdev-test-dir/fng-cli.json")
}

type refusingKeychain struct{}

func (refusingKeychain) Get(string) (string, error) { panic("test touched the real keychain") }
func (refusingKeychain) Set(string, string) error   { panic("test touched the real keychain") }
func (refusingKeychain) Delete(string) error        { panic("test touched the real keychain") }

// memKeychain is the in-memory fake.
type memKeychain struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemKeychain() *memKeychain { return &memKeychain{m: map[string]string{}} }

func (k *memKeychain) Get(a string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[a]
	if !ok {
		return "", ErrKeychainNotFound
	}
	return v, nil
}

func (k *memKeychain) Set(a, s string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[a] = s
	return nil
}

func (k *memKeychain) Delete(a string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.m[a]; !ok {
		return ErrKeychainNotFound
	}
	delete(k.m, a)
	return nil
}

const testSecret = "S3CR3T-sentinel-7f3a"

// fnTestEnv isolates one test: temp profile file and dev-credential paths,
// cleared connection variables, a fake keychain, and a fake terminal switch.
type fnTestEnv struct {
	dir      string
	profiles string
	devCreds string
	kc       *memKeychain
	tty      bool
}

func newFnTestEnv(t *testing.T) *fnTestEnv {
	t.Helper()
	e := &fnTestEnv{dir: t.TempDir(), kc: newMemKeychain()}
	e.profiles = filepath.Join(e.dir, "cfg", "profiles.toml")
	e.devCreds = filepath.Join(e.dir, "dev", "fng-cli.json")
	t.Setenv("HOME", e.dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(e.dir, "xdg"))
	t.Setenv(fnProfilesEnv, e.profiles)
	t.Setenv(fnCLICredentialsEnv, e.devCreds)
	for _, v := range append([]string{"FC_PROFILE"}, fnConnEnvVars...) {
		t.Setenv(v, "")
	}
	oldKC, oldTTY := fnKeychain, fnStdinIsTerminal
	fnKeychain = e.kc
	fnStdinIsTerminal = func() bool { return e.tty }
	t.Cleanup(func() { fnKeychain, fnStdinIsTerminal = oldKC, oldTTY })
	return e
}

func (e *fnTestEnv) writeProfiles(t *testing.T, mode os.FileMode, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(e.profiles), 0o700))
	require.NoError(t, os.WriteFile(e.profiles, []byte(body), mode))
	require.NoError(t, os.Chmod(e.profiles, mode))
}

func (e *fnTestEnv) writeDev(t *testing.T, c fnCredentials) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(e.devCreds), 0o700))
	require.NoError(t, writeFnCLICredentials(c))
}

func profileTOML(name, platform, env, ref string, extra ...string) string {
	s := fmt.Sprintf("[profiles.%s]\nplatform = %q\nenv = %q\nclient_id = \"cid\"\nsecret_ref = %q\n", name, platform, env, ref)
	return s + strings.Join(extra, "\n") + "\n"
}

// runCobra executes cmd with stdin and returns stdout, stderr.
func runCobra(t *testing.T, cmd *cobra.Command, stdin string, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

func jwt(claims string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(claims)) + ".sig"
}

// profilePlatform is a fake platform that checks the client secret.
type profilePlatform struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []string
	secret   string
	tokenReq int
}

func newProfilePlatform(t *testing.T, secret string) *profilePlatform {
	p := &profilePlatform{secret: secret}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.requests = append(p.requests, r.Method+" "+r.URL.Path)
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oauth/token":
			_ = r.ParseForm()
			p.mu.Lock()
			p.tokenReq++
			p.mu.Unlock()
			if got := r.PostForm.Get("client_secret"); got != p.secret {
				w.WriteHeader(http.StatusUnauthorized)
				// A hostile/sloppy server echoing what it was sent.
				_, _ = fmt.Fprintf(w, `{"error":"invalid_client","message":"bad secret %s"}`, got)
				return
			}
			tok := jwt(`{"sub":"prn_TEST","type":"SERVICE","tier":"ANCHOR","scope":"function:manage function:view","roles":["platform:function-publisher"]}`)
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"expires_in":300,"token_type":"Bearer","scope":"function:manage function:view"}`, tok)
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions":
			_, _ = w.Write([]byte(`{"data":[{"id":"fn_1","address":"demo.logger","applicationCode":"demo"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions/fn_1/versions":
			_, _ = w.Write([]byte(`[{"number":1,"status":"READY","runtime":"wasm","digest":"aaaaaaaaaaaaaaaa"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions/fn_1/aliases":
			_, _ = w.Write([]byte(`[{"name":"live","version":1}]`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/functions/fn_1/aliases/"):
			_, _ = w.Write([]byte(`{"wiring":{}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/functions/fn_1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/functions/fn_1/config/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *profilePlatform) count(prefix string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, r := range p.requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func (p *profilePlatform) writes() int {
	return p.count("PUT ") + p.count("POST /api") + p.count("DELETE ")
}

// --- file format ----------------------------------------------------------

func TestProfileFileRoundTrip(t *testing.T) {
	e := newFnTestEnv(t)
	set := profileSet{
		"prod-ci": {Name: "prod-ci", Platform: "https://fc.example.com", Runner: "https://run.example.com", Env: "prod", ClientID: "c1", SecretRef: "command:aws ssm get-parameter --name \"/a b\"", AllowNoninteractive: true},
		"dev":     {Name: "dev", Platform: "http://localhost:8080", Env: "local", ClientID: "c2", SecretRef: "keychain"},
	}
	require.NoError(t, saveProfiles(e.profiles, set))
	st, err := os.Stat(e.profiles)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), st.Mode().Perm())
	}
	got, _, err := loadProfiles()
	require.NoError(t, err)
	require.Equal(t, set, got)
}

func TestProfileParseRejections(t *testing.T) {
	const plain = "hunter2-plaintext"
	valid := func(extra string) string {
		return "[profiles.p]\nplatform = \"https://fc.example.com\"\nenv = \"np\"\nclient_id = \"c\"\n" + extra
	}
	cases := []struct {
		name, src, want string
	}{
		{"secret-looking key", valid("client_secret = \"" + plain + "\"\n"), "looks like a secret"},
		{"password key", valid("password = \"" + plain + "\"\n"), "looks like a secret"},
		{"token key", valid("api_token = \"" + plain + "\"\n"), "looks like a secret"},
		{"unknown key", valid("color = \"red\"\n"), "unknown key"},
		{"key outside table", "platform = \"https://x\"\n", "outside a [profiles.NAME] table"},
		{"bad header", "[other.p]\n", "only [profiles.NAME]"},
		{"duplicate", valid("") + valid(""), "duplicate profile"},
		{"plain-text http remote", strings.Replace(valid(""), "https://fc", "http://fc", 1), "https is required"},
		{"creds in url", strings.Replace(valid(""), "https://fc", "https://u:p@fc", 1), "must not embed credentials"},
		{"bad env", strings.Replace(valid(""), `"np"`, `"staging"`, 1), "env must be"},
		{"missing client id", "[profiles.p]\nplatform = \"https://x.example\"\nenv = \"np\"\n", "client_id is required"},
		{"pasted secret as ref", valid("secret_ref = \"" + plain + "\"\n"), "secret_ref must be"},
		{"array value", valid("runner = [\"a\"]\n"), "quoted string"},
		{"unterminated string", valid("runner = \"https://x\n"), "unterminated"},
		{"bad bool", valid("allow_noninteractive = yes\n"), "true or false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseProfiles(c.src)
			require.Error(t, err)
			require.Contains(t, err.Error(), c.want)
			// An error never repeats a value from the file.
			require.NotContains(t, err.Error(), plain)
		})
	}
}

func TestProfileParseAcceptsCommentsAndLoopbackHTTP(t *testing.T) {
	set, err := parseProfiles("# top\n\n[profiles.l] # trailing\nplatform = 'http://127.0.0.1:9' # c\nenv = \"local\"\nclient_id = \"c#1\"\nallow_noninteractive = true\n")
	require.NoError(t, err)
	require.Equal(t, "c#1", set["l"].ClientID)
	require.Equal(t, "keychain", set["l"].SecretRef)
	require.True(t, set["l"].AllowNoninteractive)
}

func TestProfileFileModeChecks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	body := profileTOML("p", "https://fc.example.com", "np", "keychain")
	for _, c := range []struct {
		mode os.FileMode
		ok   bool
	}{{0o600, true}, {0o644, true}, {0o400, true}, {0o664, false}, {0o620, false}, {0o602, false}, {0o666, false}} {
		e := newFnTestEnv(t)
		e.writeProfiles(t, c.mode, body)
		_, _, err := loadProfiles()
		if c.ok {
			require.NoError(t, err, "%04o", c.mode)
		} else {
			require.Error(t, err, "%04o", c.mode)
			require.Contains(t, err.Error(), "writable")
		}
	}
}

// --- secret references -----------------------------------------------------

func TestParseSecretRef(t *testing.T) {
	cases := []struct {
		in   string
		kind string
		ok   bool
	}{
		{"", "keychain", true},
		{"keychain", "keychain", true},
		{"env:FC_SECRET", "env", true},
		{"env:", "", false},
		{"env:1BAD", "", false},
		{"env:A B", "", false},
		{"file:/etc/fc/secret", "file", true},
		{"file:relative/p", "", false},
		{"file:", "", false},
		{"command:aws ssm get-parameter", "command", true},
		{"command:", "", false},
		{"command:\"unterminated", "", false},
		{"plaintext-secret", "", false},
		{"vault:x", "", false},
	}
	for _, c := range cases {
		ref, err := parseSecretRef(c.in)
		if !c.ok {
			require.Error(t, err, c.in)
			require.NotContains(t, err.Error(), "plaintext-secret")
			continue
		}
		require.NoError(t, err, c.in)
		require.Equal(t, c.kind, ref.Kind, c.in)
	}
}

func TestSplitArgvNoShell(t *testing.T) {
	got, err := splitArgv(`aws ssm get-parameter --name "/fc/prod secret" --query 'Parameter.Value' --x=$(rm -rf /) ; echo a|b > f`)
	require.NoError(t, err)
	require.Equal(t, []string{"aws", "ssm", "get-parameter", "--name", "/fc/prod secret", "--query", "Parameter.Value", "--x=$(rm", "-rf", "/)", ";", "echo", "a|b", ">", "f"}, got)
}

func TestResolveSecretKinds(t *testing.T) {
	ctx := context.Background()
	e := newFnTestEnv(t)

	// keychain
	require.NoError(t, e.kc.Set("kc", testSecret+"\n"))
	got, err := resolveSecret(ctx, &Profile{Name: "kc", SecretRef: "keychain"})
	require.NoError(t, err)
	require.Equal(t, testSecret, got)
	_, err = resolveSecret(ctx, &Profile{Name: "none", SecretRef: "keychain"})
	require.ErrorContains(t, err, "fcdev profile set-secret none")

	// env
	t.Setenv("FC_TEST_SECRET", testSecret)
	got, err = resolveSecret(ctx, &Profile{Name: "e", SecretRef: "env:FC_TEST_SECRET"})
	require.NoError(t, err)
	require.Equal(t, testSecret, got)
	_, err = resolveSecret(ctx, &Profile{Name: "e", SecretRef: "env:FC_TEST_UNSET"})
	require.ErrorContains(t, err, "FC_TEST_UNSET")

	// file
	f := filepath.Join(e.dir, "secret")
	require.NoError(t, os.WriteFile(f, []byte(testSecret+"\n"), 0o600))
	got, err = resolveSecret(ctx, &Profile{Name: "f", SecretRef: "file:" + f})
	require.NoError(t, err)
	require.Equal(t, testSecret, got)
	if runtime.GOOS != "windows" {
		for _, mode := range []os.FileMode{0o640, 0o604, 0o644, 0o660, 0o666, 0o700 | 0o070} {
			require.NoError(t, os.Chmod(f, mode))
			_, err = resolveSecret(ctx, &Profile{Name: "f", SecretRef: "file:" + f})
			require.Error(t, err, "%04o", mode)
			require.Contains(t, err.Error(), "refusing")
			require.NotContains(t, err.Error(), testSecret)
		}
		require.NoError(t, os.Chmod(f, 0o400))
		_, err = resolveSecret(ctx, &Profile{Name: "f", SecretRef: "file:" + f})
		require.NoError(t, err)
	}
	_, err = resolveSecret(ctx, &Profile{Name: "f", SecretRef: "file:" + filepath.Join(e.dir, "missing")})
	require.Error(t, err)

	// command: output trimmed; failure scrubs its own stdout from the error
	old := fnRunSecretCommand
	t.Cleanup(func() { fnRunSecretCommand = old })
	fnRunSecretCommand = func(_ context.Context, argv []string) (string, string, error) {
		require.Equal(t, []string{"get", "it"}, argv)
		return "  " + testSecret + "\n", "", nil
	}
	got, err = resolveSecret(ctx, &Profile{Name: "c", SecretRef: "command:get it"})
	require.NoError(t, err)
	require.Equal(t, testSecret, got)
	fnRunSecretCommand = func(context.Context, []string) (string, string, error) {
		return testSecret, "boom " + testSecret, errors.New("exit status 3")
	}
	_, err = resolveSecret(ctx, &Profile{Name: "c", SecretRef: "command:get it"})
	require.ErrorContains(t, err, "exit status 3")
	require.NotContains(t, err.Error(), testSecret)
	fnRunSecretCommand = func(context.Context, []string) (string, string, error) { return "\n", "", nil }
	_, err = resolveSecret(ctx, &Profile{Name: "c", SecretRef: "command:get it"})
	require.ErrorContains(t, err, "empty")
}

// TestSecretCommandRunsWithoutShell re-executes the test binary as the
// "secret program" and feeds it shell metacharacters that would do
// something under /bin/sh.
func TestSecretCommandRunsWithoutShell(t *testing.T) {
	if os.Getenv("FC_TEST_HELPER_SECRET") == "1" {
		fmt.Print(strings.Join(os.Args[len(os.Args)-3:], "|"))
		os.Exit(0)
	}
	marker := filepath.Join(t.TempDir(), "pwned")
	exe, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("FC_TEST_HELPER_SECRET", "1")
	ref := fmt.Sprintf("command:%s -test.run=TestSecretCommandRunsWithoutShell $(touch %s) ;touch|%s", exe, marker, marker)
	got, err := resolveSecret(context.Background(), &Profile{Name: "x", SecretRef: ref})
	require.NoError(t, err)
	require.Contains(t, got, "$(touch")
	_, statErr := os.Stat(marker)
	require.True(t, os.IsNotExist(statErr), "a shell interpreted the command")
}

// --- resolution -------------------------------------------------------------

func TestResolveTarget(t *testing.T) {
	type tc struct {
		name      string
		profiles  string // profile file body ("" = none)
		dev       *fnCredentials
		env       map[string]string
		flags     fnPlatformFlags
		anonymous bool
		wantErr   []string
		wantWarn  []string
		check     func(t *testing.T, tg *fnTarget)
	}
	remote := "https://fc.example.com"
	p := profileTOML("np1", remote, "np", "env:FC_TEST_SECRET", "runner = \"https://run.example.com\"")
	cases := []tc{
		{
			name: "loopback flags unchanged", flags: fnPlatformFlags{platform: "http://127.0.0.1:1", clientID: "a", clientSecret: "b"},
			check: func(t *testing.T, tg *fnTarget) { require.Nil(t, tg.profile); require.Equal(t, "a", tg.creds.ClientID) },
		},
		{
			name: "dev credential file, loopback", dev: &fnCredentials{PlatformURL: "http://localhost:8080/", RunnerURL: "http://127.0.0.1:8095", ClientID: "dev", ClientSecret: "s"},
			check: func(t *testing.T, tg *fnTarget) {
				require.Equal(t, "http://localhost:8080", tg.creds.PlatformURL)
				require.Equal(t, "local", tg.envTag())
			},
		},
		{name: "remote via --platform refused", flags: fnPlatformFlags{platform: remote, clientID: "a", clientSecret: "b"}, wantErr: []string{"refusing remote platform", "fcdev profile add"}},
		{name: "remote via FC_PLATFORM_URL refused", env: map[string]string{"FC_PLATFORM_URL": remote, "FC_CLIENT_ID": "a", "FC_CLIENT_SECRET": "b"}, wantErr: []string{"refusing remote platform", "fcdev profile add"}},
		{name: "remote in dev file refused", dev: &fnCredentials{PlatformURL: remote, ClientID: "dev", ClientSecret: "s"}, wantErr: []string{"fcdev profile add"}},
		{name: "remote runner with token refused", flags: fnPlatformFlags{platform: "http://127.0.0.1:1", clientID: "a", clientSecret: "b", runner: "https://run.example.com"}, wantErr: []string{"refusing remote runner"}},
		{
			name: "remote runner anonymous allowed", anonymous: true, flags: fnPlatformFlags{runner: "https://run.example.com"},
			check: func(t *testing.T, tg *fnTarget) { require.Equal(t, "https://run.example.com", tg.creds.RunnerURL) },
		},
		{name: "no credential at all", wantErr: []string{"no platform credential", "fcdev profile add"}},
		{
			name: "unprofiled env use warns", env: map[string]string{"FC_PLATFORM_URL": "http://127.0.0.1:2", "FC_CLIENT_ID": "a", "FC_CLIENT_SECRET": "b"},
			wantWarn: []string{"using FC_PLATFORM_URL", "using FC_CLIENT_ID", "using FC_CLIENT_SECRET"},
		},
		{
			name: "flag beats env, no warning for it", env: map[string]string{"FC_CLIENT_ID": "env-id"}, flags: fnPlatformFlags{platform: "http://127.0.0.1:2", clientID: "flag-id", clientSecret: "b"},
			check: func(t *testing.T, tg *fnTarget) { require.Equal(t, "flag-id", tg.creds.ClientID) },
		},
		{
			name: "profile fully determines", profiles: p, flags: fnPlatformFlags{profile: "np1"}, env: map[string]string{"FC_TEST_SECRET": testSecret},
			check: func(t *testing.T, tg *fnTarget) {
				require.Equal(t, remote, tg.creds.PlatformURL)
				require.Equal(t, "https://run.example.com", tg.creds.RunnerURL)
				require.Equal(t, "cid", tg.creds.ClientID)
				require.Equal(t, testSecret, tg.creds.ClientSecret)
				require.Equal(t, "np", tg.envTag())
			},
		},
		{
			name: "FC_PROFILE selects", profiles: p, env: map[string]string{"FC_PROFILE": "np1", "FC_TEST_SECRET": testSecret},
			check: func(t *testing.T, tg *fnTarget) { require.Equal(t, remote, tg.creds.PlatformURL) },
		},
		{
			name: "--profile beats FC_PROFILE", profiles: p + "\n" + profileTOML("other", "https://other.example.com", "np", "env:FC_TEST_SECRET"),
			flags: fnPlatformFlags{profile: "np1"}, env: map[string]string{"FC_PROFILE": "other", "FC_TEST_SECRET": testSecret},
			check: func(t *testing.T, tg *fnTarget) { require.Equal(t, "np1", tg.profile.Name) },
		},
		{
			name: "--runner may override", profiles: p, flags: fnPlatformFlags{profile: "np1", runner: "https://r2.example.com/"}, env: map[string]string{"FC_TEST_SECRET": testSecret},
			check: func(t *testing.T, tg *fnTarget) { require.Equal(t, "https://r2.example.com", tg.creds.RunnerURL) },
		},
		{name: "--runner must still be https", profiles: p, flags: fnPlatformFlags{profile: "np1", runner: "http://r2.example.com"}, env: map[string]string{"FC_TEST_SECRET": testSecret}, wantErr: []string{"--runner", "https is required"}},
		{name: "--platform with profile refused", profiles: p, flags: fnPlatformFlags{profile: "np1", platform: "http://127.0.0.1:1"}, env: map[string]string{"FC_TEST_SECRET": testSecret}, wantErr: []string{"--platform cannot be combined"}},
		{name: "--client-id/--client-secret with profile refused", profiles: p, flags: fnPlatformFlags{profile: "np1", clientID: "x", clientSecret: "y"}, env: map[string]string{"FC_TEST_SECRET": testSecret}, wantErr: []string{"--client-id, --client-secret"}},
		{
			name: "FC_* ignored with warning", profiles: p, flags: fnPlatformFlags{profile: "np1"},
			env:      map[string]string{"FC_TEST_SECRET": testSecret, "FC_PLATFORM_URL": "https://evil.example.com", "FC_CLIENT_ID": "evil", "FC_CLIENT_SECRET": "evil", "FC_RUNNER_URL": "https://evil.example.com"},
			wantWarn: []string{"FC_PLATFORM_URL is set but ignored", "FC_CLIENT_ID is set but ignored", "FC_CLIENT_SECRET is set but ignored", "FC_RUNNER_URL is set but ignored"},
			check: func(t *testing.T, tg *fnTarget) {
				require.Equal(t, remote, tg.creds.PlatformURL)
				require.Equal(t, "cid", tg.creds.ClientID)
				require.Equal(t, testSecret, tg.creds.ClientSecret)
				require.Equal(t, "https://run.example.com", tg.creds.RunnerURL)
			},
		},
		{name: "unknown profile", profiles: p, flags: fnPlatformFlags{profile: "nope"}, wantErr: []string{`no profile "nope"`, "fcdev profile add"}},
		{name: "profile secret unavailable", profiles: p, flags: fnPlatformFlags{profile: "np1"}, wantErr: []string{"FC_TEST_SECRET"}},
		{
			name: "anonymous profile skips the secret", profiles: p, flags: fnPlatformFlags{profile: "np1"}, anonymous: true,
			check: func(t *testing.T, tg *fnTarget) {
				require.Empty(t, tg.creds.ClientSecret)
				require.Equal(t, "np1", tg.profile.Name)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newFnTestEnv(t)
			if c.profiles != "" {
				e.writeProfiles(t, 0o600, c.profiles)
			}
			if c.dev != nil {
				e.writeDev(t, *c.dev)
			}
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			var warn bytes.Buffer
			f := c.flags
			tg, err := f.resolveTarget(context.Background(), &warn, !c.anonymous)
			if len(c.wantErr) > 0 {
				require.Error(t, err)
				for _, w := range c.wantErr {
					require.Contains(t, err.Error(), w)
				}
				require.NotContains(t, err.Error(), testSecret)
				return
			}
			require.NoError(t, err)
			for _, w := range c.wantWarn {
				require.Contains(t, warn.String(), w)
			}
			require.NotContains(t, warn.String(), testSecret)
			if c.check != nil {
				c.check(t, tg)
			}
		})
	}
}

// --- prod confirmation ------------------------------------------------------

func TestConfirmMutation(t *testing.T) {
	mk := func(env string, allow bool) *fnTarget {
		return &fnTarget{creds: fnCredentials{PlatformURL: "https://fc.example.com"}, profile: &Profile{Name: "p", Env: env, AllowNoninteractive: allow}}
	}
	cases := []struct {
		name    string
		t       *fnTarget
		tty     bool
		yes     bool
		input   string
		wantErr string
	}{
		{"no profile never asks", &fnTarget{}, false, false, "", ""},
		{"np never asks", mk("np", false), false, false, "", ""},
		{"local never asks", mk("local", false), true, false, "", ""},
		{"prod tty typed prod", mk("prod", false), true, false, "prod\n", ""},
		{"prod tty wrong text", mk("prod", false), true, false, "yes\n", "did not match"},
		{"prod tty empty", mk("prod", false), true, false, "\n", "did not match"},
		{"prod tty near miss", mk("prod", false), true, false, "prod \n", ""},
		{"prod tty PROD uppercase refused", mk("prod", false), true, false, "PROD\n", "did not match"},
		{"prod tty --yes does not bypass", mk("prod", true), true, true, "", "did not match"},
		{"prod tty --yes with typed prod", mk("prod", true), true, true, "prod\n", ""},
		{"prod no tty, no yes", mk("prod", true), false, false, "prod\n", "--yes"},
		{"prod no tty, yes, not allowed", mk("prod", false), false, true, "", "allow_noninteractive"},
		{"prod no tty, yes + allowed", mk("prod", true), false, true, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			pr := &fnPrompter{in: bufio.NewReader(strings.NewReader(c.input)), out: &out, tty: c.tty}
			err := c.t.confirmMutation(pr, "deploy demo.logger", c.yes)
			if c.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), c.wantErr)
		})
	}
}

// --- commands against a fake platform ---------------------------------------

// profileWith writes a profile that targets the fake platform (loopback
// http is allowed), with the secret supplied through the named env var.
func profileWith(t *testing.T, e *fnTestEnv, p *profilePlatform, env string, extra ...string) {
	t.Helper()
	t.Setenv("FC_TEST_SECRET", testSecret)
	e.writeProfiles(t, 0o600, profileTOML("p", p.srv.URL, env, "env:FC_TEST_SECRET", append([]string{"runner = \"" + p.srv.URL + "\""}, extra...)...))
}

func TestFnStatusViaProfilePrintsBannerFirst(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newProfilePlatform(t, testSecret)
	profileWith(t, e, pl, "np")
	_, errOut, err := runCobra(t, newFnStatusCmd(), "", "demo.logger", "--profile", "p")
	require.NoError(t, err)
	first, _, _ := strings.Cut(errOut, "\n")
	require.Equal(t, fmt.Sprintf("profile p (np) %s", pl.srv.URL), first)
	require.Positive(t, pl.tokenReq)
	require.Positive(t, pl.count("GET /api/functions"))
	require.NotContains(t, errOut, testSecret)
}

func TestFnUnprofiledLoopbackPrintsBanner(t *testing.T) {
	newFnTestEnv(t)
	pl := newProfilePlatform(t, "s")
	_, errOut, err := runCobra(t, newFnStatusCmd(), "", "demo.logger", "--platform", pl.srv.URL, "--client-id", "c", "--client-secret", "s")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(errOut, "profile (none) (local) "+pl.srv.URL+"\n"), errOut)
}

func TestFnRemoteWithoutProfileIsRefused(t *testing.T) {
	newFnTestEnv(t)
	_, _, err := runCobra(t, newFnStatusCmd(), "", "demo.logger", "--platform", "https://fc.example.com", "--client-id", "c", "--client-secret", "s")
	require.ErrorContains(t, err, "fcdev profile add")
}

func TestFnProdMutationsAreGuarded(t *testing.T) {
	type tc struct {
		name  string
		extra []string // profile extra lines
		tty   bool
		input string
		args  []string
		want  string // error substring, "" = success
		sends bool
	}
	base := []string{"demo.logger", "2", "--profile", "p"}
	cases := []tc{
		{"promote prod tty confirmed", nil, true, "prod\n", base, "", true},
		{"promote prod tty declined", nil, true, "no\n", base, "did not match", false},
		{"promote prod non-tty", nil, false, "prod\n", base, "--yes", false},
		{"promote prod non-tty --yes, not allowed", nil, false, "", append([]string{"--yes"}, base...), "allow_noninteractive", false},
		{"promote prod non-tty --yes allowed", []string{"allow_noninteractive = true"}, false, "", append([]string{"--yes"}, base...), "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newFnTestEnv(t)
			e.tty = c.tty
			pl := newProfilePlatform(t, testSecret)
			profileWith(t, e, pl, "prod", c.extra...)
			_, errOut, err := runCobra(t, newFnPromoteCmd(), c.input, c.args...)
			if c.want != "" {
				require.ErrorContains(t, err, c.want)
			} else {
				require.NoError(t, err)
			}
			if c.sends {
				require.Equal(t, 1, pl.count("PUT /api/functions/fn_1/aliases/live"))
			} else {
				require.Zero(t, pl.writes(), "a write reached prod without confirmation")
			}
			require.NotContains(t, errOut, testSecret)
		})
	}
}

func TestFnSetAndDeleteProdGuard(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newProfilePlatform(t, testSecret)
	profileWith(t, e, pl, "prod")
	// set: no terminal, no --yes
	_, _, err := runCobra(t, newFnSetCmd(), "", "demo.logger", "--config", "K=V", "--profile", "p")
	require.Error(t, err)
	require.Zero(t, pl.writes())
	// delete: --yes passes the address check but prod still needs allow_noninteractive
	_, _, err = runCobra(t, newFnDeleteCmd(), "", "demo.logger", "--yes", "--profile", "p")
	require.ErrorContains(t, err, "allow_noninteractive")
	require.Zero(t, pl.writes())
	// delete on a terminal: prod typed first, then the address
	e.tty = true
	_, _, err = runCobra(t, newFnDeleteCmd(), "prod\nwrong\n", "demo.logger", "--profile", "p")
	require.Error(t, err)
	require.Zero(t, pl.writes())
	_, _, err = runCobra(t, newFnDeleteCmd(), "wrong-first\ndemo.logger\n", "demo.logger", "--profile", "p")
	require.ErrorContains(t, err, "did not match")
	require.Zero(t, pl.writes())
	_, _, err = runCobra(t, newFnDeleteCmd(), "prod\ndemo.logger\n", "demo.logger", "--profile", "p")
	require.NoError(t, err)
	require.Equal(t, 1, pl.count("DELETE "))
}

func TestFnNPNeedsNoConfirmation(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newProfilePlatform(t, testSecret)
	profileWith(t, e, pl, "np")
	_, _, err := runCobra(t, newFnPromoteCmd(), "", "demo.logger", "2", "--profile", "p")
	require.NoError(t, err)
	require.Equal(t, 1, pl.count("PUT /api/functions/fn_1/aliases/live"))
}

func TestWrongSecretIsScrubbedFromErrors(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newProfilePlatform(t, "the-real-one")
	profileWith(t, e, pl, "np")
	out, errOut, err := runCobra(t, newFnStatusCmd(), "", "demo.logger", "--profile", "p")
	require.Error(t, err)
	for _, s := range []string{out, errOut, err.Error()} {
		require.NotContains(t, s, testSecret)
	}
}

// --- profile commands -------------------------------------------------------

func TestProfileAddRefusesPromptWithoutTerminal(t *testing.T) {
	e := newFnTestEnv(t)
	_, _, err := runCobra(t, newProfileCmd(), "", "add", "x", "--platform", "https://fc.example.com", "--env", "np", "--client-id", "c")
	require.ErrorContains(t, err, "--secret-ref")
	_, statErr := os.Stat(e.profiles)
	require.True(t, os.IsNotExist(statErr))
	require.Empty(t, e.kc.m)
}

func TestProfileLifecycle(t *testing.T) {
	e := newFnTestEnv(t)
	e.tty = true
	answers := []string{testSecret, testSecret}
	old := fnReadSecret
	t.Cleanup(func() { fnReadSecret = old })
	fnReadSecret = func(string, io.Writer) (string, error) { a := answers[0]; answers = answers[1:]; return a, nil }

	// add with the keychain default
	out, errOut, err := runCobra(t, newProfileCmd(), "", "add", "prod1", "--platform", "https://fc.example.com", "--env", "prod", "--client-id", "cid")
	require.NoError(t, err)
	require.Equal(t, testSecret, e.kc.m["prod1"])
	require.NotContains(t, out+errOut, testSecret)
	raw, err := os.ReadFile(e.profiles)
	require.NoError(t, err)
	require.NotContains(t, string(raw), testSecret, "the profile file must never hold the secret")

	// duplicate refused
	_, _, err = runCobra(t, newProfileCmd(), "", "add", "prod1", "--platform", "https://fc.example.com", "--env", "prod", "--client-id", "cid")
	require.ErrorContains(t, err, "already exists")

	// add with an env ref needs no prompt
	_, _, err = runCobra(t, newProfileCmd(), "", "add", "ci", "--platform", "https://fc.example.com", "--env", "prod", "--client-id", "cid",
		"--secret-ref", "env:FC_PROD_SECRET", "--allow-noninteractive", "--runner", "https://run.example.com")
	require.NoError(t, err)

	// bad inputs
	_, _, err = runCobra(t, newProfileCmd(), "", "add", "bad", "--platform", "http://fc.example.com", "--env", "np", "--client-id", "c", "--secret-ref", "env:X")
	require.ErrorContains(t, err, "https is required")
	_, _, err = runCobra(t, newProfileCmd(), "", "add", "bad", "--platform", "https://fc.example.com", "--env", "np", "--client-id", "c", "--secret-ref", testSecret)
	require.Error(t, err)
	require.NotContains(t, err.Error(), testSecret)

	// list / show never print secrets
	out, _, err = runCobra(t, newProfileCmd(), "", "list")
	require.NoError(t, err)
	require.Contains(t, out, "prod1")
	require.Contains(t, out, "keychain")
	require.Contains(t, out, "ci")
	require.NotContains(t, out, testSecret)
	out, _, err = runCobra(t, newProfileCmd(), "", "show", "ci")
	require.NoError(t, err)
	require.Contains(t, out, "env:FC_PROD_SECRET")
	require.Contains(t, out, "allow_noninteractive  true")

	// set-secret: mismatch is refused and stores nothing new
	answers = []string{"one", "two"}
	_, _, err = runCobra(t, newProfileCmd(), "", "set-secret", "prod1")
	require.ErrorContains(t, err, "differ")
	require.Equal(t, testSecret, e.kc.m["prod1"])
	answers = []string{"rotated", "rotated"}
	_, _, err = runCobra(t, newProfileCmd(), "", "set-secret", "prod1")
	require.NoError(t, err)
	require.Equal(t, "rotated", e.kc.m["prod1"])
	_, _, err = runCobra(t, newProfileCmd(), "", "set-secret", "ci")
	require.ErrorContains(t, err, "outside fcdev")

	// remove: wrong confirmation keeps everything; right one deletes the entry
	_, _, err = runCobra(t, newProfileCmd(), "nope\n", "remove", "prod1")
	require.ErrorContains(t, err, "did not match")
	require.Contains(t, e.kc.m, "prod1")
	_, _, err = runCobra(t, newProfileCmd(), "prod1\n", "remove", "prod1")
	require.NoError(t, err)
	require.NotContains(t, e.kc.m, "prod1")
	e.tty = false
	_, _, err = runCobra(t, newProfileCmd(), "ci\n", "remove", "ci")
	require.ErrorContains(t, err, "--yes")
	_, _, err = runCobra(t, newProfileCmd(), "", "remove", "ci", "--yes")
	require.NoError(t, err)
	set, _, err := loadProfiles()
	require.NoError(t, err)
	require.Empty(t, set)
}

func TestProfileTestAgainstFakePlatform(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newProfilePlatform(t, testSecret)
	profileWith(t, e, pl, "np")
	out, errOut, err := runCobra(t, newProfileCmd(), "", "test", "p")
	require.NoError(t, err)
	for _, want := range []string{"ok", "prn_TEST", "function:manage function:view", "ANCHOR", "platform:function-publisher"} {
		require.Contains(t, out, want)
	}
	require.NotContains(t, out+errOut, testSecret)
	require.NotContains(t, out+errOut, "sig", "the token must not be printed")
	require.True(t, strings.HasPrefix(errOut, "profile p (np) "), errOut)

	// wrong secret: reported, nothing leaks
	pl.secret = "different"
	out, errOut, err = runCobra(t, newProfileCmd(), "", "test", "p")
	require.ErrorContains(t, err, "401")
	require.NotContains(t, out+errOut+err.Error(), testSecret)
}

func TestProfileTestDoesNotFollowRedirects(t *testing.T) {
	e := newFnTestEnv(t)
	leak := false
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leak = true }))
	defer other.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/oauth/token", http.StatusTemporaryRedirect)
	}))
	defer redir.Close()
	t.Setenv("FC_TEST_SECRET", testSecret)
	e.writeProfiles(t, 0o600, profileTOML("p", redir.URL, "np", "env:FC_TEST_SECRET"))
	_, _, err := runCobra(t, newProfileCmd(), "", "test", "p")
	require.Error(t, err)
	require.False(t, leak, "the client secret was replayed to a redirect target")
}

// --- deploy --dry-run -------------------------------------------------------

func TestDeployDryRunWritesNothing(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newProfilePlatform(t, testSecret)
	profileWith(t, e, pl, "prod")
	e.tty = false
	art := filepath.Join(e.dir, "function.js")
	require.NoError(t, os.WriteFile(art, []byte(`export default {}`), 0o600))
	// Not --yes, not allowed: a real deploy to prod would be refused; the
	// dry run must not need confirmation and must write nothing. (If the JS
	// artifact does not validate in this environment the run stops earlier,
	// still with no write.)
	out, errOut, _ := runCobra(t, newFnDeployCmd(), "", "demo.logger", art, "--dry-run", "--profile", "p")
	require.Zero(t, pl.writes())
	require.True(t, strings.HasPrefix(errOut, "profile p (prod) "), errOut)
	_ = out
}

func TestDeployDryRunReport(t *testing.T) {
	e := newFnTestEnv(t)
	art := filepath.Join(e.dir, "function.wasm")
	require.NoError(t, os.WriteFile(art, []byte("0123456789"), 0o600))
	pl := newProfilePlatform(t, "s")
	var out bytes.Buffer
	api := &fnAPI{c: fnCredentials{PlatformURL: pl.srv.URL}}
	require.NoError(t, deployDryRun(&out, api, nil, "demo.new", art, "wasm", &abi.Describe{}, 0))
	require.Contains(t, out.String(), "dry run")
	require.Contains(t, out.String(), "would create function demo.new")
	require.Contains(t, out.String(), "10 bytes")
	require.Zero(t, pl.writes()+len(pl.requests))
}
