//go:build integration

package migrate_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// upSection returns the Up half of a migration file.
func upSection(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile("sql/" + file)
	require.NoError(t, err)
	s := string(b)
	_, after, ok := strings.Cut(s, "-- +goose Up")
	require.True(t, ok)
	up, _, _ := strings.Cut(after, "-- +goose Down")
	return up
}

// Migration 076 (F18): a role assignment with whitespace in its name is
// re-written under the normalised name only when that names an existing role;
// anything else is left alone. Re-running changes nothing.
func TestMigration076_NormalisesOnlyUnambiguousRoleAssignments(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig076")

	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name, application_code) VALUES
		('rol_mig076real001', 'example:dispatcher', 'D', 'example'),
		('rol_mig076real002', 'example:viewer', 'V', 'example')`)
	require.NoError(t, err)
	for _, id := range []string{"prn_mig076aaaaa1", "prn_mig076aaaaa2", "prn_mig076aaaaa3"} {
		_, err = pool.Exec(ctx, `INSERT INTO iam_principals (id, type, name) VALUES ($1, 'SERVICE', $1)`, id)
		require.NoError(t, err)
	}
	ins := func(p, r, src string) {
		_, err := pool.Exec(ctx, `INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1,$2,$3)`, p, r, src)
		require.NoError(t, err)
	}
	ins("prn_mig076aaaaa1", "example: dispatcher", "ADMIN_ASSIGNED") // fixable
	ins("prn_mig076aaaaa2", "example: dispatcher", "ADMIN_ASSIGNED") // fixable, already holds the real one
	ins("prn_mig076aaaaa2", "example:dispatcher", "SDK_SYNC")
	ins("prn_mig076aaaaa3", " example :viewer ", "ADMIN_ASSIGNED")   // fixable (both ends)
	ins("prn_mig076aaaaa3", "example: nosuchrole", "ADMIN_ASSIGNED") // normalised role does not exist: left
	ins("prn_mig076aaaaa3", "exa mple:viewer", "ADMIN_ASSIGNED")     // whitespace inside a segment: left

	up := upSection(t, "076_normalise_role_assignment_names.sql")
	for range 2 { // idempotent
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}

	rows, err := pool.Query(ctx, `SELECT principal_id, role_name, assignment_source FROM iam_principal_roles ORDER BY 1,2`)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var p, r, s string
		require.NoError(t, rows.Scan(&p, &r, &s))
		got = append(got, p+"|"+r+"|"+s)
	}
	assert.Equal(t, []string{
		"prn_mig076aaaaa1|example:dispatcher|ADMIN_ASSIGNED",
		"prn_mig076aaaaa2|example:dispatcher|SDK_SYNC",
		"prn_mig076aaaaa3|exa mple:viewer|ADMIN_ASSIGNED",
		"prn_mig076aaaaa3|example: nosuchrole|ADMIN_ASSIGNED",
		"prn_mig076aaaaa3|example:viewer|ADMIN_ASSIGNED",
	}, got)
}

// Migration 077 (F18): application_id is set from the application whose code
// matches application_code, only where it is NULL; nothing is guessed.
func TestMigration077_BackfillsRoleApplicationID(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "mig077")

	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name) VALUES
		('app_mig077aaaaa1', 'mig077a', 'A'), ('app_mig077aaaaa2', 'platform', 'P')
		ON CONFLICT (code) DO NOTHING`)
	require.NoError(t, err)
	var platformID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM app_applications WHERE code='platform'`).Scan(&platformID))
	_, err = pool.Exec(ctx, `INSERT INTO iam_roles (id, application_id, name, display_name, application_code) VALUES
		('rol_mig077r00001', NULL, 'mig077a:one', 'one', 'mig077a'),
		('rol_mig077r00002', NULL, 'platform:admin', 'admin', 'platform'),
		('rol_mig077r00003', NULL, 'ghost:one', 'ghost', 'ghost'),
		('rol_mig077r00004', NULL, 'mig077a:nocode', 'nocode', NULL),
		('rol_mig077r00005', 'app_keepexisting1', 'mig077a:keep', 'keep', 'mig077a')`)
	require.NoError(t, err)

	up := upSection(t, "077_backfill_role_application_id.sql")
	for range 2 {
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
	}
	id := func(role string) *string {
		var v *string
		require.NoError(t, pool.QueryRow(ctx, `SELECT application_id FROM iam_roles WHERE name=$1`, role).Scan(&v))
		return v
	}
	require.NotNil(t, id("mig077a:one"))
	assert.Equal(t, "app_mig077aaaaa1", *id("mig077a:one"))
	require.NotNil(t, id("platform:admin"))
	assert.Equal(t, platformID, *id("platform:admin"))
	assert.Nil(t, id("ghost:one"), "no such application: left NULL, reported")
	assert.Nil(t, id("mig077a:nocode"), "no application_code: nothing guessed from the name")
	assert.Equal(t, "app_keepexisting1", *id("mig077a:keep"), "an existing id is never overwritten")
}
