package sqs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An SQS call on a stalled connection must be cut off, not block its caller for
// good. The SDK client has no timeout of its own; an acknowledgement that never
// returns held a pool worker and its concurrency slot indefinitely.
func TestAckOnAStalledConnectionReturnsAfterTheCallTimeout(t *testing.T) {
	stalled := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-stalled // never answers
	}))
	defer srv.Close()
	defer close(stalled)

	old := apiCallTimeout
	apiCallTimeout = 200 * time.Millisecond
	q := newGuardQueue()
	// Drainers read apiCallTimeout; wait for them before restoring it.
	defer func() { q.Stop(); q.del.drainers.Wait(); apiCallTimeout = old }()
	q.queueURL = srv.URL + "/000000000000/q"
	q.client = sqs.NewFromConfig(aws.Config{
		Region:           "us-east-1",
		Credentials:      aws.AnonymousCredentials{},
		BaseEndpoint:     aws.String(srv.URL),
		RetryMaxAttempts: 1,
	})

	start := time.Now()
	err := q.Ack(context.Background(), "receipt-1", "msg-1")
	elapsed := time.Since(start)

	require.Error(t, err, "a call that never answers must fail, not hang")
	assert.Less(t, elapsed, 5*time.Second, "the call timeout (200ms here) must bound it; took %s", elapsed)
}
