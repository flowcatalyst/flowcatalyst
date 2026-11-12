package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/oauthapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
)

// registry is a stand-in client registry: client_id -> registered redirect
// URIs, with the matching rule /oauth/authorize applies.
var registry = map[string][]string{
	"app1":  {"https://app.example.com/cb"},
	"app2":  {"http://localhost:8090/auth/callback", "https://*.tenants.example.com/*"},
	"other": {"https://other.example.org/cb"},
}

var testDestination FormDestination = func(_ context.Context, clientID, redirectURI string) (string, bool) {
	if !oauthapi.MatchRedirectURI(redirectURI, registry[clientID]) {
		return "", false
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

func oauthCtx(clientID, redirectURI string) []string {
	return []string{"oauth", "true", "response_type", "code", "client_id", clientID, "redirect_uri", redirectURI, "state", "s1"}
}

func formAction(rec *httptest.ResponseRecorder) string {
	for d := range strings.SplitSeq(rec.Header().Get("Content-Security-Policy"), ";") {
		if d = strings.TrimSpace(d); strings.HasPrefix(d, "form-action") {
			return d
		}
	}
	return ""
}

func destRouter(up http.Handler, dest FormDestination) http.Handler {
	r := chi.NewRouter()
	csrf, _ := guard.CSRF(nil, guard.PlainDenier)
	r.Route("/ui", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(guard.SecurityHeaders, csrf)
			(&SignIn{Upstream: up, Authenticate: func(n http.Handler) http.Handler { return n }, FormDestination: dest}).Mount(r)
		})
	})
	return r
}

func TestSignInFormActionNamesTheRegisteredClientOrigin(t *testing.T) {
	cases := []struct {
		name   string
		client string
		uri    string
		want   string
	}{
		{"registered, exact", "app1", "https://app.example.com/cb", "form-action 'self' https://app.example.com"},
		{"registered, loopback with port", "app2", "http://localhost:8090/auth/callback", "form-action 'self' http://localhost:8090"},
		{"registered by wildcard", "app2", "https://a.tenants.example.com/x", "form-action 'self' https://a.tenants.example.com"},
		{"unregistered uri of a known client", "app1", "https://evil.example/cb", "form-action 'self'"},
		{"unknown client", "nope", "https://app.example.com/cb", "form-action 'self'"},
		{"crafted pair: another client's registered uri", "app1", "https://other.example.org/cb", "form-action 'self'"},
		{"crafted pair: userinfo host confusion", "app1", "https://app.example.com@evil.example/cb", "form-action 'self'"},
	}
	up := siThemed()
	h := destRouter(up, testDestination)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := url.Values{}
			ctx := oauthCtx(c.client, c.uri)
			for i := 0; i+1 < len(ctx); i += 2 {
				q.Set(ctx[i], ctx[i+1])
			}
			// The email page (GET) and the password step (POST) are both
			// documents whose forms end in the landing.
			rec := siReq{target: "/ui/auth/login?" + q.Encode()}.do(h)
			assert.Equal(t, c.want, formAction(rec), "login page")
			up.reply("POST /auth/check-domain", 200, `{"authMethod":"internal"}`)
			rec = siPost("/ui/auth/login/email", append([]string{"email", "a@b.c"}, ctx...)...).do(h)
			assert.Equal(t, c.want, formAction(rec), "password step")
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "script-src 'self' 'unsafe-eval'", "other directives unchanged")
			assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
		})
	}
	t.Run("a sign-in without an OAuth round trip is unchanged", func(t *testing.T) {
		for _, target := range []string{"/ui/auth/login", "/ui/auth/login?redirect=%2Fui%2Froles", "/ui/auth/login?client_id=app1&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcb"} {
			rec := siReq{target: target}.do(h)
			assert.Equal(t, "form-action 'self'", formAction(rec), target)
		}
	})
	t.Run("no resolver: today's behaviour", func(t *testing.T) {
		ctx := oauthCtx("app1", "https://app.example.com/cb")
		rec := siPost("/ui/auth/login/email", append([]string{"email", "a@b.c"}, ctx...)...).do(destRouter(up, nil))
		assert.Equal(t, "form-action 'self'", formAction(rec))
	})
	t.Run("other pages keep form-action 'self'", func(t *testing.T) {
		for _, target := range []string{"/ui/auth/forgot-password?" + url.Values{"oauth": {"true"}, "client_id": {"app1"}, "redirect_uri": {"https://app.example.com/cb"}}.Encode(), "/ui/auth/logout"} {
			assert.Equal(t, "form-action 'self'", formAction(siReq{target: target}.do(h)), target)
		}
	})
}

func TestSignInLeavesWith303ExactlyWhenTheDocumentAllowedTheOrigin(t *testing.T) {
	up := siThemed()
	up.reply("POST /auth/login", 200, `{"status":"ok"}`, "Set-Cookie", sessionSetCookie)
	h := destRouter(up, testDestination)

	t.Run("registered redirect_uri: a direct 303 to /oauth/authorize, no page", func(t *testing.T) {
		rec := siPost("/ui/auth/login/password", append([]string{"email", "a@b.c", "password", "pw"}, oauthCtx("app1", "https://app.example.com/cb")...)...).do(h)
		require.Equal(t, http.StatusSeeOther, rec.Code)
		loc, err := url.Parse(rec.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "/oauth/authorize", loc.Path)
		assert.Equal(t, "https://app.example.com/cb", loc.Query().Get("redirect_uri"))
		assert.Equal(t, "s1", loc.Query().Get("state"))
		assert.NotContains(t, rec.Body.String(), "refresh")
		assert.Equal(t, "form-action 'self' https://app.example.com", formAction(rec), "the answer carries the same computation")
		assert.NotEmpty(t, rec.Header().Get("Set-Cookie"))
	})
	for name, ctx := range map[string][]string{
		"unregistered":        oauthCtx("app1", "https://evil.example/cb"),
		"crafted pair":        oauthCtx("app1", "https://other.example.org/cb"),
		"unknown client":      oauthCtx("zzz", "https://app.example.com/cb"),
		"interaction landing": {"interaction", "uid_1"},
	} {
		t.Run(name+": the silent page", func(t *testing.T) {
			rec := siPost("/ui/auth/login/password", append([]string{"email", "a@b.c", "password", "pw"}, ctx...)...).do(h)
			leftFor(t, rec)
			assert.Equal(t, "form-action 'self'", formAction(rec))
		})
	}
	t.Run("an SSO domain still hops (the IdP origin is never published)", func(t *testing.T) {
		up.reply("POST /auth/check-domain", 200, `{"authMethod":"external","loginUrl":"/auth/oidc/login?domain=acme.example"}`)
		rec := siPost("/ui/auth/login/email", append([]string{"email", "a@acme.example"}, oauthCtx("app1", "https://app.example.com/cb")...)...).do(h)
		target := leftFor(t, rec)
		assert.True(t, strings.HasPrefix(target, "/auth/oidc/login?"), target)
		assert.NotContains(t, rec.Header().Get("Content-Security-Policy"), "idp")
	})
}

// The origin a document allows and the decision at leave time come from the
// same fields through the same function: for every context, leave answers 303
// to /oauth/authorize exactly when the login document's CSP named the
// redirect_uri's origin.
func TestFormActionAndLeaveAgree(t *testing.T) {
	up := siThemed()
	up.reply("POST /auth/login", 200, `{"status":"ok"}`)
	h := destRouter(up, testDestination)
	for _, c := range [][2]string{
		{"app1", "https://app.example.com/cb"},
		{"app2", "http://localhost:8090/auth/callback"},
		{"app2", "https://x.tenants.example.com/p"},
		{"app1", "https://evil.example/cb"},
		{"app1", "https://other.example.org/cb"},
		{"zzz", "https://app.example.com/cb"},
		{"app1", "javascript:alert(1)"},
		{"app1", "https://app.example.com:8443/cb"},
	} {
		ctx := oauthCtx(c[0], c[1])
		doc := siPost("/ui/auth/login/email", append([]string{"email", "a@b.c"}, ctx...)...).do(destRouter(upWithDomain(up), testDestination))
		leave := siPost("/ui/auth/login/password", append([]string{"email", "a@b.c", "password", "pw"}, ctx...)...).do(h)
		allowed := strings.Contains(formAction(doc), "://")
		assert.Equal(t, allowed, leave.Code == http.StatusSeeOther, "%v: document %q, leave %d", c, formAction(doc), leave.Code)
	}
}

func upWithDomain(up *fakeUpstream) *fakeUpstream {
	up.reply("POST /auth/check-domain", 200, `{"authMethod":"internal"}`)
	return up
}

func TestLandingOriginRefusesWhatIsNotACleanOrigin(t *testing.T) {
	landing := "/oauth/authorize?client_id=a&redirect_uri=x"
	for _, bad := range []string{"https://a.example; script-src *", "https://a.example, evil", "https://a b", "ftp://a.example", "https://", "https://a.example/path", "https://u@a.example", "*", "'self'"} {
		fd := FormDestination(func(context.Context, string, string) (string, bool) { return bad, true })
		assert.Empty(t, fd.landingOrigin(context.Background(), landing), bad)
	}
	for _, good := range []string{"https://a.example", "http://localhost:8090", "https://[::1]:8443"} {
		fd := FormDestination(func(context.Context, string, string) (string, bool) { return good, true })
		assert.Equal(t, good, fd.landingOrigin(context.Background(), landing))
	}
	var nilFD FormDestination
	assert.Empty(t, nilFD.landingOrigin(context.Background(), landing))
	fd := FormDestination(func(context.Context, string, string) (string, bool) { return "https://a.example", false })
	assert.Empty(t, fd.landingOrigin(context.Background(), landing), "an unregistered pair adds nothing")
	assert.Empty(t, fd.landingOrigin(context.Background(), "/ui/roles"))
}

func TestSilentHop(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", http.NoBody)
	leaveTo(rec, req, "https://elsewhere.example/p?a=1&b=%22", "", nil, nil)
	body := rec.Body.String()
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, body, "Continuing")
	assert.Contains(t, body, `http-equiv="refresh"`)
	assert.Contains(t, body, "background:linear-gradient", "the sign-in background")
	assert.Contains(t, body, "animation:fc-late 0s linear 3s forwards", "the link appears after about three seconds")
	assert.Contains(t, body, `aria-label="Continue to the next page"`)
	assert.NotContains(t, body, "<script")
	assert.NotContains(t, body, `content="0;url=https://elsewhere.example/p?a=1&b="`, "quotes in the target are encoded")
}

func TestSecondFactorFormActionAndLeave(t *testing.T) {
	h := newMFAHarness()
	h.build()
	h.page.FormDestination = testDestination
	landing := "/oauth/authorize?client_id=app1&redirect_uri=" + url.QueryEscape("https://app.example.com/cb") + "&response_type=code&state=s1"
	crafted := "/oauth/authorize?client_id=app1&redirect_uri=" + url.QueryEscape("https://other.example.org/cb") + "&response_type=code&state=s1"
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}

	rec := h.serve(t, nil, req{target: "/ui/auth/2fa?next=" + url.QueryEscape(landing), cookie: pendingCk("prn_ann")})
	assert.Equal(t, "form-action 'self' https://app.example.com", formAction(rec), "the challenge page")
	rec = h.serve(t, nil, req{target: "/ui/auth/2fa?next=" + url.QueryEscape(crafted), cookie: pendingCk("prn_ann")})
	assert.Equal(t, "form-action 'self'", formAction(rec), "crafted pair")
	rec = h.serve(t, nil, req{target: "/ui/auth/2fa/enroll?next=" + url.QueryEscape(landing), cookie: enrollCk("prn_ann")})
	assert.Equal(t, "form-action 'self' https://app.example.com", formAction(rec), "the enrol page")
	rec = h.serve(t, nil, req{target: "/ui/auth/2fa?next=%2Fui%2Froles", cookie: pendingCk("prn_ann")})
	assert.Equal(t, "form-action 'self'", formAction(rec), "a /ui landing")

	h2 := newMFAHarness()
	h2.mfa.confirmed["prn_ann"] = nil
	h2.build()
	h2.page.FormDestination = testDestination
	for next, wantStatus := range map[string]int{landing: http.StatusSeeOther, crafted: http.StatusOK} {
		h2.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
		rec = h2.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {"123456"}, "next": {next}}, pendingCk("prn_ann")))
		assert.Equal(t, wantStatus, rec.Code, next)
		if wantStatus == http.StatusSeeOther {
			assert.Equal(t, authLanding(next), rec.Header().Get("Location"))
		} else {
			assert.Contains(t, rec.Body.String(), `http-equiv="refresh"`)
		}
	}
}
