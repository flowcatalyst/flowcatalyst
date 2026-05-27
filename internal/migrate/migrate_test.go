package migrate

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

// TestMigrationsParse ensures every embedded migration file is something
// goose can parse — catches missing `-- +goose Up` headers, unbalanced
// StatementBegin/End blocks, and other structural problems without
// needing a database.
func TestMigrationsParse(t *testing.T) {
	require.NoError(t, goose.SetDialect("postgres"))
	goose.SetBaseFS(migrationsFS)

	entries, err := fs.ReadDir(migrationsFS, "sql")
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	migrations, err := goose.CollectMigrations("sql", 0, goose.MaxVersion)
	require.NoError(t, err, "goose failed to collect migrations from embedded FS")

	// Every .sql file should have been picked up.
	var sqlCount int
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqlCount++
		}
	}
	require.Equal(t, sqlCount, len(migrations),
		"goose found %d migrations but FS contains %d .sql files",
		len(migrations), sqlCount)
}
