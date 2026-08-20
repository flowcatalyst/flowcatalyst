//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestConfirmPortalReset_PolicyViolationIs400 pins the regression where a
// portal set-password with the user's name in the password returned a bare
// 500 (the *passwordpolicy.Violation was handed to httperror.Write
// unwrapped, which can't classify it). Policy violations must be 400s with
// their code; a compliant password must succeed.
func TestConfirmPortalReset_PolicyViolationIs400(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	tokens := passwordreset.NewRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Confirm Portal Co", Identifier: "confirm-portal-co"}, testpg.TestEC())
	require.NoError(t, err)

	name := "Bartholomew Kuznetsov"
	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients),
		portalidentity.EnsureCommand{
			ClientID: clientEv.ClientID, Email: "bart@confirmportal.test",
			Name: &name, Source: "INVITE",
		}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Tokens: tokens, PortalIdentities: identities, UoW: uow}

	confirm := func(t *testing.T, password string) *httptest.ResponseRecorder {
		t.Helper()
		// A fresh token per attempt: the failure paths under test must not
		// depend on earlier attempts having (not) consumed the token set.
		raw, terr := generateRawToken()
		require.NoError(t, terr)
		tok := passwordreset.New(identEv.IdentityID, hashToken(raw), time.Now().UTC().Add(15*time.Minute))
		require.NoError(t, tokens.Insert(ctx, tok))

		body, _ := json.Marshal(map[string]string{"token": raw, "password": password})
		rec := httptest.NewRecorder()
		s.confirmReset(rec, httptest.NewRequest(http.MethodPost,
			"/auth/password-reset/confirm", bytes.NewReader(body)))
		return rec
	}

	// Name in the password → 400 with the policy code, NOT a 500.
	rec := confirm(t, "Bartholomew-2026!")
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "PASSWORD_CONTAINS_IDENTITY")

	// Same for the email local part.
	rec = confirm(t, "bart@confirmportal.test")
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "PASSWORD_CONTAINS_IDENTITY")

	// Too short still reports its own code.
	rec = confirm(t, "ab")
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "PASSWORD_TOO_SHORT")

	// A compliant password succeeds and marks the confirm as portal.
	rec = confirm(t, "entirely-unrelated-9481!")
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"portal":true`)

	ident, err := identities.FindByID(ctx, identEv.IdentityID)
	require.NoError(t, err)
	require.NotNil(t, ident)
	assert.True(t, ident.CanSignInWithPassword(), "password hash stored")
}
