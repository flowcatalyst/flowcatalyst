package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// saPlatform is a fake platform for the --service-account flags: an existing
// function demo.hook (fn_7) whose PATCH body is recorded.
type saPlatform struct {
	srv       *httptest.Server
	mu        sync.Mutex
	patches   []map[string]any
	createReq map[string]any
	created   bool
}

func newSAPlatform(t *testing.T) *saPlatform {
	p := &saPlatform{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":` + strings.TrimSpace(mustJSON(jwt(`{"sub":"prn_T","tier":"ANCHOR"}`))) + `,"expires_in":300,"token_type":"Bearer"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions":
			data := `{"id":"fn_7","address":"demo.hook","applicationCode":"demo"}`
			if p.created {
				data += `,{"id":"fn_9","address":"demo.new","applicationCode":"demo"}`
			}
			_, _ = w.Write([]byte(`{"data":[` + data + `]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/functions/fn_7":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			p.patches = append(p.patches, body)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/functions/fn_7/config/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/applications/by-code/"):
			_, _ = w.Write([]byte(`{"id":"app_1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/functions":
			p.created = true
			_ = json.NewDecoder(r.Body).Decode(&p.createReq)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case strings.HasPrefix(r.URL.Path, "/api/functions/fn_9/artifacts/"):
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

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func (p *saPlatform) set(t *testing.T, args ...string) error {
	t.Helper()
	newFnTestEnv(t)
	_, _, err := runCobra(t, newFnSetCmd(), "", append([]string{
		"demo.hook", "--platform", p.srv.URL, "--client-id", "c", "--client-secret", "s", "--yes",
	}, args...)...)
	return err
}

func (p *saPlatform) lastPatch(t *testing.T) map[string]any {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	require.NotEmpty(t, p.patches)
	return p.patches[len(p.patches)-1]
}

func TestFnSet_ServiceAccountSendsCode(t *testing.T) {
	p := newSAPlatform(t)
	require.NoError(t, p.set(t, "--service-account", "bevco-integration"))
	require.Equal(t, map[string]any{"serviceAccountId": "bevco-integration"}, p.lastPatch(t))
}

func TestFnSet_ClearServiceAccount(t *testing.T) {
	for _, args := range [][]string{{"--clear-service-account"}, {"--service-account", ""}} {
		p := newSAPlatform(t)
		require.NoError(t, p.set(t, args...))
		require.Equal(t, map[string]any{"clearServiceAccount": true}, p.lastPatch(t), "%v", args)
	}
}

func TestFnSet_ServiceAccountAndClearConflict(t *testing.T) {
	p := newSAPlatform(t)
	err := p.set(t, "--service-account", "x", "--clear-service-account")
	require.ErrorContains(t, err, "mutually exclusive")
	require.Empty(t, p.patches)
}

func TestFnSet_NoServiceAccountFlagSendsNoPatch(t *testing.T) {
	p := newSAPlatform(t)
	require.NoError(t, p.set(t, "--config", "A=b"))
	require.Empty(t, p.patches)
}

func TestPublishCreate_ServiceAccountOnCreate(t *testing.T) {
	p := newSAPlatform(t)
	newFnTestEnv(t)
	art := filepath.Join(t.TempDir(), "function.js")
	require.NoError(t, os.WriteFile(art, []byte(`export default {}`), 0o600))
	_, _, err := runCobra(t, newFnPublishCmd(), "", "demo.new", art, "--platform", p.srv.URL,
		"--client-id", "c", "--client-secret", "s", "--yes", "--create", "--service-account", "bevco-integration")
	require.NoError(t, err)
	require.Equal(t, "bevco-integration", p.createReq["serviceAccountId"])
}
