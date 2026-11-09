//go:build integration

package oauthapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// TestTokenExchangeRefusesPlainPKCE: an authorization code stored with
// code_challenge_method=plain is refused at /oauth/token even when the
// verifier equals the stored challenge (the match a plain-method server
// accepts). /oauth/authorize only mints S256 codes; the token endpoint must
// agree. The same verifier under S256 redeems.
func TestTokenExchangeRefusesPlainPKCE(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	principals := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)

	userEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(principals),
		principalops.CreateCommand{Email: "pkce-plain@token.test", Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)

	client := &auth.OAuthClient{
		ClientID: "pkce-app", Active: true,
		RedirectURIs: []string{"https://pkce.test/cb"},
		GrantTypes:   []string{"authorization_code"},
	}
	authCodes := grantstore.NewAuthorizationCodeRepository(pool)
	s := &State{
		OAuthClients:  fakeClientFinder{client: client},
		Principals:    principals,
		AuthCodes:     authCodes,
		RefreshTokens: grantstore.NewRefreshTokenRepository(pool),
		Auth:          testAuthService(t),
	}

	verifier := strings.Repeat("v", 50)
	mint := func(raw, challenge, method string) string {
		code := grantstore.NewAuthorizationCode(raw, "pkce-app", string(userEv.UserID), "https://pkce.test/cb")
		scope := "openid"
		code.Scope = &scope
		code.CodeChallenge = &challenge
		code.CodeChallengeMethod = &method
		require.NoError(t, authCodes.Insert(ctx, code))
		return raw
	}
	redeem := func(code string) (int, string) {
		rr := doTokenRequest(t, s, url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {code},
			"redirect_uri":  {"https://pkce.test/cb"},
			"client_id":     {"pkce-app"},
			"code_verifier": {verifier},
		})
		return rr.Code, rr.Body.String()
	}

	status, body := redeem(mint("pkce-plain-code-xxxxxxxxxxxxxxxxxxxxxxxx", verifier, "plain"))
	assert.Equal(t, 400, status, body)
	assert.Contains(t, body, "invalid_grant")

	sum := sha256.Sum256([]byte(verifier))
	status, body = redeem(mint("pkce-s256-code-xxxxxxxxxxxxxxxxxxxxxxxxx", base64.RawURLEncoding.EncodeToString(sum[:]), "S256"))
	assert.Equal(t, 200, status, body)
}
