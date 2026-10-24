package pages

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

type fakeLoginStore struct {
	attempts []loginattempt.LoginAttempt
	params   []loginattempt.ListParams
}

func (f *fakeLoginStore) FindPage(_ context.Context, p loginattempt.ListParams) ([]loginattempt.LoginAttempt, error) {
	f.params = append(f.params, p)
	var out []loginattempt.LoginAttempt
	for _, a := range f.attempts {
		switch {
		case p.AttemptType != nil && string(a.AttemptType) != *p.AttemptType,
			p.Outcome != nil && string(a.Outcome) != *p.Outcome,
			p.Identifier != nil && deref(a.Identifier) != *p.Identifier,
			p.DateFrom != nil && a.AttemptedAt.Before(*p.DateFrom),
			p.DateTo != nil && a.AttemptedAt.After(*p.DateTo),
			p.AfterTime != nil && (a.AttemptedAt.After(*p.AfterTime) || (a.AttemptedAt.Equal(*p.AfterTime) && a.ID >= *p.AfterID)):
			continue
		}
		out = append(out, a)
	}
	return out[:min(len(out), p.Limit)], nil
}

type fakeLoginPrincipals map[string]*principal.Principal

func (f fakeLoginPrincipals) FindByID(_ context.Context, id ids.PrincipalID) (*principal.Principal, error) {
	return f[string(id)], nil
}

var loginBase = time.Date(2026, 10, 6, 8, 30, 0, 0, time.UTC)

func attempt(id string, typ loginattempt.AttemptType, outcome loginattempt.Outcome, ident string, minutesAgo int) loginattempt.LoginAttempt {
	return loginattempt.LoginAttempt{ID: id, AttemptType: typ, Outcome: outcome, Identifier: &ident, AttemptedAt: loginBase.Add(-time.Duration(minutesAgo) * time.Minute)}
}

func newLoginStore() *fakeLoginStore {
	reason, ip, agent, pid := "BAD_PASSWORD", "10.0.0.7", "Mozilla <x>", "prn_1"
	first := attempt("lat_1", loginattempt.AttemptUserLogin, loginattempt.OutcomeFailure, "ada@example.com", 1)
	first.FailureReason, first.IPAddress, first.UserAgent, first.PrincipalID = &reason, &ip, &agent, &pid
	svcPid := "prn_2"
	svc := attempt("lat_2", loginattempt.AttemptServiceAccountToken, loginattempt.OutcomeSuccess, "oauth_client_1", 2)
	svc.PrincipalID = &svcPid
	return &fakeLoginStore{attempts: []loginattempt.LoginAttempt{first, svc, attempt("lat_3", loginattempt.AttemptUserLogin, loginattempt.OutcomeSuccess, "bob@example.com", 3)}}
}

func loginPrincipals() fakeLoginPrincipals {
	sa := "sac_7"
	return fakeLoginPrincipals{
		"prn_1": {ID: "prn_1", Type: principal.TypeUser},
		"prn_2": {ID: "prn_2", Type: principal.TypeService, ServiceAccountID: &sa},
	}
}

func loginRec(t *testing.T, store *fakeLoginStore, ac *auth.AuthContext, q req, status int) string {
	t.Helper()
	page := &LoginAttempts{Store: store, Principals: loginPrincipals(), Shell: shellForTest()}
	rec := serveMounted(t, ac, q, page)
	if rec.Code != status {
		t.Errorf("%s: status %d, want %d", q.target, rec.Code, status)
	}
	return rec.Body.String()
}

const loginViewPermTest = "platform:admin:login-attempt:view"

// keyOf is the row key (and URL segment) of an attempt.
func keyOf(a loginattempt.LoginAttempt) string {
	return strconv.FormatInt(a.AttemptedAt.UnixMicro(), 10) + "." + a.ID
}

func TestLoginAttemptsList(t *testing.T) {
	store := newLoginStore()
	body := loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts"}, http.StatusOK)
	mustContain(t, body, "<title>Login Attempts · FlowCatalyst</title>", "Authentication attempt history for users and service accounts",
		"Failures at the identity provider itself", "ada@example.com", "oauth_client_1", "User Login", "Service Account", "fc-tag-danger", "fc-tag-success",
		"Bad password", "10.0.0.7", `href="/ui/login-attempts/`+keyOf(store.attempts[0])+`"`, "Showing the 3 most recent attempts",
		`datetime="2026-10-06T08:29:00Z"`, `name="from"`, `name="to"`,
		`href="/ui/login-attempts" class="fc-nav-item" title="Login Attempts" aria-current="page"`)

	// An anchor with the permission only: another permission, or a client-scoped caller, is refused.
	loginRec(t, store, connUser("platform:admin:audit-log:view"), req{target: "/ui/login-attempts"}, http.StatusForbidden)
	client := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{loginViewPermTest}, PrincipalType: auth.PrincipalTypeUser}
	loginRec(t, store, client, req{target: "/ui/login-attempts"}, http.StatusForbidden)
	mustContain(t, loginRec(t, store, client, req{target: "/ui/login-attempts/" + keyOf(store.attempts[0]), datastar: true}, http.StatusOK), "fc-flash-error")
	loginRec(t, store, connUser(loginViewPermTest), req{target: "/ui/login-attempts"}, http.StatusOK)

	live := loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts?attemptType=USER_LOGIN&outcome=SUCCESS&q=bob@example.com&from=2026-10-06&to=2026-10-06", datastar: true}, http.StatusOK)
	mustContain(t, live, "bob@example.com", "data: mode replace", "history.replaceState")
	mustNotContain(t, live, "ada@example.com", "oauth_client_1")
	p := store.params[len(store.params)-1]
	wantFrom, wantTo := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 6, 23, 59, 59, 999999000, time.UTC)
	if *p.AttemptType != "USER_LOGIN" || *p.Outcome != "SUCCESS" || *p.Identifier != "bob@example.com" || !p.DateFrom.Equal(wantFrom) || !p.DateTo.Equal(wantTo) {
		t.Errorf("params %+v", p)
	}
	mustContain(t, loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts?outcome=NOPE"}, http.StatusOK), "No login attempts found", `href="/ui/login-attempts"`)
	// A hand-edited date is no bound.
	loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts?from=yesterday"}, http.StatusOK)
	if p := store.params[len(store.params)-1]; p.DateFrom != nil {
		t.Errorf("date %v", p.DateFrom)
	}
}

func TestLoginAttemptsQueryAndPaging(t *testing.T) {
	q := parseLoginQuery(url.Values{"attemptType": {"USER_LOGIN"}, "q": {" ada "}, "from": {"2026-10-01"}, "to": {"x"}, "after": {"1759739400000000.lat_7"}})
	again := parseLoginQuery(mustQuery(t, q.URL()))
	if q.Identifier != "ada" || q.To != "" || q.After == nil || q.After.ID != "lat_7" || again.URL() != q.URL() {
		t.Errorf("round trip %s vs %s", q.URL(), again.URL())
	}
	store := newLoginStore()
	for i := range 120 {
		store.attempts = append(store.attempts, attempt("lat_"+strconv.Itoa(1000+i), loginattempt.AttemptUserLogin, loginattempt.OutcomeSuccess, "u@x.y", 10+i))
	}
	first := loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts"}, http.StatusOK)
	last := store.attempts[loginPageSize-1]
	mustContain(t, first, "Showing the 100 most recent attempts", "/ui/login-attempts?after="+strconv.FormatInt(last.AttemptedAt.UnixMicro(), 10)+"."+last.ID)
	second := loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts?after=" + strconv.FormatInt(last.AttemptedAt.UnixMicro(), 10) + "." + last.ID}, http.StatusOK)
	mustContain(t, second, "Showing 23 older attempts", `href="/ui/login-attempts"`)
}

func TestLoginAttemptDrawer(t *testing.T) {
	store := newLoginStore()
	body := loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts/" + keyOf(store.attempts[0]), datastar: true}, http.StatusOK)
	mustContain(t, body, "data: selector #login-drawer\n", "Login Attempt Details", "Bad password", "10.0.0.7", "Mozilla &lt;x&gt;",
		`href="/ui/users/prn_1"`, ">ada@example.com<", ">prn_1<")
	// The identifier of a user login links to the user too.
	if got := strings.Count(body, `href="/ui/users/prn_1"`); got != 2 {
		t.Errorf("user links %d, want identifier and principal", got)
	}
	svc := loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts/" + keyOf(store.attempts[1]), datastar: true}, http.StatusOK)
	mustContain(t, svc, `href="/ui/service-accounts/sac_7"`, "oauth_client_1")
	if got := strings.Count(svc, `href="/ui/service-accounts/sac_7"`); got != 1 {
		t.Errorf("a service account token's client id has no page: %d links", got)
	}
	// Without the permission to read principals the ids stay plain text.
	plain := loginRec(t, store, connUser(loginViewPermTest), req{target: "/ui/login-attempts/" + keyOf(store.attempts[0]), datastar: true}, http.StatusOK)
	mustNotContain(t, plain, `href="/ui/users/`)
	page := loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts/" + keyOf(store.attempts[0])}, http.StatusOK)
	mustContain(t, page, `<aside id="login-drawer"`, `{selLogin: &#39;`+keyOf(store.attempts[0])+`&#39;}`)

	// A key that is not a row, or that could break out of an expression, is not found.
	for _, key := range []string{"junk", "1759739400000000.lat_404", keyOf(store.attempts[0])[:10] + ".lat_1", "9.lat_1%27%29"} {
		loginRec(t, store, anchorAdmin(), req{target: "/ui/login-attempts/" + key}, http.StatusNotFound)
	}
}
