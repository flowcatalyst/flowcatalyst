package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	fcauth "github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/auth"
)

// fnTarget is the resolved connection: credentials plus, when a profile
// supplied them, the profile (its env tag drives the prod guard).
type fnTarget struct {
	creds   fnCredentials
	profile *Profile // nil: loopback, unprofiled
	user    string   // login profiles: who is signed in (from non-secret metadata), "" if unknown
}

// tokenSource is how this target obtains bearer tokens: the stored login
// session for a login profile, a client_credentials grant otherwise.
func (t *fnTarget) tokenSource() tokenSource {
	if t.profile != nil && t.profile.IsLogin() {
		return newLoginTokenSource(t.profile)
	}
	return fcauth.NewClientCredentialsProvider(fcauth.ClientCredentialsConfig{
		IssuerURL: t.creds.PlatformURL, ClientID: t.creds.ClientID, ClientSecret: t.creds.ClientSecret,
		HTTPClient: noRedirectClient(30 * time.Second),
	})
}

// wrapAuthErr frames a token failure; session errors already say what to do.
func (t *fnTarget) wrapAuthErr(prefix string, err error) error {
	if _, ok := errors.AsType[*sessionError](err); ok {
		return err
	}
	return fmt.Errorf("%s: %w", prefix, scrubErr(err, t.creds.ClientSecret))
}

func (t *fnTarget) envTag() string {
	if t.profile == nil {
		return envLocal
	}
	return t.profile.Env
}

func (t *fnTarget) isProd() bool { return t.envTag() == envProd }

// banner is the line every platform-talking command prints first.
func (t *fnTarget) banner() string {
	name := "(none)"
	if t.profile != nil {
		name = t.profile.Name
	}
	u := firstNonEmpty(t.creds.PlatformURL, t.creds.RunnerURL, "(no URL)")
	line := fmt.Sprintf("profile %s (%s) %s", name, t.envTag(), redactURL(u))
	if t.profile != nil && t.profile.IsLogin() {
		line += " user " + firstNonEmpty(t.user, "(not signed in)")
	}
	return line
}

// fnConnEnvVars are the legacy connection variables a profile supersedes.
var fnConnEnvVars = []string{"FC_PLATFORM_URL", "FC_RUNNER_URL", "FC_CLIENT_ID", "FC_CLIENT_SECRET"}

const fnProfileHint = "create one with `fcdev profile add <name> --platform URL --env np|prod --client-id ID` and pass --profile <name> (or set FC_PROFILE)"

// resolveTarget settles what `fcdev fn` talks to and with which credential.
//
//   - A profile (--profile, else FC_PROFILE) fully determines platform,
//     runner, client id and secret. --platform/--client-id/--client-secret
//     are refused alongside it; only --runner may override. FC_* connection
//     variables are ignored, with a warning.
//   - Without a profile the old sources apply (flags, FC_* env, the local dev
//     credential file) but ONLY for loopback targets: a remote URL is
//     refused and the user pointed at `fcdev profile add`.
//
// needSecret is false for calls that send no credential (invoke
// --anonymous): then no secret is fetched and the loopback rule is moot.
func (f *fnPlatformFlags) resolveTarget(ctx context.Context, stderr io.Writer, needSecret bool) (*fnTarget, error) {
	if name := firstNonEmpty(f.profile, os.Getenv("FC_PROFILE")); name != "" {
		return f.resolveProfile(ctx, stderr, name, needSecret)
	}
	return f.resolveUnprofiled(stderr, needSecret)
}

func (f *fnPlatformFlags) resolveProfile(ctx context.Context, stderr io.Writer, name string, needSecret bool) (*fnTarget, error) {
	var clash []string
	for flag, v := range map[string]string{"--platform": f.platform, "--client-id": f.clientID, "--client-secret": f.clientSecret} {
		if v != "" {
			clash = append(clash, flag)
		}
	}
	if len(clash) > 0 {
		slices.Sort(clash)
		return nil, fmt.Errorf("profile %q fully determines the platform and credential; %s cannot be combined with it (only --runner may override)", name, strings.Join(clash, ", "))
	}
	for _, v := range fnConnEnvVars {
		if os.Getenv(v) != "" {
			fmt.Fprintf(stderr, "fcdev: warning: %s is set but ignored because profile %q is in use\n", v, name)
		}
	}
	set, path, err := loadProfiles()
	if err != nil {
		return nil, err
	}
	p := set[name]
	if p == nil {
		return nil, fmt.Errorf("no profile %q in %s — %s", name, path, fnProfileHint)
	}
	runner := p.Runner
	if f.runner != "" {
		if err := validateEndpointURL(f.runner); err != nil {
			return nil, fmt.Errorf("--runner: %w", err)
		}
		runner = f.runner
	}
	c := fnCredentials{PlatformURL: strings.TrimRight(p.Platform, "/"), RunnerURL: strings.TrimRight(runner, "/"), ClientID: p.ClientID}
	if p.IsLogin() {
		// The session is fetched lazily by the token source; nothing secret is
		// read here. Only the non-secret metadata, for the banner.
		t := &fnTarget{creds: c, profile: p}
		if meta, err := readLoginMeta(p.Name); err == nil && meta.Platform == c.PlatformURL {
			t.user = meta.who()
		}
		return t, nil
	}
	if needSecret {
		if c.ClientSecret, err = resolveSecret(ctx, p); err != nil {
			return nil, err
		}
	}
	return &fnTarget{creds: c, profile: p}, nil
}

func (f *fnPlatformFlags) resolveUnprofiled(stderr io.Writer, needSecret bool) (*fnTarget, error) {
	used := map[string]bool{} // env vars that supplied a value
	pick := func(flag, env string) string {
		if flag != "" {
			return flag
		}
		if v := os.Getenv(env); v != "" {
			used[env] = true
			return v
		}
		return ""
	}
	c := fnCredentials{
		PlatformURL:  pick(f.platform, "FC_PLATFORM_URL"),
		RunnerURL:    pick(f.runner, "FC_RUNNER_URL"),
		ClientID:     pick(f.clientID, "FC_CLIENT_ID"),
		ClientSecret: pick(f.clientSecret, "FC_CLIENT_SECRET"),
	}
	if c.ClientID == "" || c.PlatformURL == "" || c.RunnerURL == "" {
		if path, err := fnCLICredentialsPath(); err == nil {
			if dev, ok := readDevCredentials(path); ok {
				c.PlatformURL = firstNonEmpty(c.PlatformURL, dev.PlatformURL)
				c.RunnerURL = firstNonEmpty(c.RunnerURL, dev.RunnerURL)
				if c.ClientID == "" {
					c.ClientID, c.ClientSecret = dev.ClientID, dev.ClientSecret
				}
			}
		}
	}
	for _, v := range fnConnEnvVars {
		if used[v] {
			fmt.Fprintf(stderr, "fcdev: warning: using %s from the environment (no profile selected)\n", v)
		}
	}
	if needSecret {
		if c.PlatformURL == "" || c.ClientID == "" || c.ClientSecret == "" {
			return nil, errors.New("no platform credential: start fcdev (it writes one), or use a profile (" + fnProfileHint + "); loopback targets also accept --platform, --client-id and --client-secret")
		}
		if !isLoopbackURL(c.PlatformURL) {
			return nil, fmt.Errorf("refusing remote platform %s without a profile: flags and FC_* variables are accepted for loopback targets only; %s", redactURL(c.PlatformURL), fnProfileHint)
		}
		if c.RunnerURL != "" && !isLoopbackURL(c.RunnerURL) {
			return nil, fmt.Errorf("refusing remote runner %s without a profile (the platform token would be sent to it); %s", redactURL(c.RunnerURL), fnProfileHint)
		}
	}
	c.PlatformURL = strings.TrimRight(c.PlatformURL, "/")
	c.RunnerURL = strings.TrimRight(c.RunnerURL, "/")
	return &fnTarget{creds: c}, nil
}

// fnConnect resolves the target for a command and prints the banner. It is
// the single entry every platform-talking command goes through.
func fnConnect(cmd *cobra.Command, f *fnPlatformFlags, needSecret bool) (*fnTarget, error) {
	t, err := f.resolveTarget(cmd.Context(), cmd.ErrOrStderr(), needSecret)
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), t.banner())
	return t, nil
}

// fnPrompter reads typed answers. One reader per command, so two prompts in
// a row do not lose buffered input.
type fnPrompter struct {
	in  *bufio.Reader
	out io.Writer
	tty bool
}

func newFnPrompter(cmd *cobra.Command) *fnPrompter {
	return &fnPrompter{in: bufio.NewReader(cmd.InOrStdin()), out: cmd.ErrOrStderr(), tty: fnStdinIsTerminal()}
}

func (p *fnPrompter) ask(prompt string) string {
	_, _ = fmt.Fprint(p.out, prompt)
	line, _ := p.in.ReadString('\n')
	return strings.TrimSpace(line)
}

// confirmMutation gates a state-changing command against a prod profile.
// Other environments (and unprofiled loopback use) need nothing.
//
// On a terminal the environment name must be typed, --yes or not. Without a
// terminal, --yes AND the profile's allow_noninteractive are both required.
func (t *fnTarget) confirmMutation(p *fnPrompter, action string, yes bool) error {
	if !t.isProd() {
		return nil
	}
	if !p.tty {
		switch {
		case !yes:
			return fmt.Errorf("refusing to %s on prod (%s) without a terminal to confirm on; non-interactive prod changes need --yes and allow_noninteractive = true in the profile", action, t.profile.Name)
		case !t.profile.AllowNoninteractive:
			return fmt.Errorf("refusing to %s on prod: profile %s does not set allow_noninteractive = true, so --yes alone is not enough without a terminal", action, t.profile.Name)
		}
		return nil
	}
	fmt.Fprintf(p.out, "This will %s on PROD: profile %s, %s\n", action, t.profile.Name, redactURL(t.creds.PlatformURL))
	if got := p.ask(fmt.Sprintf("Type %q to continue: ", envProd)); got != envProd {
		return fmt.Errorf("confirmation did not match; nothing was changed")
	}
	return nil
}
