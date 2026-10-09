package sqs

import (
	"context"
	"crypto/md5" //nolint:gosec // G501: SQS's MD5OfBody, not a security use
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPollAcksMalformedMessageWithoutWaitingOutTheLinger pins the urgent mark on
// the malformed-message ack inside Poll. That ack runs on the consumer's poll
// goroutine and blocks until its DeleteMessageBatch answers; as a plain ack it
// sat in the delete batcher for the whole linger cap (5 s in production)
// waiting for a full batch, holding the queue's next poll for that long. The
// linger is set to an hour here, so only the urgent cut can return in time.
func TestPollAcksMalformedMessageWithoutWaitingOutTheLinger(t *testing.T) {
	const body = "this is not json"
	sum := md5.Sum([]byte(body)) //nolint:gosec // G401: see import
	var (
		mu      sync.Mutex
		deleted []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		raw, _ := io.ReadAll(r.Body)
		switch target := r.Header.Get("X-Amz-Target"); {
		case strings.HasSuffix(target, "ReceiveMessage"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Messages": []map[string]string{{
				"MessageId":     "broker-1",
				"ReceiptHandle": "rh-malformed",
				"Body":          body,
				"MD5OfBody":     hex.EncodeToString(sum[:]),
			}}})
		case strings.HasSuffix(target, "DeleteMessageBatch"):
			var in struct {
				Entries []struct{ Id, ReceiptHandle string }
			}
			_ = json.Unmarshal(raw, &in)
			ok := make([]map[string]string, 0, len(in.Entries))
			mu.Lock()
			for _, e := range in.Entries {
				deleted = append(deleted, e.ReceiptHandle)
				ok = append(ok, map[string]string{"Id": e.Id})
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"Successful": ok, "Failed": []any{}})
		default:
			http.Error(w, "unexpected target "+target, http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	q := newGuardQueue()
	q.queueName = "q"
	q.queueURL = srv.URL + "/000000000000/q"
	q.client = sqs.NewFromConfig(aws.Config{
		Region:           "us-east-1",
		Credentials:      aws.AnonymousCredentials{},
		BaseEndpoint:     aws.String(srv.URL),
		RetryMaxAttempts: 1,
	})
	q.del.lingerMax = time.Hour
	q.running.Store(true)
	defer q.Stop()

	start := time.Now()
	msgs, err := q.Poll(context.Background(), 10)
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 2*time.Second,
		"the malformed message's ack must not wait out the delete linger inside Poll")
	assert.Empty(t, msgs, "a malformed message is acked, not returned")
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"rh-malformed"}, deleted, "the malformed message is deleted from the broker")
}
