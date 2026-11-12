//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// GET .../secret is a read that writes an audit row and returns a plaintext
// secret, so a request authenticated by the session cookie must be a
// same-origin fetch; a cross-site navigation carrying the cookie is refused
// before anything is audited. Bearer callers are unaffected.
func TestRevealSecret_CookieRequiresSameOrigin(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	testpg.EnsureApplication(t, "pcapi-origin")
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	s := &State{Svc: &platformconfig.Service{
		Repo: platformconfig.NewRepository(pool).WithEncryption(enc), UoW: testpg.NewUoW(t), Clients: client.NewRepository(pool), Audit: audit.NewRepository(pool),
	}}
	admin := &auth.AuthContext{
		PrincipalID: "prn_pcapiorigin01", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:admin:config:update", "platform:admin:config:view", "platform:admin:config:reveal"},
	}
	plain := testpg.WithAuth(ctx, admin)
	secret := "SECRET"
	_, err = s.setProperty(plain, &setPropertyInput{App: "pcapi-origin", Section: "s", Property: "k", Body: SetPropertyRequest{Value: "the-secret", ValueType: &secret}})
	require.NoError(t, err)
	cfg, err := s.Svc.Repo.FindByCoordinate(ctx, "pcapi-origin", "s", "k", platformconfig.ScopeGlobal, nil)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	audited := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SECRET_REVEALED'`, cfg.ID).Scan(&n))
		return n
	}
	reveal := func(c context.Context, site string) (*revealOutput, error) {
		return s.revealSecret(c, &revealInput{App: "pcapi-origin", Section: "s", Property: "k", SecFetchSite: site})
	}
	cookie := middleware.WithSessionCookieAuth(plain)

	for _, site := range []string{"", "cross-site", "same-site", "none"} {
		_, err := reveal(cookie, site)
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, middleware.CodeCrossOriginRequest)
		assert.Zero(t, audited(), "Sec-Fetch-Site %q: no audit row", site)
	}

	out, err := reveal(cookie, "same-origin")
	require.NoError(t, err)
	assert.Equal(t, "the-secret", out.Body.Value)
	assert.Equal(t, 1, audited())

	// Bearer-marked and plain contexts: as before, whatever the header says.
	out, err = reveal(middleware.WithBearerAuth(plain), "cross-site")
	require.NoError(t, err)
	assert.Equal(t, "the-secret", out.Body.Value)
	_, err = reveal(plain, "")
	require.NoError(t, err)
	assert.Equal(t, 3, audited())
}
