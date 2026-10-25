package pages

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForgotPassword(t *testing.T) {
	t.Run("the same page whether or not the account exists", func(t *testing.T) {
		// The endpoint answers one silent success; the page adds nothing.
		up := siThemed()
		up.reply("POST /auth/password-reset/request", 200, `{"message":"If an account exists, a reset email has been sent."}`)
		h := signInRouter(up, false)
		a := siPost("/ui/auth/forgot-password", "email", "known@b.c").do(h)
		b := siPost("/ui/auth/forgot-password", "email", "known@b.c").do(h)
		require.Equal(t, http.StatusOK, a.Code)
		assert.Equal(t, a.Body.String(), b.Body.String())
		assert.Contains(t, a.Body.String(), "If an account exists for known@b.c")
		assert.NotContains(t, a.Body.String(), "has been sent", "the endpoint's text is not echoed")
	})
	t.Run("an OAuth round trip is carried through the reset", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/password-reset/request", 200, `{}`)
		siPost("/ui/auth/forgot-password", "email", "a@b.c", "oauth", "true", "client_id", "app1", "state", "s").do(signInRouter(up, false))
		assert.Equal(t, "/oauth/authorize?client_id=app1&state=s", up.callsTo("POST /auth/password-reset/request")[0].body["redirectUri"])
		up2 := siThemed()
		up2.reply("POST /auth/password-reset/request", 200, `{}`)
		siPost("/ui/auth/forgot-password", "email", "a@b.c", "redirect", "https://evil.example/").do(signInRouter(up2, false))
		assert.NotContains(t, up2.callsTo("POST /auth/password-reset/request")[0].body, "redirectUri")
	})
	t.Run("the form is prefilled from the sign-in page and an empty email is not sent", func(t *testing.T) {
		up := siThemed()
		rec := siReq{target: "/ui/auth/forgot-password?email=a%40b.c&client=acme"}.do(signInRouter(up, false))
		assert.Contains(t, rec.Body.String(), `value="a@b.c"`)
		assert.Contains(t, rec.Body.String(), `name="client" value="acme"`)
		rec = siPost("/ui/auth/forgot-password", "email", " ").do(signInRouter(up, false))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Empty(t, up.callsTo("POST /auth/password-reset/request"))
	})
	t.Run("a throttled request says so", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/password-reset/request", 429, `{"message":"slow down"}`)
		rec := siPost("/ui/auth/forgot-password", "email", "a@b.c").do(signInRouter(up, false))
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		assert.Contains(t, rec.Body.String(), "slow down")
	})
}

func TestResetPage(t *testing.T) {
	t.Run("a valid token shows the form with the token and the strength meter", func(t *testing.T) {
		up := siThemed()
		up.reply("GET /auth/password-reset/validate", 200, `{"valid":true,"reason":null,"requiresFactor":false}`)
		rec := siReq{target: "/ui/auth/reset-password?token=TOK%2B1"}.do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "Reset your password")
		assert.Contains(t, body, `action="/ui/auth/reset-password"`)
		assert.Contains(t, body, `name="token" value="TOK+1"`)
		assert.Contains(t, body, `autocomplete="new-password"`)
		assert.Contains(t, body, "fc-password-meter")
		assert.NotContains(t, body, `name="factorCode"`)
		assert.Equal(t, "token=TOK%2B1", up.callsTo("GET /auth/password-reset/validate")[0].query)
	})
	t.Run("the invite framing and a factor gate", func(t *testing.T) {
		up := siThemed()
		up.reply("GET /auth/password-reset/validate", 200, `{"valid":true,"requiresFactor":true}`)
		body := siReq{target: "/ui/auth/set-password?token=T"}.do(signInRouter(up, false)).Body.String()
		assert.Contains(t, body, "Set your password")
		assert.Contains(t, body, `action="/ui/auth/set-password"`)
		assert.Contains(t, body, `name="factorCode"`)
		assert.Contains(t, body, `name="requiresFactor" value="1"`)
	})
	t.Run("an expired, used or missing token shows why and never the token", func(t *testing.T) {
		up := siThemed()
		up.reply("GET /auth/password-reset/validate", 200, `{"valid":false,"reason":"expired"}`)
		body := siReq{target: "/ui/auth/reset-password?token=SECRETTOKEN"}.do(signInRouter(up, false)).Body.String()
		assert.Contains(t, body, "has expired. Reset links are valid for 15 minutes")
		assert.Contains(t, body, "Request a new reset link")
		assert.NotContains(t, body, "SECRETTOKEN")

		up2 := siThemed()
		up2.reply("GET /auth/password-reset/validate", 200, `{"valid":false,"reason":"not_found"}`)
		body = siReq{target: "/ui/auth/set-password?token=X"}.do(signInRouter(up2, false)).Body.String()
		assert.Contains(t, body, "invalid or has already been used")
		assert.NotContains(t, body, "Request a new reset link", "an invite cannot be requested again by the invitee")

		up3 := siThemed()
		body = siReq{target: "/ui/auth/reset-password"}.do(signInRouter(up3, false)).Body.String()
		assert.Contains(t, body, "Link invalid or expired")
		assert.Empty(t, up3.callsTo("GET /auth/password-reset/validate"), "no token, no lookup")
	})
	t.Run("a portal token drops the platform brand", func(t *testing.T) {
		up := siThemed()
		up.reply("GET /auth/password-reset/validate", 200, `{"valid":true,"portal":true}`)
		body := siReq{target: "/ui/auth/reset-password?token=T"}.do(signInRouter(up, false)).Body.String()
		assert.Contains(t, body, ">Portal Login<")
		assert.NotContains(t, body, "Secure access to your FlowCatalyst platform")
		assert.Contains(t, body, `name="portal" value="1"`)
	})
}

func TestResetConfirm(t *testing.T) {
	confirm := func(extra ...string) []string {
		return append([]string{"token", "TOK", "password", "Correct-Horse-9!", "confirmPassword", "Correct-Horse-9!"}, extra...)
	}
	t.Run("two different entries never reach the endpoint", func(t *testing.T) {
		up := siThemed()
		rec := siPost("/ui/auth/reset-password", "token", "TOK", "password", "Correct-Horse-9!", "confirmPassword", "other").do(signInRouter(up, false))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Contains(t, rec.Body.String(), "Passwords do not match.")
		assert.NotContains(t, rec.Body.String(), "Correct-Horse-9!", "the password is not echoed")
		assert.Contains(t, rec.Body.String(), `name="token" value="TOK"`, "the form stays usable")
		assert.Empty(t, up.callsTo("POST /auth/password-reset/confirm"))
	})
	t.Run("a missing factor code is asked for", func(t *testing.T) {
		up := siThemed()
		rec := siPost("/ui/auth/reset-password", confirm("requiresFactor", "1")...).do(signInRouter(up, false))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Contains(t, rec.Body.String(), "Enter the code from your authenticator app.")
		assert.Empty(t, up.callsTo("POST /auth/password-reset/confirm"))
	})
	t.Run("the endpoint's password policy message is shown", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/password-reset/confirm", 400, `{"code":"PASSWORD_POLICY","message":"Password is too common."}`)
		rec := siPost("/ui/auth/reset-password", confirm()...).do(signInRouter(up, false))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Contains(t, rec.Body.String(), "Password is too common.")
		assert.NotContains(t, rec.Body.String(), "Correct-Horse-9!")
		body := up.callsTo("POST /auth/password-reset/confirm")[0].body
		assert.Equal(t, "TOK", body["token"])
		assert.Equal(t, "Correct-Horse-9!", body["password"])
		assert.NotContains(t, body, "factorCode")
	})
	t.Run("done: back to sign-in with the banner", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/password-reset/confirm", 200, `{"status":"ok","message":"done"}`)
		rec := siPost("/ui/auth/reset-password", confirm("client", "acme")...).do(signInRouter(up, false))
		require.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Equal(t, "/ui/auth/login?client=acme&reset=success", rec.Header().Get("Location"))
		login := siReq{target: rec.Header().Get("Location")}.do(signInRouter(siThemed(), false))
		assert.Contains(t, login.Body.String(), "Your password has been reset.")
	})
	t.Run("an invite that signed the user in goes straight to the UI, cookie passed on", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/password-reset/confirm", 200, `{"status":"ok","sessionEstablished":true}`, "Set-Cookie", sessionSetCookie)
		rec := siPost("/ui/auth/set-password", confirm()...).do(signInRouter(up, false))
		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Equal(t, "/ui/", rec.Header().Get("Location"))
		assert.Equal(t, []string{sessionSetCookie}, rec.Header().Values("Set-Cookie"))
	})
	t.Run("the redirect the invite carried is followed; a script URL is not", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/password-reset/confirm", 200, `{"status":"ok","redirectUri":"https://app.example.com/cb?x=1"}`)
		rec := siPost("/ui/auth/set-password", confirm()...).do(signInRouter(up, false))
		assert.Equal(t, "https://app.example.com/cb?x=1", leftFor(t, rec))

		up2 := siThemed()
		up2.reply("POST /auth/password-reset/confirm", 200, `{"status":"ok","redirectUri":"javascript:alert(1)"}`)
		rec = siPost("/ui/auth/set-password", confirm()...).do(signInRouter(up2, false))
		assert.Equal(t, "/ui/auth/login?reset=success", rec.Header().Get("Location"))
	})
	t.Run("a domain that compels 2FA is handed to the current sign-in, token unrendered", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/password-reset/confirm", 200, `{"status":"enrollment_required","enrollToken":"ENROLLTOK"}`)
		rec := siPost("/ui/auth/reset-password", confirm()...).do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `href="/auth/login"`)
		assert.NotContains(t, rec.Body.String(), "ENROLLTOK")
	})
	t.Run("a portal identity with no redirect stays off the platform login", func(t *testing.T) {
		up := siThemed()
		up.reply("POST /auth/password-reset/confirm", 200, `{"status":"ok","portal":true}`)
		rec := siPost("/ui/auth/set-password", confirm("portal", "1")...).do(signInRouter(up, false))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Your password is set.")
		assert.Empty(t, rec.Header().Get("Location"))
	})
}
