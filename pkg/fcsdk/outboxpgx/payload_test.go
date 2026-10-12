package outboxpgx

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The audit row is the platform's POST /api/audit-logs/batch item: camelCase
// members only (the endpoint reads no snake_case alias and refuses a row
// without principalId), operationData an object, applicationCode only when
// configured.
func TestAuditPayloadIsTheAuditBatchItem(t *testing.T) {
	type cmd struct {
		OrderID string `json:"orderId"`
	}
	for _, c := range []struct {
		app  string
		want map[string]any
	}{
		{"", map[string]any{
			"entityType": "Thing", "entityId": "thg_1", "operation": "cmd",
			"operationData": map[string]any{"orderId": "ord_1"},
			"principalId":   "prn_1", "performedAt": "1970-01-01T00:00:00.000000000Z",
		}},
		{"orders", map[string]any{
			"entityType": "Thing", "entityId": "thg_1", "operation": "cmd",
			"operationData": map[string]any{"orderId": "ord_1"},
			"principalId":   "prn_1", "performedAt": "1970-01-01T00:00:00.000000000Z",
			"applicationCode": "orders",
		}},
	} {
		raw, err := buildAuditPayload(testEvent{}, cmd{OrderID: "ord_1"}, c.app)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("applicationCode %q:\n got  %v\n want %v", c.app, got, c.want)
		}
	}
}
