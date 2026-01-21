package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"go.flowcatalyst.tech/internal/queue"
)

// MockSQSClient implements a mock SQS client for testing
type MockSQSClient struct {
	receiveMessageFunc          func(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	deleteMessageFunc           func(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
	changeMessageVisibilityFunc func(ctx context.Context, params *sqs.ChangeMessageVisibilityInput, optFns ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityOutput, error)
	sendMessageFunc             func(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	sendMessageBatchFunc        func(ctx context.Context, params *sqs.SendMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error)
	getQueueAttributesFunc      func(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)

	receiveMessageCalls          atomic.Int32
	deleteMessageCalls           atomic.Int32
	changeMessageVisibilityCalls atomic.Int32
	sendMessageCalls             atomic.Int32
	sendMessageBatchCalls        atomic.Int32

	mu                    sync.Mutex
	deletedReceiptHandles []string
	visibilityChanges     []visibilityChange
}

type visibilityChange struct {
	receiptHandle string
	timeout       int32
}

func NewMockSQSClient() *MockSQSClient {
	return &MockSQSClient{
		deletedReceiptHandles: make([]string, 0),
		visibilityChanges:     make([]visibilityChange, 0),
	}
}

func (m *MockSQSClient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	m.receiveMessageCalls.Add(1)
	if m.receiveMessageFunc != nil {
		return m.receiveMessageFunc(ctx, params, optFns...)
	}
	return &sqs.ReceiveMessageOutput{Messages: []types.Message{}}, nil
}

func (m *MockSQSClient) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	m.deleteMessageCalls.Add(1)
	m.mu.Lock()
	if params.ReceiptHandle != nil {
		m.deletedReceiptHandles = append(m.deletedReceiptHandles, *params.ReceiptHandle)
	}
	m.mu.Unlock()
	if m.deleteMessageFunc != nil {
		return m.deleteMessageFunc(ctx, params, optFns...)
	}
	return &sqs.DeleteMessageOutput{}, nil
}

func (m *MockSQSClient) ChangeMessageVisibility(ctx context.Context, params *sqs.ChangeMessageVisibilityInput, optFns ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityOutput, error) {
	m.changeMessageVisibilityCalls.Add(1)
	m.mu.Lock()
	if params.ReceiptHandle != nil {
		m.visibilityChanges = append(m.visibilityChanges, visibilityChange{
			receiptHandle: *params.ReceiptHandle,
			timeout:       params.VisibilityTimeout,
		})
	}
	m.mu.Unlock()
	if m.changeMessageVisibilityFunc != nil {
		return m.changeMessageVisibilityFunc(ctx, params, optFns...)
	}
	return &sqs.ChangeMessageVisibilityOutput{}, nil
}

func (m *MockSQSClient) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	m.sendMessageCalls.Add(1)
	if m.sendMessageFunc != nil {
		return m.sendMessageFunc(ctx, params, optFns...)
	}
	return &sqs.SendMessageOutput{
		MessageId: aws.String("mock-message-id"),
	}, nil
}

func (m *MockSQSClient) SendMessageBatch(ctx context.Context, params *sqs.SendMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error) {
	m.sendMessageBatchCalls.Add(1)
	if m.sendMessageBatchFunc != nil {
		return m.sendMessageBatchFunc(ctx, params, optFns...)
	}
	successful := make([]types.SendMessageBatchResultEntry, len(params.Entries))
	for i, entry := range params.Entries {
		successful[i] = types.SendMessageBatchResultEntry{
			Id:        entry.Id,
			MessageId: aws.String("mock-batch-msg-" + *entry.Id),
		}
	}
	return &sqs.SendMessageBatchOutput{
		Successful: successful,
	}, nil
}

func (m *MockSQSClient) GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	if m.getQueueAttributesFunc != nil {
		return m.getQueueAttributesFunc(ctx, params, optFns...)
	}
	return &sqs.GetQueueAttributesOutput{
		Attributes: map[string]string{
			"ApproximateNumberOfMessages": "0",
		},
	}, nil
}

func (m *MockSQSClient) GetDeletedReceiptHandles() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.deletedReceiptHandles...)
}

func (m *MockSQSClient) GetVisibilityChanges() []visibilityChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]visibilityChange{}, m.visibilityChanges...)
}

// TestSQSMessageAck tests that Ack deletes the message from SQS
func TestSQSMessageAck(t *testing.T) {
	mockClient := NewMockSQSClient()

	msg := &SQSMessage{
		msg: &types.Message{
			MessageId:     aws.String("test-msg-1"),
			Body:          aws.String(`{"test": true}`),
			ReceiptHandle: aws.String("receipt-handle-1"),
		},
		client:        mockClient,
		queueURL:      "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		sqsMessageID:  "test-msg-1",
		receiptHandle: "receipt-handle-1",
	}

	err := msg.Ack()
	if err != nil {
		t.Fatalf("Ack returned error: %v", err)
	}

	if mockClient.deleteMessageCalls.Load() != 1 {
		t.Errorf("Expected 1 delete call, got %d", mockClient.deleteMessageCalls.Load())
	}

	deleted := mockClient.GetDeletedReceiptHandles()
	if len(deleted) != 1 || deleted[0] != "receipt-handle-1" {
		t.Errorf("Expected receipt-handle-1 to be deleted, got %v", deleted)
	}
}

// TestSQSMessageNak tests that Nak does NOT delete the message (relies on visibility timeout)
func TestSQSMessageNak(t *testing.T) {
	mockClient := NewMockSQSClient()

	msg := &SQSMessage{
		msg: &types.Message{
			MessageId:     aws.String("test-msg-nack"),
			Body:          aws.String(`{"test": true}`),
			ReceiptHandle: aws.String("receipt-handle-nack"),
		},
		client:        mockClient,
		queueURL:      "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		sqsMessageID:  "test-msg-nack",
		receiptHandle: "receipt-handle-nack",
	}

	err := msg.Nak()
	if err != nil {
		t.Fatalf("Nak returned error: %v", err)
	}

	// Nack should NOT delete the message
	if mockClient.deleteMessageCalls.Load() != 0 {
		t.Errorf("Expected 0 delete calls for nack, got %d", mockClient.deleteMessageCalls.Load())
	}
}

// TestSQSMessageSetFastFailVisibility tests setting visibility to 10 seconds for rate limits
func TestSQSMessageSetFastFailVisibility(t *testing.T) {
	mockClient := NewMockSQSClient()

	consumer := &Consumer{
		client:   mockClient,
		queueURL: "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		name:     "test-consumer",
	}

	msg := &SQSMessage{
		msg: &types.Message{
			MessageId:     aws.String("test-msg-visibility"),
			Body:          aws.String(`{"test": true}`),
			ReceiptHandle: aws.String("receipt-visibility"),
		},
		client:        mockClient,
		queueURL:      "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		sqsMessageID:  "test-msg-visibility",
		receiptHandle: "receipt-visibility",
		consumer:      consumer,
	}

	err := msg.SetFastFailVisibility()
	if err != nil {
		t.Fatalf("SetFastFailVisibility returned error: %v", err)
	}

	if mockClient.changeMessageVisibilityCalls.Load() != 1 {
		t.Errorf("Expected 1 visibility change call, got %d", mockClient.changeMessageVisibilityCalls.Load())
	}

	changes := mockClient.GetVisibilityChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 visibility change, got %d", len(changes))
	}

	if changes[0].timeout != FastFailVisibilitySeconds {
		t.Errorf("Expected fast-fail visibility %d, got %d", FastFailVisibilitySeconds, changes[0].timeout)
	}
}

// TestSQSMessageResetVisibilityToDefault tests resetting visibility to 30 seconds
func TestSQSMessageResetVisibilityToDefault(t *testing.T) {
	mockClient := NewMockSQSClient()

	consumer := &Consumer{
		client:   mockClient,
		queueURL: "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		name:     "test-consumer",
	}

	msg := &SQSMessage{
		msg: &types.Message{
			MessageId:     aws.String("test-msg-reset"),
			Body:          aws.String(`{"test": true}`),
			ReceiptHandle: aws.String("receipt-reset"),
		},
		client:        mockClient,
		queueURL:      "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		sqsMessageID:  "test-msg-reset",
		receiptHandle: "receipt-reset",
		consumer:      consumer,
	}

	err := msg.ResetVisibilityToDefault()
	if err != nil {
		t.Fatalf("ResetVisibilityToDefault returned error: %v", err)
	}

	changes := mockClient.GetVisibilityChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 visibility change, got %d", len(changes))
	}

	if changes[0].timeout != DefaultVisibilitySeconds {
		t.Errorf("Expected default visibility %d, got %d", DefaultVisibilitySeconds, changes[0].timeout)
	}
}

// TestSQSMessageNakWithDelay tests nack with custom delay
func TestSQSMessageNakWithDelay(t *testing.T) {
	mockClient := NewMockSQSClient()

	msg := &SQSMessage{
		msg: &types.Message{
			MessageId:     aws.String("test-msg-delay"),
			Body:          aws.String(`{"test": true}`),
			ReceiptHandle: aws.String("receipt-delay"),
		},
		client:        mockClient,
		queueURL:      "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		sqsMessageID:  "test-msg-delay",
		receiptHandle: "receipt-delay",
	}

	delay := 60 * time.Second
	err := msg.NakWithDelay(delay)
	if err != nil {
		t.Fatalf("NakWithDelay returned error: %v", err)
	}

	changes := mockClient.GetVisibilityChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 visibility change, got %d", len(changes))
	}

	if changes[0].timeout != 60 {
		t.Errorf("Expected visibility 60, got %d", changes[0].timeout)
	}
}

// TestSQSMessageData tests retrieving message data
func TestSQSMessageData(t *testing.T) {
	msgBody := `{"jobId": "job-123", "payload": "test data"}`

	msg := &SQSMessage{
		msg: &types.Message{
			MessageId: aws.String("test-msg-data"),
			Body:      aws.String(msgBody),
		},
		sqsMessageID: "test-msg-data",
	}

	data := msg.Data()
	if string(data) != msgBody {
		t.Errorf("Expected message body %s, got %s", msgBody, string(data))
	}
}

// TestSQSMessageSubject tests retrieving message subject from attributes
func TestSQSMessageSubject(t *testing.T) {
	msg := &SQSMessage{
		msg: &types.Message{
			MessageId: aws.String("test-msg-subject"),
			Body:      aws.String(`{}`),
			MessageAttributes: map[string]types.MessageAttributeValue{
				"Subject": {
					DataType:    aws.String("String"),
					StringValue: aws.String("dispatch.jobs"),
				},
			},
		},
		sqsMessageID: "test-msg-subject",
	}

	subject := msg.Subject()
	if subject != "dispatch.jobs" {
		t.Errorf("Expected subject 'dispatch.jobs', got '%s'", subject)
	}
}

// TestSQSMessageMetadata tests retrieving all message attributes
func TestSQSMessageMetadata(t *testing.T) {
	msg := &SQSMessage{
		msg: &types.Message{
			MessageId: aws.String("test-msg-meta"),
			Body:      aws.String(`{}`),
			MessageAttributes: map[string]types.MessageAttributeValue{
				"Subject": {
					DataType:    aws.String("String"),
					StringValue: aws.String("test.subject"),
				},
				"Priority": {
					DataType:    aws.String("String"),
					StringValue: aws.String("high"),
				},
			},
		},
		sqsMessageID: "test-msg-meta",
	}

	metadata := msg.Metadata()
	if len(metadata) != 2 {
		t.Errorf("Expected 2 metadata entries, got %d", len(metadata))
	}

	if metadata["Subject"] != "test.subject" {
		t.Errorf("Expected Subject 'test.subject', got '%s'", metadata["Subject"])
	}

	if metadata["Priority"] != "high" {
		t.Errorf("Expected Priority 'high', got '%s'", metadata["Priority"])
	}
}

// TestSQSMessageHandleExpiredReceiptHandle tests handling of expired receipt handles
func TestSQSMessageHandleExpiredReceiptHandle(t *testing.T) {
	mockClient := NewMockSQSClient()
	mockClient.deleteMessageFunc = func(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
		return nil, errors.New("The receipt handle has expired")
	}

	consumer := &Consumer{
		client:         mockClient,
		queueURL:       "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		name:           "test-consumer",
		pendingDeletes: make(map[string]struct{}),
	}

	msg := &SQSMessage{
		msg: &types.Message{
			MessageId:     aws.String("test-msg-expired"),
			Body:          aws.String(`{}`),
			ReceiptHandle: aws.String("expired-receipt"),
		},
		client:        mockClient,
		queueURL:      "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		sqsMessageID:  "test-msg-expired",
		receiptHandle: "expired-receipt",
		consumer:      consumer,
	}

	// Ack should not return error for expired receipt handle
	err := msg.Ack()
	if err != nil {
		t.Fatalf("Ack should handle expired receipt gracefully, got error: %v", err)
	}

	// Should be marked for deletion on next poll
	consumer.pendingDeletesMu.RLock()
	_, marked := consumer.pendingDeletes[msg.sqsMessageID]
	consumer.pendingDeletesMu.RUnlock()

	if !marked {
		t.Error("Message should be marked for deletion on next poll")
	}
}

// TestDispatchMessageEncodeDecode tests JSON encoding/decoding of dispatch messages
func TestDispatchMessageEncodeDecode(t *testing.T) {
	original := &DispatchMessage{
		JobID:          "job-123",
		DispatchPoolID: "pool-abc",
		MessageGroup:   "group-1",
		BatchID:        "batch-456",
		Sequence:       1,
		TargetURL:      "http://localhost:8080/webhook",
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"Content-Type":  "application/json",
		},
		Payload:        `{"event": "test"}`,
		ContentType:    "application/json",
		TimeoutSeconds: 30,
		MaxRetries:     3,
		AttemptNumber:  1,
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeDispatchMessage(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.JobID != original.JobID {
		t.Errorf("JobID mismatch: got %s, want %s", decoded.JobID, original.JobID)
	}
	if decoded.DispatchPoolID != original.DispatchPoolID {
		t.Errorf("DispatchPoolID mismatch: got %s, want %s", decoded.DispatchPoolID, original.DispatchPoolID)
	}
	if decoded.MessageGroup != original.MessageGroup {
		t.Errorf("MessageGroup mismatch: got %s, want %s", decoded.MessageGroup, original.MessageGroup)
	}
	if decoded.TargetURL != original.TargetURL {
		t.Errorf("TargetURL mismatch: got %s, want %s", decoded.TargetURL, original.TargetURL)
	}
	if decoded.TimeoutSeconds != original.TimeoutSeconds {
		t.Errorf("TimeoutSeconds mismatch: got %d, want %d", decoded.TimeoutSeconds, original.TimeoutSeconds)
	}
	if decoded.Headers["Authorization"] != original.Headers["Authorization"] {
		t.Errorf("Headers mismatch: got %v, want %v", decoded.Headers, original.Headers)
	}
}

// TestDecodeDispatchMessageInvalidJSON tests handling invalid JSON
func TestDecodeDispatchMessageInvalidJSON(t *testing.T) {
	_, err := DecodeDispatchMessage([]byte("{ invalid json }"))
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

// TestPublisherPublish tests basic message publishing
func TestPublisherPublish(t *testing.T) {
	mockClient := NewMockSQSClient()
	var capturedInput *sqs.SendMessageInput

	mockClient.sendMessageFunc = func(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
		capturedInput = params
		return &sqs.SendMessageOutput{MessageId: aws.String("published-msg-1")}, nil
	}

	publisher := &Publisher{
		client:   mockClient,
		queueURL: "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
	}

	ctx := context.Background()
	err := publisher.Publish(ctx, "test.subject", []byte(`{"event": "test"}`))
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if mockClient.sendMessageCalls.Load() != 1 {
		t.Errorf("Expected 1 send call, got %d", mockClient.sendMessageCalls.Load())
	}

	if capturedInput == nil {
		t.Fatal("No input captured")
	}

	if aws.ToString(capturedInput.QueueUrl) != publisher.queueURL {
		t.Errorf("Queue URL mismatch")
	}

	if aws.ToString(capturedInput.MessageBody) != `{"event": "test"}` {
		t.Errorf("Message body mismatch")
	}

	if capturedInput.MessageAttributes["Subject"].StringValue == nil ||
		*capturedInput.MessageAttributes["Subject"].StringValue != "test.subject" {
		t.Errorf("Subject attribute not set correctly")
	}
}

// TestPublisherPublishWithGroup tests publishing with message group
func TestPublisherPublishWithGroup(t *testing.T) {
	mockClient := NewMockSQSClient()
	var capturedInput *sqs.SendMessageInput

	mockClient.sendMessageFunc = func(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
		capturedInput = params
		return &sqs.SendMessageOutput{MessageId: aws.String("published-msg-2")}, nil
	}

	publisher := &Publisher{
		client:   mockClient,
		queueURL: "https://sqs.us-east-1.amazonaws.com/123456789/test-queue.fifo",
	}

	ctx := context.Background()
	err := publisher.PublishWithGroup(ctx, "test.subject", []byte(`{}`), "group-abc")
	if err != nil {
		t.Fatalf("PublishWithGroup failed: %v", err)
	}

	if capturedInput.MessageGroupId == nil || *capturedInput.MessageGroupId != "group-abc" {
		t.Errorf("MessageGroupId not set correctly")
	}
}

// TestPublisherPublishWithDeduplication tests publishing with deduplication ID
func TestPublisherPublishWithDeduplication(t *testing.T) {
	mockClient := NewMockSQSClient()
	var capturedInput *sqs.SendMessageInput

	mockClient.sendMessageFunc = func(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
		capturedInput = params
		return &sqs.SendMessageOutput{MessageId: aws.String("published-msg-3")}, nil
	}

	publisher := &Publisher{
		client:   mockClient,
		queueURL: "https://sqs.us-east-1.amazonaws.com/123456789/test-queue.fifo",
	}

	ctx := context.Background()
	err := publisher.PublishWithDeduplication(ctx, "test.subject", []byte(`{}`), "dedup-123")
	if err != nil {
		t.Fatalf("PublishWithDeduplication failed: %v", err)
	}

	if capturedInput.MessageDeduplicationId == nil || *capturedInput.MessageDeduplicationId != "dedup-123" {
		t.Errorf("MessageDeduplicationId not set correctly")
	}
}

// TestPublisherPublishBatch tests batch publishing
func TestPublisherPublishBatch(t *testing.T) {
	mockClient := NewMockSQSClient()

	publisher := &Publisher{
		client:   mockClient,
		queueURL: "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
	}

	messages := make([]*queue.MessageBuilder, 0, 15)
	for i := 0; i < 15; i++ {
		msg := queue.NewMessageBuilder("test.subject").
			WithData([]byte(`{"index": ` + string(rune('0'+i)) + `}`)).
			WithMessageGroup("group-1")
		messages = append(messages, msg)
	}

	ctx := context.Background()
	err := publisher.PublishBatch(ctx, messages)
	if err != nil {
		t.Fatalf("PublishBatch failed: %v", err)
	}

	// 15 messages should require 2 batches (10 + 5)
	if mockClient.sendMessageBatchCalls.Load() != 2 {
		t.Errorf("Expected 2 batch calls for 15 messages, got %d", mockClient.sendMessageBatchCalls.Load())
	}
}

// TestVisibilityConstants tests that visibility constants match Java implementation
func TestVisibilityConstants(t *testing.T) {
	if FastFailVisibilitySeconds != 10 {
		t.Errorf("FastFailVisibilitySeconds should be 10, got %d", FastFailVisibilitySeconds)
	}

	if DefaultVisibilitySeconds != 30 {
		t.Errorf("DefaultVisibilitySeconds should be 30, got %d", DefaultVisibilitySeconds)
	}

	if MaxVisibilitySeconds != 43200 {
		t.Errorf("MaxVisibilitySeconds should be 43200 (12 hours), got %d", MaxVisibilitySeconds)
	}
}

// TestSQSMessageID tests message ID extraction
func TestSQSMessageID(t *testing.T) {
	msg := &SQSMessage{
		msg: &types.Message{
			MessageId: aws.String("sqs-msg-id-123"),
		},
		sqsMessageID: "sqs-msg-id-123",
	}

	if msg.ID() != "sqs-msg-id-123" {
		t.Errorf("Expected ID 'sqs-msg-id-123', got '%s'", msg.ID())
	}
}

// TestSQSMessageInProgress tests extending visibility timeout
func TestSQSMessageInProgress(t *testing.T) {
	mockClient := NewMockSQSClient()

	msg := &SQSMessage{
		msg: &types.Message{
			MessageId:     aws.String("test-msg-progress"),
			ReceiptHandle: aws.String("receipt-progress"),
		},
		client:            mockClient,
		queueURL:          "https://sqs.us-east-1.amazonaws.com/123456789/test-queue",
		sqsMessageID:      "test-msg-progress",
		receiptHandle:     "receipt-progress",
		visibilityTimeout: 120,
	}

	err := msg.InProgress()
	if err != nil {
		t.Fatalf("InProgress returned error: %v", err)
	}

	changes := mockClient.GetVisibilityChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 visibility change, got %d", len(changes))
	}

	if changes[0].timeout != 120 {
		t.Errorf("Expected visibility 120, got %d", changes[0].timeout)
	}
}

// TestSQSMessageUpdateReceiptHandle tests receipt handle update
func TestSQSMessageUpdateReceiptHandle(t *testing.T) {
	msg := &SQSMessage{
		sqsMessageID:  "test-msg",
		receiptHandle: "old-receipt-handle",
	}

	msg.UpdateReceiptHandle("new-receipt-handle")

	if msg.GetReceiptHandle() != "new-receipt-handle" {
		t.Errorf("Expected 'new-receipt-handle', got '%s'", msg.GetReceiptHandle())
	}
}

// TestIsReceiptHandleExpiredError tests error detection
func TestIsReceiptHandleExpiredError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "receipt handle expired",
			err:      errors.New("The receipt handle has expired"),
			expected: true,
		},
		{
			name:     "receipt handle invalid",
			err:      errors.New("ReceiptHandleIsInvalid: some details"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isReceiptHandleExpiredError(tt.err)
			if result != tt.expected {
				t.Errorf("isReceiptHandleExpiredError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestDispatchMessageJSON tests JSON field naming
func TestDispatchMessageJSON(t *testing.T) {
	msg := &DispatchMessage{
		JobID:          "job-1",
		DispatchPoolID: "pool-1",
		MessageGroup:   "group-1",
	}

	data, _ := json.Marshal(msg)
	jsonStr := string(data)

	// Verify camelCase field names
	if !containsString(jsonStr, `"jobId"`) {
		t.Error("Expected camelCase 'jobId' in JSON")
	}
	if !containsString(jsonStr, `"dispatchPoolId"`) {
		t.Error("Expected camelCase 'dispatchPoolId' in JSON")
	}
	if !containsString(jsonStr, `"messageGroup"`) {
		t.Error("Expected camelCase 'messageGroup' in JSON")
	}
}

// Ensure MockSQSClient implements SQSClientAPI
var _ SQSClientAPI = (*MockSQSClient)(nil)
