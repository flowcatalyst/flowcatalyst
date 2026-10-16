package seed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// SeedFcdevCLIClient installs (and on every boot re-asserts) the built-in
// PUBLIC OAuth client behind `fcdev login` (auth.FcdevCLIClientID).
//
// Security-relevant fields are overwritten on every start so a console edit
// can never widen them: public (no secret), PKCE required, apiAccess (the
// user's own roles become the token's authority), authorization_code +
// refresh_token only, the loopback redirects, no application scoping, not a
// portal client, no service-account link. Only `active` is left alone, so an
// operator can switch fcdev logins off by deactivating the client and a
// restart will not turn them back on.
func (s *Seeder) SeedFcdevCLIClient(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO oauth_clients
		    (id, client_id, client_name, client_type, client_secret_ref, default_scopes,
		     pkce_required, service_account_principal_id, active, created_at, updated_at,
		     portal_client_id, api_access, portal_app_id,
		     previous_secret_ref, previous_secret_expires_at, previous_secret_last_used_at)
		VALUES ($1, $2, $3, 'PUBLIC', NULL, NULL, TRUE, NULL, TRUE, $4, $4, NULL, TRUE, NULL, NULL, NULL, NULL)
		ON CONFLICT (client_id) DO UPDATE SET
		    client_name = EXCLUDED.client_name,
		    client_type = 'PUBLIC',
		    client_secret_ref = NULL,
		    previous_secret_ref = NULL,
		    previous_secret_expires_at = NULL,
		    previous_secret_last_used_at = NULL,
		    default_scopes = NULL,
		    pkce_required = TRUE,
		    service_account_principal_id = NULL,
		    portal_client_id = NULL,
		    portal_app_id = NULL,
		    api_access = TRUE,
		    updated_at = EXCLUDED.updated_at
		RETURNING id`,
		tsid.Generate(tsid.OAuthClient), auth.FcdevCLIClientID, auth.FcdevCLIClientName, now).Scan(&id)
	if err != nil {
		return fmt.Errorf("upsert fcdev-cli client: %w", err)
	}
	for _, t := range []string{
		"oauth_client_redirect_uris", "oauth_client_grant_types", "oauth_client_application_ids",
		"oauth_client_allowed_origins", "oauth_client_post_logout_redirect_uris",
	} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+t+" WHERE oauth_client_id = $1", id); err != nil {
			return fmt.Errorf("clear %s: %w", t, err)
		}
	}
	for _, u := range auth.FcdevCLIRedirectURIs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO oauth_client_redirect_uris (oauth_client_id, redirect_uri) VALUES ($1, $2)`, id, u); err != nil {
			return fmt.Errorf("insert redirect uri: %w", err)
		}
	}
	for _, g := range auth.FcdevCLIGrantTypes {
		if _, err := tx.Exec(ctx,
			`INSERT INTO oauth_client_grant_types (oauth_client_id, grant_type) VALUES ($1, $2)`, id, g); err != nil {
			return fmt.Errorf("insert grant type: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Debug("fcdev-cli OAuth client asserted")
	return nil
}
