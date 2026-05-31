package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestExtractBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"Bearer abc.def.ghi", "abc.def.ghi"},
		{"bearer abc.def.ghi", "abc.def.ghi"}, // case-insensitive scheme
		{"Bearer  spaced", "spaced"},          // trims
		{"Basic dXNlcjpwYXNz", ""},            // wrong scheme
		{"Bearer", ""},                        // no token
		{"BearerNoSpace", ""},                 // malformed
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			r.Header.Set("Authorization", tc.header)
		}
		got, _ := extractToken(r)
		if got != tc.want {
			t.Errorf("extractToken(%q) = %q; want %q", tc.header, got, tc.want)
		}
	}
}

func TestExtractBearerTokenSessionCookie(t *testing.T) {
	// No Authorization header — cookie wins.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-jwt"})
	if got, fromCookie := extractToken(r); got != "cookie-jwt" || !fromCookie {
		t.Errorf("cookie path: got %q fromCookie=%v want cookie-jwt/true", got, fromCookie)
	}

	// Both present — Authorization wins (and is not flagged as a cookie).
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer header-jwt")
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-jwt"})
	if got, fromCookie := extractToken(r); got != "header-jwt" || fromCookie {
		t.Errorf("header preferred: got %q fromCookie=%v want header-jwt/false", got, fromCookie)
	}

	// Non-Bearer Authorization — do NOT fall through to cookie.
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-jwt"})
	if got, _ := extractToken(r); got != "" {
		t.Errorf("non-bearer header should suppress cookie fallback: got %q", got)
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c ", []string{"a", "b", "c"}}, // whitespace trimmed
		{",,a,,b,", []string{"a", "b"}},        // empty segments dropped
	}
	for _, tc := range cases {
		got := splitCSV(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCSV(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

func TestStringSliceFromJWTExtra(t *testing.T) {
	// JWT round-tripped through JSON arrives as []interface{}; freshly
	// minted tokens hand back []string. Both must coerce.
	if got := stringSlice([]string{"a", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("stringSlice([]string): %v", got)
	}
	if got := stringSlice([]interface{}{"a", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("stringSlice([]interface{}): %v", got)
	}
	if got := stringSlice("nope"); got != nil {
		t.Errorf("stringSlice(string): %v", got)
	}
	if got := stringSlice(nil); got != nil {
		t.Errorf("stringSlice(nil): %v", got)
	}
}
