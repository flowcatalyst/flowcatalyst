package sqs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// Poll must request no message attributes (nothing in the package reads them)
// and must tolerate a body whose MD5 does not match (checksum validation is
// off, to save the per-message hash).
func TestPollRequestsNoAttributesAndSkipsChecksumValidation(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{"Messages":[{"MessageId":"m1","ReceiptHandle":"r1",` +
			`"Body":"{\"id\":\"x\"}","MD5OfBody":"00000000000000000000000000000000"}]}`))
	}))
	defer srv.Close()

	q := newGuardQueue()
	q.receiptPolledAt = map[string]time.Time{}
	q.queueURL = srv.URL + "/000000000000/q"
	q.client = newSQSClient(aws.Config{
		Region:           "us-east-1",
		Credentials:      aws.AnonymousCredentials{},
		BaseEndpoint:     aws.String(srv.URL),
		RetryMaxAttempts: 1,
	})
	q.running.Store(true)

	msgs, err := q.Poll(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1, "a mismatched MD5OfBody must not fail the poll")
	for _, k := range []string{"MessageSystemAttributeNames", "MessageAttributeNames", "AttributeNames"} {
		assert.False(t, strings.Contains(gotBody, k), "request must not ask for %s: %s", k, gotBody)
	}
}

func TestReceiptPruneIsPeriodicButStillExpiresEntries(t *testing.T) {
	q := &Queue{receiptPolledAt: map[string]time.Time{}}
	old := time.Now().Add(-MaxVisibility - time.Minute)

	q.receiptPolledAt["a"] = old
	q.evictStaleReceiptTimestampsLocked()
	assert.NotContains(t, q.receiptPolledAt, "a", "first scan must run and drop the expired entry")

	q.receiptPolledAt["b"] = old
	q.evictStaleReceiptTimestampsLocked()
	assert.Contains(t, q.receiptPolledAt, "b", "a second scan inside the interval is skipped")

	q.lastReceiptPrune = time.Now().Add(-receiptPruneInterval - time.Second)
	q.receiptPolledAt["fresh"] = time.Now()
	q.evictStaleReceiptTimestampsLocked()
	assert.NotContains(t, q.receiptPolledAt, "b", "after the interval the expired entry goes")
	assert.Contains(t, q.receiptPolledAt, "fresh", "live entries are kept")
}

// The platform creates a dispatch queue on first publish, so the router can be
// configured to consume a queue that does not exist yet (or lose one under a
// running consumer). Poll reports that as queue.ErrQueueMissing, which the
// router handles without a warning, not as an ordinary poll failure.
func TestPollReportsAMissingQueueAsErrQueueMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.Header().Set("X-Amzn-Query-Error", "AWS.SimpleQueueService.NonExistentQueue;Sender")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"__type":"com.amazonaws.sqs#QueueDoesNotExist","message":"The specified queue does not exist."}`))
	}))
	defer srv.Close()

	q := newGuardQueue()
	q.receiptPolledAt = map[string]time.Time{}
	q.queueURL = srv.URL + "/000000000000/q"
	q.client = newSQSClient(aws.Config{
		Region:           "us-east-1",
		Credentials:      aws.AnonymousCredentials{},
		BaseEndpoint:     aws.String(srv.URL),
		RetryMaxAttempts: 1,
	})
	q.running.Store(true)

	msgs, err := q.Poll(context.Background(), 10)
	require.ErrorIs(t, err, queue.ErrQueueMissing, "a missing queue is its own outcome, not a poll failure")
	assert.Empty(t, msgs)
}
