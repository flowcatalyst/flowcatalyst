package queue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

func TestRegistryRoutesByScheme(t *testing.T) {
	called := ""
	queue.RegisterConsumer("test", func(_ context.Context, _ common.QueueConfig) (queue.Consumer, error) {
		called = "test"
		return nil, nil
	})

	_, err := queue.NewConsumer(context.Background(), common.QueueConfig{URI: "test://foo"})
	require.NoError(t, err)
	assert.Equal(t, "test", called)
}

func TestUnknownSchemeErrors(t *testing.T) {
	_, err := queue.NewConsumer(context.Background(), common.QueueConfig{URI: "unknown://foo"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no consumer registered")
}

// AWS SQS queue URLs are full https endpoints; the registry must route them to
// the "sqs" backend, not a nonexistent "https" one.
func TestSQSHTTPSURLRoutesToSQSBackend(t *testing.T) {
	called := ""
	queue.RegisterConsumer("sqs", func(_ context.Context, _ common.QueueConfig) (queue.Consumer, error) {
		called = "sqs"
		return nil, nil
	})

	_, err := queue.NewConsumer(context.Background(), common.QueueConfig{
		URI: "https://sqs.eu-west-1.amazonaws.com/392314734354/FC-staging-coca-cola-release-staging.fifo",
	})
	require.NoError(t, err)
	assert.Equal(t, "sqs", called)

	// A non-SQS https URL must NOT be hijacked by the sqs backend.
	_, err = queue.NewConsumer(context.Background(), common.QueueConfig{URI: "https://example.com/foo"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `scheme "https"`)
}
