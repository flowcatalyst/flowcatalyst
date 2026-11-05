package pages

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestUserMatchEmailsPage(t *testing.T) {
	h := newUsrHarness()

	// The list links to it.
	mustContain(t, usrBody(t, h.do(t, usrAnchor(usrPermView), req{target: "/ui/users"})), `href="/ui/users/match-emails"`, "Match emails")

	// GET: the form, no results.
	body := usrBody(t, h.do(t, usrAnchor(usrPermView), req{target: "/ui/users/match-emails"}))
	mustContain(t, body, "Email addresses", "Paste emails separated by commas, semicolons or new lines. &#39;Name &lt;email&gt;&#39; works too.", `method="post"`, `action="/ui/users/match-emails"`, "<textarea", ">Match<")
	mustNotContain(t, body, "matched,", "Not found")

	// POST: matches in input order, then the two copyable lists.
	text := "Cara <cara@partner.test>; nobody@example.com\nBOB@acme.test, junk, dan@globex.test, nobody@example.com"
	rec := h.do(t, usrAnchor(usrPermView), req{method: http.MethodPost, target: "/ui/users/match-emails", form: url.Values{"emails": {text}}})
	body = usrBody(t, rec)
	if h.users.matchBody.Text != text {
		t.Errorf("text = %q", h.users.matchBody.Text)
	}
	mustContain(t, body, "3 matched, 1 not found, 1 not an email", text[:4], `href="/ui/users/prn_3"`, `href="/ui/users/prn_2"`, `href="/ui/users/prn_4"`,
		"cara@partner.test", "Not found", "Not an email", "nobody@example.com</textarea>", "junk</textarea>", "readonly")
	if i, j := strings.Index(body, "/ui/users/prn_3"), strings.Index(body, "/ui/users/prn_2"); i < 0 || j < 0 || i > j {
		t.Errorf("matches not in input order: %d %d", i, j)
	}
	// Bob's name is escaped, not injected.
	mustContain(t, body, "Bob &lt;Builder&gt;")
	mustNotContain(t, body, "Bob <Builder>")
}

func TestUserMatchEmailsGateAndErrors(t *testing.T) {
	h := newUsrHarness()
	// Same gate as the list: nothing runs without user:view.
	for _, q := range []req{
		{target: "/ui/users/match-emails"},
		{method: http.MethodPost, target: "/ui/users/match-emails", form: url.Values{"emails": {"a@b.test"}}},
	} {
		if rec := h.do(t, usrAnchor(), q); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s without user:view: %d", q.method, q.target, rec.Code)
		}
	}
	if len(h.users.calls) != 0 {
		t.Errorf("ran %v", h.users.calls)
	}
	// Nothing pasted: the field's error, the page re-rendered with the status.
	rec := h.do(t, usrAnchor(usrPermView), req{method: http.MethodPost, target: "/ui/users/match-emails", form: url.Values{"emails": {" , \n"}}})
	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	mustContain(t, rec.Body.String(), "No email addresses supplied", "<textarea")
}
