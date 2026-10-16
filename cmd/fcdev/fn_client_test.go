package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

const (
	testClientID    = "clt_0HZXEQ5Y8JY5Z" // a valid clt_ + 13-char TSID
	testClientIDAlt = "clt_0HZXEQ5Y8JY50"
)

// clientPlatform is a fake platform for `fn publish --create --client`.
type clientPlatform struct {
	srv *httptest.Server
	mu  sync.Mutex
	// lookups counts GET /api/clients/by-identifier/* requests.
	lookups int
	// lookupStatus is the lookup's response status (200, 403, 404, 500).
	lookupStatus int
	created      bool
	createBody   map[string]any
}

func newClientPlatform(t *testing.T) *clientPlatform {
	p := &clientPlatform{lookupStatus: http.StatusOK}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oauth/token":
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"expires_in":300,"token_type":"Bearer"}`, jwt(`{"sub":"prn_T","tier":"ANCHOR"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/clients/by-identifier/"):
			p.lookups++
			switch p.lookupStatus {
			case http.StatusOK:
				code := strings.TrimPrefix(r.URL.Path, "/api/clients/by-identifier/")
				_, _ = fmt.Fprintf(w, `{"id":%q,"identifier":%q,"name":"Bev Co"}`, testClientID, code)
			case http.StatusForbidden:
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"FORBIDDEN","message":"missing permission platform:client:view"}`))
			case http.StatusNotFound:
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"NOT_FOUND","message":"Client not found"}`))
			default:
				w.WriteHeader(p.lookupStatus)
			}
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/applications/by-code/"):
			_, _ = w.Write([]byte(`{"id":"app_1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/functions":
			p.created = true
			_ = json.NewDecoder(r.Body).Decode(&p.createBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions":
			if p.created {
				_, _ = w.Write([]byte(`{"data":[{"id":"fn_9","address":"demo.new","applicationCode":"demo"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[]}`))
			}
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/functions/fn_9/artifacts/"):
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/functions/fn_9/versions":
			_, _ = w.Write([]byte(`{"number":1,"digest":"aaaaaaaaaaaaaaaa","runtime":"js"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *clientPlatform) publish(t *testing.T, args ...string) error {
	t.Helper()
	newFnTestEnv(t)
	art := filepath.Join(t.TempDir(), "function.js")
	require.NoError(t, os.WriteFile(art, []byte(`export default {}`), 0o600))
	_, _, err := runCobra(t, newFnPublishCmd(), "", append([]string{
		"demo.new", art, "--platform", p.srv.URL, "--client-id", "c", "--client-secret", "s", "--yes",
	}, args...)...)
	return err
}

func (p *clientPlatform) ownerSent() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.createBody["clientId"]
}

func TestLooksLikeClientID(t *testing.T) {
	for v, want := range map[string]bool{
		testClientID:        true,
		testClientIDAlt:     true,
		"bevco":             false,
		"clt_bevco":         false, // prefix but not a TSID
		"clt_":              false,
		"clt_0HZXEQ5Y8JY5":  false, // 12 chars
		"CLT_0HZXEQ5Y8JY5Z": false,
		"app_0HZXEQ5Y8JY5Z": false,
		"clt-corp":          false,
	} {
		require.Equal(t, want, looksLikeClientID(v), v)
	}
}

func TestPublishCreateClientIDPassesThroughWithoutLookup(t *testing.T) {
	p := newClientPlatform(t)
	require.NoError(t, p.publish(t, "--create", "--client", testClientIDAlt))
	require.Zero(t, p.lookups, "an id must not trigger a clients lookup")
	require.Equal(t, testClientIDAlt, p.ownerSent())
}

func TestPublishCreateClientCodeResolves(t *testing.T) {
	p := newClientPlatform(t)
	require.NoError(t, p.publish(t, "--create", "--client", "bevco"))
	require.Equal(t, 1, p.lookups)
	require.Equal(t, testClientID, p.ownerSent(), "the resolved id, not the code, goes to POST /api/functions")
}

func TestPublishCreateUnknownClientCode(t *testing.T) {
	p := newClientPlatform(t)
	p.lookupStatus = http.StatusNotFound
	err := p.publish(t, "--create", "--client", "nope")
	require.ErrorContains(t, err, `no client with code "nope" on `)
	require.ErrorContains(t, err, p.srv.URL)
	require.False(t, p.created, "nothing is created when the client does not resolve")
}

func TestPublishCreateClientLookupForbidden(t *testing.T) {
	p := newClientPlatform(t)
	p.lookupStatus = http.StatusForbidden
	err := p.publish(t, "--create", "--client", "bevco")
	require.ErrorContains(t, err, "platform:client:view")
	require.ErrorContains(t, err, "clt_ id")
	require.False(t, p.created)
}

func TestPublishCreateClientLookupServerError(t *testing.T) {
	p := newClientPlatform(t)
	p.lookupStatus = http.StatusInternalServerError
	err := p.publish(t, "--create", "--client", "bevco")
	require.ErrorContains(t, err, `look up client code "bevco"`)
	require.False(t, p.created)
}

func TestPublishClientWithoutCreateIsIgnored(t *testing.T) {
	// The function does not exist and --create is absent: today's refusal,
	// before any client lookup.
	p := newClientPlatform(t)
	err := p.publish(t, "--client", "bevco")
	require.ErrorContains(t, err, "pass --create")
	require.Zero(t, p.lookups)
	require.False(t, p.created)
}

func TestPublishNoClientFlagIsPlatformOwned(t *testing.T) {
	p := newClientPlatform(t)
	require.NoError(t, p.publish(t, "--create"))
	require.Zero(t, p.lookups)
	require.Nil(t, p.ownerSent())
}

func TestDeployDryRunShowsResolvedClient(t *testing.T) {
	e := newFnTestEnv(t)
	art := filepath.Join(e.dir, "function.wasm")
	require.NoError(t, os.WriteFile(art, []byte("0123456789"), 0o600))
	var out bytes.Buffer
	api := &fnAPI{c: fnCredentials{PlatformURL: "http://x.invalid"}}
	owner := fnClient{ID: testClientID, Code: "bevco"}
	require.NoError(t, deployDryRun(&out, api, nil, "demo.new", art, "wasm", &abi.Describe{}, 0, owner))
	require.Contains(t, out.String(), "owning client: bevco ("+testClientID+")")
	// A function that already exists is not re-owned.
	out.Reset()
	require.NoError(t, deployDryRun(&out, api, &fnFunction{ID: "fn_1", Address: "demo.new"}, "demo.new", art, "wasm", &abi.Describe{}, 0, owner))
	require.NotContains(t, out.String(), "owning client")
}
