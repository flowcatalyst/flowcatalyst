//go:build integration

package migrate_test

import (
	"context"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// foreignKey is one FOREIGN KEY constraint: the child table, the constraint
// name, and the parent table.
type foreignKey struct{ Table, Name, Parent string }

// allowedForeignKeys is every foreign key the schema may hold
// (docs/foreign-keys.md, owner ruling 2026-11-10): only between master and
// configuration tables. A transactional, high-volume, pruned or partitioned
// table carries none. Adding a foreign key means adding it here and to the
// doc's table, and it must be master data on both ends.
var allowedForeignKeys = []foreignKey{
	{"app_application_openapi_specs", "app_application_openapi_specs_application_id_fkey", "app_applications"},
	{"app_applications", "app_applications_service_account_fk", "iam_principals"},
	{"field_badges", "field_badges_identity_id_fkey", "field_identities"},
	{"field_device_apps", "field_device_apps_device_id_fkey", "field_devices"},
	{"field_identity_apps", "field_identity_apps_identity_id_fkey", "field_identities"},
	{"fng_aliases", "fng_aliases_function_id_fkey", "fng_functions"},
	{"fng_routes", "fng_routes_function_id_fkey", "fng_functions"},
	{"fng_settings", "fng_settings_function_id_fkey", "fng_functions"},
	{"fng_versions", "fng_versions_function_id_fkey", "fng_functions"},
	{"iam_principal_roles", "iam_principal_roles_principal_id_fkey", "iam_principals"},
	{"iam_role_permissions", "iam_role_permissions_role_id_fkey", "iam_roles"},
	{"iam_user_mfa_methods", "iam_user_mfa_methods_principal_id_fkey", "iam_principals"},
	{"iam_user_mfa_recovery_codes", "iam_user_mfa_recovery_codes_principal_id_fkey", "iam_principals"},
	{"ing_ingestions", "ing_ingestions_application_id_fkey", "app_applications"},
	{"ing_ingestions", "ing_ingestions_client_id_fkey", "tnt_clients"},
	{"ing_ingestions", "ing_ingestions_managed_by_fkey", "ing_sets"},
	{"ing_set_applies", "ing_set_applies_set_id_fkey", "ing_sets"},
	{"ing_state", "ing_state_ingestion_id_fkey", "ing_ingestions"},
	{"ing_versions", "ing_versions_ingestion_id_fkey", "ing_ingestions"},
	{"oauth_client_allowed_origins", "oauth_client_allowed_origins_oauth_client_id_fkey", "oauth_clients"},
	{"oauth_client_application_ids", "oauth_client_application_ids_oauth_client_id_fkey", "oauth_clients"},
	{"oauth_client_grant_types", "oauth_client_grant_types_oauth_client_id_fkey", "oauth_clients"},
	{"oauth_client_post_logout_redirect_uris", "fk_oauth_client_post_logout_redirect_uris_client", "oauth_clients"},
	{"oauth_client_redirect_uris", "oauth_client_redirect_uris_oauth_client_id_fkey", "oauth_clients"},
	{"oauth_clients", "oauth_clients_service_account_fk", "iam_principals"},
	{"portal_identity_apps", "portal_identity_apps_identity_id_fkey", "portal_identities"},
	{"portal_identity_apps", "portal_identity_apps_portal_app_id_fkey", "portal_apps"},
	{"push_app_settings", "push_app_settings_application_id_fkey", "app_applications"},
	{"push_rules", "push_rules_application_id_fkey", "app_applications"},
	{"push_ticket_keys", "push_ticket_keys_application_id_fkey", "app_applications"},
	{"push_trace_windows", "push_trace_windows_application_id_fkey", "app_applications"},
	{"webauthn_credentials", "webauthn_credentials_principal_id_fkey", "iam_principals"},
}

func listForeignKeys(t *testing.T, pool *pgxpool.Pool) []foreignKey {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT c.conrelid::regclass::text, c.conname, c.confrelid::regclass::text
		  FROM pg_constraint c
		 WHERE c.contype = 'f' AND c.connamespace = current_schema()::regnamespace
		 ORDER BY 1, 2`)
	require.NoError(t, err)
	defer rows.Close()
	var out []foreignKey
	for rows.Next() {
		var fk foreignKey
		require.NoError(t, rows.Scan(&fk.Table, &fk.Name, &fk.Parent))
		out = append(out, fk)
	}
	require.NoError(t, rows.Err())
	return out
}

// The fully migrated schema holds exactly the allowed foreign keys: none on a
// transactional table, and none added without a decision.
func TestForeignKeys_OnlyMasterDataKeysRemain(t *testing.T) {
	got := listForeignKeys(t, testpg.Pool(t))
	want := append([]foreignKey(nil), allowedForeignKeys...)
	sort.Slice(want, func(i, j int) bool {
		if want[i].Table != want[j].Table {
			return want[i].Table < want[j].Table
		}
		return want[i].Name < want[j].Name
	})
	assert.Equal(t, want, got, "foreign keys differ from the allowed list: see docs/foreign-keys.md")
}

// droppedBy114 is what migration 114 drops: the foreign keys whose child table
// is transactional.
var droppedBy114 = []foreignKey{
	{"field_sign_in_codes", "field_sign_in_codes_identity_id_fkey", "field_identities"},
	{"iam_mfa_email_pins", "iam_mfa_email_pins_principal_id_fkey", "iam_principals"},
	{"iam_mfa_trusted_devices", "iam_mfa_trusted_devices_principal_id_fkey", "iam_principals"},
	{"iam_reset_approval_requests", "iam_reset_approval_requests_principal_id_fkey", "iam_principals"},
	{"ing_leases", "ing_leases_ingestion_id_fkey", "ing_ingestions"},
	{"ing_route_states", "ing_route_states_ingestion_id_fkey", "ing_ingestions"},
	{"ing_secret_fetches", "ing_secret_fetches_ingestion_id_fkey", "ing_ingestions"},
	{"push_ingest_ids", "push_ingest_ids_application_id_fkey", "app_applications"},
}

// Migration 114 drops the transactional foreign keys and nothing else; its
// Down puts them back NOT VALID, so it succeeds over orphans written while
// they were gone; the Up runs again over that, and twice, without error.
func TestMigration114_DropsTheTransactionalForeignKeys(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig114")
	const file = "114_drop_transactional_foreign_keys.sql"
	// goose runs each migration in a transaction; SET LOCAL needs one.
	run := func(sql string) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		// Released on failure too: the scratch pool's Close waits for it.
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, sql)
		require.NoError(t, err)
		require.NoError(t, tx.Commit(ctx))
	}
	names := func(fks []foreignKey) map[string]bool {
		out := map[string]bool{}
		for _, fk := range fks {
			out[fk.Name] = true
		}
		return out
	}
	migrated := listForeignKeys(t, pool)
	for _, fk := range droppedBy114 {
		assert.False(t, names(migrated)[fk.Name], "%s is gone after the migration", fk.Name)
	}

	// An orphan written while the key was gone.
	_, err := pool.Exec(ctx, `INSERT INTO push_ingest_ids (application_id, push_id) VALUES ('app_mig114orphan', 'p1')`)
	require.NoError(t, err)
	run(downSection(t, file))
	restored := listForeignKeys(t, pool)
	assert.Len(t, restored, len(migrated)+len(droppedBy114), "the Down restores exactly the dropped keys")
	for _, fk := range droppedBy114 {
		assert.True(t, names(restored)[fk.Name], "%s is back after the Down", fk.Name)
		var valid bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT convalidated FROM pg_constraint WHERE conname = $1`, fk.Name).Scan(&valid))
		assert.False(t, valid, "%s is restored NOT VALID", fk.Name)
	}

	for range 2 {
		run(upSection(t, file))
	}
	assert.Equal(t, migrated, listForeignKeys(t, pool), "the Up, run again, leaves the migrated set")
}
