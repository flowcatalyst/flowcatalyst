//go:build integration

package operations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMigration063_ScrubsSecretAndDBAuditValues(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	for i, row := range []string{
		`{"functionId":"fn_x","kind":"SECRET","key":"K","value":"leak1"}`,
		`{"functionId":"fn_x","kind":"DB","key":"d","value":"leak2"}`,
		`{"functionId":"fn_x","kind":"CONFIG","key":"c","value":"keep"}`,
	} {
		_, err := pool.Exec(ctx, `INSERT INTO aud_logs (id, entity_type, entity_id, operation, operation_json, performed_at) VALUES ($1,'function','fn_x','PutSettingCommand',$2::jsonb, now())`, "aud_scrubtest"+string(rune('a'+i))+"xx", row)
		require.NoError(t, err)
	}
	b, err := os.ReadFile("../../../migrate/sql/063_scrub_function_setting_audit.sql")
	require.NoError(t, err)
	up := strings.Split(strings.Split(string(b), "-- +goose Up")[1], "-- +goose Down")[0]
	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	var v []string
	rows, _ := pool.Query(ctx, `SELECT operation_json->>'value' FROM aud_logs WHERE entity_id='fn_x' ORDER BY id`)
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		v = append(v, s)
	}
	require.Equal(t, []string{"***", "***", "keep"}, v)
	_, _ = pool.Exec(ctx, `DELETE FROM aud_logs WHERE entity_id='fn_x'`)
}
