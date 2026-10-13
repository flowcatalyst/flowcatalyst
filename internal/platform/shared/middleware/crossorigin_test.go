package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sessionCSRFCase is one request through SessionCSRF; cookie marks it as
// authenticated by the fc_session cookie (what Authenticator records).
type sessionCSRFCase struct {
	name         string
	method       string
	host         string
	cookie       bool
	secFetchSite string
	origin       string
	contentType  string
	body         string
	wantStatus   int
	wantCode     string
}

func runSessionCSRF(t *testing.T, trusted []string, tc sessionCSRFCase) {
	t.Helper()
	guard, err := SessionCSRF(trusted)
	if err != nil {
		t.Fatalf("SessionCSRF: %v", err)
	}
	reached := false
	h := guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	var r *http.Request
	if tc.body != "" {
		r = httptest.NewRequest(tc.method, "http://platform.example.com/api/thing", strings.NewReader(tc.body))
	} else {
		r = httptest.NewRequest(tc.method, "http://platform.example.com/api/thing", nil)
	}
	if tc.host != "" {
		r.Host = tc.host
	}
	if tc.cookie {
		r = r.WithContext(context.WithValue(r.Context(), cookieSessionKey{}, true))
	}
	if tc.secFetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", tc.secFetchSite)
	}
	if tc.origin != "" {
		r.Header.Set("Origin", tc.origin)
	}
	if tc.contentType != "" {
		r.Header.Set("Content-Type", tc.contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != tc.wantStatus {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.wantStatus, w.Body.String())
	}
	if tc.wantStatus == http.StatusOK {
		if !reached {
			t.Fatal("handler not reached")
		}
		return
	}
	if reached {
		t.Fatal("handler reached on a refused request")
	}
	var env struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || env.Error != tc.wantCode {
		t.Fatalf("envelope code = %q (%v), want %q; body %s", env.Error, err, tc.wantCode, w.Body.String())
	}
}

func TestSessionCSRF(t *testing.T) {
	const json = "application/json"
	const self = "https://platform.example.com"
	cases := []sessionCSRFCase{
		// Cookie-authenticated unsafe requests: provenance.
		{name: "same-origin", method: "POST", cookie: true, secFetchSite: "same-origin", origin: self, contentType: json, body: "{}", wantStatus: 200},
		{name: "user-initiated", method: "POST", cookie: true, secFetchSite: "none", wantStatus: 200},
		{name: "same-site sibling", method: "POST", cookie: true, secFetchSite: "same-site", origin: "https://evil.example.com", contentType: json, body: "{}", wantStatus: 403, wantCode: CodeCrossOriginRequest},
		{name: "cross-site", method: "DELETE", cookie: true, secFetchSite: "cross-site", origin: "https://attacker.test", wantStatus: 403, wantCode: CodeCrossOriginRequest},
		{name: "old browser, Origin matches Host", method: "PUT", cookie: true, origin: "http://platform.example.com", contentType: json, body: "{}", wantStatus: 200},
		{name: "old browser, foreign Origin", method: "PATCH", cookie: true, origin: "https://evil.example.com", contentType: json, body: "{}", wantStatus: 403, wantCode: CodeCrossOriginRequest},
		{name: "neither header", method: "POST", cookie: true, contentType: json, body: "{}", wantStatus: 403, wantCode: CodeCrossOriginRequest},
		{name: "Origin null", method: "POST", cookie: true, origin: "null", contentType: json, body: "{}", wantStatus: 403, wantCode: CodeCrossOriginRequest},
		{name: "rewritten Host, trusted public Origin", method: "POST", host: "internal:8080", cookie: true, origin: self, contentType: json, body: "{}", wantStatus: 200},
		{name: "rewritten Host, trusted public Origin, cross-site fetch metadata", method: "POST", host: "internal:8080", cookie: true, secFetchSite: "same-site", origin: self, contentType: json, body: "{}", wantStatus: 200},

		// Cookie-authenticated unsafe requests: JSON only.
		{name: "text/plain body", method: "POST", cookie: true, secFetchSite: "same-origin", contentType: "text/plain;charset=UTF-8", body: "{}", wantStatus: 415, wantCode: CodeUnsupportedMediaType},
		{name: "form body", method: "POST", cookie: true, secFetchSite: "same-origin", contentType: "application/x-www-form-urlencoded", body: "a=b", wantStatus: 415, wantCode: CodeUnsupportedMediaType},
		{name: "multipart body", method: "POST", cookie: true, secFetchSite: "same-origin", contentType: "multipart/form-data; boundary=x", body: "--x--", wantStatus: 415, wantCode: CodeUnsupportedMediaType},
		{name: "body without Content-Type", method: "POST", cookie: true, secFetchSite: "same-origin", body: "{}", wantStatus: 415, wantCode: CodeUnsupportedMediaType},
		{name: "malformed Content-Type", method: "POST", cookie: true, secFetchSite: "same-origin", contentType: "application/json; =", body: "{}", wantStatus: 415, wantCode: CodeUnsupportedMediaType},
		{name: "json with charset", method: "POST", cookie: true, secFetchSite: "same-origin", contentType: "application/json; charset=utf-8", body: "{}", wantStatus: 200},
		{name: "merge-patch+json", method: "PATCH", cookie: true, secFetchSite: "same-origin", contentType: "application/merge-patch+json", body: "{}", wantStatus: 200},
		{name: "no body, no Content-Type", method: "POST", cookie: true, secFetchSite: "same-origin", wantStatus: 200},
		{name: "no body, text/plain declared", method: "DELETE", cookie: true, secFetchSite: "same-origin", contentType: "text/plain", wantStatus: 200},

		// Safe methods are never refused.
		{name: "GET cross-site with cookie", method: "GET", cookie: true, secFetchSite: "cross-site", origin: "https://attacker.test", wantStatus: 200},
		{name: "HEAD neither header", method: "HEAD", cookie: true, wantStatus: 200},
		{name: "OPTIONS cross-site", method: "OPTIONS", cookie: true, secFetchSite: "cross-site", wantStatus: 200},

		// Not cookie-authenticated (bearer, test headers, stale cookie, anonymous).
		{name: "bearer cross-site text/plain", method: "POST", secFetchSite: "cross-site", origin: "https://attacker.test", contentType: "text/plain", body: "{}", wantStatus: 200},
		{name: "bearer no headers", method: "POST", contentType: json, body: "{}", wantStatus: 200},
		{name: "bearer wasm upload", method: "PUT", contentType: "application/wasm", body: "\x00asm", wantStatus: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runSessionCSRF(t, []string{"https://platform.example.com/"}, tc)
		})
	}
}

func TestSessionCSRFRejectsBadTrustedOrigin(t *testing.T) {
	for _, o := range []string{"platform.example.com", "ftp://x", "https://"} {
		if _, err := SessionCSRF([]string{o}); err == nil {
			t.Errorf("SessionCSRF(%q): want an error", o)
		}
	}
	if _, err := SessionCSRF([]string{"", "https://ok.example.com/path"}); err != nil {
		t.Errorf("empty and path-bearing origins: %v", err)
	}
}

func TestAuthenticatedBySessionCookieDefaultFalse(t *testing.T) {
	if AuthenticatedBySessionCookie(context.Background()) {
		t.Fatal("a bare context is not cookie-authenticated")
	}
}
