package migrate

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// UpTo applies the shipped migrations through version v on pool's database,
// with no bootstrap, exactly as goose would on a database it owns. Test-only.
func UpTo(ctx context.Context, pool *pgxpool.Pool, v int64) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.SetBaseFS(migrationsFS)
	return goose.UpToContext(ctx, db, "sql", v)
}

// ProbeResults evaluates every effect probe on pool's database. Test-only.
func ProbeResults(ctx context.Context, t interface {
	Helper()
	Fatalf(string, ...any)
}, pool *pgxpool.Pool,
) map[int64]bool {
	t.Helper()
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	out := map[int64]bool{}
	for v := range effectProbes {
		ok, _, err := effectVisible(ctx, db, v)
		if err != nil {
			t.Fatalf("probe %d: %v", v, err)
		}
		out[v] = ok
	}
	return out
}
