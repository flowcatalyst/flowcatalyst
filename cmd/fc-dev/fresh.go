package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/config"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
)

// freshTables is the explicit list of FlowCatalyst tables `fresh`
// truncates. Preserves the schema + the _fc_migrations tracker so the
// next boot doesn't reapply migrations. Order is leaf-tables-first so
// FK cascades don't trip.
var freshTables = []string{
	// Audit + event read tables (high-volume, leaf).
	"aud_logs",
	"msg_events_read",
	"msg_events",
	"msg_dispatch_jobs",
	"msg_dispatch_job_attempts",
	"msg_scheduled_job_instances",
	"msg_subscription_event_types",
	"msg_event_type_spec_versions",
	"msg_subscriptions",
	"msg_event_types",
	"msg_connections",
	"msg_dispatch_pools",
	"msg_scheduled_jobs",
	// OAuth + login state.
	"oauth_oidc_payloads",
	"oauth_oidc_login_states",
	"oauth_client_grant_types",
	"oauth_client_allowed_origins",
	"oauth_client_redirect_uris",
	"oauth_client_application_ids",
	"oauth_clients",
	"oauth_idp_role_mappings",
	"oauth_identity_provider_allowed_domains",
	"oauth_identity_providers",
	// Webauthn.
	"webauthn_credentials",
	// Auth tracking.
	"iam_login_attempts",
	"iam_password_reset_tokens",
	// IAM + tenancy relations. additional_client_ids and
	// granted_client_ids are JSONB columns on tnt_client_auth_configs,
	// not separate junction tables.
	"tnt_client_auth_configs",
	"tnt_anchor_domains",
	"iam_principal_application_access",
	"iam_client_access_grants",
	"iam_principal_roles",
	"iam_role_permissions",
	"iam_principals",
	"iam_roles",
	"oauth_idp_role_mappings",
	"oauth_clients",
	"tnt_cors_allowed_origins",
	"iam_service_accounts",
	"app_client_configs",
	"app_applications",
	"tnt_clients",
	// Platform config.
	"app_platform_config_access",
	"app_platform_configs",
}

// newFreshCmd truncates every FlowCatalyst table (preserving schema +
// migration tracker). Refuses to run without --yes. Idempotent.
func newFreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fresh",
		Short: "Truncate every FlowCatalyst table (preserves schema)",
		Long: `Removes every row from every FlowCatalyst table, returning the database
to an immediately-post-migration state. The _fc_migrations tracker is
preserved so the next boot skips re-migrating.

Refuses to run without --yes to prevent accidental data loss.`,
		RunE: runFresh,
	}
	cmd.Flags().String("database-url", envStrDefault("FC_DATABASE_URL", ""), "Postgres URL (defaults to local embedded)")
	cmd.Flags().Bool("yes", false, "confirm truncation (required)")
	return cmd
}

func runFresh(cmd *cobra.Command, _ []string) error {
	getStr := func(k string) string { v, _ := cmd.Flags().GetString(k); return v }
	getBool := func(k string) bool { v, _ := cmd.Flags().GetBool(k); return v }
	if !getBool("yes") {
		return errors.New("refusing to truncate without --yes")
	}
	url := getStr("database-url")
	if url == "" {
		url = "postgresql://postgres:postgres@localhost:5433/flowcatalyst?sslmode=disable"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := database.NewPool(ctx, config.DBConfig{URL: url})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	// Single TRUNCATE with CASCADE so FK ordering doesn't matter at the
	// SQL level. The explicit table list is still the source of truth
	// for which tables are "FlowCatalyst's" — anything not listed
	// belongs to a consumer app and is intentionally left alone.
	stmt := "TRUNCATE TABLE "
	for i, t := range freshTables {
		if i > 0 {
			stmt += ", "
		}
		stmt += t
	}
	stmt += " RESTART IDENTITY CASCADE"
	if _, err := pool.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	slog.Info("FlowCatalyst tables truncated", "table_count", len(freshTables))
	return nil
}
