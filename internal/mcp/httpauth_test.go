package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowcatalyst/go-sdk/client"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestRequireBearerToken(t *testing.T) {
	h := RequireBearerToken("s3cret-token", okHandler())
	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"prefix of the token", "Bearer s3cret", http.StatusUnauthorized},
		{"token with extra suffix", "Bearer s3cret-token-and-more", http.StatusUnauthorized},
		{"wrong scheme", "Basic s3cret-token", http.StatusUnauthorized},
		{"bare token, no scheme", "s3cret-token", http.StatusUnauthorized},
		{"right token", "Bearer s3cret-token", http.StatusOK},
		{"scheme is case-insensitive", "bearer s3cret-token", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestRequireBearerTokenUnsetPassesThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	RequireBearerToken("", okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("no token configured must behave as before, got %d", rec.Code)
	}
}

// The real MCP streamable-HTTP handler behind the guard: 401 without the
// bearer, and a real initialize round trip (200) with it.
func TestMCPHTTPHandlerBehindBearer(t *testing.T) {
	srv := NewWithClient(client.New("http://127.0.0.1:1"))
	ts := httptest.NewServer(RequireBearerToken("tok", srv.HTTPHandler()))
	defer ts.Close()

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	post := func(bearer string) int {
		req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(initialize))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(""); got != http.StatusUnauthorized {
		t.Fatalf("no bearer: %d", got)
	}
	if got := post("wrong"); got != http.StatusUnauthorized {
		t.Fatalf("wrong bearer: %d", got)
	}
	if got := post("tok"); got != http.StatusOK {
		t.Fatalf("right bearer: %d", got)
	}
}

func TestCheckHTTPBind(t *testing.T) {
	cases := []struct {
		addr, token string
		wantErr     bool
	}{
		{"127.0.0.1:8090", "", false},
		{"localhost:8090", "", false},
		{"[::1]:8090", "", false},
		{"127.0.0.5:8090", "", false},
		{"0.0.0.0:8090", "", true},
		{":8090", "", true},
		{"[::]:8090", "", true},
		{"192.168.1.10:8090", "", true},
		{"example.test:8090", "", true},
		{"0.0.0.0:8090", "tok", false},
		{":8090", "tok", false},
		{"127.0.0.1:8090", "tok", false},
	}
	for _, tc := range cases {
		err := CheckHTTPBind(tc.addr, tc.token)
		if (err != nil) != tc.wantErr {
			t.Errorf("CheckHTTPBind(%q, %q) = %v, wantErr %v", tc.addr, tc.token, err, tc.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), EnvAuthToken) {
			t.Errorf("error must name %s: %v", EnvAuthToken, err)
		}
	}
}
