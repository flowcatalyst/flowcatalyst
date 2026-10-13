// Command dispatch-jobs shows dispatch jobs declared on a use case's plan:
// one operation (IssueInvoice) saves an invoice and returns
// usecaseop.Save(...).Dispatch(...), so the invoice row, its event, two
// dispatch jobs and the audit row commit in ONE transaction, or none of them
// does.
//
// The rows land in outbox_messages in the contract's order (framework
// write-path 7a): the event, the jobs (in the order the plan lists them),
// the audit row. The platform's outbox processor then sends the
// DISPATCH_JOB rows to POST /api/dispatch-jobs/batch, and the platform
// delivers each job to its target URL.
//
// # Run
//
//	FC_DATABASE_URL=postgres://localhost:5432/billing go run ./examples/dispatch-jobs
//
// It issues one invoice and prints the outbox rows it wrote.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outbox"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxpgx"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

const app = "billing"

// ─── Aggregate + repository ──────────────────────────────────────────

type Invoice struct {
	ID          string
	Customer    string
	AmountCents int64
}

func (i Invoice) IDStr() string { return i.ID }

type InvoiceRepository struct{}

func (InvoiceRepository) Persist(ctx context.Context, i *Invoice, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO invoices (id, customer, amount_cents) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET customer = $2, amount_cents = $3`,
		i.ID, i.Customer, i.AmountCents)
	return err
}

func (InvoiceRepository) Delete(ctx context.Context, i *Invoice, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM invoices WHERE id = $1`, i.ID)
	return err
}

const schema = `CREATE TABLE IF NOT EXISTS invoices (
    id VARCHAR(17) PRIMARY KEY,
    customer TEXT NOT NULL,
    amount_cents BIGINT NOT NULL
)`

// ─── Domain event ────────────────────────────────────────────────────

type InvoiceIssued struct {
	Meta        usecase.EventMetadata
	InvoiceID   string
	AmountCents int64
}

func (e InvoiceIssued) EventID() string       { return e.Meta.EventID }
func (e InvoiceIssued) EventType() string     { return e.Meta.Type }
func (e InvoiceIssued) SpecVersion() string   { return e.Meta.SpecVersion }
func (e InvoiceIssued) Source() string        { return e.Meta.Source }
func (e InvoiceIssued) Subject() string       { return e.Meta.Subject }
func (e InvoiceIssued) Time() time.Time       { return e.Meta.OccurredAt }
func (e InvoiceIssued) PrincipalID() string   { return e.Meta.PrincipalID }
func (e InvoiceIssued) CorrelationID() string { return e.Meta.CorrelationID }
func (e InvoiceIssued) CausationID() string   { return e.Meta.CausationID }
func (e InvoiceIssued) ExecutionID() string   { return e.Meta.ExecutionID }
func (e InvoiceIssued) MessageGroup() string  { return e.Meta.MessageGroup }
func (e InvoiceIssued) ToDataJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"invoiceId": e.InvoiceID, "amountCents": e.AmountCents})
}

// ─── The operation ───────────────────────────────────────────────────

// IssueInvoiceCommand carries the customer's client (its identifier slug on
// the platform) and the URL their system wants to be told at.
type IssueInvoiceCommand struct {
	Customer    string `json:"customer"`
	ClientCode  string `json:"clientCode"`
	AmountCents int64  `json:"amountCents"`
	NotifyURL   string `json:"notifyUrl"`
}

func (IssueInvoiceCommand) AuditOperation() string { return "issue-invoice" }

// notifyJob is the job that tells the customer's system. Built from the
// command, so Validate checks it first: a bad URL is the caller's 400, not
// the plan's INTERNAL DISPATCH_JOB_INVALID.
func notifyJob(cmd IssueInvoiceCommand, invoiceID string) outbox.DispatchJob {
	return outbox.NewDispatchJob(app+":invoice:notify", cmd.NotifyURL).
		WithClientCode(cmd.ClientCode). // the platform resolves the code to its client (F14)
		WithMessageGroup("invoice:"+invoiceID).
		WithPayload(`<invoice id="`+invoiceID+`"/>`, "application/xml").
		WithMode(outbox.DispatchModeBlockOnError).
		WithMaxRetries(5).
		WithMetadata("invoice", invoiceID)
}

func IssueInvoice(repo InvoiceRepository) usecaseop.Operation[IssueInvoiceCommand, InvoiceIssued] {
	return usecaseop.Operation[IssueInvoiceCommand, InvoiceIssued]{
		Name: "IssueInvoice",
		Validate: func(_ context.Context, cmd IssueInvoiceCommand) error {
			if strings.TrimSpace(cmd.Customer) == "" || cmd.AmountCents <= 0 {
				return usecase.Validation("INVOICE_INVALID", "customer and a positive amountCents are required")
			}
			if err := notifyJob(cmd, "inv_check").Validate(); err != nil {
				return usecase.Validation("NOTIFY_INVALID", err.Error())
			}
			return nil
		},
		Authorize: usecaseop.Public[IssueInvoiceCommand],
		Execute: func(_ context.Context, cmd IssueInvoiceCommand, ec usecase.ExecutionContext) (usecaseop.Plan[InvoiceIssued], error) {
			inv := &Invoice{ID: tsid.GenerateWithPrefix("inv"), Customer: cmd.Customer, AmountCents: cmd.AmountCents}
			meta := usecase.NewEventMetadata(ec, usecase.BuildEventType(app, "billing", "invoice", "issued"),
				app+":billing", usecase.BuildSubject("billing", "invoice", inv.ID))
			meta.MessageGroup = usecase.BuildMessageGroup("billing", "invoice", inv.ID)
			event := InvoiceIssued{Meta: meta, InvoiceID: inv.ID, AmountCents: inv.AmountCents}

			// An internal task: export to the ledger. Kind TASK, source
			// "billing" and payload {} would be the defaults; this one sends
			// JSON. No client named: the job is platform-scoped.
			export, err := outbox.NewDispatchJob(app+":ledger:export", "https://ledger.example.com/exports").
				WithEventID(event.EventID()).
				WithJSONPayload(map[string]any{"invoiceId": inv.ID, "amountCents": inv.AmountCents})
			if err != nil {
				return nil, err
			}

			// Both jobs take the plan's correlation id (ec.CorrelationID).
			return usecaseop.Save(inv, repo, event).Dispatch(export, notifyJob(cmd, inv.ID)), nil
		},
	}
}

// ─── main ────────────────────────────────────────────────────────────

func main() {
	ctx := context.Background()
	dbURL := os.Getenv("FC_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("FC_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := outboxpgx.InitSchema(ctx, pool); err != nil { // use your migrations in production
		log.Fatal(err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		log.Fatal(err)
	}

	uow := usecasepgx.New(pool, outboxpgx.NewSink(outboxpgx.Config{ApplicationCode: app, AuditEnabled: true}))
	ec := usecase.NewExecutionContext("prn_demo")
	cmd := IssueInvoiceCommand{Customer: "Acme", ClientCode: "acme", AmountCents: 4250, NotifyURL: "https://acme.example.com/hooks/invoices"}
	event, err := usecaseop.Run(ctx, uow, IssueInvoice(InvoiceRepository{}), cmd, ec)
	if err != nil {
		log.Fatal(err)
	}

	rows, err := pool.Query(ctx, `SELECT type, id, payload FROM outbox_messages ORDER BY created_at`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	fmt.Printf("issued %s; outbox rows:\n", event.InvoiceID)
	for rows.Next() {
		var typ, id, payload string
		if err := rows.Scan(&typ, &id, &payload); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-12s %s %s\n", typ, id, payload)
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
}
