//go:build integration

package sqs

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// localstackEndpoint is where these tests expect LocalStack's SQS. Override with
// LOCALSTACK_ENDPOINT. Tests SKIP (not fail) when nothing is listening, so a
// machine or CI runner without Docker still runs the rest of the suite:
//
//	docker run -d -p 4566:4566 -e SERVICES=sqs localstack/localstack:3.0
func localstackEndpoint() string {
	if v := os.Getenv("LOCALSTACK_ENDPOINT"); v != "" {
		return v
	}
	return "http://localhost:4566"
}

// newLocalstackQueue provisions a fresh SQS queue and returns a *Queue wired to
// it. Visibility timeout is 1s so a redelivery can be provoked without a long
// sleep; long-poll wait is 0 for the same reason.
func newLocalstackQueue(t *testing.T, name string) *Queue {
	t.Helper()
	endpoint := localstackEndpoint()

	conn, err := net.DialTimeout("tcp", stripScheme(endpoint), 300*time.Millisecond)
	if err != nil {
		t.Skipf("LocalStack not reachable at %s (%v) — start it with:\n"+
			"  docker run -d -p 4566:4566 -e SERVICES=sqs localstack/localstack:3.0", endpoint, err)
	}
	_ = conn.Close()

	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	client := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  aws.String(name),
		Attributes: map[string]string{"VisibilityTimeout": "1"},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = client.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{QueueUrl: out.QueueUrl})
	})

	q := &Queue{
		client:            client,
		queueURL:          *out.QueueUrl,
		queueName:         name,
		visibilityTimeout: 1,
		waitSeconds:       0,
		pendingDelete:     make(map[string]time.Time),
	}
	q.running.Store(true) // build() does this; Poll returns ErrStopped without it
	return q
}

func stripScheme(endpoint string) string {
	for _, p := range []string{"http://", "https://"} {
		if len(endpoint) > len(p) && endpoint[:len(p)] == p {
			return endpoint[len(p):]
		}
	}
	return endpoint
}

func pollUntil(t *testing.T, q *Queue, want int, within time.Duration) []common.QueuedMessage {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		msgs, err := q.Poll(context.Background(), 10)
		require.NoError(t, err)
		if len(msgs) >= want {
			return msgs
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// TestAckRecordsPendingDeleteFromSuppliedID drives a real SQS round trip. The
// MessageId used to be recovered from a receipt→id map populated at poll time;
// if that entry had been evicted first the lookup missed, nothing was recorded,
// and the message was deleted anyway — silently — so a redelivery already in
// flight was not recognised and got delivered to the target twice. The id now
// arrives from the caller, so no map state can make the guard forget.
func TestAckRecordsPendingDeleteFromSuppliedID(t *testing.T) {
	ctx := context.Background()
	q := newLocalstackQueue(t, "fc-ack-id-test")

	_, err := q.Publish(ctx, common.Message{
		ID: "evt-1", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/1",
	})
	require.NoError(t, err)

	msgs := pollUntil(t, q, 1, 5*time.Second)
	require.Len(t, msgs, 1)
	got := msgs[0]
	require.NotEmpty(t, got.BrokerMessageID, "poll must surface the broker id the ack now depends on")

	require.NoError(t, q.Ack(ctx, got.ReceiptHandle, got.BrokerMessageID))

	require.True(t, q.alreadyDeleted(got.BrokerMessageID),
		"the ack must remember this MessageId so an in-flight redelivery is suppressed")
}

// TestPollSuppressesRedeliveryOfAckedMessage is the behaviour the guard exists
// for, end to end against real SQS: a copy that becomes visible again after we
// have recorded the delete must be dropped on poll rather than handed to the
// mediator, and removed from the queue.
//
// The delete is recorded without deleting the row (markDeleted rather than Ack)
// precisely so a copy remains for SQS to redeliver — that is the race the guard
// covers, which cannot otherwise be provoked deterministically.
func TestPollSuppressesRedeliveryOfAckedMessage(t *testing.T) {
	ctx := context.Background()
	q := newLocalstackQueue(t, "fc-suppress-test")

	_, err := q.Publish(ctx, common.Message{
		ID: "evt-dup", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/dup",
	})
	require.NoError(t, err)

	first := pollUntil(t, q, 1, 5*time.Second)
	require.Len(t, first, 1)
	brokerID := first[0].BrokerMessageID

	// Stand in for "we acked this" without removing the row, leaving a copy to
	// come back once the 1s visibility window lapses.
	q.markDeleted(brokerID)
	time.Sleep(1500 * time.Millisecond)

	msgs, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, msgs, "a redelivery of an acked MessageId must never reach the mediator")

	// And it is gone, not merely hidden: nothing comes back on later polls.
	time.Sleep(1500 * time.Millisecond)
	msgs, err = q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, msgs, "the suppressed copy must be deleted, not left to cycle")
}

// TestPollDeliversMessagesWithNoPendingDelete keeps the guard honest: it must
// suppress only what was actually acked.
func TestPollDeliversMessagesWithNoPendingDelete(t *testing.T) {
	ctx := context.Background()
	q := newLocalstackQueue(t, "fc-passthrough-test")

	_, err := q.Publish(ctx, common.Message{
		ID: "evt-ok", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/ok",
	})
	require.NoError(t, err)

	msgs := pollUntil(t, q, 1, 5*time.Second)
	require.Len(t, msgs, 1)
	require.Equal(t, "evt-ok", msgs[0].Message.ID)
}
