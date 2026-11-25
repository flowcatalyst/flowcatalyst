//go:build integration

package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestCredentialValuesReadWhatThePlatformCanRecover(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	v := &credentialValues{
		accounts:   serviceaccount.NewRepository(pool),
		principals: principal.NewRepository(pool),
		clients:    auth.NewRepository(pool).OAuthClients,
	}
	sa := tsid.Generate(tsid.ServiceAccount)
	prn := tsid.Generate(tsid.Principal)
	oac := tsid.Generate(tsid.OAuthClient)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_service_accounts (id, code, name, wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref)
		 VALUES ($1, $2, 'cv', 'BEARER_TOKEN', 'the-token', 'encrypted:not-decryptable')`, sa, "cv-"+sa)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO iam_principals (id, type, name, service_account_id) VALUES ($1, 'SERVICE', 'cv', $2)`, prn, sa)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO oauth_clients (id, client_id, client_name, client_type, service_account_principal_id)
		 VALUES ($1, $2, 'cv', 'CONFIDENTIAL', $3)`, oac, "cid-"+oac, prn)
	require.NoError(t, err)
	acct := credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: sa}

	val, ok, err := v.Value(ctx, acct, credentialpublish.CredClientID)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "cid-"+oac, val)

	val, ok, err = v.Value(ctx, acct, credentialpublish.CredAuthToken)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "the-token", val)

	// A signing secret that cannot be decrypted is an error to retry, never a value.
	_, ok, err = v.Value(ctx, acct, credentialpublish.CredSigningSecret)
	require.Error(t, err)
	assert.False(t, ok)

	// The client secret is never recoverable.
	_, ok, err = v.Value(ctx, acct, credentialpublish.CredClientSecret)
	require.NoError(t, err)
	assert.False(t, ok)

	// A stand-alone OAuth client: its id.
	val, ok, err = v.Value(ctx, credentialpublish.Owner{Type: credentialpublish.OwnerOAuthClient, ID: oac}, credentialpublish.CredClientID)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "cid-"+oac, val)

	// Gone owners are "not present".
	_, ok, err = v.Value(ctx, credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: "sac_gone"}, credentialpublish.CredAuthToken)
	require.NoError(t, err)
	assert.False(t, ok)
	_, ok, err = v.Value(ctx, credentialpublish.Owner{Type: credentialpublish.OwnerOAuthClient, ID: "oac_gone"}, credentialpublish.CredClientID)
	require.NoError(t, err)
	assert.False(t, ok)
}
