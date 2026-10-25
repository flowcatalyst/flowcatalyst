package pages

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortalLogin(t *testing.T) {
	expired := `{"code":"FLOW_EXPIRED","message":"The login flow has expired"}`
	t.Run("no flow is the expired page and shows no platform brand", func(t *testing.T) {
		up := siThemed()
		rec := siReq{target: "/ui/auth/portal-login"}.do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "Sign-in link expired")
		assert.Contains(t, body, ">Portal Login<")
		assert.NotContains(t, body, "FlowCatalyst")
		assert.NotContains(t, body, "Platform Administration")
	})
	t.Run("the email step carries the flow", func(t *testing.T) {
		body := siReq{target: "/ui/auth/portal-login?flow=FLOW1"}.do(signInRouter(siThemed(), false)).Body.String()
		assert.Contains(t, body, `action="/ui/auth/portal-login/email"`)
		assert.Contains(t, body, `name="flow" value="FLOW1"`)
	})
	t.Run("a domain owned by an IdP goes to the portal bridge", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /portal/auth/check-domain", 200, `{"method":"SSO","redirectUrl":"/portal/auth/oidc/login?flow=FLOW1&provider_id=idp_1"}`)
		rec := siPost("/ui/auth/portal-login/email", "flow", "FLOW1", "email", "a@acme.example").do(signInRouter(up, false))
		assert.Equal(t, "/portal/auth/oidc/login?flow=FLOW1&provider_id=idp_1", leftFor(t, rec))
		c := up.callsTo("POST /portal/auth/check-domain")[0].body
		assert.Equal(t, "FLOW1", c["flowId"])
		assert.Equal(t, "a@acme.example", c["email"])

		up2 := siThemed()
		up2.reply("POST /portal/auth/check-domain", 200, `{"method":"SSO","redirectUrl":"https://evil.example/x"}`)
		rec = siPost("/ui/auth/portal-login/email", "flow", "FLOW1", "email", "a@acme.example").do(signInRouter(up2, false))
		assert.NotEqual(t, "https://evil.example/x", rec.Header().Get("Location"), "only the bridge's own path is followed")
		assert.NotContains(t, rec.Body.String(), "evil.example")
	})
	t.Run("a password domain gets the password step", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /portal/auth/check-domain", 200, `{"method":"PASSWORD"}`)
		body := siPost("/ui/auth/portal-login/email", "flow", "FLOW1", "email", "a@b.c").do(signInRouter(up, false)).Body.String()
		assert.Contains(t, body, `action="/ui/auth/portal-login/password"`)
		assert.Contains(t, body, "Forgot password?")
	})
	t.Run("a flow that ran out is the expired page at any step", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /portal/auth/check-domain", 400, expired)
		up.reply("POST /portal/auth/login", 400, expired)
		h := signInRouter(up, false)
		assert.Equal(t, http.StatusGone, siPost("/ui/auth/portal-login/email", "flow", "F", "email", "a@b.c").do(h).Code)
		assert.Equal(t, http.StatusGone, siPost("/ui/auth/portal-login/password", "flow", "F", "email", "a@b.c", "password", "pw").do(h).Code)
		assert.Contains(t, siPost("/ui/auth/portal-login/password", "flow", "F", "email", "a@b.c", "password", "pw").do(h).Body.String(), "Sign-in link expired")
	})
	t.Run("sign-in redirects to the portal app and sets no cookie", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /portal/auth/login", 200, `{"redirectUrl":"https://portal.example.com/cb?code=C&state=S"}`)
		rec := siPost("/ui/auth/portal-login/password", "flow", "FLOW1", "email", "a@b.c", "password", "pw").do(signInRouter(up, false))
		assert.Equal(t, "https://portal.example.com/cb?code=C&state=S", leftFor(t, rec))
		assert.Empty(t, rec.Header().Values("Set-Cookie"))
		assert.Equal(t, "pw", up.callsTo("POST /portal/auth/login")[0].body["password"])
	})
	t.Run("a refused sign-in is uniform and never echoes the password", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /portal/auth/login", 401, `{"code":"INVALID_CREDENTIALS","message":"Invalid credentials"}`)
		h := signInRouter(up, false)
		a := siPost("/ui/auth/portal-login/password", "flow", "F", "email", "known@b.c", "password", "pw1").do(h)
		b := siPost("/ui/auth/portal-login/password", "flow", "F", "email", "known@b.c", "password", "pw2").do(h)
		assert.Equal(t, http.StatusUnauthorized, a.Code)
		assert.Equal(t, a.Body.String(), b.Body.String())
		assert.Contains(t, a.Body.String(), "Invalid credentials")
		assert.NotContains(t, a.Body.String(), "pw1")
	})
	t.Run("the portal's rate limit is shown with Retry-After", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /portal/auth/login", 429, `{"message":"too many attempts"}`, "Retry-After", "20")
		rec := siPost("/ui/auth/portal-login/password", "flow", "F", "email", "a@b.c", "password", "pw").do(signInRouter(up, false))
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		assert.Equal(t, "20", rec.Header().Get("Retry-After"))
	})
	t.Run("forgot password", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /portal/auth/password-reset", 200, `{"message":"If an account exists, a reset email has been sent."}`)
		rec := siPost("/ui/auth/portal-login/reset", "flow", "F", "email", "a@b.c").do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "If an account exists, a reset email has been sent.")
		assert.NotContains(t, rec.Body.String(), "Forgot password?")
		assert.Equal(t, "F", up.callsTo("POST /portal/auth/password-reset")[0].body["flowId"])
	})
}
