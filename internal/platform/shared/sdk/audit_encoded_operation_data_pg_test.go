//go:build integration

package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// postAuditItems posts items to the audit batch and returns the results.
func postAuditItems(t *testing.T, url string, items ...string) []BatchResultItem {
	t.Helper()
	resp, err := http.Post(url+"/api/audit-logs/batch", "application/json",
		strings.NewReader(`{"items":[`+strings.Join(items, ",")+`]}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body BatchResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Results, len(items))
	return body.Results
}

// storedOperationJSON reads aud_logs.operation_json's JSON type and text.
func storedOperationJSON(t *testing.T, id string) (string, string) {
	t.Helper()
	var typ, data string
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT jsonb_typeof(operation_json), operation_json::text FROM aud_logs WHERE id = $1`, id).Scan(&typ, &data))
	return typ, data
}

// F13 (owner ruling 2026-10-12): POST /api/audit-logs/batch unwraps an
// `operationData` the SDK outbox writers sent as a JSON string holding an
// encoded object, so the audit view gets the object, and the redaction
// backstop, which runs after the unwrap, now sees inside it. Before the fix
// operation_json held a JSON string and a secret inside it was stored.
func TestAuditBatchIngest_UnwrapsEncodedOperationData(t *testing.T) {
	ac := &auth.AuthContext{PrincipalID: "prn_audit_unwrap", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}}
	srv, _ := newAuditBatchServer(t, ac)

	// The Rust SDK's golden audit row, less the application and client codes
	// this database does not have (an unknown code is SKIPPED).
	var fixture map[string]any
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "clients", "rust-sdk", "tests", "fixtures", "wire", "audit-log-full.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.IsType(t, "", fixture["operationData"], "the fixture carries operationData as an encoded string")
	delete(fixture, "applicationCode")
	delete(fixture, "clientCode")
	item, err := json.Marshal(fixture)
	require.NoError(t, err)

	res := postAuditItems(t, srv.URL,
		string(item),
		`{"entityType":"Order","entityId":"ord_2","operation":"PlaceOrder","principalId":"prn_x","operationData":"{\"password\":\"hunter2\",\"total\":7}"}`,
		`{"entityType":"Order","entityId":"ord_3","operation":"PlaceOrder","principalId":"prn_x","operationData":{"total":8}}`,
		`{"entityType":"Order","entityId":"ord_4","operation":"PlaceOrder","principalId":"prn_x","operationData":"plain text"}`,
	)
	for _, r := range res {
		require.Equal(t, "SUCCESS", r.Status, r.Error)
	}

	typ, data := storedOperationJSON(t, res[0].ID)
	assert.Equal(t, "object", typ)
	assert.JSONEq(t, `{"apiKey":"***","total":1500}`, data)

	typ, data = storedOperationJSON(t, res[1].ID)
	assert.Equal(t, "object", typ)
	assert.JSONEq(t, `{"password":"***","total":7}`, data, "the redaction backstop sees inside an unwrapped value")

	typ, data = storedOperationJSON(t, res[2].ID)
	assert.Equal(t, "object", typ, "a value sent as itself is stored as sent")
	assert.JSONEq(t, `{"total":8}`, data)

	typ, data = storedOperationJSON(t, res[3].ID)
	assert.Equal(t, "string", typ, "a string meant as a string is kept")
	assert.JSONEq(t, `"plain text"`, data)
}
