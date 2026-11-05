//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// TestValidateToken_ReturnsTheAccountEmail pins the email on a valid token's
// validate answer (user and portal identity alike): the set-password page puts
// it in an autocomplete="username" field so a password manager saves the new
// password against the right account. An invalid token carries none.
func TestValidateToken_ReturnsTheAccountEmail(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	principals := principal.NewRepository(pool)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	tokens := passwordreset.NewRepository(pool)
	uow := testpg.NewUoW(t)

	user, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(principals),
		principalops.CreateCommand{Email: "validate-email@example.com", Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Validate Email Co", Identifier: "validate-email-co"}, testpg.TestEC())
	require.NoError(t, err)
	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients, nil),
		portalidentity.EnsureCommand{ClientID: clientEv.ClientID, Email: "portal-validate@example.com", Source: "INVITE"},
		testpg.TestEC())
	require.NoError(t, err)

	s := &State{Principals: principals, Tokens: tokens, PortalIdentities: identities, UoW: uow}

	validate := func(t *testing.T, raw string) validateTokenResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		s.validateToken(rec, httptest.NewRequest(http.MethodGet,
			"/auth/password-reset/validate?token="+url.QueryEscape(raw), nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var out validateTokenResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}
	mint := func(t *testing.T, subject string) string {
		t.Helper()
		raw, terr := generateRawToken()
		require.NoError(t, terr)
		require.NoError(t, tokens.Insert(ctx, passwordreset.New(subject, hashToken(raw), time.Now().UTC().Add(15*time.Minute))))
		return raw
	}

	got := validate(t, mint(t, string(user.UserID)))
	assert.True(t, got.Valid)
	assert.Equal(t, "validate-email@example.com", got.Email)

	got = validate(t, mint(t, identEv.IdentityID))
	assert.True(t, got.Valid)
	assert.True(t, got.Portal)
	assert.Equal(t, "portal-validate@example.com", got.Email)

	got = validate(t, "not-a-token")
	assert.False(t, got.Valid)
	assert.Empty(t, got.Email)
}
