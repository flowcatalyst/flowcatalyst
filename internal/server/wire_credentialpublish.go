package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// newCredentialPublisher builds the background worker that writes published
// credentials to the store (docs/credential-publishing.md). Leader-gated like
// the other single-active subsystems. The AWS client is built from the default
// credential chain; no call is made until the first item is due.
func newCredentialPublisher(ctx context.Context, pool *pgxpool.Pool, cfg EnvCfg) (*credentialpublish.Worker, error) {
	api, err := credentialpublish.NewSSMClient(ctx)
	if err != nil {
		return nil, err
	}
	oauthClients := auth.NewRepository(pool).OAuthClients
	return &credentialpublish.Worker{
		Repo:  credentialpublish.NewRepository(pool),
		Store: credentialpublish.NewParameterStore(api, cfg.CredentialPublish.KMSKey),
		Values: &credentialValues{
			accounts:   serviceaccount.NewRepository(pool),
			principals: principal.NewRepository(pool),
			clients:    oauthClients,
		},
		Enc:      encryption.MustFromEnv(),
		IsLeader: newLeaderGate(ctx, cfg, "credential-publish"),
	}, nil
}

// credentialValues reads the current plaintext of the credentials the platform
// can recover, for the worker (credentialpublish.ValueSource). The client
// secret is not among them: its plaintext travels in the queued item.
type credentialValues struct {
	accounts   *serviceaccount.Repository
	principals *principal.Repository
	clients    *auth.OAuthClientRepo
}

func (v *credentialValues) Value(ctx context.Context, owner credentialpublish.Owner, credential string) (string, bool, error) {
	switch owner.Type {
	case credentialpublish.OwnerServiceAccount:
		return v.accountValue(ctx, owner.ID, credential)
	case credentialpublish.OwnerOAuthClient:
		if credential != credentialpublish.CredClientID {
			return "", false, nil
		}
		c, err := v.clients.FindByID(ctx, owner.ID)
		if err != nil || c == nil {
			return "", false, err
		}
		return c.ClientID, c.ClientID != "", nil
	}
	return "", false, fmt.Errorf("unknown owner type %q", owner.Type)
}

func (v *credentialValues) accountValue(ctx context.Context, id, credential string) (string, bool, error) {
	switch credential {
	case credentialpublish.CredClientID:
		p, err := v.principals.FindByServiceAccount(ctx, id)
		if err != nil || p == nil {
			return "", false, err
		}
		cs, err := v.clients.FindByPrincipalID(ctx, string(p.ID))
		if err != nil || len(cs) == 0 {
			return "", false, err
		}
		return cs[0].ClientID, cs[0].ClientID != "", nil
	case credentialpublish.CredAuthToken, credentialpublish.CredSigningSecret:
		sa, err := v.accounts.FindByID(ctx, id)
		if err != nil || sa == nil {
			return "", false, err
		}
		creds := sa.WebhookCredentials
		ref := creds.SigningSecret
		if credential == credentialpublish.CredAuthToken {
			if creds.AuthType != serviceaccount.AuthBearer && creds.AuthType != serviceaccount.AuthAPIKey {
				return "", false, nil
			}
			ref = creds.Token
		}
		if serviceaccount.IsUndecryptable(ref) {
			return "", false, errors.New("the service account's stored " + credential + " cannot be decrypted")
		}
		if ref == nil || *ref == "" {
			return "", false, nil
		}
		return *ref, true, nil
	}
	// client-secret is held only as a hash; its plaintext is in the queued item.
	return "", false, nil
}
