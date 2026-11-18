//go:build integration

package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// postAuditBatch posts items to the audit batch as ac and returns the per-item results.
func postAuditBatch(t *testing.T, ac *auth.AuthContext, items string) ([]BatchResultItem, *audit.Repository) {
	t.Helper()
	srv, repo := newAuditBatchServer(t, ac)
	resp, err := http.Post(srv.URL+"/api/audit-logs/batch", "application/json", strings.NewReader(`{"items":[`+items+`]}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body BatchResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.Results, repo
}

// auditItem is one audit batch item for entity id, with the given extra members (",..." or "").
func auditItem(entity, extra string) string {
	return fmt.Sprintf(`{"entityType":"widget","entityId":%q,"operation":"CREATE","principalId":"prn_actor"%s}`, entity, extra)
}

// storedAudit returns the rows stored for an entity id.
func storedAudit(t *testing.T, repo *audit.Repository, entity string) []audit.Log {
	t.Helper()
	logs, err := repo.FindWithFilters(context.Background(), audit.FilterParams{EntityID: &entity, Limit: 10})
	require.NoError(t, err)
	return logs
}

func requireNoSkipped(t *testing.T, results []BatchResultItem) {
	t.Helper()
	for i, r := range results {
		require.NotEqual(t, "SKIPPED", r.Status, "item %d: the audit batch never answers SKIPPED (a processor clears a SKIPPED row as delivered)", i)
	}
}

// TestAuditBatchIngest_AnchorUnknownClientCodeIsKeptPlatformScoped pins the events batch's rule
// for an audit row: an anchor's clientCode that names no client stores the row platform-scoped
// (no client), where it used to be answered SKIPPED and not stored, which a processor then
// deleted from the writer's outbox as delivered. A known code still resolves to its client.
func TestAuditBatchIngest_AnchorUnknownClientCodeIsKeptPlatformScoped(t *testing.T) {
	pool := testpg.Pool(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())
	knownCode := "known-" + sfx
	knownID := insertClient(t, pool, knownCode)
	anchor := &auth.AuthContext{
		PrincipalID: "prn_audit_anchor", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:admin:batch:audit-logs-write"},
	}

	unknown, known := "w_unknown_"+sfx, "w_known_"+sfx
	results, repo := postAuditBatch(t, anchor,
		auditItem(unknown, `,"clientCode":"no-such-client-`+sfx+`"`)+","+
			auditItem(known, `,"clientCode":"`+knownCode+`"`))
	require.Len(t, results, 2)
	requireNoSkipped(t, results)
	assert.Equal(t, "SUCCESS", results[0].Status)
	assert.NotEmpty(t, results[0].ID)
	assert.Equal(t, "SUCCESS", results[1].Status)

	logs := storedAudit(t, repo, unknown)
	require.Len(t, logs, 1, "an anchor's row with an unknown clientCode is stored")
	assert.Nil(t, logs[0].ClientID, "and stored platform-scoped")
	assert.Equal(t, results[0].ID, logs[0].ID)

	logs = storedAudit(t, repo, known)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].ClientID)
	assert.Equal(t, knownID, *logs[0].ClientID)
}

// TestAuditBatchIngest_ClientCallerOutsideItsReachIsRefusedPerItem pins the refusal for a caller
// that is not an anchor: a clientCode naming a client it does not reach, or no client at all, is
// FORBIDDEN "No access to client: <clientCode>" in that item's slot (one answer for both), not
// stored, and the rest of the batch lands. A blank clientCode counts as none.
func TestAuditBatchIngest_ClientCallerOutsideItsReachIsRefusedPerItem(t *testing.T) {
	pool := testpg.Pool(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())
	ownCode, otherCode, unknownCode := "own-"+sfx, "other-"+sfx, "no-such-client-"+sfx
	ownID := insertClient(t, pool, ownCode)
	insertClient(t, pool, otherCode)
	caller := &auth.AuthContext{
		PrincipalID: "prn_audit_client", Scope: auth.ScopeClient, Clients: []string{ownID},
		AllApplications: true, Permissions: []string{"platform:admin:batch:audit-logs-write"},
	}

	other, unknown, own, blank := "w_other_"+sfx, "w_unknown_"+sfx, "w_own_"+sfx, "w_blank_"+sfx
	results, repo := postAuditBatch(t, caller,
		auditItem(other, `,"clientCode":"`+otherCode+`"`)+","+
			auditItem(unknown, `,"clientCode":"`+unknownCode+`"`)+","+
			auditItem(own, `,"clientCode":"`+ownCode+`"`)+","+
			auditItem(blank, `,"clientCode":"  "`))
	require.Len(t, results, 4)
	requireNoSkipped(t, results)

	assert.Equal(t, BatchResultItem{Status: "FORBIDDEN", Error: "No access to client: " + otherCode}, results[0])
	assert.Equal(t, BatchResultItem{Status: "FORBIDDEN", Error: "No access to client: " + unknownCode}, results[1])
	assert.Equal(t, "SUCCESS", results[2].Status)
	assert.Equal(t, "SUCCESS", results[3].Status)

	assert.Empty(t, storedAudit(t, repo, other), "a refused item is not stored")
	assert.Empty(t, storedAudit(t, repo, unknown), "a refused item is not stored")
	logs := storedAudit(t, repo, own)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].ClientID)
	assert.Equal(t, ownID, *logs[0].ClientID)
	logs = storedAudit(t, repo, blank)
	require.Len(t, logs, 1, "a blank clientCode is no clientCode")
	assert.Nil(t, logs[0].ClientID)
}

// TestAuditBatchIngest_UnknownApplicationCodeIsRefusedPerItem pins that an applicationCode naming
// no application is BAD_REQUEST "Application not found: <code>" in its slot, not SKIPPED: the
// row is not stored, and a processor keeps it for an operator. The rest of the batch lands.
func TestAuditBatchIngest_UnknownApplicationCodeIsRefusedPerItem(t *testing.T) {
	sfx := strings.ToLower(tsid.GenerateUntyped())
	anchor := &auth.AuthContext{
		PrincipalID: "prn_audit_anchor_app", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:admin:batch:audit-logs-write"},
	}
	appCode := "no-such-app-" + sfx

	refused, kept := "w_app_unknown_"+sfx, "w_app_none_"+sfx
	results, repo := postAuditBatch(t, anchor,
		auditItem(refused, `,"applicationCode":"`+appCode+`"`)+","+auditItem(kept, ""))
	require.Len(t, results, 2)
	requireNoSkipped(t, results)
	assert.Equal(t, BatchResultItem{Status: "BAD_REQUEST", Error: "Application not found: " + appCode}, results[0])
	assert.Equal(t, "SUCCESS", results[1].Status)
	assert.Empty(t, storedAudit(t, repo, refused))
	assert.Len(t, storedAudit(t, repo, kept), 1)
}
