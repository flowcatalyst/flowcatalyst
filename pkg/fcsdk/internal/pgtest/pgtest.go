//go:build integration

// Package pgtest is the SDK module's embedded-Postgres fixture for
// integration tests (build tag integration). Each test package that needs a
// database boots one embedded Postgres in TestMain:
//
//	func TestMain(m *testing.M) { pgtest.Main(m) }
//
// and each test takes a pool on a fresh schema of its own, which holds the
// outbox table (Schema). The SDK imports nothing from the platform, so this
// is its own small copy of the platform's internal/testpg.
package pgtest

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxpgx"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
)

var (
	mu   sync.Mutex
	base *pgxpool.Pool
)

// Main boots embedded Postgres, runs the package's tests and stops it.
func Main(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	tmp, err := os.MkdirTemp("", "fcsdk-pgtest-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgtest: temp dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgtest: allocate port: %v\n", err)
		return 1
	}
	var log bytes.Buffer // Postgres's own output, shown only when it fails to start
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Logger(&log).
		Port(uint32(port)). // #nosec G115 -- a TCP port fits in uint32
		DataPath(filepath.Join(tmp, "data")).
		RuntimePath(filepath.Join(tmp, "runtime")).
		Username("postgres").Password("postgres").Database("fcsdk").
		StartTimeout(90 * time.Second))
	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "pgtest: start embedded postgres: %v\n%s", err, log.String())
		return 1
	}
	defer func() { _ = pg.Stop() }()

	p, err := pgxpool.New(ctx, fmt.Sprintf("postgresql://postgres:postgres@localhost:%d/fcsdk?sslmode=disable", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgtest: connect: %v\n", err)
		return 1
	}
	defer p.Close()

	mu.Lock()
	base = p
	mu.Unlock()
	return m.Run()
}

// Schema creates a fresh schema with the outbox table (outboxpgx.InitSchema)
// and returns a pool whose search_path is that schema. Dropped when the test
// ends.
func Schema(t *testing.T) *pgxpool.Pool {
	t.Helper()
	mu.Lock()
	b := base
	mu.Unlock()
	if b == nil {
		t.Fatal("pgtest: not started; add `func TestMain(m *testing.M) { pgtest.Main(m) }` to the package's integration test")
	}
	ctx := context.Background()
	name := "t_" + strings.ToLower(tsid.GenerateUntyped())
	if _, err := b.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		t.Fatalf("pgtest: create schema: %v", err)
	}
	cfg := b.Config().Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = name
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgtest: connect: %v", err)
	}
	t.Cleanup(func() {
		p.Close()
		_, _ = b.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+name+" CASCADE")
	})
	if err := outboxpgx.InitSchema(ctx, p); err != nil {
		t.Fatalf("pgtest: outbox schema: %v", err)
	}
	return p
}

// Row is one outbox_messages row as stored.
type Row struct {
	ID, Type, Payload      string
	MessageGroup, ClientID *string
	ErrorMessage, Headers  *string
	Status, RetryCount     int
	PayloadSize            *int
	CreatedAt, UpdatedAt   time.Time
}

// OutboxRows reads every outbox row in insertion order (the heap order of a
// table written by one test, ctid).
func OutboxRows(t *testing.T, p *pgxpool.Pool) []Row {
	t.Helper()
	rows, err := p.Query(context.Background(), `SELECT id, type, payload, message_group, client_id, error_message,
		headers::text, status, retry_count, payload_size, created_at, updated_at FROM outbox_messages ORDER BY ctid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Type, &r.Payload, &r.MessageGroup, &r.ClientID, &r.ErrorMessage,
			&r.Headers, &r.Status, &r.RetryCount, &r.PayloadSize, &r.CreatedAt, &r.UpdatedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
