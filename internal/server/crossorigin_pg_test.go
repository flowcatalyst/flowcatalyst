//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/tsid"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// These tests drive the whole platform router (WirePlatform on embedded
// Postgres) with the requests a browser sends, to pin the cookie-session
// cross-origin rule (docs/platform-bug-fixes.md, F16):
//
//   - an unsafe request authenticated by the fc_session cookie must come from
//     this origin (Sec-Fetch-Site same-origin/none, or else an Origin naming
//     this host or the public base URL); with neither header it is refused;
//   - its body, when it has one, must be JSON;
//   - a request authenticated by a bearer token is not subject to either rule,
//     and neither are the OAuth endpoints (outside the authenticated group).

const xoPublicBase = "https://platform.example.com"

// xoPerms is what the seeded anchor user's role grants: client create and
// activate (huma routes with and without a body) and role create (the chi BFF
// permission route).
var xoPerms = []string{
	"platform:admin:client:view",
	"platform:admin:client:create",
	"platform:admin:client:activate",
	"platform:iam:role:view",
	"platform:iam:role:create",
}

type xoHarness struct {
	t      *testing.T
	h      http.Handler
	cookie string
	bearer string
}

func newXOHarness(t *testing.T) *xoHarness {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)

	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	cfg := EnvCfg{
		JWTIssuer:              xoPublicBase,
		JWTSigningKeyPath:      keyPath,
		SessionTTLSecs:         3600,
		AccessTokenTTLSecs:     3600,
		RefreshTokenTTLSecs:    3600,
		FunctionsArtifactStore: "file://" + t.TempDir(),
	}
	r := chi.NewRouter()
	_, err = WirePlatform(r, pool, cfg, dispatch.Settings{})
	require.NoError(t, err)

	// A second service set over the same signing key mints the credentials
	// the wired router accepts.
	repos := buildRepos(pool)
	svcs, err := buildServices(cfg, pool, repos)
	require.NoError(t, err)

	pid := ids.NewPrincipalID()
	roleID := tsid.Generate(tsid.Role)
	roleName := "xotest:admin-" + strings.ToLower(string(pid))
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email)
		 VALUES ($1, 'USER', 'ANCHOR', 'XO Admin', TRUE, $2)`,
		pid, strings.ToLower(string(pid))+"@xo.example.com")
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_roles (id, name, display_name) VALUES ($1, $2, 'XO Admin')`, roleID, roleName)
	require.NoError(t, err)
	for _, p := range xoPerms {
		_, err = pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission) VALUES ($1, $2)`, roleID, p)
		require.NoError(t, err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, $2, 'TEST')`,
		pid, roleName)
	require.NoError(t, err)

	cookie, err := svcs.authProvider.MintSessionToken(ctx, string(pid), 0)
	require.NoError(t, err)
	p, err := repos.principalRepo.FindByID(ctx, pid)
	require.NoError(t, err)
	bearer, err := svcs.authSvc.GenerateAccessTokenWithScope(p, xoPerms)
	require.NoError(t, err)

	return &xoHarness{t: t, h: r, cookie: cookie, bearer: bearer}
}

// xoReq describes a request as a browser (or SDK) would send it.
type xoReq struct {
	method, path string
	host         string // default platform.example.com
	contentType  string
	body         string
	cookie       bool
	bearer       bool
	secFetchSite string
	origin       string
}

type xoResp struct {
	status int
	code   string // the envelope's "error"
	body   string
}

func (x *xoHarness) do(q xoReq) xoResp {
	x.t.Helper()
	var body io.Reader
	if q.body != "" {
		body = strings.NewReader(q.body)
	}
	r := httptest.NewRequest(q.method, "http://platform.example.com"+q.path, body)
	if q.host != "" {
		r.Host = q.host
	}
	if q.contentType != "" {
		r.Header.Set("Content-Type", q.contentType)
	}
	if q.cookie {
		r.AddCookie(&http.Cookie{Name: platformmw.SessionCookieName, Value: x.cookie})
	}
	if q.bearer {
		r.Header.Set("Authorization", "Bearer "+x.bearer)
	}
	if q.secFetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", q.secFetchSite)
	}
	if q.origin != "" {
		r.Header.Set("Origin", q.origin)
	}
	w := httptest.NewRecorder()
	x.h.ServeHTTP(w, r)
	var env struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return xoResp{status: w.Code, code: env.Error, body: w.Body.String()}
}

func xoClientBody() string {
	id := strings.ToLower(string(ids.NewClientID()))
	return `{"name":"XO ` + id + `","identifier":"xo-` + strings.ReplaceAll(id, "_", "-") + `"}`
}

func xoPermBody() string {
	return `{"application":"xotest","context":"ctx","aggregate":"agg","action":"a` +
		strings.ToLower(strings.TrimPrefix(string(ids.NewClientID()), "clt_")) + `"}`
}

// xoSuspendedClient inserts a suspended client for the body-less activate route.
func xoSuspendedClient(t *testing.T) string {
	t.Helper()
	id := string(ids.NewClientID())
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO tnt_clients (id, name, identifier, status) VALUES ($1, 'XO suspended', $2, 'SUSPENDED')`,
		id, "xo-s-"+strings.ToLower(strings.ReplaceAll(id, "_", "-")))
	require.NoError(t, err)
	return id
}

// Cross-site and same-site-sibling requests carrying the session cookie: what
// a page on another origin can make a browser send. SameSite=Lax keeps the
// cookie off cross-SITE POSTs, but a sibling subdomain of the same registrable
// domain is same-site, so the cookie rides along (Sec-Fetch-Site: same-site).
func TestCookieSessionCrossOriginUnsafeRequestsRefused(t *testing.T) {
	x := newXOHarness(t)
	sibling := "https://evil.example.com"

	cases := []struct {
		name string
		q    xoReq
	}{
		{"chi BFF route, text/plain JSON (a no-preflight simple request)", xoReq{
			method: http.MethodPost, path: "/bff/roles/permissions", contentType: "text/plain;charset=UTF-8",
			body: xoPermBody(), cookie: true, secFetchSite: "same-site", origin: sibling,
		}},
		{"chi BFF route, application/json from a sibling", xoReq{
			method: http.MethodPost, path: "/bff/roles/permissions", contentType: "application/json",
			body: xoPermBody(), cookie: true, secFetchSite: "same-site", origin: sibling,
		}},
		{"huma route without a body", xoReq{
			method: http.MethodPost, path: "/api/clients/" + xoSuspendedClient(t) + "/activate",
			cookie: true, secFetchSite: "same-site", origin: sibling,
		}},
		{"huma route, JSON body without Content-Type (a Blob body)", xoReq{
			method: http.MethodPost, path: "/api/clients", body: xoClientBody(),
			cookie: true, secFetchSite: "same-site", origin: sibling,
		}},
		{"huma route, cross-site", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			cookie: true, secFetchSite: "cross-site", origin: "https://attacker.test",
		}},
		{"browser without Sec-Fetch-Site, foreign Origin", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			cookie: true, origin: sibling,
		}},
		{"neither Sec-Fetch-Site nor Origin", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			cookie: true,
		}},
		{"DELETE from a sibling", xoReq{
			method: http.MethodDelete, path: "/api/clients/" + string(ids.NewClientID()),
			cookie: true, secFetchSite: "same-site", origin: sibling,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := x.do(tc.q)
			require.Equal(t, http.StatusForbidden, got.status, got.body)
			require.Equal(t, "CROSS_ORIGIN_REQUEST", got.code, got.body)
		})
	}
}

func TestCookieSessionSameOriginRequestsAllowed(t *testing.T) {
	x := newXOHarness(t)

	cases := []struct {
		name string
		q    xoReq
		want int
	}{
		{"same-origin JSON create (the SPA)", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			cookie: true, secFetchSite: "same-origin", origin: xoPublicBase,
		}, http.StatusCreated},
		{"same-origin chi BFF route", xoReq{
			method: http.MethodPost, path: "/bff/roles/permissions", contentType: "application/json; charset=utf-8",
			body: xoPermBody(), cookie: true, secFetchSite: "same-origin", origin: xoPublicBase,
		}, http.StatusCreated},
		{"same-origin body-less action", xoReq{
			method: http.MethodPost, path: "/api/clients/" + xoSuspendedClient(t) + "/activate",
			cookie: true, secFetchSite: "same-origin", origin: xoPublicBase,
		}, http.StatusOK},
		{"user-initiated (Sec-Fetch-Site none)", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			cookie: true, secFetchSite: "none",
		}, http.StatusCreated},
		{"browser without Sec-Fetch-Site, Origin matches Host", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			cookie: true, origin: xoPublicBase,
		}, http.StatusCreated},
		{"proxy rewrote Host, Origin is the public base URL", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			host: "platform-internal:8080", cookie: true, origin: xoPublicBase,
		}, http.StatusCreated},
		{"safe method from a sibling (GET is never refused)", xoReq{
			method: http.MethodGet, path: "/api/clients", cookie: true, secFetchSite: "same-site", origin: "https://evil.example.com",
		}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := x.do(tc.q)
			require.Equal(t, tc.want, got.status, got.body)
		})
	}
}

// A cookie-authenticated unsafe request from this origin must carry JSON:
// text/plain, form and multipart bodies are the ones a page on another origin
// can send without a CORS preflight.
func TestCookieSessionNonJSONBodyRefused(t *testing.T) {
	x := newXOHarness(t)
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x", ""} {
		for _, path := range []string{"/api/clients", "/bff/roles/permissions"} {
			t.Run(path+" "+ct, func(t *testing.T) {
				body := xoClientBody()
				if strings.HasPrefix(path, "/bff") {
					body = xoPermBody()
				}
				got := x.do(xoReq{
					method: http.MethodPost, path: path, contentType: ct, body: body,
					cookie: true, secFetchSite: "same-origin", origin: xoPublicBase,
				})
				require.Equal(t, http.StatusUnsupportedMediaType, got.status, got.body)
				require.Equal(t, "UNSUPPORTED_MEDIA_TYPE", got.code, got.body)
			})
		}
	}
}

// Bearer callers (SDKs, service accounts, the router) send no cookie and no
// browser headers, or arbitrary ones; neither rule applies to them.
func TestBearerRequestsExemptFromCookieRules(t *testing.T) {
	x := newXOHarness(t)

	cases := []struct {
		name string
		q    xoReq
		want int
	}{
		{"bearer, foreign Origin and Sec-Fetch-Site cross-site", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			bearer: true, secFetchSite: "cross-site", origin: "https://attacker.test",
		}, http.StatusCreated},
		{"bearer, no browser headers", xoReq{
			method: http.MethodPost, path: "/bff/roles/permissions", contentType: "application/json", body: xoPermBody(),
			bearer: true,
		}, http.StatusCreated},
		{"bearer and cookie: the bearer authenticates, so the cookie rules do not apply", xoReq{
			method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
			bearer: true, cookie: true, secFetchSite: "cross-site", origin: "https://attacker.test",
		}, http.StatusCreated},
		{"bearer, body-less action without Content-Type", xoReq{
			method: http.MethodPost, path: "/api/clients/" + xoSuspendedClient(t) + "/activate", bearer: true,
		}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := x.do(tc.q)
			require.Equal(t, tc.want, got.status, got.body)
		})
	}
}

// The OAuth endpoints live outside the authenticated group and authenticate
// their own callers: a cross-site token request carrying the cookie is not
// refused by the cookie rule (it fails, or not, on its own grant).
func TestOAuthEndpointsExemptFromCookieRules(t *testing.T) {
	x := newXOHarness(t)
	got := x.do(xoReq{
		method: http.MethodPost, path: "/oauth/token", contentType: "application/x-www-form-urlencoded",
		body: "grant_type=client_credentials&client_id=nope&client_secret=nope", cookie: true,
		secFetchSite: "cross-site", origin: "https://rp.example.org",
	})
	require.NotEqual(t, "CROSS_ORIGIN_REQUEST", got.code, got.body)
	require.NotEqual(t, "UNSUPPORTED_MEDIA_TYPE", got.code, got.body)
	require.NotEqual(t, http.StatusForbidden, got.status, got.body)
}

// A stale or invalid cookie authenticates nothing, so the cookie rules do not
// apply: the request proceeds unauthenticated and the handler refuses it as
// such (UNAUTHENTICATED, which the platform answers with 403).
func TestInvalidCookieIsNotACookieSession(t *testing.T) {
	x := newXOHarness(t)
	x.cookie = "not-a-jwt"
	got := x.do(xoReq{
		method: http.MethodPost, path: "/api/clients", contentType: "application/json", body: xoClientBody(),
		cookie: true, secFetchSite: "cross-site", origin: "https://attacker.test",
	})
	require.Equal(t, "UNAUTHENTICATED", got.code, got.body)
}
