//go:build integration

// Package testpg is the shared embedded-Postgres fixture for integration
// tests (build tag `integration`, run via `make test-integration`). It boots
// ONE embedded Postgres per test binary, applies the full migration set, and
// hands out a shared pgxpool.
//
// Usage — two lines per package:
//
//	//go:build integration
//	package mypkg
//
//	func TestMain(m *testing.M) { testpg.RunMain(m) }
//
//	func TestSomething(t *testing.T) {
//	    pool := testpg.Pool(t)
//	    ...
//	}
//
// Isolation model: there is NO between-test truncation — migrations seed
// bootstrap rows (permissions, platform config) that tests rely on, so a
// blanket TRUNCATE would do more harm than good. Tests must seed their own
// rows under fresh TSIDs and assert on that subset, never on table-wide
// counts. (Same discipline as the pre-fixture embedded-PG tests.)
//
// The Postgres port is allocated dynamically so test binaries for different
// packages can run concurrently; even so, each embedded instance is a real
// Postgres boot (~2-4s) plus a one-time binary download on first ever run —
// keep integration tests to behaviors that genuinely need the database.
package testpg

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/migrate"
)

var (
	mu   sync.Mutex
	pool *pgxpool.Pool
)

// RunMain is the TestMain adapter: boots embedded Postgres, runs the
// migrations, executes the package's tests, and tears the instance down.
// The non-zero exit path matters — without the explicit teardown the
// postgres child process would outlive the test binary.
func RunMain(m *testing.M) {
	code := run(m)
	os.Exit(code)
}

func run(m *testing.M) int {
	ctx := context.Background()

	tmp, err := os.MkdirTemp("", "fc-testpg-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: temp dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: allocate port: %v\n", err)
		return 1
	}

	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(uint32(port)).
		DataPath(filepath.Join(tmp, "data")).
		RuntimePath(filepath.Join(tmp, "runtime")).
		Username("postgres").Password("postgres").Database("flowcatalyst").
		StartTimeout(90 * time.Second))
	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: start embedded postgres: %v\n", err)
		return 1
	}
	defer func() { _ = pg.Stop() }()

	p, err := pgxpool.New(ctx, fmt.Sprintf(
		"postgresql://postgres:postgres@localhost:%d/flowcatalyst?sslmode=disable", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: connect: %v\n", err)
		return 1
	}
	defer p.Close()

	if err := migrate.Run(ctx, p); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: migrate: %v\n", err)
		return 1
	}

	mu.Lock()
	pool = p
	mu.Unlock()

	return m.Run()
}

// Pool returns the shared, fully-migrated pool. Fails the test when the
// package forgot the TestMain hookup.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if pool == nil {
		t.Fatal("testpg: pool not initialized — add `func TestMain(m *testing.M) { testpg.RunMain(m) }` to this package's integration test file")
	}
	return pool
}

// WithConstraintDropped runs fn with the named CHECK constraint on table
// temporarily removed, then restores it. This is how an X-06 "corrupt row
// fails loudly at read time" test seeds the corrupt row in the first
// place: since migration 051 (and the serviceaccount/loginattempt phase-1
// constraints), the enum columns these tests target are now guarded by a
// CHECK constraint at the write boundary, so a plain INSERT of a bad value
// is rejected before the read-boundary code under test ever sees it.
// Dropping the constraint for the duration of the seed insert honestly
// simulates the scenario the read-boundary check exists for: a row written
// before the constraint existed (or one that arrives via direct DBA
// action, or survives the constraint later being dropped) — legacy or
// out-of-band corruption, not a new write through the app.
//
// The constraint is restored via t.Cleanup — including on a panic or
// t.Fatal in fn — so a failing test can't leave the table unconstrained
// for whatever runs next.
//
// NOT safe under t.Parallel(): every package using this helper shares one
// pgxpool against one embedded Postgres instance (see the package doc),
// and the constraint is genuinely off table-wide for the window between
// the DROP and the deferred ADD. A concurrently running parallel test that
// writes to the same table during that window could either slip an
// unconstrained row past validation or itself fail if it depended on the
// constraint being enforced. Callers MUST NOT call t.Parallel() in a test
// (or any test in the same package that touches the same table) that uses
// this helper.
func WithConstraintDropped(t *testing.T, pool *pgxpool.Pool, table, constraint string, fn func()) {
	t.Helper()
	ctx := context.Background()

	var definition string
	if err := pool.QueryRow(ctx,
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`,
		constraint).Scan(&definition); err != nil {
		t.Fatalf("with_constraint_dropped: look up %s: %v", constraint, err)
	}

	// #nosec G202 -- table/constraint are hardcoded test-file constants, never
	// user input; pg_get_constraintdef's output is Postgres-quoted already.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT %s`, table, constraint)); err != nil {
		t.Fatalf("with_constraint_dropped: drop %s: %v", constraint, err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(
			`ALTER TABLE %s ADD CONSTRAINT %s %s`, table, constraint, definition)); err != nil {
			t.Errorf("with_constraint_dropped: restore %s: %v", constraint, err)
		}
	})

	fn()
}

// freePort grabs an ephemeral TCP port from the kernel and releases it for
// the embedded instance. The tiny claim-to-bind race window is acceptable
// for test infrastructure.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// SyncDispatchQueue makes msg_dispatch_queue agree with the CURRENT msg_dispatch_jobs
// rows of ids, for a test that wrote the job table directly (a raw INSERT, UPDATE
// or DELETE the dispatch-job lifecycle did not see): a PENDING job gets exactly one
// queue row mirroring it, any other job — or a missing one —
// gets none. Production code never does this; the lifecycle keeps the queue exact
// in the same statement as every job change. Call it after the raw write.
func SyncDispatchQueue(t testing.TB, p *pgxpool.Pool, ids ...string) {
	t.Helper()
	if len(ids) == 0 {
		return
	}
	ctx := context.Background()
	// Not PENDING (or missing): no queue row. PENDING: exactly one, upserted — a
	// sweep running in another test (the reaper, stale recovery) may enter the same
	// job at the same moment.
	if _, err := p.Exec(ctx, `
		DELETE FROM msg_dispatch_queue q WHERE q.job_id = ANY($1::text[])
		   AND NOT EXISTS (SELECT 1 FROM msg_dispatch_jobs j
		                    WHERE j.id = q.job_id AND j.created_at = q.job_created_at AND j.status = 'PENDING')`, ids); err != nil {
		t.Fatalf("testpg: sync dispatch queue (delete): %v", err)
	}
	if _, err := p.Exec(ctx, `
		INSERT INTO msg_dispatch_queue (job_id, job_created_at, message_group, sequence, scheduled_for,
		       subscription_id, dispatch_pool_id, client_id, mode, queue, version)
		SELECT id, created_at, message_group, sequence, scheduled_for, subscription_id,
		       dispatch_pool_id, client_id, mode, queue, updated_at
		  FROM msg_dispatch_jobs WHERE id = ANY($1::text[]) AND status = 'PENDING'
		ON CONFLICT (job_id) DO UPDATE SET job_created_at = EXCLUDED.job_created_at,
		       message_group = EXCLUDED.message_group, sequence = EXCLUDED.sequence,
		       scheduled_for = EXCLUDED.scheduled_for, subscription_id = EXCLUDED.subscription_id,
		       dispatch_pool_id = EXCLUDED.dispatch_pool_id, client_id = EXCLUDED.client_id,
		       mode = EXCLUDED.mode, queue = EXCLUDED.queue, version = EXCLUDED.version`, ids); err != nil {
		t.Fatalf("testpg: sync dispatch queue (insert): %v", err)
	}
}

// ScratchDB creates a fresh, fully migrated database beside the shared one and
// returns a pool on it, dropped when the test ends. For tests that need a table
// to themselves — plan tests that seed a few hundred thousand rows and control
// the statistics — without disturbing the rows every other test shares.
func ScratchDB(t *testing.T, name string) *pgxpool.Pool {
	t.Helper()
	return ScratchDBWith(t, name, nil)
}

// ScratchDBWith is ScratchDB with session settings applied to every connection of
// the returned pool (pgx RuntimeParams) — the scheduler's planner settings, for
// plan tests. The database is migrated over an unconfigured connection first.
func ScratchDBWith(t *testing.T, name string, runtimeParams map[string]string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	base := Pool(t)
	if _, err := base.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
		t.Fatalf("testpg: drop scratch database: %v", err)
	}
	if _, err := base.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("testpg: create scratch database: %v", err)
	}
	cfg := base.Config().Copy()
	cfg.ConnConfig.Database = name
	mp, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("testpg: connect to scratch database: %v", err)
	}
	if err := migrate.Run(ctx, mp); err != nil {
		mp.Close()
		t.Fatalf("testpg: migrate scratch database: %v", err)
	}
	mp.Close()
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	for k, v := range runtimeParams {
		cfg.ConnConfig.RuntimeParams[k] = v
	}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("testpg: connect to scratch database: %v", err)
	}
	t.Cleanup(func() {
		p.Close()
		_, _ = base.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return p
}
