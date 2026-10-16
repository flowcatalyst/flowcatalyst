package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// fnReadSecret reads a secret from the terminal without echo. A var so tests
// can supply one without a terminal.
var fnReadSecret = func(prompt string, out io.Writer) (string, error) {
	_, _ = fmt.Fprint(out, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd())) // #nosec G115 -- fd fits int
	fmt.Fprintln(out)
	return string(b), err
}

// promptNewSecret asks twice and requires a match.
func promptNewSecret(out io.Writer, who string) (string, error) {
	a, err := fnReadSecret(fmt.Sprintf("Client secret for %s: ", who), out)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(a) == "" {
		return "", errors.New("empty secret")
	}
	b, err := fnReadSecret("Again: ", out)
	if err != nil {
		return "", err
	}
	if a != b {
		return "", errors.New("the two entries differ; nothing stored")
	}
	return strings.TrimSpace(a), nil
}

// newProfileCmd is `fcdev profile`: manage `fcdev fn` connection profiles
// (docs/fcdev-profiles.md).
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage connection profiles for remote platforms (np / prod)",
		Long: `A profile names a platform, an environment tag (local, np, prod), a client id
and a REFERENCE to the client secret (keychain, env:NAME, file:/path or
command:<argv...>). The profile file never holds a secret value.

See docs/fcdev-profiles.md.`,
	}
	cmd.AddCommand(newProfileAddCmd(), newProfileListCmd(), newProfileShowCmd(), newProfileRemoveCmd(), newProfileSetSecretCmd(), newProfileTestCmd())
	return cmd
}

func newProfileAddCmd() *cobra.Command {
	var p Profile
	var login bool
	cmd := &cobra.Command{
		Use:   "add <name> --platform URL --env np|prod|local (--login | --client-id ID [--secret-ref REF]) [--runner URL]",
		Short: "Add a profile: --login for a person (browser sign-in), else a client id + secret reference",
		Long: `Adds a profile.

--login makes a PERSON profile: no client id, no secret. Run
"fcdev login --profile NAME" to sign in with your browser; you then act as
yourself. Otherwise the profile authenticates as an OAuth client
(client_credentials), for CI and service accounts, and takes:

--secret-ref is one of:
  keychain               the OS secret store (default; you are prompted, no echo)
  env:NAME               read the secret from environment variable NAME
  file:/abs/path         read the secret from a file (mode 0600 or tighter)
  command:<argv...>      run a program (NO shell) and use its trimmed stdout,
                         e.g. command:aws ssm get-parameter --name /fc/prod/secret --with-decryption --query Parameter.Value --output text

A command: reference runs whatever the profile says, as you. Keep the
profile file owner-writable only (fcdev refuses it otherwise).

Without a terminal, --secret-ref env:/file:/command: is required.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p.Name = args[0]
			if login {
				if p.ClientID != "" || p.SecretRef != "" {
					return errors.New("--login takes no --client-id or --secret-ref: a login profile has no client credential")
				}
				p.Auth = authLogin
			} else {
				if p.ClientID == "" {
					return errors.New("--client-id is required (or pass --login for a browser-login profile)")
				}
				if p.SecretRef == "" {
					p.SecretRef = refKeychain
				}
			}
			if err := validateProfile(&p); err != nil {
				return err
			}
			ref, _ := parseSecretRef(p.SecretRef)
			set, path, err := loadProfiles()
			if err != nil {
				return err
			}
			if set[p.Name] != nil {
				return fmt.Errorf("profile %q already exists in %s; `fcdev profile remove %s` first, or `fcdev profile set-secret %s` to change its secret", p.Name, path, p.Name, p.Name)
			}
			errOut := cmd.ErrOrStderr()
			stored := false
			if login {
				set[p.Name] = &p
				if err := saveProfiles(path, set); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "added login profile %s (%s) → %s; sign in with: fcdev login --profile %s\n", p.Name, p.Env, p.Platform, p.Name)
				return nil
			}
			if ref.Kind == refKeychain {
				if !fnStdinIsTerminal() {
					return errors.New("no terminal to prompt for the secret on: pass --secret-ref env:NAME, file:/path or command:<argv...> (keychain needs an interactive `fcdev profile add` or `set-secret`)")
				}
				secret, err := promptNewSecret(errOut, p.Name)
				if err != nil {
					return err
				}
				if err := fnKeychain.Set(p.Name, secret); err != nil {
					return scrubErr(fmt.Errorf("store the secret in the keychain: %w", err), secret)
				}
				stored = true
			}
			set[p.Name] = &p
			if err := saveProfiles(path, set); err != nil {
				if stored {
					_ = fnKeychain.Delete(p.Name)
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added profile %s (%s) → %s; secret: %s\n", p.Name, p.Env, p.Platform, ref)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&p.Platform, "platform", "", "platform URL (https unless loopback)")
	f.StringVar(&p.Runner, "runner", "", "runner URL used by `fcdev fn invoke` (private entry or public URL)")
	f.StringVar(&p.Env, "env", "", "environment tag: local, np or prod")
	f.BoolVar(&login, "login", false, "a person profile: sign in with `fcdev login` (no client id or secret)")
	f.StringVar(&p.ClientID, "client-id", "", "OAuth client id (client-credentials profiles)")
	f.StringVar(&p.SecretRef, "secret-ref", "", "keychain (default), env:NAME, file:/path or command:<argv...>")
	f.BoolVar(&p.AllowNoninteractive, "allow-noninteractive", false, "prod: allow --yes without a terminal (CI profiles)")
	_ = cmd.MarkFlagRequired("platform")
	_ = cmd.MarkFlagRequired("env")
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List profiles (never shows secrets)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			set, path, err := loadProfiles()
			if err != nil {
				return err
			}
			if len(set) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no profiles in %s — add one with `fcdev profile add`\n", path)
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tENV\tPLATFORM\tAUTH\tCREDENTIAL")
			for _, n := range set.names() {
				p := set[n]
				if p.IsLogin() {
					fmt.Fprintf(w, "%s\t%s\t%s\tlogin\t%s\n", p.Name, p.Env, p.Platform, loginStatus(p, time.Now()))
					continue
				}
				ref, _ := parseSecretRef(p.SecretRef)
				fmt.Fprintf(w, "%s\t%s\t%s\tclient\t%s\n", p.Name, p.Env, p.Platform, ref.Kind)
			}
			return w.Flush()
		},
	}
}

func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show one profile (never shows the secret)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, path, err := loadProfiles()
			if err != nil {
				return err
			}
			p := set[args[0]]
			if p == nil {
				return fmt.Errorf("no profile %q in %s", args[0], path)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "name                  %s\nplatform              %s\n", p.Name, p.Platform)
			fmt.Fprintf(out, "runner                %s\nenv                   %s\n", firstNonEmpty(p.Runner, "(none)"), p.Env)
			if p.IsLogin() {
				fmt.Fprintf(out, "auth                  login\nsession               %s\n", loginStatus(p, time.Now()))
			} else {
				fmt.Fprintf(out, "auth                  client_credentials\nclient_id             %s\nsecret_ref            %s\n", p.ClientID, p.SecretRef)
			}
			fmt.Fprintf(out, "allow_noninteractive  %v\nfile                  %s\n", p.AllowNoninteractive, path)
			return nil
		},
	}
}

func newProfileRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a profile (and its keychain entry) after confirmation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, path, err := loadProfiles()
			if err != nil {
				return err
			}
			p := set[args[0]]
			if p == nil {
				return fmt.Errorf("no profile %q in %s", args[0], path)
			}
			if !yes {
				if !fnStdinIsTerminal() {
					return errors.New("refusing to remove without a terminal to confirm on; pass --yes")
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Type the profile name (%s) to remove it: ", p.Name)
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if strings.TrimSpace(line) != p.Name {
					return errors.New("confirmation did not match; nothing removed")
				}
			}
			delete(set, p.Name)
			if err := saveProfiles(path, set); err != nil {
				return err
			}
			if p.IsLogin() {
				// Local removal only: use `fcdev logout` first to also revoke at the platform.
				if err := deleteLoginSession(p.Name, false); err != nil {
					return fmt.Errorf("profile %s removed, but its stored login session could not be deleted: %w", p.Name, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed profile %s\n", p.Name)
				return nil
			}
			ref, _ := parseSecretRef(p.SecretRef)
			if ref.Kind == refKeychain {
				if err := fnKeychain.Delete(p.Name); err != nil && !errors.Is(err, ErrKeychainNotFound) {
					return fmt.Errorf("profile %s removed, but its keychain entry (service %s, account %s) could not be deleted: %w", p.Name, keychainService, p.Name, err)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed profile %s\n", p.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the typed confirmation")
	return cmd
}

func newProfileSetSecretCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-secret <name>",
		Short: "Prompt for a new client secret and store it in the OS keychain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, path, err := loadProfiles()
			if err != nil {
				return err
			}
			p := set[args[0]]
			if p == nil {
				return fmt.Errorf("no profile %q in %s", args[0], path)
			}
			if p.IsLogin() {
				return fmt.Errorf("profile %s is a login profile: it has no secret; run fcdev login --profile %s", p.Name, p.Name)
			}
			if ref, _ := parseSecretRef(p.SecretRef); ref.Kind != refKeychain {
				return fmt.Errorf("profile %s keeps its secret outside fcdev (%s); rotate it there", p.Name, ref)
			}
			if !fnStdinIsTerminal() {
				return errors.New("no terminal to prompt for the secret on")
			}
			secret, err := promptNewSecret(cmd.ErrOrStderr(), p.Name)
			if err != nil {
				return err
			}
			if err := fnKeychain.Set(p.Name, secret); err != nil {
				return scrubErr(fmt.Errorf("store the secret in the keychain: %w", err), secret)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "stored a new secret for profile %s\n", p.Name)
			return nil
		},
	}
}

func newProfileTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Mint a token with the profile's credential and report the principal and granted scope",
		Long: `Resolves the secret, performs a client_credentials grant against the profile's
platform and reports success, the principal id and the granted scope names.
The token and the secret are never printed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pf := fnPlatformFlags{profile: args[0]}
			t, err := fnConnect(cmd, &pf, true)
			if err != nil {
				return err
			}
			var res *tokenReport
			out := cmd.OutOrStdout()
			if t.profile.IsLogin() {
				tok, err := t.tokenSource().Token(cmd.Context())
				if err != nil {
					return t.wrapAuthErr("sign in", err)
				}
				res = reportFromJWT(tok, "")
				fmt.Fprintf(out, "ok: signed in as %s\n", firstNonEmpty(t.user, "(unknown)"))
			} else {
				if res, err = mintTokenReport(cmd.Context(), t.creds); err != nil {
					return err
				}
				fmt.Fprintf(out, "ok: token minted for client %s\n", t.creds.ClientID)
			}
			fmt.Fprintf(out, "principal  %s\n", firstNonEmpty(res.Principal, "(unknown)"))
			if res.Type != "" {
				fmt.Fprintf(out, "type       %s\n", res.Type)
			}
			if res.Tier != "" {
				fmt.Fprintf(out, "tier       %s\n", res.Tier)
			}
			fmt.Fprintf(out, "scope      %s\n", firstNonEmpty(strings.Join(res.Scope, " "), "(none reported)"))
			if len(res.Roles) > 0 {
				fmt.Fprintf(out, "roles      %s\n", strings.Join(res.Roles, " "))
			}
			return nil
		},
	}
}

// tokenReport is what `profile test` shows of a minted token.
type tokenReport struct {
	Principal, Type, Tier string
	Scope, Roles          []string
}

// noRedirectClient never follows redirects: a 307/308 on a token POST would
// replay the client secret to wherever it points.
func noRedirectClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func mintTokenReport(ctx context.Context, c fnCredentials) (*tokenReport, error) {
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.PlatformURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, scrubErr(err, c.ClientSecret)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := noRedirectClient(30 * time.Second).Do(req)
	if err != nil {
		return nil, scrubErr(fmt.Errorf("reach %s: %w", redactURL(c.PlatformURL), err), c.ClientSecret)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		msg := scrubText(strings.TrimSpace(string(raw)), c.ClientSecret)
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return nil, fmt.Errorf("token request failed: HTTP %d %s", resp.StatusCode, msg)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil || tr.AccessToken == "" {
		return nil, errors.New("token response had no access_token")
	}
	return reportFromJWT(tr.AccessToken, tr.Scope), nil
}

// reportFromJWT summarises an access token for `profile test` (display only:
// the token is not verified here).
func reportFromJWT(accessToken, scope string) *tokenReport {
	rep := &tokenReport{Scope: strings.Fields(scope)}
	if claims := jwtClaims(accessToken); claims != nil {
		rep.Principal = claimString(claims["sub"])
		rep.Type = claimString(claims["type"])
		rep.Tier = claimString(claims["tier"])
		rep.Roles = claimList(claims["roles"])
		if len(rep.Scope) == 0 {
			rep.Scope = claimList(claims["scope"])
		}
	}
	return rep
}

// loginStatus describes a login profile's session from its non-secret
// metadata alone; no token is read.
func loginStatus(p *Profile, now time.Time) string {
	meta, err := readLoginMeta(p.Name)
	switch {
	case errors.Is(err, ErrKeychainNotFound):
		return "not logged in"
	case err != nil:
		return "unknown (" + err.Error() + ")"
	case meta.Platform != strings.TrimRight(p.Platform, "/"):
		return "stale: made for another platform URL; run fcdev login"
	case meta.RefreshExpiresAt.IsZero():
		return "logged in as " + meta.who() + " (end time unknown)"
	case !now.Before(meta.RefreshExpiresAt):
		return "expired (was " + meta.who() + ")"
	}
	return fmt.Sprintf("%s, until %s (%s)", meta.who(), meta.RefreshExpiresAt.Local().Format("2006-01-02 15:04"), humanDuration(meta.RefreshExpiresAt.Sub(now)))
}

func claimString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// claimList reads a claim that is a JSON array of strings or a
// space-separated string.
func claimList(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	return strings.Fields(claimString(raw))
}
