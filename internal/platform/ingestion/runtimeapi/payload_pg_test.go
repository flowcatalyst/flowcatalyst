//go:build integration

package runtimeapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
)

// An ingestion's records with the payload table written
// (FC_PAYLOAD_TABLE_WRITES, docs/payload-offload-design.md, section 3.1):
// each job or event gets a payload row of its own, and a record whose payload
// is refused answers why in its own result and makes nothing.
func TestJobs_PayloadTableWrites(t *testing.T) {
	store := payloadtest.NewCountingStore()
	e := newEnvWith(t, payloadtest.WritingService(t, store, func(c *payloadstore.Config) { c.InlineMaxBytes = 8192; c.MaxBytes = 16384 }))
	tg := e.makeTargets()
	jobs := e.saveTarget("payload-jobs", sqsSource, tg.viaAccount(), true)
	code := e.eventType()
	events := e.saveTarget("payload-events", sqsSource, e.eventTarget(code, ""), true)
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion).code)
	big := `{"pad":"` + strings.Repeat("i", 20000) + `"}`
	offloaded := `{"pad":"` + strings.Repeat("o", 3*payloadtest.OffloadMinBytes) + `"}`

	// payloadOf answers the bytes of the payload row a job or event points at.
	payloadOf := func(table, id string) (data *string, ref *string) {
		t.Helper()
		require.NoError(t, e.pool.QueryRow(context.Background(),
			`SELECT p.data, p.ref FROM `+table+` x JOIN msg_payloads p ON p.id = x.payload_id AND p.created_at = x.created_at WHERE x.id = $1`, id).
			Scan(&data, &ref))
		return data, ref
	}

	for _, tc := range []struct {
		name  string
		id    string
		table string
	}{{"jobs", jobs.ID, "msg_dispatch_jobs"}, {"events", events.ID, "msg_events"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := e.do(http.MethodPost, jobsPath(tc.id, rt), map[string]any{"records": []map[string]any{
				{"payload": `{"a":  1}`, "externalId": tc.name + "-0"},
				{"payload": big, "externalId": tc.name + "-1"},
				{"payload": offloaded, "externalId": tc.name + "-2"},
			}})
			require.Equal(t, http.StatusOK, r.code, r.raw)
			assert.EqualValues(t, 2, r.json["created"])
			results := r.list("results")
			require.Len(t, results, 3)
			data, _ := payloadOf(tc.table, results[0]["id"].(string))
			require.NotNil(t, data)
			assert.Equal(t, `{"a":  1}`, *data, "the record's bytes")
			assert.Contains(t, results[1]["error"], "PAYLOAD_TOO_LARGE")
			assert.NotContains(t, results[1], "id")
			_, ref := payloadOf(tc.table, results[2]["id"].(string))
			assert.NotNil(t, ref, "offloaded")
		})
	}
	assert.Zero(t, e.count(`SELECT count(*) FROM ing_job_keys WHERE ingestion_id = $1 AND external_id = 'jobs-1'`, jobs.ID),
		"a refused record claims no key")
}
