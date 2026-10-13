//go:build integration

package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxpgx"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxsql"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// wireEvent is a Go SDK domain event with a fresh id per test run (the batch
// endpoint drops an event whose deduplicationId is already stored).
type wireEvent struct{ id string }

func (e wireEvent) EventID() string     { return e.id }
func (wireEvent) EventType() string     { return "gosdkw:sales:order:placed" }
func (wireEvent) SpecVersion() string   { return "1.0" }
func (wireEvent) Source() string        { return "gosdkw:sales" }
func (wireEvent) Subject() string       { return "sales.order.ord_1" }
func (wireEvent) Time() time.Time       { return time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC) }
func (wireEvent) PrincipalID() string   { return "prn_0HZXEQ5Y8JY5A" }
func (wireEvent) CorrelationID() string { return "corr-w" }
func (wireEvent) CausationID() string   { return "" }
func (wireEvent) ExecutionID() string   { return "run-w" }
func (wireEvent) MessageGroup() string  { return "sales:order:ord_1" }
func (wireEvent) ToDataJSON() ([]byte, error) {
	return []byte(`{"orderId":"ord_1","lines":[{"sku":"A","qty":2}]}`), nil
}

// outboxPayloads returns the payload of every outbox row of the given type,
// oldest first: the bytes the outbox processor sends unchanged.
func outboxPayloads(t *testing.T, pool *pgxpool.Pool, typ string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT payload FROM outbox_messages WHERE type = $1 ORDER BY created_at, id`, typ)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		require.NoError(t, rows.Scan(&p))
		out = append(out, p)
	}
	require.NoError(t, rows.Err())
	return out
}

// The Go SDK's outbox event rows in every shape it has written are accepted
// by POST /api/events/batch, the endpoint the outbox processor posts each
// row's payload to unchanged:
//   - legacy rows, already in applications' outbox tables, from the SDK
//     before it followed the shared vector: snake_case members, data as the
//     value itself, deduplication_id "<type>-<id>", context_data with
//     aggregateType (read through the endpoint's snake_case aliases);
//   - rows of the current outboxpgx Sink and outboxsql Writer: the batch
//     item's camelCase members, data as the value itself, deduplicationId the
//     event id, clientCode resolved to the client;
//   - the application framework's conformance rows (data encoded as a string,
//     unwrapped at ingest, F13) and the Go SDK's pinned bytes for them.
func TestBatchIngest_GoSDKOutboxShapes(t *testing.T) {
	ctx := anchorCtx()
	pool := testpg.Pool(t)
	require.NoError(t, outboxpgx.InitSchema(context.Background(), pool))
	_, err := pool.Exec(context.Background(), `DELETE FROM outbox_messages`)
	require.NoError(t, err)
	s := &State{Repo: event.NewRepository(pool), Clients: client.NewRepository(pool)}

	clientCode := "gosdkw-" + strings.ToLower(tsid.GenerateUntyped())
	clientID := tsid.Generate(tsid.Client)
	_, err = pool.Exec(context.Background(), `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $2, $2)`, clientID, clientCode)
	require.NoError(t, err)

	ingest := func(raw string) BatchResultItem {
		t.Helper()
		var item BatchEventItem
		require.NoError(t, json.Unmarshal([]byte(raw), &item), raw)
		out, err := s.batchIngest(ctx, &apicommon.In[BatchRequest]{Body: BatchRequest{Items: []BatchEventItem{item}}})
		require.NoError(t, err)
		require.Len(t, out.Body.Results, 1)
		require.Equal(t, "SUCCESS", out.Body.Results[0].Status, "%s: %s", out.Body.Results[0].Error, raw)
		return out.Body.Results[0]
	}
	var dataType string
	stored := func(id string) (eventRow, string, string) {
		t.Helper()
		var dedup string
		require.NoError(t, pool.QueryRow(context.Background(),
			`SELECT jsonb_typeof(data), deduplication_id FROM msg_events WHERE id = $1`, id).Scan(&dataType, &dedup))
		return fetchEventRow(t, context.Background(), pool, id), dataType, dedup
	}

	// 1. A legacy row, as the SDK wrote it before (sink.go buildEventPayload
	// up to 2026-10-13), already sitting in an application's outbox.
	legacyID := tsid.GenerateUntyped()
	legacy := `{"causation_id":"","context_data":[{"key":"principalId","value":"prn_0HZXEQ5Y8JY5A"},{"key":"aggregateType","value":"Order"}],` +
		`"correlation_id":"corr-w","data":{"orderId":"ord_1","lines":[{"sku":"A","qty":2}]},` +
		`"deduplication_id":"gosdkw:sales:order:placed-` + legacyID + `","event_type":"gosdkw:sales:order:placed",` +
		`"message_group":"sales:order:ord_1","source":"gosdkw:sales","spec_version":"1.0","subject":"sales.order.ord_1"}`
	_, err = pool.Exec(context.Background(),
		`INSERT INTO outbox_messages (id, type, message_group, payload, status, retry_count, created_at, updated_at, payload_size)
		 VALUES ($1, 'EVENT', 'sales:order:ord_1', $2, 0, 0, NOW(), NOW(), $3)`, tsid.GenerateUntyped(), legacy, len(legacy))
	require.NoError(t, err)
	res := ingest(outboxPayloads(t, pool, "EVENT")[0])
	row, typ, dedup := stored(res.ID)
	assert.Equal(t, "object", typ)
	assert.JSONEq(t, `{"orderId":"ord_1","lines":[{"sku":"A","qty":2}]}`, row.Data)
	assert.Equal(t, "gosdkw:sales:order:placed", row.Type)
	assert.Equal(t, "gosdkw:sales", row.Source)
	assert.Equal(t, "gosdkw:sales:order:placed-"+legacyID, dedup)
	require.NotNil(t, row.CorrelationID)
	assert.Equal(t, "corr-w", *row.CorrelationID)
	require.NotNil(t, row.MessageGroup)
	assert.Equal(t, "sales:order:ord_1", *row.MessageGroup)
	assert.Contains(t, row.ContextData, "prn_0HZXEQ5Y8JY5A")
	assert.Nil(t, row.ClientID, "legacy rows carried no client code")

	// 2. The current pgx Sink and 3. the database/sql Writer, both naming the
	// client by its identifier.
	pgxEvent := wireEvent{id: tsid.GenerateUntyped()}
	tx, err := pool.Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, outboxpgx.NewSink(outboxpgx.Config{ClientCode: clientCode}).
		WriteEvent(context.Background(), usecasepgx.WrapTxForBootstrap(tx), pgxEvent))
	require.NoError(t, tx.Commit(context.Background()))

	sqlEvent := wireEvent{id: tsid.GenerateUntyped()}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	w, err := outboxsql.NewWriter(outboxsql.DialectPostgres, outboxsql.WriterConfig{ClientCode: clientCode})
	require.NoError(t, err)
	sqlTx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, w.WriteEvent(context.Background(), sqlTx, sqlEvent))
	require.NoError(t, sqlTx.Commit())

	payloads := outboxPayloads(t, pool, "EVENT")
	require.Len(t, payloads, 3)
	for i, ev := range []wireEvent{pgxEvent, sqlEvent} {
		raw := payloads[i+1]
		var members map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &members))
		for k := range members {
			assert.NotContains(t, k, "_", "camelCase members only")
		}
		assert.Equal(t, byte('{'), members["data"][0], "data is the value itself, never an encoded string")

		res := ingest(raw)
		row, typ, dedup := stored(res.ID)
		assert.Equal(t, "object", typ)
		assert.JSONEq(t, `{"orderId":"ord_1","lines":[{"sku":"A","qty":2}]}`, row.Data)
		assert.Equal(t, ev.id, dedup, "the deduplicationId is the event id")
		require.NotNil(t, row.ClientID, "clientCode resolved")
		assert.Equal(t, clientID, *row.ClientID)
		assert.Contains(t, row.ContextData, "executionId")
		assert.NotContains(t, row.ContextData, "aggregateType")
	}
	var rowID string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id FROM outbox_messages WHERE payload = $1`, payloads[1]).Scan(&rowID))
	assert.Equal(t, pgxEvent.id, rowID, "the outbox row id is the event id")

	// 4. The framework's conformance rows (data encoded) and the Go SDK's
	// pinned bytes for the same events (data the value itself). Each gets a
	// fresh deduplicationId: they share the vector's event ids.
	vectorDir := filepath.Join("..", "..", "..", "..", "pkg", "fcsdk", "outbox", "testdata", "conformance")
	var vectors struct {
		Cases []struct {
			Rows []struct {
				Type        string          `json:"type"`
				Payload     json.RawMessage `json:"payload"`
				PayloadText string          `json:"payloadText"`
			} `json:"rows"`
		} `json:"cases"`
	}
	raw, err := os.ReadFile(filepath.Join(vectorDir, "outbox.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &vectors))
	var goRows []struct {
		Type        string `json:"type"`
		PayloadText string `json:"payloadText"`
	}
	raw, err = os.ReadFile(filepath.Join(vectorDir, "go-payloads.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &goRows))

	type sample struct{ text, wantData string }
	var samples []sample
	for _, c := range vectors.Cases {
		for _, r := range c.Rows {
			if r.Type != "EVENT" {
				continue
			}
			var p struct {
				Data json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(r.Payload, &p))
			samples = append(samples, sample{r.PayloadText, string(p.Data)})
		}
	}
	i := 0
	for _, r := range goRows {
		if r.Type == "EVENT" {
			samples = append(samples, sample{r.PayloadText, samples[i].wantData})
			i++
		}
	}
	require.Len(t, samples, 8)
	for _, smp := range samples {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(smp.text), &m))
		m["deduplicationId"] = tsid.GenerateUntyped()
		b, err := json.Marshal(m)
		require.NoError(t, err)
		res := ingest(string(b))
		row, typ, _ := stored(res.ID)
		assert.Equal(t, "object", typ, smp.text)
		assert.JSONEq(t, smp.wantData, row.Data, smp.text)
	}
}
