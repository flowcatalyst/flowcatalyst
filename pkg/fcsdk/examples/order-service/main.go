// Command order-service is the end-to-end example of a FlowCatalyst consumer
// application: one business operation (PlaceOrder) through the use-case
// envelope, its aggregate, domain event and audit row written to the outbox
// in one transaction, and a second operation whose plan carries a dispatch
// job, written in the same transaction as the event that asks for it.
//
// The platform's outbox processor (fcdev / fc-server with the outbox
// subsystem pointed at this database) then sends each outbox row to the
// platform: events to /api/events/batch, audit rows to
// /api/audit-logs/batch, dispatch jobs to /api/dispatch-jobs/batch.
//
// # Run
//
//	FC_DATABASE_URL=postgres://localhost:5432/orders \
//	FC_CLIENT_CODE=acme \
//	go run ./examples/order-service
//
//	curl -XPOST localhost:8081/orders -H 'X-Principal-ID: prn_demo' \
//	     -d '{"customerId":"cus_42","totalCents":1500}'
//	curl -XPOST localhost:8081/orders/<id>/export -H 'X-Principal-ID: prn_demo'
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
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

const app = "orders"

// ─── Aggregate + repository ──────────────────────────────────────────

// Order is the aggregate. It does not persist itself; OrderRepository does.
type Order struct {
	ID         string
	CustomerID string
	TotalCents int64
}

func (o Order) IDStr() string { return o.ID }

// OrderRepository implements usecasepgx.Persist[Order].
type OrderRepository struct{}

func (OrderRepository) Persist(ctx context.Context, o *Order, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO orders (id, customer_id, total_cents) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET customer_id = $2, total_cents = $3`,
		o.ID, o.CustomerID, o.TotalCents)
	return err
}

func (OrderRepository) Delete(ctx context.Context, o *Order, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM orders WHERE id = $1`, o.ID)
	return err
}

const schema = `CREATE TABLE IF NOT EXISTS orders (
    id VARCHAR(17) PRIMARY KEY,
    customer_id TEXT NOT NULL,
    total_cents BIGINT NOT NULL
)`

// ─── Domain events ───────────────────────────────────────────────────

// OrderPlaced is emitted when an order is accepted. It embeds the
// CloudEvents-style metadata and implements usecase.DomainEvent; ToDataJSON
// is the event's own data, written to the outbox as the JSON value itself.
type OrderPlaced struct {
	Meta       usecase.EventMetadata
	OrderID    string `json:"orderId"`
	CustomerID string `json:"customerId"`
	TotalCents int64  `json:"totalCents"`
}

func (e OrderPlaced) EventID() string       { return e.Meta.EventID }
func (e OrderPlaced) EventType() string     { return e.Meta.Type }
func (e OrderPlaced) SpecVersion() string   { return e.Meta.SpecVersion }
func (e OrderPlaced) Source() string        { return e.Meta.Source }
func (e OrderPlaced) Subject() string       { return e.Meta.Subject }
func (e OrderPlaced) Time() time.Time       { return e.Meta.OccurredAt }
func (e OrderPlaced) PrincipalID() string   { return e.Meta.PrincipalID }
func (e OrderPlaced) CorrelationID() string { return e.Meta.CorrelationID }
func (e OrderPlaced) CausationID() string   { return e.Meta.CausationID }
func (e OrderPlaced) ExecutionID() string   { return e.Meta.ExecutionID }
func (e OrderPlaced) MessageGroup() string  { return e.Meta.MessageGroup }
func (e OrderPlaced) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		OrderID    string `json:"orderId"`
		CustomerID string `json:"customerId"`
		TotalCents int64  `json:"totalCents"`
	}{e.OrderID, e.CustomerID, e.TotalCents})
}

// newMeta builds the metadata the framework convention prescribes: type
// {app}:{domain}:{aggregate}:{action}, source {app}:{domain}, subject
// {domain}.{aggregate}.{id}, message group {domain}:{aggregate}:{id}.
func newMeta(ec usecase.ExecutionContext, domain, aggregate, action, id string) usecase.EventMetadata {
	m := usecase.NewEventMetadata(ec,
		usecase.BuildEventType(app, domain, aggregate, action),
		app+":"+domain,
		usecase.BuildSubject(domain, aggregate, id))
	m.MessageGroup = usecase.BuildMessageGroup(domain, aggregate, id)
	return m
}

// ─── The operation ───────────────────────────────────────────────────

// PlaceOrderCommand is the command; its audit row records it redacted, under
// the operation name it gives.
type PlaceOrderCommand struct {
	CustomerID string `json:"customerId"`
	TotalCents int64  `json:"totalCents"`
}

func (PlaceOrderCommand) AuditOperation() string { return "place-order" }

// PlaceOrder is one business operation: validate, authorize, then return a
// Plan that usecaseop.Run applies atomically (order row + event + audit).
func PlaceOrder(repo OrderRepository) usecaseop.Operation[PlaceOrderCommand, OrderPlaced] {
	return usecaseop.Operation[PlaceOrderCommand, OrderPlaced]{
		Name: "PlaceOrder",
		Validate: func(_ context.Context, cmd PlaceOrderCommand) error {
			if strings.TrimSpace(cmd.CustomerID) == "" {
				return usecase.Validation("CUSTOMER_REQUIRED", "customerId is required")
			}
			if cmd.TotalCents <= 0 {
				return usecase.Validation("TOTAL_INVALID", "totalCents must be positive")
			}
			return nil
		},
		// The transport already authenticated the caller; any authenticated
		// principal may place an order.
		Authorize: usecaseop.Public[PlaceOrderCommand],
		Execute: func(_ context.Context, cmd PlaceOrderCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OrderPlaced], error) {
			order := &Order{ID: tsid.GenerateWithPrefix("ord"), CustomerID: cmd.CustomerID, TotalCents: cmd.TotalCents}
			event := OrderPlaced{
				Meta:    newMeta(ec, "sales", "order", "placed", order.ID),
				OrderID: order.ID, CustomerID: order.CustomerID, TotalCents: order.TotalCents,
			}
			return usecaseop.Save(order, repo, event), nil
		},
	}
}

// OrderExportRequested is emitted with the dispatch job that does the export.
type OrderExportRequested struct {
	Meta    usecase.EventMetadata
	OrderID string
}

func (e OrderExportRequested) EventID() string       { return e.Meta.EventID }
func (e OrderExportRequested) EventType() string     { return e.Meta.Type }
func (e OrderExportRequested) SpecVersion() string   { return e.Meta.SpecVersion }
func (e OrderExportRequested) Source() string        { return e.Meta.Source }
func (e OrderExportRequested) Subject() string       { return e.Meta.Subject }
func (e OrderExportRequested) Time() time.Time       { return e.Meta.OccurredAt }
func (e OrderExportRequested) PrincipalID() string   { return e.Meta.PrincipalID }
func (e OrderExportRequested) CorrelationID() string { return e.Meta.CorrelationID }
func (e OrderExportRequested) CausationID() string   { return e.Meta.CausationID }
func (e OrderExportRequested) ExecutionID() string   { return e.Meta.ExecutionID }
func (e OrderExportRequested) MessageGroup() string  { return e.Meta.MessageGroup }
func (e OrderExportRequested) ToDataJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"orderId": e.OrderID})
}

// RequestExportCommand asks for an order's export to the billing service.
type RequestExportCommand struct {
	OrderID string `json:"orderId"`
}

func (RequestExportCommand) AuditOperation() string { return "request-export" }

// RequestExport is a business operation whose plan carries a dispatch job:
// usecaseop.Run writes the event, the job and the audit row in one
// transaction (the job after the event, before the audit row). The platform
// delivers the job to the billing service once the outbox processor has sent
// it. The job takes the plan's correlation id.
func RequestExport() usecaseop.Operation[RequestExportCommand, OrderExportRequested] {
	return usecaseop.Operation[RequestExportCommand, OrderExportRequested]{
		Name: "RequestExport",
		Validate: func(_ context.Context, cmd RequestExportCommand) error {
			if strings.TrimSpace(cmd.OrderID) == "" {
				return usecase.Validation("ORDER_REQUIRED", "orderId is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[RequestExportCommand],
		Execute: func(_ context.Context, cmd RequestExportCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OrderExportRequested], error) {
			event := OrderExportRequested{Meta: newMeta(ec, "sales", "order", "export-requested", cmd.OrderID), OrderID: cmd.OrderID}
			job, err := outbox.NewDispatchJob("orders:billing:export-order", "https://billing.example.com/exports").
				WithEventID(event.EventID()).
				WithMessageGroup(event.MessageGroup()).
				WithJSONPayload(map[string]string{"orderId": cmd.OrderID})
			if err != nil {
				return nil, err
			}
			return usecaseop.Emit(event).Dispatch(job), nil
		},
	}
}

// ─── HTTP ────────────────────────────────────────────────────────────

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if ue := usecase.AsError(err); ue != nil {
		status = ue.HTTPStatus()
	}
	http.Error(w, err.Error(), status)
}

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

	sink := outboxpgx.NewSink(outboxpgx.Config{
		ClientCode:      os.Getenv("FC_CLIENT_CODE"), // the client's identifier; empty = scoped by the credential
		ApplicationCode: app,
		AuditEnabled:    true,
	})
	uow := usecasepgx.New(pool, sink)
	repo := OrderRepository{}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
		var cmd PlaceOrderCommand
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		ec := usecase.NewExecutionContext(r.Header.Get("X-Principal-ID"))
		event, err := usecaseop.Run(r.Context(), uow, PlaceOrder(repo), cmd, ec)
		if err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": event.OrderID, "eventId": event.EventID()})
	})
	mux.HandleFunc("POST /orders/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		ec := usecase.NewExecutionContext(r.Header.Get("X-Principal-ID"))
		cmd := RequestExportCommand{OrderID: r.PathValue("id")}
		if _, err := usecaseop.Run(r.Context(), uow, RequestExport(), cmd, ec); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	srv := &http.Server{Addr: ":8081", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("order-service listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
