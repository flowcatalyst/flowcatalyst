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
	return bootstrapLocalCredentials(ctx, pool, routerLocalClientID, "fcdev router", "FlowCatalyst Router (local dev)", "platform:router")
}

// bootstrapLocalCredentials provisions (or re-secrets) an anchor-scoped
// client_credentials principal holding exactly one role, for an in-process
// subsystem that authenticates to this fcdev's platform like any deployment
// would (the router, the function runner). See bootstrapRouterCredentials.
func bootstrapLocalCredentials(ctx context.Context, pool *pgxpool.Pool, clientID, principalName, clientName, role string) (routerCredentials, error) {
	var zero routerCredentials

	enc, err := encryption.FromEnv()
	if err != nil {
		return zero, fmt.Errorf("init encryption: %w", err)
	}
	if enc == nil {
		return zero, fmt.Errorf("FLOWCATALYST_APP_KEY not set; cannot encrypt the %s client secret", clientID)
	}
	secret, err := generateSecret()
	if err != nil {
		return zero, err
	}
	secretRef, err := enc.Encrypt(secret)
	if err != nil {
		return zero, fmt.Errorf("encrypt %s client secret: %w", clientID, err)
	}

	authRepo := auth.NewRepository(pool)
	principalRepo := principal.NewRepository(pool)

	existing, err := authRepo.OAuthClients.FindByClientID(ctx, clientID)
	if err != nil {
		return zero, fmt.Errorf("look up %s client: %w", clientID, err)
	}

	if existing != nil {
		// Re-secret the existing client in place: the principal, its role and
		// the client row are all still correct.
		existing.SecretRef = &secretRef
		if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
			return authRepo.OAuthClients.Persist(ctx, existing, tx)
		}); err != nil {
			return zero, fmt.Errorf("re-secret %s client: %w", clientID, err)
		}
		slog.Info("refreshed a dev credential", "client_id", clientID)
		return routerCredentials{ClientID: clientID, Secret: secret}, nil
	}

	routerPrincipal := principal.NewService("", principalName)
	routerPrincipal.ServiceAccountID = nil
	routerPrincipal.Scope = principal.ScopeAnchor

	oauthClient := auth.NewOAuthClient(clientID, clientName, auth.OAuthClientConfidential)
	oauthClient.SecretRef = &secretRef
	oauthClient.GrantTypes = []string{"client_credentials"}
	oauthClient.PrincipalID = &routerPrincipal.ID

	if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
		if err := principalRepo.Persist(ctx, routerPrincipal, tx); err != nil {
			return fmt.Errorf("%s principal: %w", clientID, err)
		}
		if err := authRepo.OAuthClients.Persist(ctx, oauthClient, tx); err != nil {
			return fmt.Errorf("%s oauth client: %w", clientID, err)
		}
		// principal.Persist does not sync iam_principal_roles, so the role is
		// written directly — it is the whole authority this credential has.
		_, err := tx.Inner().Exec(ctx,
			`INSERT INTO iam_principal_roles
			     (principal_id, role_name, assignment_source, assigned_at)
			 VALUES ($1, $2, 'BOOTSTRAP', NOW())
			 ON CONFLICT DO NOTHING`,
			routerPrincipal.ID, role)
		return err
	}); err != nil {
		return zero, err
	}

	slog.Info("bootstrapped a dev credential", "client_id", clientID, "role", role)
	return routerCredentials{ClientID: clientID, Secret: secret}, nil
}
