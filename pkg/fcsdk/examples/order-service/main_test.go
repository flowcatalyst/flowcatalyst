package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/go-sdk/outboxpgx"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Runs the example's operation and export against a real Postgres when
// FC_TEST_DATABASE_URL is set (CI's go-sdk workflow sets it); skipped
// otherwise. It uses its own schema, so any scratch database will do.
func TestOrderServiceWritesTheOutboxRows(t *testing.T) {
	url := os.Getenv("FC_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FC_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS fcsdk_example CASCADE`, `CREATE SCHEMA fcsdk_example`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	cfg := pool.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = "fcsdk_example"
	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := outboxpgx.InitSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}

	sink := outboxpgx.NewSink(outboxpgx.Config{ClientCode: "acme", ApplicationCode: app, AuditEnabled: true})
	uow := usecasepgx.New(pool, sink)
	ec := usecase.NewExecutionContext("prn_demo")

	if _, err := usecaseop.Run(ctx, uow, PlaceOrder(OrderRepository{}), PlaceOrderCommand{CustomerID: ""}, ec); err == nil {
		t.Fatal("an invalid command was accepted")
	}
	placed, err := usecaseop.Run(ctx, uow, PlaceOrder(OrderRepository{}), PlaceOrderCommand{CustomerID: "cus_42", TotalCents: 1500}, ec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := usecaseop.Run(ctx, uow, RequestExport(), RequestExportCommand{OrderID: placed.OrderID}, ec); err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, `SELECT type, id, payload FROM outbox_messages ORDER BY created_at, type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var typ, id, payload string
		if err := rows.Scan(&typ, &id, &payload); err != nil {
			t.Fatal(err)
		}
		counts[typ]++
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("%s payload: %v", typ, err)
		}
		switch typ {
		case "EVENT":
			if string(m["clientCode"]) != `"acme"` || m["data"][0] != '{' {
				t.Errorf("event payload: %s", payload)
			}
		case "AUDIT_LOG":
			if string(m["applicationCode"]) != `"orders"` || m["principalId"] == nil {
				t.Errorf("audit payload: %s", payload)
			}
		case "DISPATCH_JOB":
			if string(m["code"]) != `"orders:billing:export-order"` || id == "" {
				t.Errorf("dispatch job payload: %s", payload)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if counts["EVENT"] != 2 || counts["AUDIT_LOG"] != 2 || counts["DISPATCH_JOB"] != 1 {
		t.Fatalf("outbox rows: %v", counts)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM orders WHERE id = $1`, placed.OrderID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("order row: %d %v", n, err)
	}
}
