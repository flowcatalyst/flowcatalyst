package outbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
)

// forwardOnce sends items through the dispatcher to a capturing server and
// returns the request body, decoded the way POST /api/dispatch-jobs/batch
// decodes it (sdk.BatchRequest).
func forwardOnce(t *testing.T, items ...Item) (sdk.BatchRequest, string) {
	t.Helper()
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/dispatch-jobs/batch", r.URL.Path)
		got, _ = io.ReadAll(r.Body)
		results := make([]map[string]any, len(items))
		for i := range items {
			results[i] = map[string]any{"id": "j", "status": "SUCCESS"}
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer srv.Close()
	out := NewHTTPDispatcher(srv.URL, "", 5*time.Second).SendBatch(context.Background(), items)
	for _, it := range items {
		require.Equal(t, common.OutboxSuccess, out[it.ID].Status, out[it.ID].Message)
	}
	var br sdk.BatchRequest
	require.NoError(t, json.Unmarshal(got, &br), "the platform must decode what the forwarder sends: %s", got)
	return br, string(got)
}

// A DISPATCH_JOB row written by the Laravel SDK before 2026-11-05 carries
// metadata as an object, which the batch endpoint cannot decode (the whole
// batch would be a terminal 400). The forwarder turns it into the batch's
// {key, value} list, key-sorted, and carries every other field as written:
// connectionId, idempotencyKey and clientCode reach the platform.
func TestSendBatch_DispatchJobPayloadIsInTheBatchShape(t *testing.T) {
	legacy := Item{ID: "ob1", ItemType: common.OutboxItemDispatchJob, Payload: json.RawMessage(`{
		"code":"integral:billing:invoice:sync","targetUrl":"https://integral.test/hook","payload":"{}",
		"connectionId":"con_0000000000001","idempotencyKey":"order-1","clientCode":"acme","clientId":"clt_0000000000001",
		"metadata":{"tenant":"acme","attempt":2,"orderId":"o-1"},"headers":{"X-A":"b"}}`)}
	current := Item{ID: "ob2", ItemType: common.OutboxItemDispatchJob, Payload: json.RawMessage(`{
		"code":"integral:billing:invoice:sync","targetUrl":"https://integral.test/hook","payload":"{}",
		"metadata":[{"key":"orderId","value":"o-2"}]}`)}
	bare := Item{ID: "ob3", ItemType: common.OutboxItemDispatchJob, Payload: json.RawMessage(`{"code":"integral:x:y","targetUrl":"https://integral.test/hook"}`)}

	br, raw := forwardOnce(t, legacy, current, bare)
	require.Len(t, br.Items, 3, raw)

	it := br.Items[0]
	assert.Equal(t, []dispatchjob.Metadata{{Key: "attempt", Value: "2"}, {Key: "orderId", Value: "o-1"}, {Key: "tenant", Value: "acme"}}, it.Metadata)
	if assert.NotNil(t, it.ConnectionID) {
		assert.Equal(t, "con_0000000000001", *it.ConnectionID)
	}
	if assert.NotNil(t, it.IdempotencyKey) {
		assert.Equal(t, "order-1", *it.IdempotencyKey)
	}
	if assert.NotNil(t, it.ClientCode) {
		assert.Equal(t, "acme", *it.ClientCode)
	}
	if assert.NotNil(t, it.ClientID) {
		assert.Equal(t, "clt_0000000000001", *it.ClientID)
	}
	assert.Equal(t, "integral:billing:invoice:sync", it.Code)

	assert.Equal(t, []dispatchjob.Metadata{{Key: "orderId", Value: "o-2"}}, br.Items[1].Metadata, "a list is forwarded as is")
	assert.Nil(t, br.Items[2].Metadata)
}

// Only DISPATCH_JOB rows are touched: an event's payload goes out byte for
// byte, metadata-shaped fields and all.
func TestSendBatch_EventPayloadIsVerbatim(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"id": "e", "status": "SUCCESS"}}})
	}))
	defer srv.Close()
	ev := Item{ID: "ob1", ItemType: common.OutboxItemEvent, Payload: json.RawMessage(`{"type":"a:b:c","metadata":{"z":"1"}}`)}
	NewHTTPDispatcher(srv.URL, "", 5*time.Second).Send(context.Background(), ev)
	assert.JSONEq(t, `{"items":[{"type":"a:b:c","metadata":{"z":"1"}}]}`, string(got))
}

func TestNormalizeDispatchJobPayload_LeavesWhatItCannotRead(t *testing.T) {
	for _, in := range []string{`not json`, `[1,2]`, `{"metadata":null}`, `{"metadata":"x"}`, `{"code":"a"}`} {
		assert.Equal(t, in, string(normalizeDispatchJobPayload(json.RawMessage(in))), in)
	}
}
