//go:build integration

// Package sqs provides AWS SQS queue implementation
// This file contains integration tests that require Docker and LocalStack
package sqs

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.flowcatalyst.tech/internal/queue"
	"go.flowcatalyst.tech/internal/queue/sqs/testutil"
)

// TestSQSIntegration_PublishAndConsume tests basic message publishing and consumption
func TestSQSIntegration_PublishAndConsume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack
	ls, err := testutil.StartLocalStack(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LocalStack: %v", err)
	}
	defer ls.Terminate(ctx)

	// Create queue
	queueURL, err := ls.CreateQueue(ctx, "test-queue")
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create client with LocalStack endpoint
	cfg := &ClientConfig{
		QueueConfig: &queue.SQSConfig{
			QueueURL:            queueURL,
			Region:              "us-east-1",
			WaitTimeSeconds:     1, // Short wait for tests
			VisibilityTimeout:   30,
			MaxNumberOfMessages: 10,
		},
		CustomEndpoint:  ls.Endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}

	client, err := NewClientWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test publish
	publisher := client.Publisher()
	testData := `{"test": "data", "value": 123}`
	err = publisher.Publish(ctx, "test.subject", []byte(testData))
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Test consume
	consumer, err := client.CreateConsumer(ctx, "test-consumer", "")
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	received := make(chan queue.Message, 1)
	consumeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		consumer.Consume(consumeCtx, func(msg queue.Message) error {
			received <- msg
			return msg.Ack()
		})
	}()

	select {
	case msg := <-received:
		if string(msg.Data()) != testData {
			t.Errorf("Unexpected message data: got %s, want %s", msg.Data(), testData)
		}
		if msg.Subject() != "test.subject" {
			t.Errorf("Unexpected subject: got %s, want test.subject", msg.Subject())
		}
	case <-consumeCtx.Done():
		t.Fatal("Timeout waiting for message")
	}
}

// TestSQSIntegration_FIFOQueue tests FIFO queue message ordering
func TestSQSIntegration_FIFOQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack
	ls, err := testutil.StartLocalStack(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LocalStack: %v", err)
	}
	defer ls.Terminate(ctx)

	// Create FIFO queue
	queueURL, err := ls.CreateFIFOQueue(ctx, "test-fifo-queue")
	if err != nil {
		t.Fatalf("Failed to create FIFO queue: %v", err)
	}

	// Create client
	cfg := &ClientConfig{
		QueueConfig: &queue.SQSConfig{
			QueueURL:            queueURL,
			Region:              "us-east-1",
			WaitTimeSeconds:     1,
			VisibilityTimeout:   30,
			MaxNumberOfMessages: 10,
		},
		CustomEndpoint:  ls.Endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}

	client, err := NewClientWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Publish messages with message group
	publisher := client.Publisher().(*Publisher)
	messageGroup := "order-group-1"
	messages := []string{"first", "second", "third", "fourth", "fifth"}

	for _, msg := range messages {
		err = publisher.PublishWithGroup(ctx, "order.test", []byte(msg), messageGroup)
		if err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Consume and verify order
	consumer, err := client.CreateConsumer(ctx, "fifo-consumer", "")
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	var received []string
	var mu sync.Mutex

	consumeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	go func() {
		consumer.Consume(consumeCtx, func(msg queue.Message) error {
			mu.Lock()
			received = append(received, string(msg.Data()))
			mu.Unlock()
			return msg.Ack()
		})
	}()

	// Wait for all messages
	deadline := time.After(10 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= len(messages) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("Timeout: received only %d/%d messages", count, len(messages))
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Verify order (FIFO guarantees order within message group)
	mu.Lock()
	defer mu.Unlock()
	for i, expected := range messages {
		if received[i] != expected {
			t.Errorf("Message %d: got %s, want %s", i, received[i], expected)
		}
	}
}

// TestSQSIntegration_VisibilityTimeout tests message redelivery after visibility timeout
func TestSQSIntegration_VisibilityTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack
	ls, err := testutil.StartLocalStack(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LocalStack: %v", err)
	}
	defer ls.Terminate(ctx)

	// Create queue
	queueURL, err := ls.CreateQueue(ctx, "visibility-test-queue")
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create client with short visibility timeout
	cfg := &ClientConfig{
		QueueConfig: &queue.SQSConfig{
			QueueURL:            queueURL,
			Region:              "us-east-1",
			WaitTimeSeconds:     1,
			VisibilityTimeout:   2, // Very short for testing
			MaxNumberOfMessages: 10,
		},
		CustomEndpoint:  ls.Endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}

	client, err := NewClientWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Publish a message
	publisher := client.Publisher()
	err = publisher.Publish(ctx, "visibility.test", []byte("test-message"))
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Create consumer
	consumer, err := client.CreateConsumer(ctx, "visibility-consumer", "")
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	deliveryCount := 0
	var mu sync.Mutex

	consumeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	go func() {
		consumer.Consume(consumeCtx, func(msg queue.Message) error {
			mu.Lock()
			deliveryCount++
			count := deliveryCount
			mu.Unlock()

			if count == 1 {
				// First delivery: NAK to trigger redelivery
				return msg.Nak()
			}
			// Second delivery: ACK
			return msg.Ack()
		})
	}()

	// Wait for redelivery
	deadline := time.After(10 * time.Second)
	for {
		mu.Lock()
		count := deliveryCount
		mu.Unlock()
		if count >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("Timeout waiting for redelivery, got %d deliveries", count)
		case <-time.After(100 * time.Millisecond):
		}
	}

	mu.Lock()
	if deliveryCount < 2 {
		t.Errorf("Expected at least 2 deliveries, got %d", deliveryCount)
	}
	mu.Unlock()
}

// TestSQSIntegration_BatchPublish tests batch message publishing
func TestSQSIntegration_BatchPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack
	ls, err := testutil.StartLocalStack(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LocalStack: %v", err)
	}
	defer ls.Terminate(ctx)

	// Create queue
	queueURL, err := ls.CreateQueue(ctx, "batch-test-queue")
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create client
	cfg := &ClientConfig{
		QueueConfig: &queue.SQSConfig{
			QueueURL:            queueURL,
			Region:              "us-east-1",
			WaitTimeSeconds:     1,
			VisibilityTimeout:   30,
			MaxNumberOfMessages: 10,
		},
		CustomEndpoint:  ls.Endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}

	client, err := NewClientWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create 25 messages (tests batching across multiple 10-message batches)
	publisher := client.Publisher().(*Publisher)
	var messages []*queue.MessageBuilder
	for i := 0; i < 25; i++ {
		msg := queue.NewMessageBuilder("batch.test").
			WithData([]byte(`{"index": ` + string(rune('0'+i%10)) + `}`))
		messages = append(messages, msg)
	}

	// Publish batch
	err = publisher.PublishBatch(ctx, messages)
	if err != nil {
		t.Fatalf("Failed to publish batch: %v", err)
	}

	// Consume and count messages
	consumer, err := client.CreateConsumer(ctx, "batch-consumer", "")
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	receivedCount := 0
	var mu sync.Mutex

	consumeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	go func() {
		consumer.Consume(consumeCtx, func(msg queue.Message) error {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			return msg.Ack()
		})
	}()

	// Wait for all messages
	deadline := time.After(15 * time.Second)
	for {
		mu.Lock()
		count := receivedCount
		mu.Unlock()
		if count >= 25 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("Timeout: received only %d/25 messages", count)
		case <-time.After(100 * time.Millisecond):
		}
	}

	mu.Lock()
	if receivedCount != 25 {
		t.Errorf("Expected 25 messages, got %d", receivedCount)
	}
	mu.Unlock()
}

// TestSQSIntegration_MessageAttributes tests message metadata/attributes
func TestSQSIntegration_MessageAttributes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack
	ls, err := testutil.StartLocalStack(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LocalStack: %v", err)
	}
	defer ls.Terminate(ctx)

	// Create queue
	queueURL, err := ls.CreateQueue(ctx, "attributes-test-queue")
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create client
	cfg := &ClientConfig{
		QueueConfig: &queue.SQSConfig{
			QueueURL:            queueURL,
			Region:              "us-east-1",
			WaitTimeSeconds:     1,
			VisibilityTimeout:   30,
			MaxNumberOfMessages: 10,
		},
		CustomEndpoint:  ls.Endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}

	client, err := NewClientWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Publish with subject (stored as attribute)
	publisher := client.Publisher()
	testSubject := "custom.subject.test"
	err = publisher.Publish(ctx, testSubject, []byte("attribute-test"))
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Consume and verify attributes
	consumer, err := client.CreateConsumer(ctx, "attributes-consumer", "")
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	received := make(chan queue.Message, 1)
	consumeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		consumer.Consume(consumeCtx, func(msg queue.Message) error {
			received <- msg
			return msg.Ack()
		})
	}()

	select {
	case msg := <-received:
		// Verify subject attribute
		if msg.Subject() != testSubject {
			t.Errorf("Subject mismatch: got %s, want %s", msg.Subject(), testSubject)
		}

		// Verify metadata contains Subject
		metadata := msg.Metadata()
		if metadata["Subject"] != testSubject {
			t.Errorf("Metadata Subject mismatch: got %s, want %s", metadata["Subject"], testSubject)
		}

		// Verify message ID exists
		if msg.ID() == "" {
			t.Error("Message ID should not be empty")
		}
	case <-consumeCtx.Done():
		t.Fatal("Timeout waiting for message")
	}
}

// TestSQSIntegration_Deduplication tests FIFO queue deduplication
func TestSQSIntegration_Deduplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack
	ls, err := testutil.StartLocalStack(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LocalStack: %v", err)
	}
	defer ls.Terminate(ctx)

	// Create FIFO queue with explicit deduplication (not content-based)
	queueURL, err := ls.CreateFIFOQueueWithDeduplication(ctx, "dedup-test-queue")
	if err != nil {
		t.Fatalf("Failed to create FIFO queue: %v", err)
	}

	// Create client
	cfg := &ClientConfig{
		QueueConfig: &queue.SQSConfig{
			QueueURL:            queueURL,
			Region:              "us-east-1",
			WaitTimeSeconds:     1,
			VisibilityTimeout:   30,
			MaxNumberOfMessages: 10,
		},
		CustomEndpoint:  ls.Endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}

	client, err := NewClientWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Publish same message multiple times with same deduplication ID
	publisher := client.Publisher().(*Publisher)
	deduplicationID := "unique-dedup-id-123"

	// Send 3 messages with same deduplication ID - only 1 should be received
	for i := 0; i < 3; i++ {
		err = publisher.PublishWithDeduplication(ctx, "dedup.test", []byte("duplicate-message"), deduplicationID)
		if err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Also send a unique message
	err = publisher.PublishWithDeduplication(ctx, "dedup.test", []byte("unique-message"), "different-dedup-id")
	if err != nil {
		t.Fatalf("Failed to publish unique message: %v", err)
	}

	// Consume messages
	consumer, err := client.CreateConsumer(ctx, "dedup-consumer", "")
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}

	var receivedMessages []string
	var mu sync.Mutex

	consumeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		consumer.Consume(consumeCtx, func(msg queue.Message) error {
			mu.Lock()
			receivedMessages = append(receivedMessages, string(msg.Data()))
			mu.Unlock()
			return msg.Ack()
		})
	}()

	// Wait for messages (should only get 2: one deduplicated + one unique)
	time.Sleep(5 * time.Second) // Give time for all messages to be processed

	mu.Lock()
	count := len(receivedMessages)
	mu.Unlock()

	if count != 2 {
		t.Errorf("Expected 2 messages (1 deduplicated + 1 unique), got %d", count)
	}
}

// TestSQSIntegration_HealthCheck tests the health check functionality
func TestSQSIntegration_HealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack
	ls, err := testutil.StartLocalStack(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LocalStack: %v", err)
	}
	defer ls.Terminate(ctx)

	// Create queue
	queueURL, err := ls.CreateQueue(ctx, "health-test-queue")
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create client
	cfg := &ClientConfig{
		QueueConfig: &queue.SQSConfig{
			QueueURL:            queueURL,
			Region:              "us-east-1",
			WaitTimeSeconds:     1,
			VisibilityTimeout:   30,
			MaxNumberOfMessages: 10,
		},
		CustomEndpoint:  ls.Endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}

	client, err := NewClientWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test health check
	err = client.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

// TestSQSIntegration_MultipleConsumers tests multiple consumers on the same queue
func TestSQSIntegration_MultipleConsumers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack
	ls, err := testutil.StartLocalStack(ctx, t)
	if err != nil {
		t.Fatalf("Failed to start LocalStack: %v", err)
	}
	defer ls.Terminate(ctx)

	// Create queue
	queueURL, err := ls.CreateQueue(ctx, "multi-consumer-queue")
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create client
	cfg := &ClientConfig{
		QueueConfig: &queue.SQSConfig{
			QueueURL:            queueURL,
			Region:              "us-east-1",
			WaitTimeSeconds:     1,
			VisibilityTimeout:   30,
			MaxNumberOfMessages: 5,
		},
		CustomEndpoint:  ls.Endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}

	client, err := NewClientWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Publish 20 messages
	publisher := client.Publisher()
	for i := 0; i < 20; i++ {
		err = publisher.Publish(ctx, "multi.test", []byte(`{"index": `+string(rune('0'+i%10))+`}`))
		if err != nil {
			t.Fatalf("Failed to publish message %d: %v", i, err)
		}
	}

	// Create 3 consumers
	var consumers []*Consumer
	for i := 0; i < 3; i++ {
		consumer, err := client.CreateConsumer(ctx, "consumer-"+string(rune('A'+i)), "")
		if err != nil {
			t.Fatalf("Failed to create consumer %d: %v", i, err)
		}
		consumers = append(consumers, consumer)
	}

	receivedCount := 0
	var mu sync.Mutex

	consumeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// Start all consumers
	for _, consumer := range consumers {
		go func(c *Consumer) {
			c.Consume(consumeCtx, func(msg queue.Message) error {
				mu.Lock()
				receivedCount++
				mu.Unlock()
				return msg.Ack()
			})
		}(consumer)
	}

	// Wait for all messages
	deadline := time.After(15 * time.Second)
	for {
		mu.Lock()
		count := receivedCount
		mu.Unlock()
		if count >= 20 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("Timeout: received only %d/20 messages", count)
		case <-time.After(100 * time.Millisecond):
		}
	}

	mu.Lock()
	if receivedCount != 20 {
		t.Errorf("Expected 20 messages, got %d", receivedCount)
	}
	mu.Unlock()
}
