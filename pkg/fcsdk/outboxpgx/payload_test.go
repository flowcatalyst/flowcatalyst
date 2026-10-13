package outboxpgx

import (
	"encoding/json"
	"maps"
	"reflect"
	"testing"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outbox"
)

// The audit row is the platform's POST /api/audit-logs/batch item: camelCase
// members only (the endpoint reads no snake_case alias and refuses a row
// without principalId), operationData an object, applicationCode and
// clientCode only when configured.
func TestAuditPayloadIsTheAuditBatchItem(t *testing.T) {
	type cmd struct {
		OrderID string `json:"orderId"`
	}
	common := map[string]any{
		"entityType": "Thing", "entityId": "thg_1", "operation": "cmd",
		"operationData": map[string]any{"orderId": "ord_1"},
		"principalId":   "prn_1", "performedAt": "1970-01-01T00:00:00Z", "source": "app:test",
	}
	with := func(extra map[string]any) map[string]any {
		out := maps.Clone(common)
		maps.Copy(out, extra)
		return out
	}
	for _, c := range []struct {
		cfg  Config
		want map[string]any
	}{
		{Config{}, common},
		{Config{ApplicationCode: "orders"}, with(map[string]any{"applicationCode": "orders"})},
		{Config{ApplicationCode: "orders", ClientCode: "acme"}, with(map[string]any{"applicationCode": "orders", "clientCode": "acme"})},
	} {
		row, err := outbox.AuditRow(NewSink(c.cfg).rowConfig(), testEvent{}, cmd{OrderID: "ord_1"})
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(row.Payload, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v:\n got  %v\n want %v", c.cfg, got, c.want)
		}
	}
}

func TestInsertStatementUsesTheConfiguredTable(t *testing.T) {
	stmt, err := outbox.InsertStatement(NewSink(Config{TableName: "app_outbox"}).cfg.TableName, outbox.PlaceholderDollar)
	if err != nil {
		t.Fatal(err)
	}
	if want := "INSERT INTO app_outbox (\n  id, type, message_group, payload, status, created_at, updated_at,\n  client_id, payload_size, headers\n)\nVALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)"; stmt != want {
		t.Fatalf("got %q", stmt)
	}
}
