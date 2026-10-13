// Command outbox-sql writes outbox rows through database/sql, for an
// application that does not use pgx (or runs on MySQL): an event, its audit
// row and a dispatch job, committed with the application's own write in one
// *sql.Tx. The rows are the same ones outboxpgx writes.
//
// # Run
//
//	FC_DATABASE_URL=postgres://localhost:5432/orders go run ./examples/outbox-sql
//
// For MySQL, open the database with go-sql-driver/mysql and pass
// outboxsql.DialectMySQL to both InitSchema and NewWriter.
package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outbox"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxsql"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
)

// invoiceIssued is a domain event implemented directly on a struct.
type invoiceIssued struct {
	id, invoiceID, principal, correlation string
	at                                    time.Time
}

func (e invoiceIssued) EventID() string       { return e.id }
func (e invoiceIssued) EventType() string     { return "billing:invoicing:invoice:issued" }
func (e invoiceIssued) SpecVersion() string   { return "1.0" }
func (e invoiceIssued) Source() string        { return "billing:invoicing" }
func (e invoiceIssued) Subject() string       { return "invoicing.invoice." + e.invoiceID }
func (e invoiceIssued) Time() time.Time       { return e.at }
func (e invoiceIssued) PrincipalID() string   { return e.principal }
func (e invoiceIssued) CorrelationID() string { return e.correlation }
func (e invoiceIssued) CausationID() string   { return "" }
func (e invoiceIssued) ExecutionID() string   { return "" }
func (e invoiceIssued) MessageGroup() string  { return "invoicing:invoice:" + e.invoiceID }
func (e invoiceIssued) ToDataJSON() ([]byte, error) {
	return []byte(`{"invoiceId":"` + e.invoiceID + `"}`), nil
}

type issueInvoice struct {
	InvoiceID string `json:"invoiceId"`
}

func (issueInvoice) AuditOperation() string { return "issue-invoice" }

func main() {
	ctx := context.Background()
	db, err := sql.Open("pgx", os.Getenv("FC_DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := outboxsql.InitSchema(ctx, db, outboxsql.DialectPostgres); err != nil { // use your migrations in production
		log.Fatal(err)
	}

	w, err := outboxsql.NewWriter(outboxsql.DialectPostgres, outboxsql.WriterConfig{
		ClientCode:      "acme", // the client's identifier slug
		ApplicationCode: "billing",
	})
	if err != nil {
		log.Fatal(err)
	}

	invoiceID := tsid.GenerateWithPrefix("inv")
	event := invoiceIssued{
		id: tsid.GenerateUntyped(), invoiceID: invoiceID,
		principal: "prn_demo", correlation: tsid.GenerateUntyped(), at: time.Now(),
	}
	job, err := outbox.NewDispatchJob("billing:invoicing:send-invoice", "https://mailer.example.com/send").
		WithClientCode("acme").
		WithEventID(event.EventID()).
		WithCorrelationID(event.CorrelationID()).
		WithJSONPayload(map[string]string{"invoiceId": invoiceID})
	if err != nil {
		log.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	// ... the application's own INSERT/UPDATE for the invoice goes here, on tx ...
	if err := w.WriteEvent(ctx, tx, event); err != nil {
		log.Fatal(err)
	}
	if err := w.WriteAudit(ctx, tx, event, issueInvoice{InvoiceID: invoiceID}); err != nil {
		log.Fatal(err)
	}
	if err := w.WriteDispatchJob(ctx, tx, job); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	log.Printf("invoice %s: event, audit row and dispatch job written to outbox_messages", invoiceID)
}
