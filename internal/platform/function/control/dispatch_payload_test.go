//go:build integration

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A function's dispatch.create with the payload table written
// (FC_PAYLOAD_TABLE_WRITES, docs/payload-offload-design.md, section 3.1): the
// job gets a payload row of its own under its created_at, and its delivery
// posts the bytes. The 256 KiB cap stays.
func TestDispatch_NewShapePayloadRow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	f := newDispatchFixture(t, nil, nil)
	store := payloadtest.NewCountingStore()
	f.s.Jobs = dispatchjob.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, store, nil))

	for name, payload := range map[string]string{
		"held in the database": `{"order": 42,  "a":1}`,
		"offloaded":            `{"pad":"` + strings.Repeat("f", 3*payloadtest.OffloadMinBytes) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := f.call(anchorCtx(), f.req(abi.Dispatch{}, payload))
			require.NoError(t, err)
			var legacy, payloadID, pData, pRef *string
			var createdAt time.Time
			var size *int32
			require.NoError(t, pool.QueryRow(ctx,
				`SELECT j.payload, j.payload_id, j.created_at, p.data, p.ref, p.size
				   FROM msg_dispatch_jobs j
				   LEFT JOIN msg_payloads p ON p.id = j.payload_id AND p.created_at = j.created_at
				  WHERE j.id = $1`, out.Body.JobID).Scan(&legacy, &payloadID, &createdAt, &pData, &pRef, &size))
			assert.Nil(t, legacy)
			require.NotNil(t, payloadID)
			require.NotNil(t, size, "the payload row is under the job's created_at")
			assert.EqualValues(t, len(payload), *size)
			assert.Equal(t, pRef != nil, len(payload) > payloadtest.OffloadMinBytes)
			assert.Equal(t, payload, deliverJob(t, f.s.Jobs, out.Body.JobID))
		})
	}

	_, err := f.call(anchorCtx(), f.req(abi.Dispatch{}, `"`+strings.Repeat("x", 300<<10)+`"`))
	require.Error(t, err, "the 256 KiB cap on dispatch.create stays")
	assert.Contains(t, err.Error(), "256 KiB")
}

// deliverJob points the job at a loopback subscriber (with no connection, so
// it is delivered bare) and runs the delivery callback; it returns the body
// the subscriber received.
func deliverJob(t *testing.T, repo *dispatchjob.Repository, jobID string) string {
	t.Helper()
	prev := netguard.Default.AllowLoopback
	netguard.Default.AllowLoopback = true
	t.Cleanup(func() { netguard.Default.AllowLoopback = prev })
	got := make(chan string, 1)
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- string(b)
	}))
	defer sub.Close()
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE msg_dispatch_jobs SET target_url = $2, connection_id = NULL WHERE id = $1`, jobID, sub.URL)
	require.NoError(t, err)

	da := scheduler.NewDispatchAuthService("control-payload-secret")
	r := chi.NewRouter()
	processing.New(repo, da).Mount(r)
	proc := httptest.NewServer(r)
	defer proc.Close()
	body, _ := json.Marshal(map[string]string{"messageId": jobID})
	req, err := http.NewRequest(http.MethodPost, proc.URL+"/api/dispatch/process", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+da.Sign(jobID))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	select {
	case b := <-got:
		return b
	default:
		t.Fatalf("the subscriber received nothing: %s", raw)
		return ""
	}
}
