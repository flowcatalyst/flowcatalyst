//go:build integration

package platformsink_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// sinkEvent is a platform domain event with the data it is given.
type sinkEvent struct {
	id   string
	data string
}

func (e sinkEvent) EventID() string             { return e.id }
func (sinkEvent) EventType() string             { return "platform:test:sink-payload:written" }
func (sinkEvent) SpecVersion() string           { return "1.0" }
func (sinkEvent) Source() string                { return "platform:test" }
func (e sinkEvent) Subject() string             { return "platform.sinkpayload." + e.id }
func (sinkEvent) Time() time.Time               { return time.Now().UTC() }
func (sinkEvent) PrincipalID() string           { return "prn_sinkpayload" }
func (sinkEvent) CorrelationID() string         { return "" }
func (sinkEvent) CausationID() string           { return "" }
func (sinkEvent) ExecutionID() string           { return "" }
func (sinkEvent) MessageGroup() string          { return "" }
func (e sinkEvent) ToDataJSON() ([]byte, error) { return []byte(e.data), nil }

type sinkCommand struct{ Note string }

// emit writes ev through the platform sink inside one use-case transaction,
// then commits it or, with rollback, fails the transaction after the write.
func emit(t *testing.T, uow *usecasepgx.UnitOfWork, ev sinkEvent, rollback bool) error {
	t.Helper()
	res := usecasepgx.Run(context.Background(), uow, func(s *usecasepgx.TxScopedUnitOfWork) usecase.Result[sinkEvent] {
		r := usecasepgx.EmitEventScoped(context.Background(), s, ev, sinkCommand{Note: "x"})
		if !usecase.IsSuccess(r) || !rollback {
			return r
		}
		return usecase.Failure[sinkEvent](usecase.Internal("LATER_STEP", "a later step of the use case failed", nil))
	})
	_, err := usecase.Into(res)
	return err
}

// The platform's own events with the payload table written
// (docs/payload-offload-design.md, section 6): the payload row and the event
// row are written in the use case's own transaction, the bytes always in the
// row (the store is never called inside a transaction), and a rollback
// leaves neither.
func TestWriteEvent_NewShapeInTheUseCaseTransaction(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.WritingService(t, store, func(c *payloadstore.Config) { c.InlineMaxBytes = 8192; c.MaxBytes = 16384 })
	uow := usecasepgx.New(pool, platformsink.New().UsePayloads(svc))

	t.Run("committed", func(t *testing.T) {
		// Over the offload threshold: still kept in the row.
		data := `{"pad":"` + strings.Repeat("s", 3*payloadtest.OffloadMinBytes) + `", "z":1}`
		ev := sinkEvent{id: tsid.GenerateUntyped(), data: data}
		require.NoError(t, emit(t, uow, ev, false))

		var legacy, payloadID, pData, pRef *string
		var size *int32
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT e.data::text, e.payload_id, p.data, p.ref, p.size
			   FROM msg_events e
			   LEFT JOIN msg_payloads p ON p.id = e.payload_id AND p.created_at = e.created_at
			  WHERE e.id = $1`, ev.id).Scan(&legacy, &payloadID, &pData, &pRef, &size))
		assert.Nil(t, legacy)
		require.NotNil(t, payloadID)
		require.NotNil(t, pData, "the payload row is under the event's created_at")
		assert.Equal(t, data, *pData, "the bytes the event marshalled")
		assert.Nil(t, pRef, "never offloaded inside a transaction")
		assert.EqualValues(t, len(data), *size)
		assert.Zero(t, store.Puts(), "the store is never called")
	})

	t.Run("rolled back", func(t *testing.T) {
		ev := sinkEvent{id: tsid.GenerateUntyped(), data: `{"n":1}`}
		require.Error(t, emit(t, uow, ev, true))
		var events, payloads int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM msg_events WHERE id = $1`, ev.id).Scan(&events))
		assert.Zero(t, events)
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM msg_payloads WHERE data = '{"n":1}' AND created_at > NOW() - INTERVAL '1 minute'`).Scan(&payloads))
		assert.Zero(t, payloads, "no payload row survives the rollback")
	})

	t.Run("over what the database takes", func(t *testing.T) {
		ev := sinkEvent{id: tsid.GenerateUntyped(), data: `"` + strings.Repeat("l", 9000) + `"`}
		err := emit(t, uow, ev, false)
		require.Error(t, err)
		var events int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM msg_events WHERE id = $1`, ev.id).Scan(&events))
		assert.Zero(t, events, "the use case fails")
	})

	t.Run("switch off: the legacy shape", func(t *testing.T) {
		off := usecasepgx.New(pool, platformsink.New().UsePayloads(payloadtest.Service(t, nil)))
		ev := sinkEvent{id: tsid.GenerateUntyped(), data: `{"b":1, "a":2}`}
		require.NoError(t, emit(t, off, ev, false))
		var legacy, payloadID *string
		require.NoError(t, pool.QueryRow(ctx, `SELECT data::text, payload_id FROM msg_events WHERE id = $1`, ev.id).Scan(&legacy, &payloadID))
		assert.Nil(t, payloadID)
		require.NotNil(t, legacy)
		assert.JSONEq(t, `{"a":2,"b":1}`, *legacy)
	})
}
