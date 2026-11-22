//go:build integration

package runtimeapi_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/runtimeapi"
)

// otherRuntimeAccount is a second runtime account: a service account at anchor
// tier that belongs to no application, holding the runtime permission, as the
// test's own account (prn_ingrtester001) does.
const otherRuntimeAccount = "prn_ingrtother001"

// A runtime id belongs to the account whose poll first registered it. Every
// route that names it refuses another account with 403 RUNTIME_ID_NOT_OWNED,
// before reading anything else, and audits the refusal: RUNTIME_ID_REFUSED on
// the runtime id, or on the secret route a refused fetch
// (SECRET_FETCH_REFUSED) whose reason is the code. The owner goes on as before.
func TestRuntimeOwner_EveryRouteRefusesAnotherAccount(t *testing.T) {
	e := newEnv(t)
	i := e.save("owned", sftpSource, true)
	rt := "rt-owned-" + unique()
	require.Len(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"), 1, "the owner holds the polled source")
	state := base + "/state/" + i.ID + "/watermark?runtimeId=" + rt
	require.Equal(t, http.StatusNoContent, e.do(http.MethodPut, state, map[string]any{"value": "mine"}).code)
	owner := e.who

	other := e.principal(permRuntime)
	other.PrincipalID = otherRuntimeAccount
	e.who = other
	calls := []struct {
		operation, method, path string
		body                    any
	}{
		{runtimeapi.PollOperationID, http.MethodPost, base + "/poll", map[string]any{"runtimeId": rt, "contractVersion": ingestion.ContractVersion}},
		{runtimeapi.ReleaseOperationID, http.MethodPost, base + "/leases/release", map[string]any{"runtimeId": rt, "ingestionIds": []string{i.ID}, "goodbye": true}},
		{runtimeapi.ReportOperationID, http.MethodPost, base + "/report", map[string]any{"runtimeId": rt, "routes": []map[string]any{{"ingestionId": i.ID, "version": 1, "status": "FAILED"}}}},
		{runtimeapi.GetStateOperationID, http.MethodGet, state, nil},
		{runtimeapi.PutStateOperationID, http.MethodPut, state, map[string]any{"value": "stolen"}},
		{runtimeapi.CreateJobsOperationID, http.MethodPost, base + "/ingestions/" + i.ID + "/jobs?runtimeId=" + rt, map[string]any{"records": []map[string]any{{"payload": `{"a":1}`}}}},
		{runtimeapi.TracesOperationID, http.MethodPost, base + "/traces", map[string]any{"runtimeId": rt, "records": []map[string]any{}}},
		{runtimeapi.GetSecretOperationID, http.MethodGet, base + "/ingestions/" + i.ID + "/secrets/sftp-password?runtimeId=" + rt, nil},
	}
	for _, c := range calls {
		r := e.do(c.method, c.path, c.body)
		assert.Equal(t, http.StatusForbidden, r.code, "%s: %s", c.operation, r.raw)
		assert.Equal(t, ingestion.RuntimeIDNotOwned, r.errCode(), c.operation)
		assert.NotContains(t, r.raw, owner.PrincipalID, "%s: the refusal does not name the owner", c.operation)
		if c.operation == runtimeapi.GetSecretOperationID {
			assert.Equal(t, 1, e.count(`SELECT count(*) FROM aud_logs WHERE operation = 'SECRET_FETCH_REFUSED' AND entity_type = 'Ingestion'
				AND entity_id = $1 AND principal_id = $2 AND operation_json->>'reason' = 'RUNTIME_ID_NOT_OWNED'
				AND operation_json->>'runtimeId' = $3`, i.ID, otherRuntimeAccount, rt), "the secret route audits it as a refused fetch")
			continue
		}
		assert.Equal(t, 1, e.count(`SELECT count(*) FROM aud_logs WHERE operation = 'RUNTIME_ID_REFUSED' AND entity_type = 'IngestionRuntime'
			AND entity_id = $1 AND principal_id = $2 AND operation_json->>'operation' = $3
			AND operation_json->>'reason' = 'RUNTIME_ID_NOT_OWNED'`, rt, otherRuntimeAccount, c.operation), c.operation)
	}
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM aud_logs WHERE operation = 'RUNTIME_ID_REFUSED' AND operation_json->>'operation' = $1`,
		runtimeapi.GetSecretOperationID), "one audit row per refusal, not two")

	// Nothing the other account sent took effect.
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_route_states WHERE runtime_id = $1`, rt), "the report was not recorded")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM msg_dispatch_jobs WHERE metadata::text LIKE '%' || $1 || '%'`, rt), "no job was made")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_leases WHERE ingestion_id = $1 AND runtime_id = $2 AND NOT released`, i.ID, rt), "the lease was not released")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_runtimes WHERE runtime_id = $1 AND left_at IS NULL AND principal_id = $2`, rt, owner.PrincipalID))

	// The owner goes on as before.
	e.who = owner
	r := e.do(http.MethodGet, state, nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "mine", r.str("value"))
	assert.Len(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"), 1)
}

// An account reaches only the bound secrets of ingestions its own runtimes
// hold: with a runtime id of its own it is refused the lease another
// account's runtime holds, as any runtime without the lease is.
func TestRuntimeOwner_AnAccountReachesOnlyWhatItsRuntimesHold(t *testing.T) {
	e := newEnv(t)
	i := e.save("owned-secret", sftpSource, true)
	require.Len(t, e.poll("rt-owner-a-"+unique(), ingestion.ContractVersion).list("ingestions"), 1)

	other := e.principal(permRuntime)
	other.PrincipalID = otherRuntimeAccount
	e.who = other
	mine := "rt-owner-b-" + unique()
	assert.Empty(t, e.poll(mine, ingestion.ContractVersion).list("ingestions"), "the lease is held by the first account's runtime")
	r := e.do(http.MethodGet, base+"/ingestions/"+i.ID+"/secrets/sftp-password?runtimeId="+mine, nil)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "LEASE_NOT_HELD", r.errCode())
}
