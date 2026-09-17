//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestGetServiceAccount_OAuthClientID pins B1 (docs/spec/login-attempt-links.md,
// T1): the single-account read must surface the OAuth client's PUBLIC
// client_id — the value a SERVICE_ACCOUNT_TOKEN login attempt's `identifier`
// actually carries — never the OAuth client row's own id, which is a
// different string entirely and would resolve nothing on the wire.
func TestGetServiceAccount_OAuthClientID(t *testing.T) {
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.CreateCommand{Code: "oauth-link-sa", Name: "OAuth Link SA"}, testpg.TestEC())
	require.NoError(t, err)
	require.NotEqual(t, res.OAuthClientRowID, res.OAuthClientID,
		"fixture sanity: the row id and the public client_id must be different strings")

	s := &State{Repo: saRepo, Principals: principals, OAuthClients: oauthRepo}
	out, err := s.getByID(testpg.AnchorCtx(), &apicommon.IDInput{ID: res.ServiceAccount.ID})
	require.NoError(t, err)
	require.NotNil(t, out.Body.OAuthClientID, "a provisioned account must report its client's public id")
	assert.Equal(t, res.OAuthClientID, *out.Body.OAuthClientID,
		"oauthClientId must be the public client_id, the value /oauth/token callers send")
	assert.NotEqual(t, res.OAuthClientRowID, *out.Body.OAuthClientID,
		"oauthClientId must NOT be the OAuth client row's own id")
}

// TestListServiceAccounts_OmitsOAuthClientID pins T2: the list endpoint never
// carries oauthClientId, matching the existing principalId omission — no
// per-row OAuth-client lookup on a list read.
func TestListServiceAccounts_OmitsOAuthClientID(t *testing.T) {
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.CreateCommand{Code: "oauth-list-sa", Name: "OAuth List SA"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Repo: saRepo, Principals: principals, OAuthClients: oauthRepo}
	out, err := s.list(testpg.AnchorCtx(), &apicommon.Empty{})
	require.NoError(t, err)

	found := false
	for _, item := range out.Body.ServiceAccounts {
		if item.ID != res.ServiceAccount.ID {
			continue
		}
		found = true
		assert.Nil(t, item.OAuthClientID, "list items must never carry oauthClientId")
	}
	require.True(t, found, "fixture sanity: the created account must appear in the list")
}

// TestGetServiceAccount_NoLinkedOAuthClient_OAuthClientIDAbsent pins T3: when
// the linked principal has no OAuth client at all, the field is ABSENT
// (nil), never present-but-empty — a caller that checks for the field's
// presence must not be fooled by an empty string standing in for "none".
func TestGetServiceAccount_NoLinkedOAuthClient_OAuthClientIDAbsent(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.CreateCommand{Code: "oauth-orphan-sa", Name: "OAuth Orphan SA"}, testpg.TestEC())
	require.NoError(t, err)

	// Sever the link: delete the provisioned OAuth client row outright, so
	// the account's principal has none — the same shape as a service account
	// whose OAuth client was independently removed.
	_, err = pool.Exec(ctx, `DELETE FROM oauth_clients WHERE id = $1`, res.OAuthClientRowID)
	require.NoError(t, err)

	s := &State{Repo: saRepo, Principals: principals, OAuthClients: oauthRepo}
	out, err := s.getByID(testpg.AnchorCtx(), &apicommon.IDInput{ID: res.ServiceAccount.ID})
	require.NoError(t, err)
	assert.Nil(t, out.Body.OAuthClientID, "no linked OAuth client must render as an absent field")
}
