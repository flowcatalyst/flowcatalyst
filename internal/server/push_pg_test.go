//go:build integration

package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The gateway wired into the server beside the platform: its routes are mounted, answer in the
// module's envelope, and a platform sign-out announces itself on the revocation channel.
func TestPushGatewayWiredIntoTheServer(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	t.Setenv("FC_PUSH_STREAM_SECRET", "0123456789abcdef0123456789abcdef-stream-secret")
	t.Setenv("FC_PUSH_RELAY_INTERVAL", "50ms")
	cc := pool.Config().ConnConfig
	dbURL := (&url.URL{
		Scheme: "postgres", User: url.UserPassword(cc.User, cc.Password),
		Host: fmt.Sprintf("%s:%d", cc.Host, cc.Port), Path: "/" + cc.Database, RawQuery: "sslmode=disable",
	}).String()
	cfg := EnvCfg{
		JWTIssuer: xoPublicBase, JWTSigningKeyPath: keyPath, DatabaseURL: dbURL, PushEnabled: true,
		SessionTTLSecs: 3600, AccessTokenTTLSecs: 3600, RefreshTokenTTLSecs: 3600,
		FunctionsArtifactStore: "file://" + t.TempDir(),
	}
	r := chi.NewRouter()
	handles, err := WirePlatform(r, pool, cfg, dispatch.Settings{})
	require.NoError(t, err)
	require.NotNil(t, handles.PushAuth)
	gw, err := startPushIfEnabled(ctx, r, pool, cfg, handles)
	require.NoError(t, err)
	require.NotNil(t, gw)
	t.Cleanup(func() { _ = gw.Shutdown() })

	do := func(method, path string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://platform.example.com"+path, strings.NewReader(`{"topics":["alpha:x:1"]}`))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	t.Run("routes are mounted in the module's envelope", func(t *testing.T) {
		rec := do(http.MethodPost, "/api/push/subscriptions", nil)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"UNAUTHENTICATED"`)
		rec = do(http.MethodGet, "/api/push/keys", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"keys":[]}`, rec.Body.String())
		assert.Equal(t, http.StatusOK, do(http.MethodGet, "/health/push", nil).Code)
	})

	t.Run("ingest is mounted beside the module, for bearer tokens", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://platform.example.com/api/push/batch", strings.NewReader(`{"items":[]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", platformmw.SessionCookieName+"=whatever") // not an ingest credential
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, "no credential: "+rec.Body.String())
		assert.Contains(t, rec.Body.String(), "UNAUTHENTICATED")
	})

	t.Run("the module's and the gateway's metrics are exposed on the listener", func(t *testing.T) {
		rec := httptest.NewRecorder()
		metricsRouter(cfg, pushGatherers(gw)...).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "fw_push_log_id", "the module's series")
		assert.Contains(t, rec.Body.String(), "go_goroutines", "and the platform's")
	})

	t.Run("the platform's own tokens authenticate (one signing key)", func(t *testing.T) {
		svcs, err := buildServices(cfg, pool, buildRepos(pool))
		require.NoError(t, err)
		pid := ids.NewPrincipalID()
		_, err = pool.Exec(ctx, `INSERT INTO iam_principals (id, type, scope, name, active, email) VALUES ($1, 'USER', 'ANCHOR', 'Push', TRUE, $2)`,
			pid, strings.ToLower(string(pid))+"@push.example.com")
		require.NoError(t, err)
		cookie, err := svcs.authProvider.MintSessionToken(ctx, string(pid), 0)
		require.NoError(t, err)
		// an anchor reaches every client: it must name one (400), which proves the cookie was accepted
		rec := do(http.MethodPost, "/api/push/subscriptions", map[string]string{
			"Cookie": platformmw.SessionCookieName + "=" + cookie, "Sec-Fetch-Site": "same-origin",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "name one in client")
	})

	t.Run("a sign-out announces the principal", func(t *testing.T) {
		svcs, err := buildServices(cfg, pool, buildRepos(pool))
		require.NoError(t, err)
		pid := ids.NewPrincipalID()
		_, err = pool.Exec(ctx, `INSERT INTO iam_principals (id, type, scope, name, active, email) VALUES ($1, 'USER', 'ANCHOR', 'Out', TRUE, $2)`,
			pid, strings.ToLower(string(pid))+"@out.example.com")
		require.NoError(t, err)
		cookie, err := svcs.authProvider.MintSessionToken(ctx, string(pid), 0)
		require.NoError(t, err)

		pooled, err := pool.Acquire(ctx)
		require.NoError(t, err)
		conn := pooled.Hijack()
		defer func() { _ = conn.Close(ctx) }()
		_, err = conn.Exec(ctx, "LISTEN "+pushgw.ChannelPrincipalEnd)
		require.NoError(t, err)

		rec := do(http.MethodPost, "/auth/logout", map[string]string{
			"Cookie": platformmw.SessionCookieName + "=" + cookie, "Sec-Fetch-Site": "same-origin",
		})
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

		wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		n, err := conn.WaitForNotification(wctx)
		require.NoError(t, err)
		assert.Equal(t, string(pid), n.Payload)
	})
}
