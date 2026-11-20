//go:build integration

package operations_test

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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The payload edit's write side (docs/payload-offload-design.md, sections
// 3.1 and 8): with the payload table written the edited job gets a payload
// row of its own, under its created_at, and the event and its other jobs keep
// the original; with it not written the payload column is written and
// payload_id cleared, so an edit of a job of the new shape is never lost. The
// cap is the payload service's largest payload either way.

// raised is an event of the new shape and the two jobs it raised, all
// pointing at one payload row.
type raised struct {
	eventID, payloadID string
	jobs               [2]string
	createdAt          time.Time
}

const editOriginal = `{"original": true,  "n":1}`

func raiseEvent(t *testing.T, pool *pgxpool.Pool, svc *payloadstore.Service) raised {
	t.Helper()
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Microsecond)
	row := payloadtest.Store(t, svc, pool, "", at, []byte(editOriginal))
	r := raised{eventID: tsid.GenerateUntyped(), payloadID: row.ID, createdAt: at}
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_events (id, type, source, time, created_at, payload_id, projected_at, fanned_out_at)
		 VALUES ($1, 'opstest:edit:raised', 'test://edit', $2, $2, $3, NOW(), NOW())`, r.eventID, at, row.ID)
	require.NoError(t, err)
	for i := range r.jobs {
		r.jobs[i] = tsid.GenerateUntyped()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs
			     (id, code, target_url, status, data_only, payload, payload_id, event_id, payload_content_type,
			      max_retries, attempt_count, created_at, updated_at)
			 VALUES ($1, 'opstest:edit:raised', 'http://example.invalid/hook', 'PENDING', TRUE, NULL, $2, $3,
			         'application/json', 3, 0, $4, $4)`, r.jobs[i], row.ID, r.eventID, at)
		require.NoError(t, err)
	}
	return r
}

// jobPayload is a job's payload columns and the data its payload row holds
// (resolved when offloaded, through repo).
func jobPayload(t *testing.T, repo *dispatchjob.Repository, pool *pgxpool.Pool, id string) (payload, payloadID *string, resolved string) {
	t.Helper()
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT payload, payload_id FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&payload, &payloadID))
	j, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	require.NoError(t, repo.ResolvePayload(context.Background(), j))
	if j.Payload != nil {
		resolved = *j.Payload
	}
	return payload, payloadID, resolved
}

// deliverEdited runs the delivery of job id to a loopback subscriber and
// returns the body it received.
func deliverEdited(t *testing.T, repo *dispatchjob.Repository, pool *pgxpool.Pool, id string) string {
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
	_, err := pool.Exec(context.Background(), `UPDATE msg_dispatch_jobs SET target_url = $2 WHERE id = $1`, id, sub.URL)
	require.NoError(t, err)
	da := scheduler.NewDispatchAuthService("edit-payload-secret")
	r := chi.NewRouter()
	processing.New(repo, da).Mount(r)
	proc := httptest.NewServer(r)
	defer proc.Close()
	body, _ := json.Marshal(map[string]string{"messageId": id})
	req, err := http.NewRequest(http.MethodPost, proc.URL+"/api/dispatch/process", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+da.Sign(id))
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

func updatedAt(t *testing.T, pool *pgxpool.Pool, id string) time.Time {
	t.Helper()
	var at time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT updated_at FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&at))
	return at
}

func TestEditPayload_NewShapeGivesTheJobItsOwnRow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.WritingService(t, store, nil)
	repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
	uow := testpg.NewUoW(t)
	r := raiseEvent(t, pool, svc)
	edited, sibling := r.jobs[0], r.jobs[1]
	claimRead := updatedAt(t, pool, edited)

	const fixed = `{"fixed":  true}`
	ev, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: edited, Payload: fixed})
	require.NoError(t, err)
	assert.Equal(t, len(editOriginal), ev.PreviousSize)

	payload, payloadID, resolved := jobPayload(t, repo, pool, edited)
	assert.Nil(t, payload, "no payload column")
	require.NotNil(t, payloadID)
	assert.NotEqual(t, r.payloadID, *payloadID, "a payload row of its own")
	assert.Equal(t, fixed, resolved)
	assert.Equal(t, r.payloadID, ev.PreviousPayloadID, "the audit event names the row replaced")
	assert.Equal(t, *payloadID, ev.PayloadID, "and the row made")
	var size int
	require.NoError(t, pool.QueryRow(ctx, `SELECT size FROM msg_payloads WHERE id = $1 AND created_at = $2`, *payloadID, r.createdAt).Scan(&size),
		"under the job's created_at")
	assert.Equal(t, len(fixed), size)

	// The sibling job and the event keep the original, which is still there.
	_, siblingID, siblingData := jobPayload(t, repo, pool, sibling)
	require.NotNil(t, siblingID)
	assert.Equal(t, r.payloadID, *siblingID)
	assert.Equal(t, editOriginal, siblingData)
	var eventPayload string
	require.NoError(t, pool.QueryRow(ctx, `SELECT payload_id FROM msg_events WHERE id = $1`, r.eventID).Scan(&eventPayload))
	assert.Equal(t, r.payloadID, eventPayload)

	// The claim guard: a mark-QUEUED by a claim that read the job before the
	// edit is refused; one that read it after goes through.
	moved, err := repo.Lifecycle().MarkQueued(ctx, []string{edited}, []time.Time{r.createdAt}, []time.Time{claimRead})
	require.NoError(t, err)
	assert.Zero(t, moved, "a claim that read the old payload is refused")
	moved, err = repo.Lifecycle().MarkQueued(ctx, []string{edited}, []time.Time{r.createdAt}, []time.Time{updatedAt(t, pool, edited)})
	require.NoError(t, err)
	assert.EqualValues(t, 1, moved)

	assert.Equal(t, fixed, deliverEdited(t, repo, pool, edited), "the edited job delivers the new bytes")
	assert.Equal(t, editOriginal, deliverEdited(t, repo, pool, sibling), "its sibling the original")
}

// The edit's cap is the payload service's largest payload: an edit larger
// than the old 256 KiB is accepted, one over the service's largest refused.
func TestEditPayload_CapIsTheLargestPayload(t *testing.T) {
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)

	t.Run("over 256 KiB, under the cap", func(t *testing.T) {
		store := payloadtest.NewCountingStore()
		svc := payloadtest.WritingService(t, store, nil)
		repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
		r := raiseEvent(t, pool, svc)
		big := `{"pad":"` + strings.Repeat("k", 300<<10) + `"}`
		_, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: r.jobs[0], Payload: big})
		require.NoError(t, err)
		_, _, resolved := jobPayload(t, repo, pool, r.jobs[0])
		assert.Equal(t, big, resolved)
		assert.EqualValues(t, 1, store.Puts(), "offloaded")
	})

	t.Run("over the cap", func(t *testing.T) {
		svc := payloadtest.WritingService(t, payloadtest.NewCountingStore(), func(c *payloadstore.Config) { c.InlineMaxBytes = 8192; c.MaxBytes = 16384 })
		repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
		r := raiseEvent(t, pool, svc)
		_, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo),
			operations.EditPayloadCommand{ID: r.jobs[0], Payload: `"` + strings.Repeat("o", 16384) + `"`})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "PAYLOAD_TOO_LARGE")
		_, payloadID, _ := jobPayload(t, repo, pool, r.jobs[0])
		assert.Equal(t, r.payloadID, *payloadID, "unchanged")
	})

	t.Run("switch off: raised too", func(t *testing.T) {
		svc := payloadtest.Service(t, nil)
		repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
		r := raiseEvent(t, pool, svc)
		big := `"` + strings.Repeat("m", 300<<10) + `"`
		_, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: r.jobs[0], Payload: big})
		require.NoError(t, err)
	})
}

// The switch off: an edit of a job of the new shape writes the payload column
// and clears payload_id (readers prefer payload_id, so leaving it would lose
// the edit), and the edited bytes are what is delivered. The event and the
// other job keep the payload row.
func TestEditPayload_SwitchOffClearsThePayloadID(t *testing.T) {
	pool := testpg.Pool(t)
	svc := payloadtest.Service(t, nil)
	require.False(t, svc.TableWrites())
	repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
	r := raiseEvent(t, pool, svc)

	const fixed = `{"fixed":"legacy"}`
	ev, err := runAuthorized(testpg.NewUoW(t), operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: r.jobs[0], Payload: fixed})
	require.NoError(t, err)
	assert.Equal(t, r.payloadID, ev.PreviousPayloadID, "the audit event names the row replaced")
	assert.Empty(t, ev.PayloadID, "and no new row: the payload column was written")
	payload, payloadID, resolved := jobPayload(t, repo, pool, r.jobs[0])
	assert.Nil(t, payloadID, "payload_id cleared")
	require.NotNil(t, payload)
	assert.Equal(t, fixed, *payload)
	assert.Equal(t, fixed, resolved)
	assert.Equal(t, fixed, deliverEdited(t, repo, pool, r.jobs[0]))

	_, siblingID, siblingData := jobPayload(t, repo, pool, r.jobs[1])
	require.NotNil(t, siblingID)
	assert.Equal(t, r.payloadID, *siblingID)
	assert.Equal(t, editOriginal, siblingData)
}

// A job of the legacy shape (its payload in the payload column) has no payload
// row to name: the audit event leaves previousPayloadId out, and names the
// new row only when the payload table is written.
func TestEditPayload_AuditPayloadIDsOfALegacyJob(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	legacyJob := func(t *testing.T) string {
		t.Helper()
		id := tsid.GenerateUntyped()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs
			     (id, code, target_url, status, data_only, payload, payload_content_type, max_retries, attempt_count)
			 VALUES ($1, 'opstest:edit:legacy', 'http://example.invalid/hook', 'FAILED', TRUE, $2, 'application/json', 3, 1)`,
			id, editOriginal)
		require.NoError(t, err)
		return id
	}
	const fixed = `{"fixed":"audit"}`

	t.Run("switch on", func(t *testing.T) {
		repo := dispatchjob.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, payloadtest.NewCountingStore(), nil))
		id := legacyJob(t)
		ev, err := runAuthorized(testpg.NewUoW(t), operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: id, Payload: fixed})
		require.NoError(t, err)
		assert.Empty(t, ev.PreviousPayloadID)
		_, payloadID, _ := jobPayload(t, repo, pool, id)
		require.NotNil(t, payloadID)
		assert.Equal(t, *payloadID, ev.PayloadID)
	})
	t.Run("switch off", func(t *testing.T) {
		repo := dispatchjob.NewRepository(pool).UsePayloads(payloadtest.Service(t, nil))
		ev, err := runAuthorized(testpg.NewUoW(t), operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: legacyJob(t), Payload: fixed})
		require.NoError(t, err)
		assert.Empty(t, ev.PreviousPayloadID)
		assert.Empty(t, ev.PayloadID)
	})
}
