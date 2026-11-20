//go:build integration

package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Jobs created directly through the SDK routes with the payload table
// written (FC_PAYLOAD_TABLE_WRITES, docs/payload-offload-design.md, sections
// 3.1, 6 to 8): a payload row of the job's own under its created_at, the job
// pointing at it with no payload column, offloaded above the threshold in one
// segment per client, refusals per item; and the delivery posts the bytes.

const jobOdd = `{"z": 1,"a":  [3, 1] , "z":2}`

type jobRig struct {
	pool  *pgxpool.Pool
	store *payloadtest.CountingStore
	repo  *dispatchjob.Repository
	api   string
	proc  string
	auth  *scheduler.DispatchAuthService
}

func newJobRig(t *testing.T, store *payloadtest.CountingStore, svc *payloadstore.Service) *jobRig {
	t.Helper()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), anchorAC())))
		})
	})
	RegisterRoutes(r, &DispatchJobsBatchState{Repo: repo})
	api := httptest.NewServer(r)
	t.Cleanup(api.Close)
	da := scheduler.NewDispatchAuthService("sdk-payload-secret")
	pr := chi.NewRouter()
	processing.New(repo, da).Mount(pr)
	proc := httptest.NewServer(pr)
	t.Cleanup(proc.Close)
	return &jobRig{pool: pool, store: store, repo: repo, api: api.URL, proc: proc.URL, auth: da}
}

// storedJob is a job's payload columns and its payload row.
type storedJob struct {
	payload, payloadID, pData, pRef *string
	pSize                           *int32
	createdAt                       time.Time
}

func readJob(t *testing.T, pool *pgxpool.Pool, id string) storedJob {
	t.Helper()
	var j storedJob
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT j.payload, j.payload_id, j.created_at, p.data, p.ref, p.size
		   FROM msg_dispatch_jobs j
		   LEFT JOIN msg_payloads p ON p.id = j.payload_id AND p.created_at = j.created_at
		  WHERE j.id = $1`, id).Scan(&j.payload, &j.payloadID, &j.createdAt, &j.pData, &j.pRef, &j.pSize))
	return j
}

// deliverTo points the job at a test subscriber on loopback and runs the
// delivery callback; it returns the body the subscriber received.
func (r *jobRig) deliverTo(t *testing.T, jobID string) string {
	t.Helper()
	prev := netguard.Default.AllowLoopback
	netguard.Default.AllowLoopback = true
	t.Cleanup(func() { netguard.Default.AllowLoopback = prev })
	var mu sync.Mutex
	var got string
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		mu.Lock()
		got = string(b)
		mu.Unlock()
	}))
	defer sub.Close()
	_, err := r.pool.Exec(context.Background(), `UPDATE msg_dispatch_jobs SET target_url = $2 WHERE id = $1`, jobID, sub.URL)
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]string{"messageId": jobID})
	req, err := http.NewRequest(http.MethodPost, r.proc+"/api/dispatch/process", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.auth.Sign(jobID))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	mu.Lock()
	defer mu.Unlock()
	return got
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jobItem(code, payload string, extra string) string {
	return `{"code":"` + code + `","targetUrl":"https://target.test/hook","dataOnly":true,` +
		`"serviceAccountId":"sac_x","payload":` + jsonString(payload) + extra + `}`
}

func TestCreateDispatchJob_NewShape(t *testing.T) {
	store := payloadtest.NewCountingStore()
	rig := newJobRig(t, store, payloadtest.WritingService(t, store, func(c *payloadstore.Config) { c.InlineMaxBytes = 8192; c.MaxBytes = 16384 }))
	code := "sdkpayload:" + strings.ToLower(tsid.GenerateUntyped()) + ":job"

	resp, body := postJSON(t, rig.api+"/api/dispatch-jobs", jobItem(code, jobOdd, ""))
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	var created CreatedResponse
	require.NoError(t, json.Unmarshal([]byte(body), &created))
	j := readJob(t, rig.pool, created.ID)
	assert.Nil(t, j.payload, "no payload column")
	require.NotNil(t, j.payloadID)
	require.NotNil(t, j.pData, "the payload row is under the job's created_at")
	assert.Equal(t, jobOdd, *j.pData)
	assert.EqualValues(t, len(jobOdd), *j.pSize)
	assert.Equal(t, jobOdd, rig.deliverTo(t, created.ID), "the delivery posts the bytes")

	t.Run("too large", func(t *testing.T) {
		resp, body := postJSON(t, rig.api+"/api/dispatch-jobs", jobItem(code, strings.Repeat("x", 20000), ""))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, body, `"error":"PAYLOAD_TOO_LARGE"`)
	})
	t.Run("store unavailable", func(t *testing.T) {
		store.FailWrites(errors.New("bucket unreachable"))
		t.Cleanup(func() { store.FailWrites(nil) })
		resp, body := postJSON(t, rig.api+"/api/dispatch-jobs", jobItem(code, strings.Repeat("y", 12000), ""))
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		assert.Contains(t, body, `"error":"PAYLOAD_STORE_UNAVAILABLE"`)
	})
}

func TestDispatchJobsBatch_NewShape(t *testing.T) {
	store := payloadtest.NewCountingStore()
	rig := newJobRig(t, store, payloadtest.WritingService(t, store, func(c *payloadstore.Config) { c.InlineMaxBytes = 8192; c.MaxBytes = 16384 }))
	code := "sdkpayload:" + strings.ToLower(tsid.GenerateUntyped()) + ":job"
	big := func(c string) string { return `{"pad":"` + strings.Repeat(c, 3*payloadtest.OffloadMinBytes) + `"}` }
	client := `,"clientId":"clt_SDKPAYLOAD1"`
	other := `,"clientId":"clt_SDKPAYLOAD2"`
	key := `,"idempotencyKey":"k-` + tsid.GenerateUntyped() + `"`

	items := []string{
		jobItem(code, big("a"), client),
		jobItem(code, jobOdd, client),
		jobItem(code, big("b"), client),
		jobItem(code, big("c"), other),
		jobItem(code, strings.Repeat("t", 20000), client), // too large
		jobItem(code, "a\u0000b", client),                 // not text: a NUL
		jobItem(code, `{"keyed":1}`, client+key),
		jobItem(code, `{"keyed":2}`, client+key), // the same key: answers the first
	}
	resp, body := postJSON(t, rig.api+"/api/dispatch-jobs/batch", `{"items":[`+strings.Join(items, ",")+`]}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	var out BatchResponse
	require.NoError(t, json.Unmarshal([]byte(body), &out))
	r := out.Results
	require.Len(t, r, len(items))
	for _, i := range []int{0, 1, 2, 3, 6, 7} {
		assert.Equal(t, "SUCCESS", r[i].Status, "item %d: %s", i, r[i].Error)
	}
	assert.Equal(t, "BAD_REQUEST", r[4].Status)
	assert.True(t, strings.HasPrefix(r[4].Error, "PAYLOAD_TOO_LARGE"), r[4].Error)
	assert.Equal(t, "BAD_REQUEST", r[5].Status)
	assert.True(t, strings.HasPrefix(r[5].Error, "PAYLOAD_NOT_TEXT"), r[5].Error)
	assert.Equal(t, r[6].ID, r[7].ID, "the idempotency key answers the first job")
	assert.EqualValues(t, 2, store.Puts(), "one segment per client")

	for _, i := range []int{0, 2, 3} {
		j := readJob(t, rig.pool, r[i].ID)
		assert.Nil(t, j.payload)
		require.NotNil(t, j.pRef, "item %d offloaded", i)
		assert.Nil(t, j.pData)
	}
	assert.Equal(t, big("a"), rig.deliverTo(t, r[0].ID), "the delivery resolves the reference")
	assert.Equal(t, jobOdd, rig.deliverTo(t, r[1].ID))
	var refused int
	require.NoError(t, rig.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM msg_dispatch_jobs WHERE id = ANY($1)`, []string{r[4].ID, r[5].ID}).Scan(&refused))
	assert.Zero(t, refused, "refused items make no job")

	t.Run("store unavailable", func(t *testing.T) {
		store.FailWrites(errors.New("bucket unreachable"))
		t.Cleanup(func() { store.FailWrites(nil) })
		resp, body := postJSON(t, rig.api+"/api/dispatch-jobs/batch", `{"items":[`+
			jobItem(code, strings.Repeat("u", 6000), client)+","+jobItem(code, strings.Repeat("v", 12000), client)+`]}`)
		require.Equal(t, http.StatusCreated, resp.StatusCode, body)
		var out BatchResponse
		require.NoError(t, json.Unmarshal([]byte(body), &out))
		assert.Equal(t, "SUCCESS", out.Results[0].Status, "kept in the database")
		assert.Equal(t, "INTERNAL_ERROR", out.Results[1].Status, "retried by the outbox processor")
		assert.True(t, strings.HasPrefix(out.Results[1].Error, "PAYLOAD_STORE_UNAVAILABLE"))
		j := readJob(t, rig.pool, out.Results[0].ID)
		require.NotNil(t, j.pData)
		assert.Nil(t, j.pRef)
	})
}

// The switch off: the legacy shape, no payload row, no new limit.
func TestDispatchJobsBatch_SwitchOffWritesTheLegacyShape(t *testing.T) {
	store := payloadtest.NewCountingStore()
	rig := newJobRig(t, store, payloadtest.Service(t, store))
	code := "sdkpayload:" + strings.ToLower(tsid.GenerateUntyped()) + ":job"
	large := `{"pad":"` + strings.Repeat("o", 3*payloadtest.OffloadMinBytes) + `"}`
	resp, body := postJSON(t, rig.api+"/api/dispatch-jobs/batch", `{"items":[`+jobItem(code, large, "")+`]}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	var out BatchResponse
	require.NoError(t, json.Unmarshal([]byte(body), &out))
	j := readJob(t, rig.pool, out.Results[0].ID)
	assert.Nil(t, j.payloadID)
	require.NotNil(t, j.payload)
	assert.Equal(t, large, *j.payload)
	assert.Zero(t, store.Puts())
}
