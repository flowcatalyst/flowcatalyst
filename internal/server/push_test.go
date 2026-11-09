package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/framework/go/metrics"
)

func TestPushEnvironment(t *testing.T) {
	got := PushEnvironment([]string{
		"FC_PUSH_ENABLED=true", "FC_PUSH_STREAM_SECRET=s3cret", "FC_PUSH_DRIVER=redis", "FC_PUSH_STREAM_MAX_LIFETIME=30m",
		"FC_ROUTER_ENABLED=true", "PUSH_DRIVER=ignored", "APP_ENV=local", "malformed",
	})
	assert.Equal(t, map[string]string{
		"APP_ENV": "production", "PUSH_STREAM_SECRET": "s3cret", "PUSH_DRIVER": "redis", "PUSH_STREAM_MAX_LIFETIME": "30m",
	}, got)
}

func TestPushEnabledFlag(t *testing.T) {
	t.Setenv("FC_PUSH_ENABLED", "")
	assert.False(t, LoadEnv().PushEnabled, "off by default in a deployed environment")
	t.Setenv("FC_PUSH_ENABLED", "true")
	assert.True(t, LoadEnv().PushEnabled)
}

func TestStandaloneGatewayNeedsThePlatformSigningKey(t *testing.T) {
	t.Setenv("FLOWCATALYST_JWT_PRIVATE_KEY", "")
	t.Setenv("FC_JWT_SIGNING_KEY_PEM", "")

	err := requireStandaloneSigningKey(EnvCfg{})
	require.Error(t, err, "no key: an ephemeral one would refuse every token")
	assert.Contains(t, err.Error(), "FLOWCATALYST_JWT_PRIVATE_KEY")
	assert.Contains(t, err.Error(), "ephemeral")

	assert.NoError(t, requireStandaloneSigningKey(EnvCfg{RouterDevMode: true}), "a development process may generate one")

	keyPath, kerr := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, kerr)
	assert.NoError(t, requireStandaloneSigningKey(EnvCfg{JWTSigningKeyPath: keyPath}), "a key file")
	assert.Error(t, requireStandaloneSigningKey(EnvCfg{JWTSigningKeyPath: filepath.Join(t.TempDir(), "missing.pem")}), "an unreadable key file is no key")

	t.Setenv("FC_JWT_SIGNING_KEY_PEM", "-----BEGIN RSA PRIVATE KEY-----\nx\n-----END RSA PRIVATE KEY-----")
	assert.NoError(t, requireStandaloneSigningKey(EnvCfg{}), "an inline key")
}

func TestFrameworkMetricsAreExposedOnThePlatformListener(t *testing.T) {
	reg, err := metrics.NewRegistry()
	require.NoError(t, err)
	c, err := reg.Counter("fw_push_published_total", "Pushes the relay logged.")
	require.NoError(t, err)
	c.Inc()
	lc, err := reg.Counter("fc_push_ingest_items_total", "Pushes of ingest requests.", "application", "result")
	require.NoError(t, err)
	lc.With("shop", "accepted").Add(3)

	rec := httptest.NewRecorder()
	metricsRouter(EnvCfg{}, frameworkGatherer{reg: reg}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "fw_push_published_total 1")
	assert.Contains(t, body, `fc_push_ingest_items_total{application="shop",result="accepted"} 3`)
	assert.Contains(t, body, "go_goroutines", "the platform's own series are still served")
}
