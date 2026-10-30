//go:build integration

package runtimeapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Two records of one request with the same key: the first makes the job and
// the second answers it, every time (C5: the keys are claimed in a stable
// order). The key the runtime sends is a sha256 hex, 64 characters.
func TestJobs_SameKeyInOneRequestFirstRecordWins(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("dup", sqsSource, tg.viaAccount(), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)
	for n := range 8 {
		sum := sha256.Sum256(fmt.Appendf(nil, "%s/%d", i.ID, n))
		key := hex.EncodeToString(sum[:])
		require.Len(t, key, 64)
		r := e.do(http.MethodPost, jobsPath(i.ID, rt), map[string]any{"records": []map[string]any{
			{"payload": `{"first":true}`, "externalId": key}, {"payload": "x", "externalId": "other-" + key[:10]}, {"payload": `{"first":false}`, "externalId": key},
		}})
		require.Equal(t, http.StatusOK, r.code, r.raw)
		results := r.list("results")
		require.Len(t, results, 3)
		assert.NotContains(t, results[0], "existing", "round %d: the first record makes the job", n)
		assert.Equal(t, true, results[2]["existing"], "round %d", n)
		assert.Equal(t, results[0]["id"], results[2]["id"])
		assert.Equal(t, `{"first":true}`, e.job(results[0]["id"].(string)).payload)
	}
}

// A suspended or deactivated client's ingestions take no jobs and reveal no
// secrets (R6), even to the runtime holding the lease.
func TestJobs_InactiveClientIsRefused(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	shared := e.saveTarget("shared", sqsSource, tg.viaAccount(), true)
	polled := e.saveTarget("polled", sftpSource, tg.viaAccount(), true)
	cid := string(e.client.ID)
	e.setSecret("sftp-password", "pw", &cid)
	rt := "rt-" + unique()
	require.Len(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"), 2)

	e.client.SetStatus(client.StatusSuspended, nil)
	e.persist(testpg.NewUoW(t), func(ctx context.Context, tx *usecasepgx.DbTx) error {
		return client.NewRepository(e.pool).Persist(ctx, e.client, tx)
	})
	body := map[string]any{"records": records(1, "susp")}
	for _, id := range []string{shared.ID, polled.ID} {
		r := e.do(http.MethodPost, jobsPath(id, rt), body)
		assert.Equal(t, http.StatusConflict, r.code, r.raw)
		assert.Equal(t, "CLIENT_NOT_ACTIVE", r.errCode())
	}
	assert.Equal(t, "CLIENT_NOT_ACTIVE", e.do(http.MethodGet, secretPath(polled.ID, "sftp-password", rt), nil).errCode())
	assert.Empty(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"), "and is listed to no runtime")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_job_keys WHERE ingestion_id = ANY($1)`, []string{shared.ID, polled.ID}))
}

// The lease is checked in the transaction that makes the jobs (R6): a lease
// that ran out is LEASE_NOT_HELD there, and no job or key is written.
func TestJobs_LeaseCheckedWithTheJobs(t *testing.T) {
	e := newEnv(t)
	tg := e.makeTargets()
	polled := e.saveTarget("polled", sftpSource, tg.viaAccount(), true)
	rt := "rt-" + unique()
	require.Len(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"), 1)
	e.now = e.now.Add(4 * time.Minute) // past the 3-minute lease
	r := e.do(http.MethodPost, jobsPath(polled.ID, rt), map[string]any{"records": records(2, "late")})
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "LEASE_NOT_HELD", r.errCode())
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_job_keys WHERE ingestion_id = $1`, polled.ID))
}

// With no test clock the runtime API times leases by the database's clock
// (C2): poll, state and jobs agree on who holds the lease.
func TestRuntime_DatabaseClock(t *testing.T) {
	e := newEnv(t)
	e.now = time.Time{} // State.Now answers zero: the database's clock
	tg := e.makeTargets()
	polled := e.saveTarget("polled", sftpSource, tg.viaAccount(), true)
	rt := "rt-" + unique()
	r := e.poll(rt, ingestion.ContractVersion)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	require.Len(t, r.list("ingestions"), 1)
	asOf, err := time.Parse(time.RFC3339Nano, r.str("asOf"))
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), asOf, time.Minute)
	assert.Equal(t, http.StatusNoContent, e.do(http.MethodPut, base+"/state/"+polled.ID+"/watermark?runtimeId="+rt, map[string]any{"value": "1"}).code)
	j := e.do(http.MethodPost, jobsPath(polled.ID, rt), map[string]any{"records": records(1, "clock")})
	require.Equal(t, http.StatusOK, j.code, j.raw)
	assert.EqualValues(t, 1, j.json["created"])
}

// builtinKinds over the wire (C1): a runtime is listed only the built-in kinds
// it declares; one that sends none is the v2 baseline.
func TestPoll_BuiltinKinds(t *testing.T) {
	e := newEnv(t)
	queue := e.save("queue", sqsSource, true)
	e.save("files", sftpSource, true)
	r := e.do(http.MethodPost, base+"/poll", map[string]any{"runtimeId": "rt-" + unique(), "contractVersion": ingestion.ContractVersion, "builtinKinds": []string{"sqs", "ftp"}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	var own []string
	for _, it := range r.list("ingestions") {
		if it["applicationCode"] == e.app.Code {
			own = append(own, it["id"].(string))
		}
	}
	assert.Equal(t, []string{queue.ID}, own, "sqs only; a name the platform does not know is ignored")
	assert.Len(t, e.poll("rt-"+unique(), ingestion.MinContractVersion).list("ingestions"), 2, "an N-1 runtime that sends none carries the baseline")
}

// A failure's category over the wire (C3): rejected is stored and shows as a
// notice; absent is payload; anything else is refused.
func TestReport_FailureCategory(t *testing.T) {
	e := newEnv(t)
	i := e.save("queue", sqsSource, true)
	rt := "rt-" + unique()
	report := func(category any) response {
		f := map[string]any{"ingestionId": i.ID, "version": 1, "at": "2030-01-01T12:00:00Z", "kind": "NOT_DISPATCHED", "message": "target refused"}
		if category != nil {
			f["category"] = category
		}
		return e.do(http.MethodPost, base+"/report", map[string]any{"runtimeId": rt, "failures": []map[string]any{f}})
	}
	assert.Equal(t, http.StatusNoContent, report("rejected").code)
	assert.Equal(t, http.StatusNoContent, report(nil).code)
	assert.Equal(t, http.StatusBadRequest, report("fatal").code)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_failures WHERE ingestion_id = $1 AND category = 'rejected'`, i.ID))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_failures WHERE ingestion_id = $1 AND category = 'payload'`, i.ID))
}
