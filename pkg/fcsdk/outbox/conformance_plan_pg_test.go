//go:build integration

package outbox_test

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/internal/pgtest"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outbox"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxpgx"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { pgtest.Main(m) }

// The conformance vector, written the way an application writes it: a use
// case whose plan is the case's event plus its dispatch jobs
// (usecaseop.Emit(...).Dispatch(...)), run by usecaseop.Run through
// outboxpgx into embedded Postgres, then read back in insertion order. The
// stored rows equal the vector's (event, jobs, audit row; every column), and
// each payload is byte for byte what the row builders write. The jobs are
// built without a correlation id: the plan gives them the event's.
func TestConformancePlanBuiltRows(t *testing.T) {
	cases, skipped := outbox.PlanCases(t)
	if len(cases) == 0 {
		t.Fatal("no single-event cases")
	}
	t.Logf("not expressible as one plan (more than one event): %q", skipped)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			pool := pgtest.Schema(t)
			sink := outboxpgx.NewSink(outboxpgx.Config{
				ClientID: c.Config.ClientID, ClientCode: c.Config.ClientCode,
				ApplicationCode: c.Config.ApplicationCode, Now: c.Config.Now, AuditEnabled: true,
			})
			op := usecaseop.Operation[any, usecase.DomainEvent]{
				Name:      "conformance",
				Authorize: usecaseop.Public[any],
				Execute: func(context.Context, any, usecase.ExecutionContext) (usecaseop.Plan[usecase.DomainEvent], error) {
					return usecaseop.Emit(c.Event).Dispatch(c.Jobs...), nil
				},
			}
			if _, err := usecaseop.Run(context.Background(), usecasepgx.New(pool, sink), op, c.Command, usecase.ExecutionContext{}); err != nil {
				t.Fatal(err)
			}
			stored := pgtest.OutboxRows(t, pool)
			rows := make([]outbox.StoredRow, len(stored))
			for i, r := range stored {
				rows[i] = outbox.StoredRow(r)
			}
			c.Check(t, rows)
		})
	}
}
