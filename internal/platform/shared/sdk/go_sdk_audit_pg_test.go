//go:build integration

package sdk

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/outboxpgx"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// goSDKEvent is a DomainEvent as an application using the Go SDK defines one.
type goSDKEvent struct{}

func (goSDKEvent) EventID() string             { return "0HZXEQ5Y8JY5Z" }
func (goSDKEvent) EventType() string           { return "gosdkaud:sales:order:placed" }
func (goSDKEvent) SpecVersion() string         { return "1.0" }
func (goSDKEvent) Source() string              { return "gosdkaud:sales" }
func (goSDKEvent) Subject() string             { return "gosdkaud.order.ord_1" }
func (goSDKEvent) Time() time.Time             { return time.Date(2026, 10, 12, 9, 30, 0, 123456789, time.UTC) }
func (goSDKEvent) PrincipalID() string         { return "prn_0HZXEQ5Y8JY5A" }
func (goSDKEvent) CorrelationID() string       { return "corr-1" }
func (goSDKEvent) CausationID() string         { return "" }
func (goSDKEvent) ExecutionID() string         { return "exec-1" }
func (goSDKEvent) MessageGroup() string        { return "orders:ord_1" }
func (goSDKEvent) ToDataJSON() ([]byte, error) { return []byte(`{"orderId":"ord_1"}`), nil }

type placeOrderCommand struct {
	OrderID  string `json:"orderId"`
	Password string `json:"password"`
}

// goSDKOutboxPayload writes one row through the Go SDK's outbox sink and
// returns its payload, the bytes the outbox processor sends unchanged as one
// element of the batch's items.
func goSDKOutboxPayload(t *testing.T, cfg outboxpgx.Config, write func(context.Context, *outboxpgx.Sink, *usecasepgx.DbTx) error) string {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	require.NoError(t, outboxpgx.InitSchema(ctx, pool))
	_, err := pool.Exec(ctx, `DELETE FROM outbox_messages`)
	require.NoError(t, err)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, write(ctx, outboxpgx.NewSink(cfg), usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(ctx))
	var payload string
	require.NoError(t, pool.QueryRow(ctx, `SELECT payload FROM outbox_messages`).Scan(&payload))
	return payload
}

// F13: the Go SDK's outbox audit rows are accepted by POST /api/audit-logs/batch.
// Before the fix outboxpgx wrote snake_case members (entity_type,
// principal_id, operation_json, ...) that the camelCase-only endpoint did not
// read, so every row was refused "principalId is required". The command
// arrives as an object, redacted; with Config.ApplicationCode set an
// application-scoped credential may write it.
func TestAuditBatchIngest_AcceptsTheGoSDKOutboxRow(t *testing.T) {
	pool := testpg.Pool(t)
	app, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t), appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: "gosdkaud", Name: "Go SDK audit"}, testpg.TestEC())
	require.NoError(t, err)

	payload := goSDKOutboxPayload(t, outboxpgx.Config{AuditEnabled: true, ApplicationCode: "gosdkaud"},
		func(ctx context.Context, s *outboxpgx.Sink, tx *usecasepgx.DbTx) error {
			return s.WriteAudit(ctx, tx, goSDKEvent{}, placeOrderCommand{OrderID: "ord_1", Password: "hunter2"})
		})

	for name, ac := range map[string]*auth.AuthContext{
		"all applications": {PrincipalID: "prn_gosdk_all", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}},
		"application-scoped": {
			PrincipalID: "prn_gosdk_app", Scope: auth.ScopeAnchor, Applications: []string{app.ApplicationID},
			Permissions: []string{"platform:admin:batch:audit-logs-write"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := newAuditBatchServer(t, ac)
			res := postAuditItems(t, srv.URL, payload)
			require.Equal(t, "SUCCESS", res[0].Status, res[0].Error)

			var entityType, entityID, operation, principalID, opJSON string
			var appID *string
			var performedAt time.Time
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT entity_type, entity_id, operation, principal_id, operation_json::text, application_id, performed_at
				   FROM aud_logs WHERE id = $1`, res[0].ID).
				Scan(&entityType, &entityID, &operation, &principalID, &opJSON, &appID, &performedAt))
			assert.Equal(t, "Order", entityType)
			assert.Equal(t, "ord_1", entityID)
			assert.Equal(t, "placeOrderCommand", operation)
			assert.Equal(t, "prn_0HZXEQ5Y8JY5A", principalID)
			assert.JSONEq(t, `{"orderId":"ord_1","password":"***"}`, opJSON)
			require.NotNil(t, appID)
			assert.Equal(t, app.ApplicationID, *appID)
			assert.True(t, performedAt.Equal(time.Date(2026, 10, 12, 9, 30, 0, 123456000, time.UTC)), performedAt)
		})
	}

	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(payload), &members))
	for k := range members {
		assert.NotContains(t, k, "_", "camelCase members only")
	}
}

// Audit rows the Go SDK wrote before it followed the shared outbox vector
// (69857fb..2026-10-13: no source/correlationId/clientCode, performedAt with
// nine fraction digits) still sit in applications' outboxes; the endpoint
// still accepts them. The current rows add source and correlationId (ignored
// by the endpoint) and clientCode.
func TestAuditBatchIngest_AcceptsLegacyGoSDKOutboxRows(t *testing.T) {
	legacy := `{"entityType":"Order","entityId":"ord_1","operation":"placeOrderCommand",` +
		`"operationData":{"orderId":"ord_1","password":"***"},"principalId":"prn_0HZXEQ5Y8JY5A",` +
		`"performedAt":"2026-10-12T09:30:00.123456789Z"}`
	current := goSDKOutboxPayload(t, outboxpgx.Config{AuditEnabled: true},
		func(ctx context.Context, s *outboxpgx.Sink, tx *usecasepgx.DbTx) error {
			return s.WriteAudit(ctx, tx, goSDKEvent{}, placeOrderCommand{OrderID: "ord_1", Password: "hunter2"})
		})
	assert.Contains(t, current, `"source":"gosdkaud:sales"`)
	assert.Contains(t, current, `"performedAt":"2026-10-12T09:30:00.123456789Z"`)

	srv, _ := newAuditBatchServer(t, &auth.AuthContext{
		PrincipalID: "prn_gosdk_legacy", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"},
	})
	res := postAuditItems(t, srv.URL, legacy, current)
	for i, r := range res {
		require.Equal(t, "SUCCESS", r.Status, "item %d: %s", i, r.Error)
	}
}
