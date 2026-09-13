package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// routerLocalClientID is the fixed client_id for the dev router's OAuth
// client. Stable so the row is found and updated rather than duplicated.
const routerLocalClientID = "fcdev-router"

// routerCredentials is what the dev router authenticates with.
type routerCredentials struct {
	ClientID string
	Secret   string
}

// bootstrapRouterCredentials provisions the credential the dev router fetches
// its configuration document with, so `fcdev start --router` needs no setup.
//
// The document is an ordinary authenticated API route, so dev needs a real
// credential like any deployment — the difference is that dev mints its own
// rather than an operator provisioning one.
//
// Idempotent by client id: the principal and OAuth client are created once and
// updated afterwards. The SECRET is fresh every boot and is never written to
// disk — it lives only in this process's environment, which is why nothing
// needs to decrypt it across restarts.
//
// The principal is anchor-scoped and holds exactly platform:router, matching
// what a deployed router's service account is given: reach over every client's
// queues, authority to read the dispatch configuration and nothing else. No
// service-account row is created — the router is not an SDK integration, and
// the linked principal is all token minting reads.
func bootstrapRouterCredentials(ctx context.Context, pool *pgxpool.Pool) (routerCredentials, error) {
	var zero routerCredentials

	enc, err := encryption.FromEnv()
	if err != nil {
		return zero, fmt.Errorf("init encryption: %w", err)
	}
	if enc == nil {
		return zero, fmt.Errorf("FLOWCATALYST_APP_KEY not set; cannot encrypt the router client secret")
	}
	secret, err := generateSecret()
	if err != nil {
		return zero, err
	}
	secretRef, err := enc.Encrypt(secret)
	if err != nil {
		return zero, fmt.Errorf("encrypt router client secret: %w", err)
	}

	authRepo := auth.NewRepository(pool)
	principalRepo := principal.NewRepository(pool)

	existing, err := authRepo.OAuthClients.FindByClientID(ctx, routerLocalClientID)
	if err != nil {
		return zero, fmt.Errorf("look up router client: %w", err)
	}

	if existing != nil {
		// Re-secret the existing client in place: the principal, its role and
		// the client row are all still correct.
		existing.SecretRef = &secretRef
		if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
			return authRepo.OAuthClients.Persist(ctx, existing, tx)
		}); err != nil {
			return zero, fmt.Errorf("re-secret router client: %w", err)
		}
		slog.Info("refreshed the dev router credential", "client_id", routerLocalClientID)
		return routerCredentials{ClientID: routerLocalClientID, Secret: secret}, nil
	}

	routerPrincipal := principal.NewService("", "fcdev router")
	routerPrincipal.ServiceAccountID = nil
	routerPrincipal.Scope = principal.ScopeAnchor

	oauthClient := auth.NewOAuthClient(routerLocalClientID, "FlowCatalyst Router (local dev)", auth.OAuthClientConfidential)
	oauthClient.SecretRef = &secretRef
	oauthClient.GrantTypes = []string{"client_credentials"}
	oauthClient.PrincipalID = &routerPrincipal.ID

	if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
		if err := principalRepo.Persist(ctx, routerPrincipal, tx); err != nil {
			return fmt.Errorf("router principal: %w", err)
		}
		if err := authRepo.OAuthClients.Persist(ctx, oauthClient, tx); err != nil {
			return fmt.Errorf("router oauth client: %w", err)
		}
		// principal.Persist does not sync iam_principal_roles, so the role is
		// written directly — it is the whole authority this credential has.
		_, err := tx.Inner().Exec(ctx,
			`INSERT INTO iam_principal_roles
			     (principal_id, role_name, assignment_source, assigned_at)
			 VALUES ($1, 'platform:router', 'BOOTSTRAP', NOW())
			 ON CONFLICT DO NOTHING`,
			routerPrincipal.ID)
		return err
	}); err != nil {
		return zero, err
	}

	slog.Info("bootstrapped the dev router credential", "client_id", routerLocalClientID)
	return routerCredentials{ClientID: routerLocalClientID, Secret: secret}, nil
}
