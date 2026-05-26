package migrate

import "embed"

// Embedded migrations — bundled into every fc-server / fc-dev binary so
// you can deploy a single executable and apply schema on first boot.
//
//go:embed all:sql
var migrationsFS embed.FS

// Embedded returns the embedded migration set. fc-server / fc-dev call
// migrate.Run(ctx, pool, migrate.Embedded()).
func Embedded() Source {
	return FS{FS: migrationsFS, Root: "sql"}
}
