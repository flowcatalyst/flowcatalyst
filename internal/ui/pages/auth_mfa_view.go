package pages

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// mfaCopyExpr copies the recovery codes and says so in the panel.
const mfaCopyExpr = "navigator.clipboard.writeText(document.getElementById('mfa-codes').innerText).then(() => $mfaCopied = 'Recovery codes copied to clipboard')"

// codeAttrs are the attributes of a one-time-code field: the browser offers
// the code from a text message, and a phone shows the numeric keypad.
func codeAttrs(numeric bool) templ.Attributes {
	a := templ.Attributes{"autocomplete": "one-time-code", "autofocus": true, "inputmode": "text"}
	if numeric {
		a["inputmode"] = "numeric"
	}
	return a
}

func codePlaceholder(method string) string {
	if method == methodRecovery {
		return "XXXXX-XXXXX"
	}
	return "123456"
}

// challengeHref is the challenge page for another method.
func challengeHref(next, method string) string {
	return authChallengePath + "?method=" + url.QueryEscape(method) + "&next=" + url.QueryEscape(next)
}

// enrollAction is the form that starts the enrollment of a method after the
// password step.
func enrollAction(method string) string {
	if method == "TOTP" {
		return authEnrollPath + "/totp"
	}
	return authEnrollPath + "/email"
}

// addAction is the form that starts it on the security page.
func addAction(method string) string {
	if method == "TOTP" {
		return authSecurityPath + "/2fa/totp"
	}
	return authSecurityPath + "/2fa/email"
}

func unusedCodes(n int) string {
	if n == 1 {
		return "1 unused code"
	}
	return strconv.Itoa(n) + " unused codes"
}

// renderAuthPage writes a page of the sign-in area (no app shell: the user is
// not signed in yet), with the named assets' scripts, in the login theme as
// the SPA's sign-in pages wear it (these steps are part of its login page).
// Pages carry secrets (a TOTP key, recovery codes): the group's headers
// already say no-store.
func renderAuthPage(w http.ResponseWriter, r *http.Request, status int, title string, th signinTheme, scripts []string, content templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = signinDocument(title, th, false, scripts, authCard(content)).Render(r.Context(), w)
}

// authThemeClient is the branding client a second-factor or passkey page was
// reached with (the sign-in form's hidden client field), "" for the
// platform's own theme.
func authThemeClient(r *http.Request) string {
	return strings.TrimSpace(r.FormValue("client"))
}
