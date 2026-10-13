//go:build integration

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/internal/pgtest"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxpgx"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { pgtest.Main(m) }

// The example's operation on embedded Postgres: the invoice, its event, both
// jobs (in plan order, with the plan's correlation id) and the audit row; a
// bad notify URL is the caller's validation error and writes nothing.
func TestIssueInvoiceWritesTheJobsWithTheEvent(t *testing.T) {
	pool := pgtest.Schema(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	uow := usecasepgx.New(pool, outboxpgx.NewSink(outboxpgx.Config{ApplicationCode: app, AuditEnabled: true}))
	ec := usecase.NewExecutionContext("prn_demo")

	bad := IssueInvoiceCommand{Customer: "Acme", ClientCode: "acme", AmountCents: 1, NotifyURL: "ftp://acme.example.com"}
	_, err := usecaseop.Run(ctx, uow, IssueInvoice(InvoiceRepository{}), bad, ec)
	if ue := usecase.AsError(err); ue == nil || ue.Code != "NOTIFY_INVALID" || ue.HTTPStatus() != 400 {
		t.Fatalf("bad notify URL: %v", err)
	}
	if rows := pgtest.OutboxRows(t, pool); len(rows) != 0 {
		t.Fatalf("a refused command wrote %d rows", len(rows))
	}

	cmd := IssueInvoiceCommand{Customer: "Acme", ClientCode: "acme", AmountCents: 4250, NotifyURL: "https://acme.example.com/hooks"}
	event, err := usecaseop.Run(ctx, uow, IssueInvoice(InvoiceRepository{}), cmd, ec)
	if err != nil {
		t.Fatal(err)
	}
	rows := pgtest.OutboxRows(t, pool)
	want := []string{"EVENT", "DISPATCH_JOB", "DISPATCH_JOB", "AUDIT_LOG"}
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %v", len(rows), want)
	}
	codes := []string{"", "billing:ledger:export", "billing:invoice:notify", ""}
	for i, r := range rows {
		if r.Type != want[i] {
			t.Fatalf("row %d is %s, want %s", i, r.Type, want[i])
		}
		if r.Type != "DISPATCH_JOB" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal([]byte(r.Payload), &p); err != nil {
			t.Fatal(err)
		}
		if p["code"] != codes[i] || p["correlationId"] != ec.CorrelationID || p["id"] != r.ID {
			t.Errorf("job %d: %s", i, r.Payload)
		}
	}
	if !json.Valid([]byte(rows[1].Payload)) || rows[2].MessageGroup == nil || *rows[2].MessageGroup != "invoice:"+event.InvoiceID {
		t.Errorf("notify job row: %+v", rows[2])
	}
}
