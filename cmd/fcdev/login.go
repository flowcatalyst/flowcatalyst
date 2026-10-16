package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

// `fcdev login` / `fcdev logout`: a person signs in to a REMOTE platform in
// their browser (authorization code + PKCE on a loopback listener, RFC 8252)
// and then acts as themselves: their own roles and permissions apply.
// docs/fcdev-profiles.md. CI keeps client_credentials profiles.

const (
	// fcdevLoginClientID is the platform's built-in public client
	// (auth.FcdevCLIClientID there; this module cannot import it).
	fcdevLoginClientID = "fcdev-cli"
	// fcdevLoginScope: identity for the confirmation line, offline_access for
	// the refresh token. No permission scopes: the token carries everything the
	// person's roles grant, and nothing the console would not give them.
	fcdevLoginScope = "openid email profile offline_access"

	// loginTimeout bounds the wait for the browser to come back.
	loginTimeout = 5 * time.Minute
	// accessRefreshSkew: refresh the access token when it is closer than this
	// to expiring.
	accessRefreshSkew = 60 * time.Second
	// loginHTTPTimeout bounds every request to the platform.
	loginHTTPTimeout = 30 * time.Second
	// loginLockWait bounds how long a command waits for a concurrent refresh.
	loginLockWait = 30 * time.Second
)

// ---- session storage -----------------------------------------------------
//
// Three keychain entries per login profile (account names cannot collide with
// a profile name, which never contains ':'):
//
//	login:NAME         SECRET  {platform, refresh_token}
//	login-access:NAME  SECRET  the cached access token (best effort: a JWT
//	                           carrying many permissions may exceed what a
//	                           backend allows; without it every command
//	                           refreshes, which still works)
//	login-meta:NAME    plain   {platform, subject, email, name, login_at,
//	                           refresh_expires_at, access_expires_at}
//
// The metadata entry exists so `profile list` and the banner can show who is
// signed in and until when without ever reading a token.

func loginSecretAccount(name string) string { return "login:" + name }
func loginAccessAccount(name string) string { return "login-access:" + name }
func loginMetaAccount(name string) string   { return "login-meta:" + name }

type loginSecret struct {
	Platform     string `json:"platform"`
	RefreshToken string `json:"refresh_token"`
}

type loginMeta struct {
	Platform         string    `json:"platform"`
	Subject          string    `json:"subject,omitempty"`
	Email            string    `json:"email,omitempty"`
	Name             string    `json:"name,omitempty"`
	LoginAt          time.Time `json:"login_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at,omitzero"` // zero: the platform did not say
	AccessExpiresAt  time.Time `json:"access_expires_at,omitzero"`
}

// who is the display identity of a session.
func (m loginMeta) who() string { return firstNonEmpty(m.Email, m.Name, m.Subject, "(unknown)") }

func readLoginMeta(name string) (*loginMeta, error) {
	raw, err := fnKeychain.Get(loginMetaAccount(name))
	if err != nil {
		return nil, err
	}
	var m loginMeta
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("stored login metadata for profile %s is unreadable: %w", name, err)
	}
	return &m, nil
}

func writeLoginMeta(name string, m *loginMeta) error {
	b, _ := json.Marshal(m)
	return fnKeychain.Set(loginMetaAccount(name), string(b))
}

// deleteLoginSession removes every stored piece of a profile's session.
func deleteLoginSession(name string, keepMeta bool) error {
	accounts := []string{loginSecretAccount(name), loginAccessAccount(name)}
	if !keepMeta {
		accounts = append(accounts, loginMetaAccount(name))
	}
	var errs []error
	for _, a := range accounts {
		if err := fnKeychain.Delete(a); err != nil && !errors.Is(err, ErrKeychainNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ---- errors --------------------------------------------------------------

// sessionError is a login-session problem whose message is meant to be shown
// as is (no wrapping prefix).
type sessionError struct{ msg string }

func (e *sessionError) Error() string { return e.msg }

func errSessionExpired(name string) error {
	return &sessionError{fmt.Sprintf("session expired: run fcdev login --profile %s", name)}
}

func errNotLoggedIn(name string) error {
	return &sessionError{fmt.Sprintf("not logged in: run fcdev login --profile %s", name)}
}

// ---- token endpoint ------------------------------------------------------

type loginTokens struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn *int64 `json:"refresh_expires_in"`
}

type tokenEndpointError struct {
	Status      int
	Code        string
	Description string
}

func (e *tokenEndpointError) Error() string {
	return fmt.Sprintf("token request failed: HTTP %d %s %s", e.Status, e.Code, e.Description)
}

// postToken calls /oauth/token. secrets are scrubbed from any message. It
// never follows redirects (a 307 would replay the code or refresh token).
func postToken(ctx context.Context, hc *http.Client, platform string, form url.Values, secrets ...string) (*loginTokens, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(platform, "/")+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, scrubErr(err, secrets...)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, scrubErr(fmt.Errorf("reach %s: %w", redactURL(platform), err), secrets...)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(raw, &e)
		te := &tokenEndpointError{Status: resp.StatusCode, Code: e.Error, Description: scrubText(e.Description, secrets...)}
		if te.Code == "" {
			te.Description = scrubText(truncate(strings.TrimSpace(string(raw)), 200), secrets...)
		}
		return nil, te
	}
	var t loginTokens
	if err := json.Unmarshal(raw, &t); err != nil || t.AccessToken == "" {
		return nil, errors.New("token response had no access_token")
	}
	return &t, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// jwtClaims decodes a JWT payload WITHOUT verifying it. Used only for display
// of tokens received directly from the platform's token endpoint over TLS
// (OIDC Core 3.1.3.7: that channel stands in for the signature check); never
// for an authorization decision.
func jwtClaims(tok string) map[string]json.RawMessage {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]json.RawMessage
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}

// metaFromTokens records who a token set belongs to and when it lapses.
func metaFromTokens(platform string, t *loginTokens, now time.Time, prev *loginMeta) *loginMeta {
	m := &loginMeta{Platform: platform, LoginAt: now}
	if prev != nil {
		*m = *prev
		m.Platform = platform
	}
	for _, tok := range []string{t.IDToken, t.AccessToken} {
		c := jwtClaims(tok)
		if c == nil {
			continue
		}
		m.Subject = firstNonEmpty(claimString(c["sub"]), m.Subject)
		m.Email = firstNonEmpty(claimString(c["email"]), m.Email)
		m.Name = firstNonEmpty(claimString(c["name"]), m.Name)
	}
	if t.ExpiresIn > 0 {
		m.AccessExpiresAt = now.Add(time.Duration(t.ExpiresIn) * time.Second)
	}
	if t.RefreshExpiresIn != nil {
		m.RefreshExpiresAt = now.Add(time.Duration(*t.RefreshExpiresIn) * time.Second)
	}
	return m
}

// storeLoginTokens persists a token set. The refresh token is stored FIRST and
// its failure is fatal: after a rotation the old one is dead, so losing the
// new one means signing in again.
func storeLoginTokens(name, platform string, t *loginTokens, meta *loginMeta) error {
	sec, _ := json.Marshal(loginSecret{Platform: platform, RefreshToken: t.RefreshToken})
	if err := fnKeychain.Set(loginSecretAccount(name), string(sec)); err != nil {
		return scrubErr(fmt.Errorf("store the session in the keychain: %w", err), t.RefreshToken, t.AccessToken)
	}
	// Best effort: an access token too large for the backend only costs a
	// refresh per command.
	if err := fnKeychain.Set(loginAccessAccount(name), t.AccessToken); err != nil {
		_ = fnKeychain.Delete(loginAccessAccount(name))
		meta.AccessExpiresAt = time.Time{}
	}
	if err := writeLoginMeta(name, meta); err != nil {
		return fmt.Errorf("store session metadata in the keychain: %w", err)
	}
	return nil
}

// ---- the login flow ------------------------------------------------------

// loginDeps are the seams the tests replace.
type loginDeps struct {
	openBrowser func(url string) error
	now         func() time.Time
	timeout     time.Duration
	listen      func() (net.Listener, error)
	http        *http.Client
	out         io.Writer
}

func defaultLoginDeps(out io.Writer) loginDeps {
	return loginDeps{
		openBrowser: openInBrowser,
		now:         time.Now,
		timeout:     loginTimeout,
		listen:      func() (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") },
		http:        noRedirectClient(loginHTTPTimeout),
		out:         out,
	}
}

// openInBrowser starts the system browser on a URL, without a shell.
func openInBrowser(target string) error {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("refusing to open a non-http(s) URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target) // #nosec G204 -- fixed program, URL validated above
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target) // #nosec G204 -- fixed program, URL validated above
	default:
		cmd = exec.Command("xdg-open", target) // #nosec G204 -- fixed program, URL validated above
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func randomURLToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// callbackResult is what the loopback handler hands to the login flow.
type callbackResult struct {
	code string
	err  error
}

const callbackPath = "/callback"

// newCallbackHandler serves the one redirect. A request is the login's answer
// only when its state matches (constant time) and its Host is exactly the
// loopback address we bound (DNS-rebinding guard); anything else is answered
// 400 and ignored. The first valid callback wins; later ones are answered
// with a generic message and dropped. Pages never contain a token or code.
func newCallbackHandler(state, wantHost string, result chan<- callbackResult) http.Handler {
	var once sync.Once
	mux := http.NewServeMux()
	page := func(w http.ResponseWriter, status int, title, msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>%s</title><body style=\"font-family:sans-serif;max-width:32em;margin:4em auto\"><h1>%s</h1><p>%s</p></body>",
			html.EscapeString(title), html.EscapeString(title), html.EscapeString(msg))
	}
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Host != wantHost {
			page(w, http.StatusBadRequest, "Bad request", "This is not a valid fcdev sign-in callback.")
			return
		}
		q := r.URL.Query()
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			page(w, http.StatusBadRequest, "Sign-in not completed", "This sign-in link does not belong to the running fcdev login. Close this tab and use the browser window fcdev opened.")
			return
		}
		first := false
		once.Do(func() {
			first = true
			switch {
			case q.Get("error") != "":
				result <- callbackResult{err: fmt.Errorf("the platform refused the sign-in: %s", truncate(q.Get("error")+": "+q.Get("error_description"), 200))}
			case q.Get("code") == "":
				result <- callbackResult{err: errors.New("the platform's redirect carried no authorization code")}
			default:
				result <- callbackResult{code: q.Get("code")}
			}
		})
		if !first {
			page(w, http.StatusConflict, "Already handled", "This sign-in was already completed. You can close this tab.")
			return
		}
		if q.Get("error") != "" || q.Get("code") == "" {
			page(w, http.StatusBadRequest, "Sign-in failed", "fcdev did not receive a sign-in. Return to the terminal for details.")
			return
		}
		page(w, http.StatusOK, "Signed in", "You are signed in to fcdev. You can close this tab and return to the terminal.")
	})
	return mux
}

// runLogin performs the browser sign-in for a login profile and stores the
// session.
func runLogin(ctx context.Context, d loginDeps, p *Profile) error {
	if !p.IsLogin() {
		return fmt.Errorf("profile %s authenticates with client credentials; `fcdev login` is for login profiles (create one with `fcdev profile add NAME --platform URL --env np|prod --login`)", p.Name)
	}
	platform := strings.TrimRight(p.Platform, "/")

	verifier, err := randomURLToken(32)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state, err := randomURLToken(32)
	if err != nil {
		return err
	}

	ln, err := d.listen()
	if err != nil {
		return fmt.Errorf("listen on the loopback interface: %w", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || !addr.IP.IsLoopback() {
		_ = ln.Close()
		return errors.New("refusing to wait for the sign-in on a non-loopback address")
	}
	hostPort := net.JoinHostPort(addr.IP.String(), fmt.Sprint(addr.Port))
	redirect := "http://" + hostPort + callbackPath

	result := make(chan callbackResult, 1)
	srv := &http.Server{
		Handler:           newCallbackHandler(state, hostPort, result),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if srv.Shutdown(sctx) != nil {
			_ = srv.Close()
		}
	}()

	authURL := platform + "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {fcdevLoginClientID},
		"redirect_uri":          {redirect},
		"scope":                 {fcdevLoginScope},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	fmt.Fprintf(d.out, "Signing in to %s (profile %s) in your browser.\n", redactURL(platform), p.Name)
	if err := d.openBrowser(authURL); err != nil {
		fmt.Fprintf(d.out, "Could not open a browser (%v). Open this URL yourself:\n", err)
	} else {
		fmt.Fprintln(d.out, "If it did not open, use this URL:")
	}
	fmt.Fprintf(d.out, "  %s\nWaiting for the sign-in (up to %s)...\n", authURL, d.timeout.Round(time.Second))

	timer := time.NewTimer(d.timeout)
	defer timer.Stop()
	var res callbackResult
	select {
	case res = <-result:
	case <-timer.C:
		return fmt.Errorf("timed out after %s waiting for the browser sign-in; run fcdev login --profile %s again", d.timeout.Round(time.Second), p.Name)
	case <-ctx.Done():
		return fmt.Errorf("sign-in cancelled: %w", ctx.Err())
	}
	if res.err != nil {
		return res.err
	}

	tokens, err := postToken(ctx, d.http, platform, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {res.code},
		"redirect_uri":  {redirect},
		"client_id":     {fcdevLoginClientID},
		"code_verifier": {verifier},
	}, res.code, verifier)
	if err != nil {
		var te *tokenEndpointError
		if errors.As(err, &te) && te.Code == "invalid_client" {
			return fmt.Errorf("%w — the platform has no %q client; it needs a release that includes fcdev login and a restart so it is seeded", err, fcdevLoginClientID)
		}
		return err
	}
	if tokens.RefreshToken == "" {
		return errors.New("the platform returned no refresh token, so the session could not be kept; is it a release that supports fcdev login?")
	}
	now := d.now()
	meta := metaFromTokens(platform, tokens, now, nil)
	if err := storeLoginTokens(p.Name, platform, tokens, meta); err != nil {
		return err
	}
	fmt.Fprintf(d.out, "Logged in to %s as %s (profile %s).\n", redactURL(platform), meta.who(), p.Name)
	fmt.Fprintln(d.out, describeExpiry(meta, now))
	return nil
}

// describeExpiry renders when the login ends.
func describeExpiry(m *loginMeta, now time.Time) string {
	if m.RefreshExpiresAt.IsZero() {
		return "Session end time: not reported by the platform."
	}
	return fmt.Sprintf("Session valid until %s (%s); then sign in again.",
		m.RefreshExpiresAt.Local().Format("Mon 2006-01-02 15:04 MST"), humanDuration(m.RefreshExpiresAt.Sub(now)))
}

func humanDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return "expired"
	case d >= 48*time.Hour:
		return fmt.Sprintf("in %dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("in %dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	}
}

// runLogout deletes the stored session and revokes its refresh token at the
// platform (POST /oauth/revoke, RFC 7009). Revocation is best effort: the
// local tokens are deleted regardless, and a failure is reported.
func runLogout(ctx context.Context, d loginDeps, p *Profile) error {
	if !p.IsLogin() {
		return fmt.Errorf("profile %s authenticates with client credentials; there is no login session to remove", p.Name)
	}
	platform := strings.TrimRight(p.Platform, "/")
	raw, err := fnKeychain.Get(loginSecretAccount(p.Name))
	had := err == nil
	if err != nil && !errors.Is(err, ErrKeychainNotFound) {
		return fmt.Errorf("read the session from the keychain: %w", err)
	}
	var revokeNote string
	if had {
		var sec loginSecret
		if json.Unmarshal([]byte(raw), &sec) == nil && sec.RefreshToken != "" {
			// Only ever send the token to the platform it came from.
			if sec.Platform != platform {
				revokeNote = fmt.Sprintf("not revoked at the platform: the session was for %s, not this profile's %s", redactURL(sec.Platform), redactURL(platform))
			} else if rerr := revokeRefreshToken(ctx, d.http, platform, sec.RefreshToken); rerr != nil {
				revokeNote = "could not revoke the session at the platform (" + rerr.Error() + "); it will still lapse on its own"
			}
		}
	}
	if err := deleteLoginSession(p.Name, false); err != nil {
		return fmt.Errorf("remove the stored session: %w", err)
	}
	switch {
	case !had:
		fmt.Fprintf(d.out, "profile %s had no stored session.\n", p.Name)
	case revokeNote != "":
		fmt.Fprintf(d.out, "Logged out of profile %s: local tokens deleted. Note: %s.\n", p.Name, revokeNote)
	default:
		fmt.Fprintf(d.out, "Logged out of profile %s: local tokens deleted and the session revoked at the platform.\n", p.Name)
	}
	return nil
}

func revokeRefreshToken(ctx context.Context, hc *http.Client, platform, refresh string) error {
	form := url.Values{"token": {refresh}, "token_type_hint": {"refresh_token"}, "client_id": {fcdevLoginClientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, platform+"/oauth/revoke", strings.NewReader(form.Encode()))
	if err != nil {
		return scrubErr(err, refresh)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return scrubErr(err, refresh)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// ---- using the session ---------------------------------------------------

// tokenSource yields a bearer token for the platform.
type tokenSource interface {
	Token(ctx context.Context) (string, error)
}

// loginTokenSource serves a login profile: the cached access token while it
// is good for more than accessRefreshSkew, otherwise one refresh (rotating the
// refresh token), serialised across fcdev processes by a lock file so two
// commands starting together cannot present the same refresh token twice (the
// platform treats a replay as theft and revokes the whole session).
//
// It never prompts, never opens a browser and never falls back to anything
// else: an unusable session is the "run fcdev login" error.
type loginTokenSource struct {
	profile *Profile
	http    *http.Client
	now     func() time.Time
}

func newLoginTokenSource(p *Profile) *loginTokenSource {
	return &loginTokenSource{profile: p, http: noRedirectClient(loginHTTPTimeout), now: time.Now}
}

func (s *loginTokenSource) cached() (string, bool) {
	name := s.profile.Name
	meta, err := readLoginMeta(name)
	if err != nil || meta.Platform != strings.TrimRight(s.profile.Platform, "/") {
		return "", false
	}
	if meta.AccessExpiresAt.IsZero() || meta.AccessExpiresAt.Sub(s.now()) <= accessRefreshSkew {
		return "", false
	}
	// The session ends at its absolute deadline even if a cached access token
	// would outlive it by a few minutes.
	if !meta.RefreshExpiresAt.IsZero() && !s.now().Before(meta.RefreshExpiresAt) {
		return "", false
	}
	tok, err := fnKeychain.Get(loginAccessAccount(name))
	if err != nil || tok == "" {
		return "", false
	}
	return tok, true
}

func (s *loginTokenSource) Token(ctx context.Context) (string, error) {
	if tok, ok := s.cached(); ok {
		return tok, nil
	}
	release, err := acquireLoginLock(ctx, s.profile.Name, loginLockWait)
	if err != nil {
		return "", err
	}
	defer release()
	// Another fcdev may have refreshed while we waited for the lock.
	if tok, ok := s.cached(); ok {
		return tok, nil
	}
	return s.refresh(ctx)
}

func (s *loginTokenSource) refresh(ctx context.Context) (string, error) {
	name := s.profile.Name
	platform := strings.TrimRight(s.profile.Platform, "/")
	raw, err := fnKeychain.Get(loginSecretAccount(name))
	if errors.Is(err, ErrKeychainNotFound) {
		if meta, merr := readLoginMeta(name); merr == nil && !meta.RefreshExpiresAt.IsZero() {
			return "", errSessionExpired(name)
		}
		return "", errNotLoggedIn(name)
	}
	if err != nil {
		return "", fmt.Errorf("read the session from the keychain: %w", err)
	}
	var sec loginSecret
	if json.Unmarshal([]byte(raw), &sec) != nil || sec.RefreshToken == "" {
		return "", &sessionError{fmt.Sprintf("the stored session for profile %s is unreadable: run fcdev login --profile %s", name, name)}
	}
	if sec.Platform != platform {
		return "", &sessionError{fmt.Sprintf("the stored session for profile %s was made for %s, not %s (the profile's platform changed): run fcdev login --profile %s",
			name, redactURL(sec.Platform), redactURL(platform), name)}
	}
	prev, _ := readLoginMeta(name)
	now := s.now()
	if prev != nil && !prev.RefreshExpiresAt.IsZero() && !now.Before(prev.RefreshExpiresAt) {
		markSessionExpired(name, now)
		return "", errSessionExpired(name)
	}
	tokens, err := postToken(ctx, s.http, platform, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {sec.RefreshToken},
		"client_id":     {fcdevLoginClientID},
	}, sec.RefreshToken)
	if err != nil {
		var te *tokenEndpointError
		if errors.As(err, &te) && (te.Code == "invalid_grant" || te.Code == "invalid_client") {
			// Expired, revoked, replayed or the account is gone/inactive: the
			// session cannot be recovered. Drop the secrets, keep the
			// metadata so `profile list` can say "expired".
			markSessionExpired(name, now)
			return "", errSessionExpired(name)
		}
		return "", fmt.Errorf("could not refresh the session for profile %s: %w", name, err)
	}
	if tokens.RefreshToken == "" {
		return "", errors.New("the platform's refresh response carried no new refresh token")
	}
	meta := metaFromTokens(platform, tokens, now, prev)
	if err := storeLoginTokens(name, platform, tokens, meta); err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

// markSessionExpired drops a dead session's tokens and records, in the
// non-secret metadata, that it ended, so `profile list` says "expired".
func markSessionExpired(name string, now time.Time) {
	_ = deleteLoginSession(name, true)
	if meta, err := readLoginMeta(name); err == nil {
		meta.RefreshExpiresAt, meta.AccessExpiresAt = now, time.Time{}
		_ = writeLoginMeta(name, meta)
	}
}

// acquireLoginLock takes a per-profile lock file next to the profile file
// (owner-only directory). A lock older than a minute is treated as left by a
// crashed process.
func acquireLoginLock(ctx context.Context, name string, wait time.Duration) (func(), error) {
	pp, err := fnProfilesPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(pp), 0o700); err != nil {
		return nil, err
	}
	lock := filepath.Join(filepath.Dir(pp), ".login-"+name+".lock")
	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- path under the profile directory, name validated
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(lock) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("lock the login session: %w", err)
		}
		if st, serr := os.Stat(lock); serr == nil && time.Since(st.ModTime()) > time.Minute {
			_ = os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another fcdev is refreshing the session for profile %s and did not finish within %s", name, wait)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// ---- commands ------------------------------------------------------------

func loadLoginProfile(name string) (*Profile, error) {
	name = firstNonEmpty(name, os.Getenv("FC_PROFILE"))
	if name == "" {
		return nil, errors.New("--profile NAME is required (create a login profile with `fcdev profile add NAME --platform URL --env np|prod --login`)")
	}
	set, path, err := loadProfiles()
	if err != nil {
		return nil, err
	}
	p := set[name]
	if p == nil {
		return nil, fmt.Errorf("no profile %q in %s — create one with `fcdev profile add %s --platform URL --env np|prod --login`", name, path, name)
	}
	return p, nil
}

func newLoginCmd() *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "login --profile NAME",
		Short: "Sign in to a remote platform in your browser (login profiles)",
		Long: `Opens your browser to sign in to the profile's platform (the normal login,
including SSO, passkeys and two-factor), then keeps the session in the OS
keychain. You act as yourself: your own roles and permissions apply.

The session lasts a fixed time from this sign-in (7 days by default, set by
the platform); after that, sign in again. CI uses client-credentials profiles
instead.

See docs/fcdev-profiles.md.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := loadLoginProfile(profile)
			if err != nil {
				return err
			}
			if !fnStdinIsTerminal() {
				return errors.New("fcdev login needs a terminal (it opens a browser for you); non-interactive use should authenticate with a client-credentials profile")
			}
			ctx, cancel := signalContext(cmd.Context())
			defer cancel()
			return runLogin(ctx, defaultLoginDeps(cmd.OutOrStdout()), p)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "login profile to sign in with (env FC_PROFILE)")
	return cmd
}

func newLogoutCmd() *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "logout --profile NAME",
		Short: "Delete a login profile's stored session and revoke it at the platform",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := loadLoginProfile(profile)
			if err != nil {
				return err
			}
			return runLogout(cmd.Context(), defaultLoginDeps(cmd.OutOrStdout()), p)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "login profile to sign out of (env FC_PROFILE)")
	return cmd
}

// signalContext cancels on Ctrl-C so a pending sign-in shuts its listener down.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(parent, os.Interrupt)
}
