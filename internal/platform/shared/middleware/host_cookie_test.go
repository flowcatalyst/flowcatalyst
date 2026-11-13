package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// setSecureForTest flips the process-wide setting and restores it. Tests that
// call it must not run in parallel with ones that depend on the setting (none of
// this package's do: no test here calls t.Parallel).
func setSecureForTest(t *testing.T, secure bool) {
	t.Helper()
	SetSessionCookieSecure(secure)
	t.Cleanup(func() { SetSessionCookieSecure(false) })
}

func TestSessionCookieNameFollowsSecure(t *testing.T) {
	t.Cleanup(func() { _ = SetSessionSameSite("lax") })
	if c := SessionCookie("t", true, time.Hour, false); c.Name != "__Host-fc_session" || !c.Secure || c.Path != "/" || c.Domain != "" || !c.HttpOnly {
		t.Errorf("secure cookie: %+v", c)
	}
	if c := SessionCookie("t", false, time.Hour, false); c.Name != "fc_session" || c.Secure {
		t.Errorf("plain-HTTP cookie: %+v", c)
	}
	// SameSite=None forces Secure, so the cookie takes the __Host- name.
	if err := SetSessionSameSite("none"); err != nil {
		t.Fatal(err)
	}
	if c := SessionCookie("t", false, time.Hour, false); c.Name != "__Host-fc_session" || !c.Secure {
		t.Errorf("SameSite=None cookie built non-secure: %+v", c)
	}
}

func TestReadSessionCookiePrefersHostName(t *testing.T) {
	both := httptest.NewRequest(http.MethodGet, "/", nil)
	both.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "old"})
	both.AddCookie(&http.Cookie{Name: HostSessionCookieName, Value: "host"})
	if tok, old := ReadSessionCookie(both); tok != "host" || old {
		t.Errorf("both present: got %q old=%v; want host/false", tok, old)
	}

	onlyOld := httptest.NewRequest(http.MethodGet, "/", nil)
	onlyOld.AddCookie(&http.Cookie{Name: SessionCookieName, Value: " old "})
	if tok, old := ReadSessionCookie(onlyOld); tok != "old" || !old {
		t.Errorf("old only: got %q old=%v; want old/true", tok, old)
	}

	emptyHost := httptest.NewRequest(http.MethodGet, "/", nil)
	emptyHost.AddCookie(&http.Cookie{Name: HostSessionCookieName, Value: ""})
	emptyHost.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "old"})
	if tok, old := ReadSessionCookie(emptyHost); tok != "old" || !old {
		t.Errorf("empty host cookie: got %q old=%v; want old/true", tok, old)
	}

	if tok, old := ReadSessionCookie(httptest.NewRequest(http.MethodGet, "/", nil)); tok != "" || old {
		t.Errorf("none: got %q old=%v", tok, old)
	}
}

func TestSessionCookieAcceptedRule(t *testing.T) {
	marked := &sessiontoken.Claims{Subject: "p", HostCookie: true}
	unmarked := &sessiontoken.Claims{Subject: "p"}
	cases := []struct {
		secure, oldName bool
		c               *sessiontoken.Claims
		want            bool
		name            string
	}{
		{true, true, marked, false, "secure, old name, marker: refused (planted)"},
		{true, true, unmarked, true, "secure, old name, no marker: accepted (pre-upgrade session)"},
		{true, false, marked, true, "secure, __Host- name, marker: accepted"},
		{false, true, marked, true, "not secure, old name, marker: accepted"},
		{false, true, unmarked, true, "not secure, old name, no marker: accepted"},
	}
	for _, tc := range cases {
		setSecureForTest(t, tc.secure)
		if got := SessionCookieAccepted(tc.oldName, tc.c); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

// The rule through the real Authenticator and AuthenticateRequest. Only the
// refused rows are here (an accepted session needs the database to resolve its
// authority; those rows live in internal/server/host_cookie_pg_test.go). A
// refused cookie must proceed unauthenticated like a stale one: no 401, and no
// database access (this provider has nil repositories and would panic).
func TestPlantedOldNameCookieIsNotASession(t *testing.T) {
	c := newClassRig(t)
	marked := c.session(t) // Mint writes the marker

	probe := func(name string) (code int, ac *auth.AuthContext) {
		h := Authenticator(AuthConfig{Provider: c.prov})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac = auth.FromContext(r.Context())
		}))
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
		r.AddCookie(&http.Cookie{Name: name, Value: marked})
		h.ServeHTTP(rec, r)
		return rec.Code, ac
	}

	setSecureForTest(t, true)
	if code, ac := probe(SessionCookieName); code != http.StatusOK || ac != nil {
		t.Errorf("Authenticator: planted fc_session on a secure process: status=%d ac=%v; want 200 and no AuthContext", code, ac)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: marked})
	cred, present, err := AuthenticateRequest(t.Context(), c.prov, r)
	if present || err != nil || cred.Auth != nil {
		t.Errorf("AuthenticateRequest: present=%v err=%v auth=%v; want the request counted as logged out", present, err, cred.Auth)
	}
}

// No non-test file under internal/ or cmd/ other than the one reader may read the
// session cookie directly: every reader goes through ReadSessionCookie, so the
// acceptance rule cannot be bypassed.
func TestOnlyOneSessionCookieReader(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\.Cookie\(\s*("fc_session"|"__Host-fc_session"|(\w+\.)?(SessionCookieName|HostSessionCookieName))\s*\)`)
	self := filepath.Join(root, "internal", "platform", "shared", "middleware", "cookie.go")
	scanned := 0
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == self {
				return nil
			}
			scanned++
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if loc := re.Find(b); loc != nil {
				t.Errorf("%s reads the session cookie directly (%q); use middleware.ReadSessionCookie", path, loc)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d files: the walk is not finding the tree", scanned)
	}
}
