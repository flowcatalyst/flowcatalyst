package pages

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
)

// fakeUpstream stands in for the platform's public auth endpoints: handlers by
// "METHOD /path", and a record of what each call carried.
type fakeUpstream struct {
	handlers map[string]func(w http.ResponseWriter, r *http.Request, body map[string]any)
	calls    []fakeCall
}

type fakeCall struct {
	method, path, query, host, remote string
	header                            http.Header
	body                              map[string]any
}

func (f *fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	call := fakeCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, host: r.Host, remote: r.RemoteAddr, header: r.Header.Clone()}
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &call.body)
	}
	f.calls = append(f.calls, call)
	h, ok := f.handlers[r.Method+" "+r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	h(w, r, call.body)
}

func (f *fakeUpstream) on(route string, h func(w http.ResponseWriter, r *http.Request, body map[string]any)) {
	if f.handlers == nil {
		f.handlers = map[string]func(http.ResponseWriter, *http.Request, map[string]any){}
	}
	f.handlers[route] = h
}

// reply answers a fixed JSON body with status.
func (f *fakeUpstream) reply(route string, status int, body string, headers ...string) {
	f.on(route, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		for i := 0; i+1 < len(headers); i += 2 {
			w.Header().Add(headers[i], headers[i+1])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

func (f *fakeUpstream) callsTo(route string) []fakeCall {
	var out []fakeCall
	for _, c := range f.calls {
		if c.method+" "+c.path == route {
			out = append(out, c)
		}
	}
	return out
}

const sessionSetCookie = "fc_session=SESSIONTOKEN; Path=/; HttpOnly; SameSite=Lax"

// signInRouter mounts the pages as /ui does (same-origin posts pass).
func signInRouter(up http.Handler, authenticated bool) http.Handler {
	r := chi.NewRouter()
	authn := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticated {
				r = r.WithContext(auth.WithContext(r.Context(), &auth.AuthContext{PrincipalID: "prn_1", Email: "a@b.c"}))
			}
			next.ServeHTTP(w, r)
		})
	}
	csrf, _ := guard.CSRF(nil, guard.PlainDenier)
	r.Route("/ui", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(guard.SecurityHeaders, csrf)
			(&SignIn{Upstream: up, Authenticate: authn}).Mount(r)
		})
	})
	return r
}

type siReq struct {
	method, target string
	form           url.Values
	cookie         string
}

func (q siReq) do(h http.Handler) *httptest.ResponseRecorder {
	method := q.method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if q.form != nil {
		body = strings.NewReader(q.form.Encode())
	}
	r := httptest.NewRequest(method, q.target, body)
	r.RemoteAddr = "203.0.113.9:4444"
	r.Header.Set("User-Agent", "sign-in-test")
	if q.form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if method == http.MethodPost {
		r.Header.Set("Origin", "http://example.com")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	if q.cookie != "" {
		r.Header.Set("Cookie", q.cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func siPost(target string, kv ...string) siReq {
	form := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		form.Add(kv[i], kv[i+1])
	}
	return siReq{method: http.MethodPost, target: target, form: form}
}

// siThemed is an upstream that already serves the login theme.
func siThemed() *fakeUpstream {
	f := &fakeUpstream{}
	f.reply("GET /api/public/login-theme", 200, `{}`)
	return f
}

func TestSafeUIRedirect(t *testing.T) {
	for in, want := range map[string]string{
		"/ui":  "/ui",
		"/ui/": "/ui/",
		"/ui/dispatch-pools?q=a%26b&status=ACTIVE": "/ui/dispatch-pools?q=a%26b&status=ACTIVE",
		"/ui/x#top":                   "/ui/x#top",
		"":                            "",
		"/dashboard":                  "",
		"/uix":                        "",
		"/ui-evil":                    "",
		"ui/x":                        "",
		"https://evil.example/ui/x":   "",
		"//evil.example/ui/x":         "",
		"/\\evil.example/ui/x":        "",
		"\\\\evil.example/ui/x":       "",
		"/\t/evil.example/ui/x":       "",
		"javascript:alert(1)//ui/":    "",
		"/ui/../dashboard":            "",
		"/ui/%2e%2e/dashboard":        "",
		"/ui/./../../oauth/authorize": "",
		"/ui/..%2f..%2fdashboard":     "",
		"/ui/a/../b":                  "/ui/b",
	} {
		assert.Equal(t, want, safeUIRedirect(in), "%q", in)
	}
}

func TestLandingPrecedence(t *testing.T) {
	ctx := func(kv ...string) signinContext {
		c := signinContext{vals: url.Values{}}
		for i := 0; i+1 < len(kv); i += 2 {
			c.vals.Set(kv[i], kv[i+1])
		}
		return c
	}
	assert.Equal(t, "/ui/", ctx().landing())
	assert.Equal(t, "/ui/roles", ctx("redirect", "/ui/roles").landing())
	assert.Equal(t, "/ui/", ctx("redirect", "https://evil.example/ui/").landing(), "an open redirect falls back")
	assert.Equal(t, "/ui/", ctx("redirect", "/dashboard").landing(), "SPA routes are not followed")
	assert.Equal(t, "/oidc/interaction/abc_1/login", ctx("interaction", "abc_1", "oauth", "true", "redirect", "/ui/x").landing())
	assert.Equal(t, "/ui/x", ctx("interaction", "../x", "redirect", "/ui/x").landing(), "a malformed interaction id is ignored")
	got := ctx("oauth", "true", "client_id", "app 1", "redirect_uri", "https://app/cb", "state", "s", "redirect", "/ui/x").landing()
	assert.Equal(t, "/oauth/authorize?client_id=app+1&redirect_uri=https%3A%2F%2Fapp%2Fcb&state=s", got)
}

func TestLoginPageIsStandaloneAndHardened(t *testing.T) {
	up := siThemed()
	rec := siReq{target: "/ui/auth/login?redirect=/ui/roles&client=acme"}.do(signInRouter(up, false))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Sign in to your account")
	assert.Contains(t, body, `action="/ui/auth/login/email"`)
	assert.Contains(t, body, `name="redirect" value="/ui/roles"`, "the context rides along as hidden fields")
	assert.Contains(t, body, `name="client" value="acme"`)
	assert.NotContains(t, body, "fc-app-shell", "no app shell before a session")
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "img-src 'self' data: https:", "tenant logos from https hosts")
	assert.NotContains(t, rec.Header().Get("Content-Security-Policy"), "script-src 'self' https", "scripts stay same-origin")
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	theme := up.callsTo("GET /api/public/login-theme")
	require.Len(t, theme, 1)
	assert.Equal(t, "client=acme", theme[0].query, "the branding client is the theme lookup's")
}

func TestLoginThemeIsSanitised(t *testing.T) {
	up := &fakeUpstream{}
	up.reply("GET /api/public/login-theme", 200, `{
		"brandName":"Acme <b>Corp</b>","brandSubtitle":"Sub","footerText":"Foot",
		"accentColor":"red;background:url(https://evil.example/x)","primaryColor":"#123456",
		"backgroundGradient":"linear-gradient(135deg, #111 0%, #222 100%)",
		"logoUrl":"javascript:alert(1)","logoSvg":"<svg onload=alert(1)><script>alert(1)</script></svg>",
		"customCss":".x{color:red}</style><script>alert(1)</script>"}`)
	rec := siReq{target: "/ui/auth/login"}.do(signInRouter(up, false))
	body := rec.Body.String()
	assert.Contains(t, body, "Acme &lt;b&gt;Corp&lt;/b&gt;", "text is escaped")
	assert.Contains(t, body, "linear-gradient(135deg, #111 0%, #222 100%)")
	assert.Contains(t, body, "--login-primary:#123456")
	assert.Contains(t, body, "--login-accent:#0967d2", "an accent that is not a plain colour keeps the default")
	assert.NotContains(t, body, "evil.example")
	assert.NotContains(t, body, "javascript:")
	assert.NotContains(t, body, "<script>alert", "no tenant markup runs: the SVG is an image data URL, the CSS cannot close its element")
	assert.NotContains(t, body, "onload")
	assert.Contains(t, body, `src="data:image/svg+xml;base64,`)
	assert.Contains(t, body, ".x{color:red}/style>")
}

func TestThemeFailureDoesNotTakeThePageDown(t *testing.T) {
	up := &fakeUpstream{} // theme 404s
	rec := siReq{target: "/ui/auth/login"}.do(signInRouter(up, false))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "FlowCatalyst")
}

func TestEmailStep(t *testing.T) {
	t.Run("an SSO domain goes to its identity provider with the round trip context", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/check-domain", 200, `{"authMethod":"external","loginUrl":"/auth/oidc/login?domain=acme.example","idpIssuer":"https://idp"}`)
		rec := siPost("/ui/auth/login/email", "email", "a@acme.example", "oauth", "true", "client_id", "app1", "redirect_uri", "https://app/cb",
			"state", "s1", "redirect", "/ui/roles").do(signInRouter(up, false))
		require.Equal(t, http.StatusSeeOther, rec.Code)
		loc, err := url.Parse(rec.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "/auth/oidc/login", loc.Path)
		q := loc.Query()
		assert.Equal(t, "acme.example", q.Get("domain"))
		assert.Equal(t, "app1", q.Get("oauth_client_id"))
		assert.Equal(t, "https://app/cb", q.Get("oauth_redirect_uri"))
		assert.Equal(t, "s1", q.Get("oauth_state"))
		assert.Equal(t, "/ui/auth/callback?redirect=%2Fui%2Froles", q.Get("return_url"))
		assert.Equal(t, "a@acme.example", up.callsTo("POST /auth/check-domain")[0].body["email"])
	})
	t.Run("a login URL that is not the bridge's is not followed", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/check-domain", 200, `{"authMethod":"external","loginUrl":"https://evil.example/auth/oidc/login?domain=x"}`)
		rec := siPost("/ui/auth/login/email", "email", "a@acme.example").do(signInRouter(up, false))
		assert.Equal(t, http.StatusBadGateway, rec.Code)
		assert.Empty(t, rec.Header().Get("Location"))
	})
	t.Run("an internal user gets the password step", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/check-domain", 200, `{"authMethod":"internal"}`)
		rec := siPost("/ui/auth/login/email", "email", "a@b.c", "redirect", "/ui/x").do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "Enter your password")
		assert.Contains(t, body, `action="/ui/auth/login/password"`)
		assert.Contains(t, body, `name="email" value="a@b.c"`)
		assert.Contains(t, body, `autocomplete="current-password"`)
		assert.Contains(t, body, "/ui/auth/forgot-password?")
	})
	t.Run("a user who never set a password is offered the emailed link", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/check-domain", 200, `{"authMethod":"internal","passwordSetupRequired":true}`)
		body := siPost("/ui/auth/login/email", "email", "a@b.c").do(signInRouter(up, false)).Body.String()
		assert.Contains(t, body, "Create your password")
		assert.Contains(t, body, "Email me a link")
		assert.NotContains(t, body, `type="password"`)
	})
	t.Run("a malformed address is asked again without a lookup", func(t *testing.T) {
		up := siThemed()
		rec := siPost("/ui/auth/login/email", "email", "nope").do(signInRouter(up, false))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Contains(t, rec.Body.String(), "Please enter a valid email address")
		assert.Empty(t, up.callsTo("POST /auth/check-domain"))
	})
	t.Run("a failed lookup is shown, not swallowed", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/check-domain", 500, `{"code":"X","message":"database down"}`)
		rec := siPost("/ui/auth/login/email", "email", "a@b.c").do(signInRouter(up, false))
		assert.Equal(t, http.StatusBadGateway, rec.Code)
		assert.Contains(t, rec.Body.String(), "database down")
	})
}

func TestPasswordStep(t *testing.T) {
	const wrong = `{"code":"UNAUTHENTICATED","message":"Invalid credentials"}`
	t.Run("a session: cookies passed on, then the landing", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 200, `{"status":"ok","principalId":"prn_1"}`, "Set-Cookie", sessionSetCookie)
		rec := siPost("/ui/auth/login/password", "email", "A@B.C", "password", "s3cret!", "redirect", "/ui/roles").do(signInRouter(up, false))
		require.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Equal(t, "/ui/roles", rec.Header().Get("Location"))
		assert.Equal(t, []string{sessionSetCookie}, rec.Header().Values("Set-Cookie"), "the endpoint's own cookie, unchanged")
		assert.NotContains(t, rec.Body.String(), "s3cret!")
	})
	t.Run("the endpoint sees the browser: host, address, provenance and cookies", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 200, `{"status":"ok"}`)
		req := siPost("/ui/auth/login/password", "email", "a@b.c", "password", "pw")
		req.cookie = "fc_td=device"
		req.do(signInRouter(up, false))
		c := up.callsTo("POST /auth/login")
		require.Len(t, c, 1)
		assert.Equal(t, "example.com", c[0].host)
		assert.Equal(t, "203.0.113.9:4444", c[0].remote, "the backoff keys on the browser's address")
		assert.Equal(t, "same-origin", c[0].header.Get("Sec-Fetch-Site"))
		assert.Equal(t, "http://example.com", c[0].header.Get("Origin"))
		assert.Equal(t, "fc_td=device", c[0].header.Get("Cookie"))
		assert.Equal(t, "application/json", c[0].header.Get("Content-Type"))
		assert.Empty(t, c[0].header.Get("Datastar-Request"))
		assert.Equal(t, "a@b.c", c[0].body["email"])
		assert.Equal(t, "pw", c[0].body["password"])
	})
	t.Run("an open redirect in ?redirect= is dropped", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 200, `{"status":"ok"}`)
		for _, bad := range []string{"https://evil.example/ui/", "//evil.example/ui/", "/\\evil.example/ui/", "/dashboard", "/ui/../dashboard"} {
			rec := siPost("/ui/auth/login/password", "email", "a@b.c", "password", "pw", "redirect", bad).do(signInRouter(up, false))
			assert.Equal(t, "/ui/", rec.Header().Get("Location"), bad)
		}
	})
	t.Run("an OAuth round trip resumes the authorize request", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 200, `{"status":"ok"}`)
		rec := siPost("/ui/auth/login/password", "email", "a@b.c", "password", "pw", "oauth", "true", "client_id", "app1", "state", "s").do(signInRouter(up, false))
		assert.Equal(t, "/oauth/authorize?client_id=app1&state=s", rec.Header().Get("Location"))
	})
	t.Run("an unknown user and a wrong password get the same page", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 401, wrong, "Www-Authenticate", `Cookie realm="fc_session"`)
		h := signInRouter(up, false)
		a := siPost("/ui/auth/login/password", "email", "known@b.c", "password", "wrong").do(h)
		b := siPost("/ui/auth/login/password", "email", "known@b.c", "password", "other-wrong").do(h)
		require.Equal(t, http.StatusUnauthorized, a.Code)
		assert.Equal(t, a.Code, b.Code)
		assert.Equal(t, a.Body.String(), b.Body.String(), "nothing in the page depends on the password or on whether the account exists")
		assert.Contains(t, a.Body.String(), "Invalid credentials")
		assert.Empty(t, a.Header().Values("Set-Cookie"))
		assert.NotContains(t, a.Body.String(), "wrong", "the password is never echoed")
		assert.Contains(t, a.Body.String(), `name="email" value="known@b.c"`)
	})
	t.Run("a locked-out address is told to wait, with Retry-After", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 429, `{"code":"TOO_MANY_REQUESTS","message":"too many failed login attempts; try again later"}`, "Retry-After", "37")
		rec := siPost("/ui/auth/login/password", "email", "a@b.c", "password", "pw").do(signInRouter(up, false))
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		assert.Equal(t, "37", rec.Header().Get("Retry-After"))
		assert.Contains(t, rec.Body.String(), "Try again in 37 seconds")
		assert.Empty(t, rec.Header().Values("Set-Cookie"))
	})
	t.Run("a domain that moved to SSO is sent back to the email step", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 403, `{"code":"SSO_REQUIRED","message":"..."}`)
		rec := siPost("/ui/auth/login/password", "email", "a@acme.example", "password", "pw").do(signInRouter(up, false))
		assert.Equal(t, http.StatusForbidden, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "identity provider")
		assert.Contains(t, body, `action="/ui/auth/login/email"`)
		assert.Contains(t, body, `value="a@acme.example"`)
	})
	t.Run("an empty password is not sent", func(t *testing.T) {
		up := siThemed()
		rec := siPost("/ui/auth/login/password", "email", "a@b.c", "password", "").do(signInRouter(up, false))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Empty(t, up.callsTo("POST /auth/login"))
	})
	t.Run("forced enrolment goes to the enrolment page by POST, and no session is issued", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 200, `{"status":"enrollment_required","enrollToken":"ENROLL","allowedMethods":["TOTP"]}`)
		rec := siPost("/ui/auth/login/password", "email", "a@b.c", "password", "pw", "redirect", "/ui/x").do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, `action="/ui/auth/2fa/enroll/start"`)
		assert.Contains(t, body, `name="token" value="ENROLL"`, "the signed short-lived enrolment token travels in the POST body")
		assert.Contains(t, body, `name="next" value="/ui/x"`)
		assert.NotContains(t, body, "/ui/auth/2fa/enroll/start?", "never in a URL")
		assert.Empty(t, rec.Header().Values("Set-Cookie"))
	})
	t.Run("an enrolment that must return to an OAuth round trip keeps using the current sign-in", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 200, `{"status":"enrollment_required","enrollToken":"ENROLL","allowedMethods":["TOTP"]}`)
		rec := siPost("/ui/auth/login/password", "email", "a@b.c", "password", "pw", "oauth", "true").do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, `href="/auth/login`)
		assert.NotContains(t, body, "ENROLL", "no token is rendered where the new enrolment page cannot return")
		assert.Empty(t, rec.Header().Values("Set-Cookie"))
	})
	t.Run("the login page offers a passkey only for a destination under /ui", func(t *testing.T) {
		up := siThemed()
		rec := siReq{target: "/ui/auth/login?redirect=%2Fui%2Fx"}.do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "/ui/auth/passkey?next=%2Fui%2Fx")
		rec = siReq{target: "/ui/auth/login?oauth=true"}.do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), "/ui/auth/passkey")
	})
}

func TestTwoFactorStep(t *testing.T) {
	challenge := `{"status":"mfa_required","mfaToken":"PENDING.TOKEN","methods":["TOTP","EMAIL_PIN"],"rememberDeviceAllowed":true}`
	t.Run("a password that owes a second factor shows the challenge and no session", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/login", 200, challenge)
		rec := siPost("/ui/auth/login/password", "email", "a@b.c", "password", "pw", "redirect", "/ui/x").do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "Verify it&#39;s you")
		assert.Contains(t, body, `name="mfaToken" value="PENDING.TOKEN"`)
		assert.Contains(t, body, `name="method" value="TOTP"`)
		assert.Contains(t, body, "Remember this device")
		assert.Contains(t, body, "Use an email code")
		assert.Contains(t, body, "Use a recovery code")
		assert.Empty(t, rec.Header().Values("Set-Cookie"))
	})
	t.Run("a good code completes the sign-in", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/2fa/verify", 200, `{"status":"ok"}`, "Set-Cookie", sessionSetCookie, "Set-Cookie", "fc_td=device; Path=/; HttpOnly")
		rec := siPost("/ui/auth/login/2fa", "mfaToken", "PENDING.TOKEN", "method", "TOTP", "methods", "TOTP,EMAIL_PIN", "remember", "1",
			"code", " 123456 ", "rememberDevice", "1", "redirect", "/ui/roles").do(signInRouter(up, false))
		require.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Equal(t, "/ui/roles", rec.Header().Get("Location"))
		assert.Len(t, rec.Header().Values("Set-Cookie"), 2, "the session and the trusted-device cookie")
		c := up.callsTo("POST /auth/2fa/verify")[0].body
		assert.Equal(t, "PENDING.TOKEN", c["mfaToken"])
		assert.Equal(t, "TOTP", c["method"])
		assert.Equal(t, "123456", c["code"])
		assert.Equal(t, true, c["rememberDevice"])
	})
	t.Run("remember device is not asked when the domain does not allow it", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/2fa/verify", 200, `{"status":"ok"}`)
		siPost("/ui/auth/login/2fa", "mfaToken", "T", "method", "TOTP", "code", "1", "rememberDevice", "1").do(signInRouter(up, false))
		assert.Equal(t, false, up.callsTo("POST /auth/2fa/verify")[0].body["rememberDevice"])
	})
	t.Run("a wrong code is refused and shown again", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/2fa/verify", 401, `{"code":"UNAUTHENTICATED","message":"Invalid or expired code"}`)
		rec := siPost("/ui/auth/login/2fa", "mfaToken", "PENDING.TOKEN", "method", "TOTP", "methods", "TOTP", "code", "000000").do(signInRouter(up, false))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid or expired code")
		assert.Contains(t, rec.Body.String(), `name="mfaToken" value="PENDING.TOKEN"`)
		assert.Empty(t, rec.Header().Values("Set-Cookie"))
	})
	t.Run("a locked-out second factor says when to retry", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/2fa/verify", 429, `{"code":"TOO_MANY_REQUESTS","message":"x"}`, "Retry-After", "12")
		rec := siPost("/ui/auth/login/2fa", "mfaToken", "T", "method", "TOTP", "code", "1").do(signInRouter(up, false))
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		assert.Equal(t, "12", rec.Header().Get("Retry-After"))
	})
	t.Run("an email code is sent, then asked for", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/2fa/challenge/email", 200, `{"message":"sent"}`)
		rec := siPost("/ui/auth/login/2fa", "mfaToken", "T", "method", "EMAIL_PIN", "methods", "TOTP,EMAIL_PIN", "action", "send").do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "T", up.callsTo("POST /auth/2fa/challenge/email")[0].body["mfaToken"])
		assert.Contains(t, rec.Body.String(), "Enter the code we emailed you.")
	})
	t.Run("switching method calls nothing", func(t *testing.T) {
		up := siThemed()
		rec := siPost("/ui/auth/login/2fa", "mfaToken", "T", "method", "TOTP", "methods", "TOTP", "switch", "RECOVERY_CODE").do(signInRouter(up, false))
		assert.Contains(t, rec.Body.String(), "Enter one of your recovery codes.")
		assert.Empty(t, up.callsTo("POST /auth/2fa/verify"))
	})
	t.Run("a missing token starts over", func(t *testing.T) {
		up := siThemed()
		rec := siPost("/ui/auth/login/2fa", "method", "TOTP", "code", "1").do(signInRouter(up, false))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Empty(t, up.callsTo("POST /auth/2fa/verify"))
	})
}

func TestSetupStep(t *testing.T) {
	up := siThemed()
	up.reply("POST /auth/password-setup/request", 200, `{"message":"ok"}`)
	rec := siPost("/ui/auth/login/setup", "email", "a@b.c", "oauth", "true", "client_id", "app1").do(signInRouter(up, false))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Check your email")
	c := up.callsTo("POST /auth/password-setup/request")[0].body
	assert.Equal(t, "a@b.c", c["email"])
	assert.Equal(t, "/oauth/authorize?client_id=app1", c["redirectUri"])

	up2 := siThemed()
	up2.reply("POST /auth/password-setup/request", 200, `{"message":"ok"}`)
	siPost("/ui/auth/login/setup", "email", "a@b.c", "redirect", "https://evil.example/").do(signInRouter(up2, false))
	assert.NotContains(t, up2.callsTo("POST /auth/password-setup/request")[0].body, "redirectUri", "only an OAuth round trip is forwarded")
}

func TestCallback(t *testing.T) {
	t.Run("a session continues to the validated destination", func(t *testing.T) {
		rec := siReq{target: "/ui/auth/callback?redirect=/ui/roles"}.do(signInRouter(siThemed(), true))
		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Equal(t, "/ui/roles", rec.Header().Get("Location"))
		rec = siReq{target: "/ui/auth/callback?redirect=https://evil.example/ui/"}.do(signInRouter(siThemed(), true))
		assert.Equal(t, "/ui/", rec.Header().Get("Location"))
	})
	t.Run("no session says the sign-in did not complete", func(t *testing.T) {
		rec := siReq{target: "/ui/auth/callback"}.do(signInRouter(siThemed(), false))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "did not complete")
		assert.Contains(t, rec.Body.String(), `href="/ui/auth/login"`)
	})
	t.Run("a provider error is shown escaped", func(t *testing.T) {
		rec := siReq{target: "/ui/auth/callback?error=access_denied&error_description=%3Cscript%3Ealert(1)%3C/script%3E"}.do(signInRouter(siThemed(), true))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "an error wins over a session")
		assert.NotContains(t, rec.Body.String(), "<script>alert")
		assert.Contains(t, rec.Body.String(), "&lt;script&gt;")
	})
}

func TestLogout(t *testing.T) {
	up := siThemed()
	up.reply("POST /auth/logout", 204, ``, "Set-Cookie", "fc_session=; Path=/; Max-Age=0; HttpOnly")
	h := signInRouter(up, false)

	page := siReq{target: "/ui/auth/logout"}.do(h)
	assert.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Log out of the Identity Server?")
	assert.Empty(t, up.callsTo("POST /auth/logout"), "a GET never logs out")

	rec := siPost("/ui/auth/logout").do(h)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/auth/login", rec.Header().Get("Location"))
	assert.Equal(t, []string{"fc_session=; Path=/; Max-Age=0; HttpOnly"}, rec.Header().Values("Set-Cookie"), "the endpoint's clearing cookie")
}

func TestPostsFromAnotherSiteAreRefused(t *testing.T) {
	up := siThemed()
	up.reply("POST /auth/login", 200, `{"status":"ok"}`, "Set-Cookie", sessionSetCookie)
	up.reply("POST /auth/logout", 204, ``)
	h := signInRouter(up, false)
	for _, target := range []string{"/ui/auth/login/email", "/ui/auth/login/password", "/ui/auth/login/2fa", "/ui/auth/login/setup", "/ui/auth/logout",
		"/ui/auth/forgot-password", "/ui/auth/reset-password", "/ui/auth/set-password",
		"/ui/auth/portal-login/email", "/ui/auth/portal-login/password", "/ui/auth/portal-login/reset"} {
		for name, headers := range map[string]map[string]string{
			"cross-site":          {"Sec-Fetch-Site": "cross-site", "Origin": "https://attacker.test"},
			"foreign origin only": {"Origin": "https://attacker.test"},
			"no provenance":       {},
		} {
			r := httptest.NewRequest(http.MethodPost, target, strings.NewReader("email=a%40b.c&password=pw"))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for k, v := range headers {
				r.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s", target, name)
			assert.Empty(t, rec.Header().Values("Set-Cookie"), "%s %s", target, name)
		}
	}
	assert.Empty(t, up.callsTo("POST /auth/login"), "a refused post never reaches the endpoint")
	assert.Empty(t, up.callsTo("POST /auth/logout"))
}
