package mcp

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// EnvAuthToken names the environment variable holding the bearer token the
// MCP HTTP endpoint requires. Unset means no caller authentication, which is
// acceptable only on a loopback bind (see CheckHTTPBind).
const EnvAuthToken = "FC_MCP_AUTH_TOKEN"

// AuthTokenFromEnv returns the configured bearer token, or "".
func AuthTokenFromEnv() string { return os.Getenv(EnvAuthToken) }

// RequireBearerToken wraps next so every request must carry
// "Authorization: Bearer <token>". The comparison is constant-time. An empty
// token disables the check (the caller has already refused that combination
// on a non-loopback bind).
func RequireBearerToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		scheme, got, ok := strings.Cut(h, " ")
		okScheme := ok && strings.EqualFold(scheme, "Bearer")
		have := sha256.Sum256([]byte(strings.TrimSpace(got)))
		// Hash both sides so the comparison does not depend on the length of
		// what the caller sent.
		if !okScheme || subtle.ConstantTimeCompare(have[:], want[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="flowcatalyst-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CheckHTTPBind refuses to listen for MCP over HTTP on an address that is not
// loopback unless a bearer token is configured: the endpoint carries the MCP
// server's own platform credential, so an unauthenticated network listener
// would hand that access to anyone who can reach it. addr is "host:port" (an
// empty host means every interface).
func CheckHTTPBind(addr, token string) error {
	if token != "" {
		return nil
	}
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	if isLoopbackHost(host) {
		return nil
	}
	shown := host
	if shown == "" {
		shown = "(all interfaces)"
	}
	return fmt.Errorf("refusing to serve MCP over HTTP on %s without authentication: set %s to a bearer token "+
		"callers must send as 'Authorization: Bearer <token>', or bind a loopback address such as 127.0.0.1", shown, EnvAuthToken)
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
