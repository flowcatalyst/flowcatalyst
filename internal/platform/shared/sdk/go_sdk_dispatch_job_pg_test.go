//go:build integration

package sdk

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outbox"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outboxpgx"
)

// The Go SDK's DISPATCH_JOB outbox rows (outboxpgx.Sink.WriteDispatchJob) are
// accepted by POST /api/dispatch-jobs/batch exactly as the outbox processor
// sends them: the job's defaults (TASK, source from the code, payload {} as
// application/json), every optional member, metadata as the key/value array,
// and a client named by its identifier (clientCode, F14) resolved to the
// client.
func TestDispatchJobsBatch_AcceptsTheGoSDKOutboxRows(t *testing.T) {
	f := newClientCodeFixture(t)
	ctx := context.Background()
	require.NoError(t, outboxpgx.InitSchema(ctx, f.pool))
	_, err := f.pool.Exec(ctx, `DELETE FROM outbox_messages`)
	require.NoError(t, err)

	defaultsID, fullID := tsid.GenerateUntyped(), tsid.GenerateUntyped()
	full, err := outbox.NewDispatchJob("cc:billing:export", "https://target.test/export?x=1").
		WithID(fullID).WithKind(outbox.DispatchKindEvent).WithSource("cc:billing").
		WithSubject("billing.invoice.inv_9").WithClientCode(f.aCode).
		WithMessageGroup("invoice:inv_9").WithMode(outbox.DispatchModeBlockOnError).
		WithSequence(2).WithTimeoutSeconds(15).WithMaxRetries(4).
		WithMetadata("tenant", "acme").WithMetadata("attempt", "1").
		WithQueue(outbox.DispatchQueueHighPriority).WithDescriptor("export one invoice").
		WithCorrelationID("corr-3").DataOnlyEnabled().
		WithJSONPayload(map[string]any{"invoiceId": "inv_9"})
	require.NoError(t, err)

	sink := outboxpgx.NewSink(outboxpgx.Config{ClientID: "local-partition"})
	tx, err := f.pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sink.WriteDispatchJob(ctx, tx, outbox.NewDispatchJob("cc:billing:export", "https://target.test/export").WithID(defaultsID)))
	require.NoError(t, sink.WriteDispatchJob(ctx, tx, full))
	require.Error(t, sink.WriteDispatchJob(ctx, tx, outbox.NewDispatchJob("cc:x", "https://user:pw@target.test/")),
		"a job the platform would refuse is never written")
	require.NoError(t, tx.Commit(ctx))

	rows, err := f.pool.Query(ctx, `SELECT id, payload FROM outbox_messages WHERE type = 'DISPATCH_JOB' ORDER BY created_at, id`)
	require.NoError(t, err)
	var ids, payloads []string
	for rows.Next() {
		var id, p string
		require.NoError(t, rows.Scan(&id, &p))
		ids = append(ids, id)
		payloads = append(payloads, p)
	}
	rows.Close()
	require.ElementsMatch(t, []string{defaultsID, fullID}, ids, "the outbox row id is the job id")

	status, body := f.post(t, "anchor", "/api/dispatch-jobs/batch", `{"items":[`+strings.Join(payloads, ",")+`]}`)
	require.Equal(t, http.StatusCreated, status, body)

	type job struct {
		code, source, kind, payload, contentType, mode         string
		dataOnly                                               bool
		sequence, timeout, retries                             int
		clientID, messageGroup, correlationID, metadata, queue *string
	}
	read := func(id string) job {
		var j job
		require.NoError(t, f.pool.QueryRow(ctx,
			`SELECT code, source, kind, payload, payload_content_type, mode, queue, data_only, sequence,
			        timeout_seconds, max_retries, client_id, message_group, correlation_id, metadata::text
			   FROM msg_dispatch_jobs WHERE id = $1`, id).
			Scan(&j.code, &j.source, &j.kind, &j.payload, &j.contentType, &j.mode, &j.queue, &j.dataOnly, &j.sequence,
				&j.timeout, &j.retries, &j.clientID, &j.messageGroup, &j.correlationID, &j.metadata))
		return j
	}

	d := read(defaultsID)
	assert.Equal(t, "cc", d.source, "source defaults to the code's application")
	assert.Equal(t, "TASK", d.kind)
	assert.Equal(t, "{}", d.payload)
	assert.Equal(t, "application/json", d.contentType)
	assert.Nil(t, d.clientID, "the client_id column is the app's own partition, never sent")

	j := read(fullID)
	assert.Equal(t, "cc:billing", j.source)
	assert.Equal(t, "EVENT", j.kind)
	assert.JSONEq(t, `{"invoiceId":"inv_9"}`, j.payload)
	assert.True(t, j.dataOnly)
	assert.Equal(t, "BLOCK_ON_ERROR", j.mode)
	require.NotNil(t, j.queue)
	assert.Equal(t, "HIGH_PRIORITY", *j.queue)
	assert.Equal(t, []int{2, 15, 4}, []int{j.sequence, j.timeout, j.retries})
	require.NotNil(t, j.clientID, "clientCode resolved (F14)")
	assert.Equal(t, f.aID, *j.clientID)
	require.NotNil(t, j.messageGroup)
	assert.Equal(t, "invoice:inv_9", *j.messageGroup)
	require.NotNil(t, j.correlationID)
	assert.Equal(t, "corr-3", *j.correlationID)
	require.NotNil(t, j.metadata)
	assert.Contains(t, *j.metadata, "tenant")
	assert.Contains(t, *j.metadata, "attempt")
}
