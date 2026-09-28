//go:build integration

package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// testTokenIssuer mints RS256 bearer tokens the SAME way the platform's own
// Authenticator middleware validates them (provider.Provider +
// sessiontoken), without going through the full OAuth client_credentials
// HTTP flow — the middleware's bearer path is stateless JWT verification
// (see provider.Provider.ValidateSessionToken / middleware.introspect), so
// this is a faithful "real token", not a shortcut around auth.
type testTokenIssuer struct {
	provider *provider.Provider
	issuer   string
}

func newTestTokenIssuer(t *testing.T) *testTokenIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	pool := testpg.Pool(t)
	issuer := "https://fn-control-e2e.test"
	prov, err := provider.NewProvider(provider.Config{
		Issuer: issuer, Audience: issuer, AccessTokenTTL: time.Hour, SigningKey: pemBytes,
	}, principal.NewRepository(pool), role.NewRepository(pool))
	require.NoError(t, err)
	return &testTokenIssuer{provider: prov, issuer: issuer}
}

// mint issues a real bearer token for a service-account-shaped principal
// holding perms, signed with this issuer's key — the exact key the
// Authenticator middleware wired to the same provider validates against.
func (ti *testTokenIssuer) mint(t *testing.T, principalID string, perms []string) string {
	t.Helper()
	tok, err := sessiontoken.Mint(sessiontoken.Claims{
		Subject:     principalID,
		Scope:       "ANCHOR", // rides the "tier" claim
		Permissions: perms,
	}, ti.provider.SigningKey(), ti.issuer, time.Hour)
	require.NoError(t, err)
	return tok
}

// newTestServer mounts s's control-plane routes behind the platform's real
// Authenticator middleware on a live httptest.Server.
func newTestServer(t *testing.T, s *State, ti *testTokenIssuer) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	r.Use(platformmw.Authenticator(platformmw.AuthConfig{Provider: ti.provider}))
	humaCfg := apiroute.PlatformAPIConfig("fn-control-e2e", "test")
	api := humachi.New(r, humaCfg)
	Register(api, s)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// TestE2E_ClientAgainstRealServer runs internal/functions/control.Client —
// the runner's own client — against an httptest server mounting these
// routes, authenticated with a real token minted for a principal holding
// platform:function:runner:control: Desired, Heartbeat, and Artifact all
// round-trip over real HTTP.
func TestE2E_ClientAgainstRealServer(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, s, ti)

	token := ti.mint(t, "sa_fn_runner_e2e", []string{"platform:function:runner:control"})
	client := fncontrol.NewClient(srv.URL, func(context.Context) (string, error) { return token, nil }, nil)

	poolName := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "e2eapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &poolName })
	body := []byte("\x00asm-e2e-bytes")
	digest := sha256Hex(body)
	require.NoError(t, s.Artifacts.Put(context.Background(), digest, bytes.NewReader(body)))
	live := seedVersionWithDigest(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t), digest)
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	ctx := context.Background()
	doc, etag, changed, err := client.Desired(ctx, poolName, "", 0)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.NotEmpty(t, etag)
	require.Len(t, doc.Functions, 1)
	assert.Equal(t, fn.Address, doc.Functions[0].Address)

	_, _, changed2, err := client.Desired(ctx, poolName, etag, 0)
	require.NoError(t, err)
	assert.False(t, changed2, "an unchanged ETag round trip must answer 304 / changed=false")

	require.NoError(t, client.Heartbeat(ctx, fncontrol.Heartbeat{
		RunnerID: "runner-e2e", Pool: poolName, StartedAt: time.Now().UTC(),
	}))

	rc, err := client.Artifact(ctx, live.Digest)
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}
