package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- a fake platform with the pieces fcdev login talks to ----------------

type fakeAuthReq struct{ challenge, redirect, state string }

type refreshFamily struct {
	expires time.Time // ABSOLUTE
	revoked bool
}

type loginPlatform struct {
	srv *httptest.Server

	mu          sync.Mutex
	email       string
	accessTTL   time.Duration
	refreshTTL  time.Duration
	codes       map[string]fakeAuthReq
	live        map[string]*refreshFamily // current refresh token -> family
	rotated     map[string]*refreshFamily // rotated-out token -> family (reuse detection)
	nextID      int
	authorizeN  int
	exchangeN   int
	refreshN    int
	revoked     []string
	revokeForm  url.Values
	apiAuth     []string // Authorization headers seen on /api
	authorizeQ  url.Values
	errorRedir  string // non-empty: /oauth/authorize redirects with this error
	exchangeErr *struct {
		status int
		body   string
	}
	refreshFail int // non-zero: refresh grant answers with this status and no OAuth body
}

func newLoginPlatform(t *testing.T) *loginPlatform {
	p := &loginPlatform{
		email: "andrew@example.com", accessTTL: time.Hour, refreshTTL: 7 * 24 * time.Hour,
		codes: map[string]fakeAuthReq{}, live: map[string]*refreshFamily{}, rotated: map[string]*refreshFamily{},
	}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *loginPlatform) tokenJSON(rt string, fam *refreshFamily, now time.Time) string {
	access := jwt(fmt.Sprintf(`{"sub":"prn_ME","email":%q,"tier":"ANCHOR","type":"USER","token_use":"api","roles":["platform:function-publisher"],"jti":"%d"}`, p.email, p.nextID))
	id := jwt(fmt.Sprintf(`{"sub":"prn_ME","email":%q,"name":"Andrew"}`, p.email))
	rem := int64(fam.expires.Sub(now).Seconds())
	return fmt.Sprintf(`{"access_token":%q,"token_type":"Bearer","expires_in":%d,"refresh_token":%q,"refresh_expires_in":%d,"id_token":%q}`,
		access, int64(p.accessTTL.Seconds()), rt, rem, id)
}

func (p *loginPlatform) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	oauthErr := func(status int, code, desc string) {
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `{"error":%q,"error_description":%q}`, code, desc)
	}
	switch {
	case r.URL.Path == "/oauth/authorize":
		p.authorizeN++
		q := r.URL.Query()
		p.authorizeQ = q
		if q.Get("client_id") != "fcdev-cli" || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" ||
			q.Get("code_challenge") == "" || q.Get("state") == "" || !strings.Contains(q.Get("scope"), "offline_access") {
			oauthErr(400, "invalid_request", "bad authorize request")
			return
		}
		ru, err := url.Parse(q.Get("redirect_uri"))
		if err != nil || ru.Scheme != "http" || ru.Hostname() != "127.0.0.1" || ru.Path != "/callback" || ru.Port() == "" {
			oauthErr(400, "invalid_request", "Invalid redirect_uri")
			return
		}
		if p.errorRedir != "" {
			http.Redirect(w, r, q.Get("redirect_uri")+"?error="+p.errorRedir+"&error_description=nope&state="+url.QueryEscape(q.Get("state")), http.StatusTemporaryRedirect)
			return
		}
		p.nextID++
		code := fmt.Sprintf("code-%d-SECRETCODE", p.nextID)
		p.codes[code] = fakeAuthReq{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), state: q.Get("state")}
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusTemporaryRedirect)
	case r.URL.Path == "/oauth/token":
		_ = r.ParseForm()
		f := r.PostForm
		if f.Get("client_id") != "fcdev-cli" || f.Get("client_secret") != "" {
			oauthErr(401, "invalid_client", "Unknown client")
			return
		}
		now := time.Now()
		switch f.Get("grant_type") {
		case "authorization_code":
			p.exchangeN++
			if p.exchangeErr != nil {
				w.WriteHeader(p.exchangeErr.status)
				_, _ = io.WriteString(w, strings.ReplaceAll(p.exchangeErr.body, "{{code}}", f.Get("code")))
				return
			}
			req, ok := p.codes[f.Get("code")]
			delete(p.codes, f.Get("code"))
			if !ok {
				oauthErr(400, "invalid_grant", "Invalid or expired authorization code")
				return
			}
			sum := sha256.Sum256([]byte(f.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != req.challenge {
				oauthErr(400, "invalid_grant", "Invalid code_verifier")
				return
			}
			if f.Get("redirect_uri") != req.redirect {
				oauthErr(400, "invalid_grant", "Redirect URI mismatch")
				return
			}
			p.nextID++
			rt := fmt.Sprintf("rt-%d", p.nextID)
			fam := &refreshFamily{expires: now.Add(p.refreshTTL)}
			p.live[rt] = fam
			_, _ = io.WriteString(w, p.tokenJSON(rt, fam, now))
		case "refresh_token":
			p.refreshN++
			if p.refreshFail != 0 {
				w.WriteHeader(p.refreshFail)
				return
			}
			old := f.Get("refresh_token")
			if fam, replay := p.rotated[old]; replay {
				fam.revoked = true
				oauthErr(400, "invalid_grant", "Invalid or expired refresh token")
				return
			}
			fam, ok := p.live[old]
			if !ok || fam.revoked || !now.Before(fam.expires) {
				oauthErr(400, "invalid_grant", "Invalid or expired refresh token")
				return
			}
			delete(p.live, old)
			p.rotated[old] = fam
			p.nextID++
			rt := fmt.Sprintf("rt-%d", p.nextID)
			p.live[rt] = fam // same family: the deadline is NOT extended
			_, _ = io.WriteString(w, p.tokenJSON(rt, fam, now))
		default:
			oauthErr(400, "unsupported_grant_type", "no")
		}
	case r.URL.Path == "/oauth/revoke":
		_ = r.ParseForm()
		p.revokeForm = r.PostForm
		tok := r.PostForm.Get("token")
		p.revoked = append(p.revoked, tok)
		if fam, ok := p.live[tok]; ok {
			fam.revoked = true
		}
		w.WriteHeader(200)
	case strings.HasPrefix(r.URL.Path, "/api/"):
		p.apiAuth = append(p.apiAuth, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/functions":
			_, _ = io.WriteString(w, `{"data":[{"id":"fn_1","address":"demo.logger","applicationCode":"demo"}]}`)
		case "/api/functions/fn_1/versions":
			_, _ = io.WriteString(w, `[{"number":1,"status":"READY","runtime":"wasm","digest":"aaaaaaaaaaaaaaaa"}]`)
		case "/api/functions/fn_1/aliases":
			_, _ = io.WriteString(w, `[{"name":"live","version":1}]`)
		default:
			w.WriteHeader(404)
		}
	default:
		w.WriteHeader(404)
	}
}

func (p *loginPlatform) counts() (authorize, exchange, refresh int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.authorizeN, p.exchangeN, p.refreshN
}

// ---- the fake browser ----------------------------------------------------

// fakeBrowser stands in for the user's browser: it follows the authorize URL
// (the fake platform "signs the user in" and redirects) to fcdev's loopback
// callback, and remembers the page it got back.
type fakeBrowser struct {
	mu       sync.Mutex
	urls     []string
	pages    []string
	statuses []int
	pre      func(authURL string) // runs first, e.g. to send a bogus callback
	failOpen error                // opener reports failure (the "browser" is still driven, as if the user pasted the URL)
	idle     bool                 // never completes the sign-in
	done     chan struct{}
}

func newFakeBrowser() *fakeBrowser { return &fakeBrowser{done: make(chan struct{})} }

func (b *fakeBrowser) open(target string) error {
	b.mu.Lock()
	b.urls = append(b.urls, target)
	b.mu.Unlock()
	if b.idle {
		return b.failOpen
	}
	go func() {
		defer close(b.done)
		if b.pre != nil {
			b.pre(target)
		}
		resp, err := http.Get(target) // follows the redirect to fcdev's listener
		if err != nil {
			b.record(0, err.Error())
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		b.record(resp.StatusCode, string(body))
	}()
	return b.failOpen
}

func (b *fakeBrowser) record(status int, page string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.statuses = append(b.statuses, status)
	b.pages = append(b.pages, page)
}

func (b *fakeBrowser) wait(t *testing.T) {
	t.Helper()
	select {
	case <-b.done:
	case <-time.After(5 * time.Second):
		t.Fatal("fake browser did not finish")
	}
}

// ---- helpers -------------------------------------------------------------

func loginProfileTOML(name, platform, env string, extra ...string) string {
	return fmt.Sprintf("[profiles.%s]\nplatform = %q\nenv = %q\nauth = \"login\"\n", name, platform, env) + strings.Join(extra, "\n") + "\n"
}

// recordingKeychain notes which accounts are read.
type recordingKeychain struct {
	*memKeychain
	mu    sync.Mutex
	reads []string
}

func (k *recordingKeychain) Get(a string) (string, error) {
	k.mu.Lock()
	k.reads = append(k.reads, a)
	k.mu.Unlock()
	return k.memKeychain.Get(a)
}

func testDeps(out io.Writer, b *fakeBrowser) loginDeps {
	return loginDeps{
		openBrowser: b.open, now: time.Now, timeout: 5 * time.Second,
		listen: func() (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") },
		http:   noRedirectClient(5 * time.Second), out: out,
	}
}

func loginProfile(t *testing.T, e *fnTestEnv, pl *loginPlatform, env string) *Profile {
	t.Helper()
	e.writeProfiles(t, 0o600, loginProfileTOML("me", pl.srv.URL, env, "runner = \""+pl.srv.URL+"\""))
	set, _, err := loadProfiles()
	require.NoError(t, err)
	return set["me"]
}

func doLogin(t *testing.T, e *fnTestEnv, pl *loginPlatform) (*Profile, string) {
	t.Helper()
	p := loginProfile(t, e, pl, "np")
	var out bytes.Buffer
	require.NoError(t, runLogin(context.Background(), testDeps(&out, newFakeBrowser()), p))
	return p, out.String()
}

// ---- the login flow ------------------------------------------------------

func TestLoginFullFlow(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	p := loginProfile(t, e, pl, "np")

	b := newFakeBrowser()
	var out bytes.Buffer
	var listenAddr net.Addr
	d := testDeps(&out, b)
	d.listen = func() (net.Listener, error) {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if ln != nil {
			listenAddr = ln.Addr()
		}
		return ln, err
	}
	require.NoError(t, runLogin(context.Background(), d, p))
	b.wait(t)

	// who and when, no secrets
	require.Contains(t, out.String(), "Logged in to "+pl.srv.URL+" as andrew@example.com (profile me)")
	require.Contains(t, out.String(), "Session valid until")
	require.Contains(t, out.String(), "in 7d 0h")
	for _, secret := range []string{"rt-", "SECRETCODE", "eyJ"} {
		require.NotContains(t, out.String(), secret)
	}

	// the request the platform saw: PKCE S256, state, loopback redirect, the offline scope
	q := pl.authorizeQ
	require.Equal(t, "fcdev-cli", q.Get("client_id"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.Len(t, q.Get("code_challenge"), 43)
	require.NotEmpty(t, q.Get("state"))
	require.Equal(t, "openid email profile offline_access", q.Get("scope"))
	ru, _ := url.Parse(q.Get("redirect_uri"))
	require.Equal(t, "127.0.0.1", ru.Hostname())
	require.Equal(t, "/callback", ru.Path)
	require.Equal(t, listenAddr.(*net.TCPAddr).IP.String(), "127.0.0.1")
	// the callback page carries no code and no token
	require.Equal(t, []int{200}, b.statuses)
	require.Contains(t, b.pages[0], "Signed in")
	for _, secret := range []string{"code", "SECRETCODE", "rt-", "eyJ"} {
		require.NotContains(t, strings.ToLower(strings.ReplaceAll(b.pages[0], "close this tab", "")), strings.ToLower(secret)+"=", secret)
	}
	require.NotContains(t, b.pages[0], "SECRETCODE")

	// stored in the keychain, per profile
	_, err := e.kc.Get("login:me")
	require.NoError(t, err)
	_, err = e.kc.Get("login-access:me")
	require.NoError(t, err)
	meta, err := readLoginMeta("me")
	require.NoError(t, err)
	require.Equal(t, "andrew@example.com", meta.Email)
	require.Equal(t, "prn_ME", meta.Subject)
	require.Equal(t, pl.srv.URL, meta.Platform)
	require.WithinDuration(t, time.Now().Add(7*24*time.Hour), meta.RefreshExpiresAt, 10*time.Second)

	// the listener is gone
	_, err = net.DialTimeout("tcp", listenAddr.String(), 500*time.Millisecond)
	require.Error(t, err, "the loopback listener must be closed after the callback")
}

func TestLoginWrongStateIsRejectedAndIgnored(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	p := loginProfile(t, e, pl, "np")

	b := newFakeBrowser()
	var bogus struct {
		status int
		body   string
	}
	b.pre = func(authURL string) {
		u, _ := url.Parse(authURL)
		cb := u.Query().Get("redirect_uri")
		resp, err := http.Get(cb + "?code=stolen-code&state=WRONG-STATE")
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			bogus.status, bogus.body = resp.StatusCode, string(body)
		}
	}
	var out bytes.Buffer
	require.NoError(t, runLogin(context.Background(), testDeps(&out, b), p))
	b.wait(t)

	require.Equal(t, 400, bogus.status)
	require.NotContains(t, bogus.body, "stolen-code")
	_, exchanges, _ := pl.counts()
	require.Equal(t, 1, exchanges, "the bogus callback's code must never be exchanged")
	require.Contains(t, out.String(), "Logged in")
}

func TestLoginCallbackHostAndMethodAreChecked(t *testing.T) {
	result := make(chan callbackResult, 1)
	h := newCallbackHandler("good-state", "127.0.0.1:5555", result)

	req := httptest.NewRequest(http.MethodGet, "/callback?code=c&state=good-state", nil)
	req.Host = "evil.example.com:5555" // DNS rebinding
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, 400, rr.Code)

	req = httptest.NewRequest(http.MethodPost, "/callback?code=c&state=good-state", nil)
	req.Host = "127.0.0.1:5555"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, 400, rr.Code)
	require.Empty(t, result)

	// exactly one valid callback counts
	for i, want := range []int{200, 409} {
		req = httptest.NewRequest(http.MethodGet, "/callback?code=c&state=good-state", nil)
		req.Host = "127.0.0.1:5555"
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, want, rr.Code, "callback %d", i)
	}
	require.Len(t, result, 1)
	require.Equal(t, "c", (<-result).code)

	// other paths are not callbacks
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "127.0.0.1:5555"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, 404, rr.Code)
}

func TestLoginPlatformErrorRedirectFailsWithoutStoring(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	pl.errorRedir = "access_denied"
	p := loginProfile(t, e, pl, "np")
	b := newFakeBrowser()
	err := runLogin(context.Background(), testDeps(io.Discard, b), p)
	b.wait(t)
	require.ErrorContains(t, err, "refused the sign-in")
	require.ErrorContains(t, err, "access_denied")
	_, kerr := e.kc.Get("login:me")
	require.ErrorIs(t, kerr, ErrKeychainNotFound)
	require.Contains(t, b.pages[0], "Sign-in failed")
}

func TestLoginTimeoutClosesTheListener(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	p := loginProfile(t, e, pl, "np")
	b := newFakeBrowser()
	b.idle = true
	var addr net.Addr
	d := testDeps(io.Discard, b)
	d.timeout = 150 * time.Millisecond
	d.listen = func() (net.Listener, error) {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		addr = ln.Addr()
		return ln, err
	}
	start := time.Now()
	err := runLogin(context.Background(), d, p)
	require.ErrorContains(t, err, "timed out")
	require.Less(t, time.Since(start), 3*time.Second)
	_, derr := net.DialTimeout("tcp", addr.String(), 500*time.Millisecond)
	require.Error(t, derr, "listener must be closed after a timeout")
	_, kerr := e.kc.Get("login:me")
	require.ErrorIs(t, kerr, ErrKeychainNotFound)
}

func TestLoginCancelledByContext(t *testing.T) {
	e := newFnTestEnv(t)
	p := loginProfile(t, e, newLoginPlatform(t), "np")
	b := newFakeBrowser()
	b.idle = true
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	err := runLogin(ctx, testDeps(io.Discard, b), p)
	require.ErrorContains(t, err, "cancelled")
}

func TestLoginBrowserOpenFailurePrintsTheURL(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	p := loginProfile(t, e, pl, "np")
	b := newFakeBrowser()
	b.failOpen = fmt.Errorf("no display")
	var out bytes.Buffer
	require.NoError(t, runLogin(context.Background(), testDeps(&out, b), p)) // the "user" pasted the URL
	b.wait(t)
	require.Contains(t, out.String(), "Could not open a browser (no display)")
	require.Contains(t, out.String(), pl.srv.URL+"/oauth/authorize?")
}

func TestLoginExchangeFailureIsScrubbedAndStoresNothing(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	pl.exchangeErr = &struct {
		status int
		body   string
	}{400, `{"error":"invalid_grant","error_description":"bad code {{code}} given"}`}
	p := loginProfile(t, e, pl, "np")
	b := newFakeBrowser()
	err := runLogin(context.Background(), testDeps(io.Discard, b), p)
	b.wait(t)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SECRETCODE")
	_, kerr := e.kc.Get("login:me")
	require.ErrorIs(t, kerr, ErrKeychainNotFound)

	// a platform that has not been re-seeded: the hint says what to do
	pl.exchangeErr = &struct {
		status int
		body   string
	}{401, `{"error":"invalid_client","error_description":"Unknown client"}`}
	b = newFakeBrowser()
	err = runLogin(context.Background(), testDeps(io.Discard, b), p)
	b.wait(t)
	require.ErrorContains(t, err, `no "fcdev-cli" client`)
}

func TestLoginRefusesNonLoopbackListener(t *testing.T) {
	e := newFnTestEnv(t)
	p := loginProfile(t, e, newLoginPlatform(t), "np")
	d := testDeps(io.Discard, newFakeBrowser())
	d.listen = func() (net.Listener, error) { return net.Listen("tcp4", "0.0.0.0:0") }
	require.ErrorContains(t, runLogin(context.Background(), d, p), "non-loopback")
}

func TestLoginCommandNeedsATerminalAndALoginProfile(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	e.writeProfiles(t, 0o600, loginProfileTOML("me", pl.srv.URL, "np")+"\n"+
		profileTOML("ci", pl.srv.URL, "np", "env:FC_TEST_SECRET"))

	e.tty = false
	_, _, err := runCobra(t, newLoginCmd(), "", "--profile", "me")
	require.ErrorContains(t, err, "needs a terminal")
	a, _, _ := pl.counts()
	require.Zero(t, a)

	_, _, err = runCobra(t, newLoginCmd(), "", "--profile", "nope")
	require.ErrorContains(t, err, "no profile")
	_, _, err = runCobra(t, newLoginCmd(), "")
	require.ErrorContains(t, err, "--profile NAME is required")

	var out bytes.Buffer
	ci, _, _ := loadProfiles()
	require.ErrorContains(t, runLogin(context.Background(), testDeps(&out, newFakeBrowser()), ci["ci"]), "client credentials")
}

// ---- using the session ---------------------------------------------------

func TestLoginTokenIsCachedThenRefreshedBeforeExpiry(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	p, _ := doLogin(t, e, pl)
	meta0, _ := readLoginMeta("me")

	src := newLoginTokenSource(p)
	first, err := src.Token(context.Background())
	require.NoError(t, err)
	second, err := src.Token(context.Background())
	require.NoError(t, err)
	require.Equal(t, first, second)
	_, _, refreshes := pl.counts()
	require.Zero(t, refreshes, "a fresh access token is served from the keychain")

	// 30s before expiry: inside the 60s skew, so one refresh
	src.now = func() time.Time { return meta0.AccessExpiresAt.Add(-30 * time.Second) }
	third, err := src.Token(context.Background())
	require.NoError(t, err)
	require.NotEqual(t, first, third)
	_, _, refreshes = pl.counts()
	require.Equal(t, 1, refreshes)

	// the refresh token rotated, and the absolute deadline did not move
	raw, _ := e.kc.Get("login:me")
	var sec loginSecret
	require.NoError(t, json.Unmarshal([]byte(raw), &sec))
	require.Equal(t, "rt-3", sec.RefreshToken)
	// the platform's family deadline is what it was: the refresh did not extend it
	pl.mu.Lock()
	require.Len(t, pl.live, 1)
	for _, fam := range pl.live {
		require.WithinDuration(t, meta0.RefreshExpiresAt, fam.expires, 3*time.Second)
	}
	pl.mu.Unlock()

	// 120s before expiry: still cached, no network
	src.now = func() time.Time { return meta0.AccessExpiresAt.Add(-120 * time.Second) }
	_, err = src.Token(context.Background())
	require.NoError(t, err)
	_, _, refreshes = pl.counts()
	require.Equal(t, 1, refreshes)
}

func TestLoginSessionExpiredMessages(t *testing.T) {
	const wantExpired = "session expired: run fcdev login --profile me"

	t.Run("platform rejects the refresh token", func(t *testing.T) {
		e := newFnTestEnv(t)
		pl := newLoginPlatform(t)
		p, _ := doLogin(t, e, pl)
		pl.mu.Lock()
		for _, fam := range pl.live {
			fam.revoked = true
		}
		pl.mu.Unlock()
		src := newLoginTokenSource(p)
		src.now = func() time.Time { return time.Now().Add(2 * time.Hour) } // access token lapsed
		_, err := src.Token(context.Background())
		require.EqualError(t, err, wantExpired)
		// dead tokens are gone, the metadata says why
		_, kerr := e.kc.Get("login:me")
		require.ErrorIs(t, kerr, ErrKeychainNotFound)
		_, kerr = e.kc.Get("login-access:me")
		require.ErrorIs(t, kerr, ErrKeychainNotFound)
		require.Contains(t, loginStatus(p, src.now()), "expired")
		// and a later call says the same without touching the platform again
		_, _, before := pl.counts()
		_, err = src.Token(context.Background())
		require.EqualError(t, err, wantExpired)
		_, _, after := pl.counts()
		require.Equal(t, before, after)
	})

	t.Run("absolute deadline passed (no network)", func(t *testing.T) {
		e := newFnTestEnv(t)
		pl := newLoginPlatform(t)
		p, _ := doLogin(t, e, pl)
		src := newLoginTokenSource(p)
		src.now = func() time.Time { return time.Now().Add(7*24*time.Hour + time.Minute) }
		_, err := src.Token(context.Background())
		require.EqualError(t, err, wantExpired)
		_, _, refreshes := pl.counts()
		require.Zero(t, refreshes)
	})

	t.Run("cached access token does not outlive the deadline", func(t *testing.T) {
		e := newFnTestEnv(t)
		pl := newLoginPlatform(t)
		pl.refreshTTL = 30 * time.Minute // shorter than the access token
		p, _ := doLogin(t, e, pl)
		src := newLoginTokenSource(p)
		src.now = func() time.Time { return time.Now().Add(31 * time.Minute) }
		_, err := src.Token(context.Background())
		require.EqualError(t, err, wantExpired)
	})

	t.Run("never logged in", func(t *testing.T) {
		e := newFnTestEnv(t)
		pl := newLoginPlatform(t)
		p := loginProfile(t, e, pl, "np")
		_, err := newLoginTokenSource(p).Token(context.Background())
		require.EqualError(t, err, "not logged in: run fcdev login --profile me")
	})

	t.Run("a transient platform failure is not an expired session", func(t *testing.T) {
		e := newFnTestEnv(t)
		pl := newLoginPlatform(t)
		p, _ := doLogin(t, e, pl)
		pl.mu.Lock()
		pl.refreshFail = 503
		pl.mu.Unlock()
		src := newLoginTokenSource(p)
		src.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
		_, err := src.Token(context.Background())
		require.ErrorContains(t, err, "could not refresh")
		require.NotContains(t, err.Error(), "session expired")
		_, kerr := e.kc.Get("login:me")
		require.NoError(t, kerr, "the session must survive an outage")
	})

	t.Run("fn command, no terminal, expired: message and no browser", func(t *testing.T) {
		e := newFnTestEnv(t)
		pl := newLoginPlatform(t)
		doLogin(t, e, pl)
		// age the stored session: access token lapsed and the platform has forgotten it
		meta, _ := readLoginMeta("me")
		meta.AccessExpiresAt = time.Now().Add(-time.Minute)
		require.NoError(t, writeLoginMeta("me", meta))
		pl.mu.Lock()
		pl.live = map[string]*refreshFamily{}
		pl.mu.Unlock()
		e.tty = false
		done := make(chan error, 1)
		go func() {
			_, _, err := runCobra(t, newFnStatusCmd(), "", "demo.logger", "--profile", "me")
			done <- err
		}()
		select {
		case err := <-done:
			require.EqualError(t, err, wantExpired)
		case <-time.After(10 * time.Second):
			t.Fatal("the command hung instead of failing")
		}
	})
}

func TestLoginConcurrentCommandsRefreshOnce(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	p, _ := doLogin(t, e, pl)
	meta, _ := readLoginMeta("me")
	meta.AccessExpiresAt = time.Now().Add(-time.Minute) // lapsed
	require.NoError(t, writeLoginMeta("me", meta))

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := newLoginTokenSource(p).Token(context.Background())
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	_, _, refreshes := pl.counts()
	require.Equal(t, 1, refreshes, "concurrent fcdev processes must not present the same refresh token twice")
	pl.mu.Lock()
	for _, fam := range pl.rotated {
		require.False(t, fam.revoked, "a replay would have revoked the session")
	}
	pl.mu.Unlock()
}

func TestLoginTokenIsNeverSentToAnotherPlatform(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	doLogin(t, e, pl)
	other := newLoginPlatform(t)
	// the profile now points elsewhere (edited, or planted)
	e.writeProfiles(t, 0o600, loginProfileTOML("me", other.srv.URL, "np"))
	set, _, err := loadProfiles()
	require.NoError(t, err)
	meta, _ := readLoginMeta("me")
	meta.AccessExpiresAt = time.Now().Add(-time.Minute)
	require.NoError(t, writeLoginMeta("me", meta))

	_, err = newLoginTokenSource(set["me"]).Token(context.Background())
	require.ErrorContains(t, err, "platform changed")
	require.ErrorContains(t, err, "run fcdev login --profile me")
	a, x, r := other.counts()
	require.Zero(t, a+x+r, "nothing may reach the other platform")
	require.Contains(t, loginStatus(set["me"], time.Now()), "another platform")

	// logout does not send the token there either
	var out bytes.Buffer
	require.NoError(t, runLogout(context.Background(), testDeps(&out, newFakeBrowser()), set["me"]))
	other.mu.Lock()
	require.Empty(t, other.revoked)
	other.mu.Unlock()
	require.Contains(t, out.String(), "not revoked at the platform")
}

func TestFnCommandUsesTheLoginSessionAndShowsTheUser(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	doLogin(t, e, pl)

	_, errOut, err := runCobra(t, newFnStatusCmd(), "", "demo.logger", "--profile", "me")
	require.NoError(t, err)
	first, _, _ := strings.Cut(errOut, "\n")
	require.Equal(t, fmt.Sprintf("profile me (np) %s user andrew@example.com", pl.srv.URL), first)
	pl.mu.Lock()
	defer pl.mu.Unlock()
	require.NotEmpty(t, pl.apiAuth)
	for _, h := range pl.apiAuth {
		require.True(t, strings.HasPrefix(h, "Bearer eyJ"), h)
	}
	require.NotContains(t, errOut, "eyJ")
}

func TestFnCommandRefusesClientFlagsWithALoginProfile(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	doLogin(t, e, pl)
	_, _, err := runCobra(t, newFnStatusCmd(), "", "demo.logger", "--profile", "me", "--client-id", "x")
	require.ErrorContains(t, err, "cannot be combined")
}

func TestLoginProfileKeepsTheProdGuards(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	e.writeProfiles(t, 0o600, loginProfileTOML("me", pl.srv.URL, "prod"))
	set, _, _ := loadProfiles()
	tgt := &fnTarget{creds: fnCredentials{PlatformURL: pl.srv.URL}, profile: set["me"], user: "andrew@example.com"}
	require.Equal(t, fmt.Sprintf("profile me (prod) %s user andrew@example.com", pl.srv.URL), tgt.banner())

	pr := &fnPrompter{in: newStringReader(""), out: io.Discard, tty: false}
	require.ErrorContains(t, tgt.confirmMutation(pr, "promote x", true), "allow_noninteractive")
	pr = &fnPrompter{in: newStringReader("prod\n"), out: io.Discard, tty: true}
	require.NoError(t, tgt.confirmMutation(pr, "promote x", false))
	pr = &fnPrompter{in: newStringReader("yes\n"), out: io.Discard, tty: true}
	require.Error(t, tgt.confirmMutation(pr, "promote x", true))

	tgt.user = ""
	require.Contains(t, tgt.banner(), "user (not signed in)")
}

// ---- logout ---------------------------------------------------------------

func TestLogoutRevokesAndDeletes(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	p, _ := doLogin(t, e, pl)

	var out bytes.Buffer
	raw, _ := e.kc.Get("login:me")
	var sec loginSecret
	require.NoError(t, json.Unmarshal([]byte(raw), &sec))
	require.NoError(t, runLogout(context.Background(), testDeps(&out, newFakeBrowser()), p))
	require.Contains(t, out.String(), "revoked at the platform")
	pl.mu.Lock()
	require.Equal(t, []string{sec.RefreshToken}, pl.revoked)
	require.Equal(t, "fcdev-cli", pl.revokeForm.Get("client_id"))
	require.Empty(t, pl.revokeForm.Get("client_secret"))
	pl.mu.Unlock()
	for _, a := range []string{"login:me", "login-access:me", "login-meta:me"} {
		_, err := e.kc.Get(a)
		require.ErrorIs(t, err, ErrKeychainNotFound, a)
	}
	require.NotContains(t, out.String(), sec.RefreshToken)

	// the revoked session is dead on the platform too, and locally "not logged in"
	_, err := newLoginTokenSource(p).Token(context.Background())
	require.EqualError(t, err, "not logged in: run fcdev login --profile me")
	require.Contains(t, loginStatus(p, time.Now()), "not logged in")

	// again: nothing to do
	out.Reset()
	require.NoError(t, runLogout(context.Background(), testDeps(&out, newFakeBrowser()), p))
	require.Contains(t, out.String(), "no stored session")
}

func TestLogoutDeletesLocallyEvenWhenTheRevokeFails(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	p, _ := doLogin(t, e, pl)
	pl.srv.Close()
	var out bytes.Buffer
	require.NoError(t, runLogout(context.Background(), testDeps(&out, newFakeBrowser()), p))
	require.Contains(t, out.String(), "could not revoke the session at the platform")
	require.Contains(t, out.String(), "local tokens deleted")
	require.NotContains(t, out.String(), "rt-")
	_, err := e.kc.Get("login:me")
	require.ErrorIs(t, err, ErrKeychainNotFound)
}

func TestLogoutCommand(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	doLogin(t, e, pl)
	out, _, err := runCobra(t, newLogoutCmd(), "", "--profile", "me")
	require.NoError(t, err)
	require.Contains(t, out, "Logged out of profile me")

	e.writeProfiles(t, 0o600, profileTOML("ci", pl.srv.URL, "np", "env:FC_TEST_SECRET"))
	_, _, err = runCobra(t, newLogoutCmd(), "", "--profile", "ci")
	require.ErrorContains(t, err, "client credentials")
}

// ---- profiles --------------------------------------------------------------

func TestLoginProfileFileRoundTripAndRules(t *testing.T) {
	e := newFnTestEnv(t)
	set := profileSet{
		"me": {Name: "me", Platform: "https://fc.example.com", Env: "prod", Auth: authLogin},
		"ci": {Name: "ci", Platform: "https://fc.example.com", Env: "np", ClientID: "c", SecretRef: "env:X"},
	}
	require.NoError(t, saveProfiles(e.profiles, set))
	body, err := os.ReadFile(e.profiles)
	require.NoError(t, err)
	require.Contains(t, string(body), `auth = "login"`)
	require.NotContains(t, string(body), "client_id = \"\"")
	got, _, err := loadProfiles()
	require.NoError(t, err)
	require.Equal(t, set, got)
	require.True(t, got["me"].IsLogin())
	require.False(t, got["ci"].IsLogin())

	// an existing client-credentials file (no auth key) loads unchanged
	_, err = parseProfiles(profileTOML("old", "https://fc.example.com", "np", "keychain"))
	require.NoError(t, err)

	hdr := "[profiles.me]\nplatform = \"https://fc.example.com\"\nenv = \"np\"\n"
	for name, c := range map[string]struct{ src, want string }{
		"login with client_id":    {hdr + "auth = \"login\"\nclient_id = \"c\"\n", "takes no client_id or secret_ref"},
		"login with secret_ref":   {hdr + "auth = \"login\"\nsecret_ref = \"env:X\"\n", "takes no client_id or secret_ref"},
		"unknown auth":            {hdr + "auth = \"hunter2\"\nclient_id = \"c\"\n", "auth must be"},
		"refresh token key":       {hdr + "auth = \"login\"\nrefresh_token = \"hunter2\"\n", "looks like a secret"},
		"client secret key":       {hdr + "auth = \"login\"\nclient_secret = \"hunter2\"\n", "looks like a secret"},
		"access token key":        {hdr + "auth = \"login\"\naccess_token = \"hunter2\"\n", "looks like a secret"},
		"client creds still need": {hdr + "auth = \"client_credentials\"\n", "client_id is required"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseProfiles(c.src)
			require.ErrorContains(t, err, c.want)
			require.NotContains(t, err.Error(), "hunter2")
		})
	}
}

func TestProfileAddLogin(t *testing.T) {
	e := newFnTestEnv(t)
	out, _, err := runCobra(t, newProfileCmd(), "", "add", "me", "--platform", "https://fc.example.com", "--env", "np", "--login")
	require.NoError(t, err)
	require.Contains(t, out, "fcdev login --profile me")
	set, _, err := loadProfiles()
	require.NoError(t, err)
	require.True(t, set["me"].IsLogin())
	require.Empty(t, set["me"].ClientID)
	require.Empty(t, set["me"].SecretRef)
	require.Empty(t, e.kc.m, "adding a login profile stores nothing in the keychain")

	_, _, err = runCobra(t, newProfileCmd(), "", "add", "x", "--platform", "https://fc.example.com", "--env", "np", "--login", "--client-id", "c")
	require.ErrorContains(t, err, "--login takes no --client-id or --secret-ref")
	_, _, err = runCobra(t, newProfileCmd(), "", "add", "y", "--platform", "https://fc.example.com", "--env", "np", "--login", "--secret-ref", "env:X")
	require.ErrorContains(t, err, "--login takes no --client-id or --secret-ref")
	_, _, err = runCobra(t, newProfileCmd(), "", "add", "z", "--platform", "https://fc.example.com", "--env", "np")
	require.ErrorContains(t, err, "--client-id is required")

	// client-credentials add is unchanged
	_, _, err = runCobra(t, newProfileCmd(), "", "add", "ci", "--platform", "https://fc.example.com", "--env", "np", "--client-id", "c", "--secret-ref", "env:S")
	require.NoError(t, err)

	_, _, err = runCobra(t, newProfileCmd(), "", "set-secret", "me")
	require.ErrorContains(t, err, "login profile")
}

func TestProfileListAndShowLoginStatusWithoutReadingSecrets(t *testing.T) {
	e := newFnTestEnv(t)
	rk := &recordingKeychain{memKeychain: e.kc}
	fnKeychain = rk
	pl := newLoginPlatform(t)
	e.writeProfiles(t, 0o600, loginProfileTOML("a-me", pl.srv.URL, "np")+"\n"+loginProfileTOML("b-out", pl.srv.URL, "prod")+"\n"+
		loginProfileTOML("c-dead", pl.srv.URL, "np")+"\n"+profileTOML("d-ci", pl.srv.URL, "np", "env:FC_TEST_SECRET"))
	set, _, _ := loadProfiles()

	// a-me is signed in; c-dead had a session that ended
	var out bytes.Buffer
	b := newFakeBrowser()
	require.NoError(t, runLogin(context.Background(), testDeps(&out, b), set["a-me"]))
	require.NoError(t, writeLoginMeta("c-dead", &loginMeta{Platform: pl.srv.URL, Email: "gone@example.com", RefreshExpiresAt: time.Now().Add(-time.Hour)}))
	rk.mu.Lock()
	rk.reads = nil
	rk.mu.Unlock()

	list, _, err := runCobra(t, newProfileCmd(), "", "list")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(list), "\n")
	require.Len(t, lines, 5, list)
	require.Contains(t, lines[0], "AUTH")
	require.Regexp(t, `^a-me\s+np\s+\S+\s+login\s+andrew@example.com, until \d{4}-\d\d-\d\d \d\d:\d\d \(in 6d 23h\)$`, lines[1])
	require.Regexp(t, `^b-out\s+prod\s+\S+\s+login\s+not logged in$`, lines[2])
	require.Regexp(t, `^c-dead\s+np\s+\S+\s+login\s+expired \(was gone@example.com\)$`, lines[3])
	require.Regexp(t, `^d-ci\s+np\s+\S+\s+client\s+env$`, lines[4])
	show, _, err := runCobra(t, newProfileCmd(), "", "show", "a-me")
	require.NoError(t, err)
	require.Contains(t, show, "andrew@example.com")

	rk.mu.Lock()
	defer rk.mu.Unlock()
	for _, acct := range rk.reads {
		require.True(t, strings.HasPrefix(acct, "login-meta:"), "profile list/show must only read non-secret metadata, read %q", acct)
	}
	require.NotContains(t, list+show, "rt-")
	require.NotContains(t, list+show, "eyJ")
}

func TestProfileTestForALoginProfile(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	doLogin(t, e, pl)
	out, _, err := runCobra(t, newProfileCmd(), "", "test", "me")
	require.NoError(t, err)
	require.Contains(t, out, "ok: signed in as andrew@example.com")
	require.Contains(t, out, "principal  prn_ME")
	require.Contains(t, out, "tier       ANCHOR")
	require.NotContains(t, out, "eyJ")
}

func TestProfileRemoveDeletesTheLoginSession(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newLoginPlatform(t)
	doLogin(t, e, pl)
	_, _, err := runCobra(t, newProfileCmd(), "", "remove", "me", "--yes")
	require.NoError(t, err)
	require.Empty(t, e.kc.m)
}

func newStringReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestDefaultLoginListenerIsLoopbackOnly(t *testing.T) {
	ln, err := defaultLoginDeps(io.Discard).listen()
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	require.Equal(t, "127.0.0.1", ln.Addr().(*net.TCPAddr).IP.String())
	require.NotZero(t, ln.Addr().(*net.TCPAddr).Port)
}
