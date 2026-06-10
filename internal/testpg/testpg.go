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
