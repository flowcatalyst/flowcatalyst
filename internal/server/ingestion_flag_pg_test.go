//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// FC_INGESTION_ENABLED decides whether the ingestion routes exist: off they
// are not mounted (404), on they answer; the public platform config the SPA
// reads says which.
func TestIngestionFlag(t *testing.T) {
	pool := testpg.Pool(t)
	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)

	boot := func(enabled bool) (*httptest.Server, string) {
		cfg := EnvCfg{
			JWTIssuer: "https://platform.example.com", JWTSigningKeyPath: keyPath,
			SessionTTLSecs: 3600, AccessTokenTTLSecs: 3600, RefreshTokenTTLSecs: 3600,
			FunctionsArtifactStore: "file://" + t.TempDir(),
			IngestionEnabled:       enabled,
		}
		r := chi.NewRouter()
		_, err := WirePlatform(r, pool, cfg, dispatch.Settings{})
		require.NoError(t, err)
		repos := buildRepos(pool)
		svcs, err := buildServices(cfg, pool, repos)
		require.NoError(t, err)
		srv := httptest.NewServer(r)
		t.Cleanup(srv.Close)

		appID := tsid.Generate(tsid.Application)
		_, err = pool.Exec(context.Background(), `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`,
			appID, "flag"+strings.ToLower(tsid.GenerateUntyped())[5:])
		require.NoError(t, err)
		return srv, mintScoped(t, pool, repos, svcs, appID, []string{"platform:ingestion:ingestion:view"})
	}
	get := func(srv *httptest.Server, bearer, path string) (int, string) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+bearer)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	feature := func(srv *httptest.Server) bool {
		resp, err := http.Get(srv.URL + "/api/config/platform")
		require.NoError(t, err)
		defer resp.Body.Close()
		var out struct {
			Features struct {
				Ingestion bool `json:"ingestion"`
			} `json:"features"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		return out.Features.Ingestion
	}

	off, offBearer := boot(false)
	for _, path := range []string{"/api/ingestions", "/api/ingestion-source-kinds"} {
		code, body := get(off, offBearer, path)
		assert.Equal(t, http.StatusNotFound, code, "flag off, %s: %s", path, body)
	}
	assert.False(t, feature(off))

	on, onBearer := boot(true)
	for _, path := range []string{"/api/ingestions", "/api/ingestion-source-kinds"} {
		code, body := get(on, onBearer, path)
		assert.Equal(t, http.StatusOK, code, "flag on, %s: %s", path, body)
	}
	assert.True(t, feature(on))
}
