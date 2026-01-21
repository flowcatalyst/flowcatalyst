package nats

import (
	"encoding/json"
	"testing"

	"go.flowcatalyst.tech/internal/queue"
)

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
	expectedFields := []string{`"jobId"`, `"dispatchPoolId"`, `"messageGroup"`}
	for _, field := range expectedFields {
		if !containsString(jsonStr, field) {
			t.Errorf("Expected %s in JSON, got %s", field, jsonStr)
		}
	}
}

// TestDispatchMessageDefaults tests default values
func TestDispatchMessageDefaults(t *testing.T) {
	msg := &DispatchMessage{}

	if msg.AttemptNumber != 0 {
		t.Errorf("Expected AttemptNumber 0, got %d", msg.AttemptNumber)
	}
	if msg.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries 0, got %d", msg.MaxRetries)
	}
	if msg.Headers != nil {
		t.Error("Expected nil Headers by default")
	}
}

// TestDispatchMessageWithHeaders tests messages with various headers
func TestDispatchMessageWithHeaders(t *testing.T) {
	msg := &DispatchMessage{
		JobID:     "job-headers",
		TargetURL: "http://example.com",
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
			"X-Request-ID":    "req-123",
			"Authorization":   "Bearer abc123",
		},
	}

	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeDispatchMessage(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded.Headers) != 3 {
		t.Errorf("Expected 3 headers, got %d", len(decoded.Headers))
	}

	if decoded.Headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("Custom header mismatch")
	}
}

// TestDispatchMessageEmptyPayload tests empty payload handling
func TestDispatchMessageEmptyPayload(t *testing.T) {
	msg := &DispatchMessage{
		JobID:   "job-empty",
		Payload: "",
	}

	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeDispatchMessage(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Payload != "" {
		t.Errorf("Expected empty payload, got %s", decoded.Payload)
	}
}

// TestDispatchMessageLargePayload tests large payload handling
func TestDispatchMessageLargePayload(t *testing.T) {
	// Create a ~1MB payload
	largePayload := make([]byte, 1024*1024)
	for i := range largePayload {
		largePayload[i] = byte('a' + (i % 26))
	}

	msg := &DispatchMessage{
		JobID:   "job-large",
		Payload: string(largePayload),
	}

	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode failed for large payload: %v", err)
	}

	decoded, err := DecodeDispatchMessage(encoded)
	if err != nil {
		t.Fatalf("Decode failed for large payload: %v", err)
	}

	if len(decoded.Payload) != len(largePayload) {
		t.Errorf("Payload length mismatch: got %d, want %d", len(decoded.Payload), len(largePayload))
	}
}

// TestDispatchMessageSequence tests sequence numbering
func TestDispatchMessageSequence(t *testing.T) {
	tests := []struct {
		sequence int
	}{
		{0},
		{1},
		{100},
		{999999},
	}

	for _, tt := range tests {
		msg := &DispatchMessage{
			JobID:    "job-seq",
			Sequence: tt.sequence,
		}

		encoded, _ := msg.Encode()
		decoded, _ := DecodeDispatchMessage(encoded)

		if decoded.Sequence != tt.sequence {
			t.Errorf("Sequence mismatch: got %d, want %d", decoded.Sequence, tt.sequence)
		}
	}
}

// TestNewPublisher tests publisher creation
func TestNewPublisher(t *testing.T) {
	// We can't test with a real JetStream without a NATS connection
	// but we can verify the constructor doesn't panic
	publisher := NewPublisher(nil, "TEST")

	if publisher == nil {
		t.Error("NewPublisher returned nil")
	}

	if publisher.stream != "TEST" {
		t.Errorf("Expected stream 'TEST', got '%s'", publisher.stream)
	}
}

// TestNewConsumer tests consumer creation
func TestNewConsumer(t *testing.T) {
	consumer := NewConsumer(nil, "test-consumer")

	if consumer == nil {
		t.Error("NewConsumer returned nil")
	}

	if consumer.name != "test-consumer" {
		t.Errorf("Expected name 'test-consumer', got '%s'", consumer.name)
	}
}

// TestPublisherClose tests publisher close
func TestPublisherClose(t *testing.T) {
	publisher := NewPublisher(nil, "TEST")

	err := publisher.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

// TestConsumerClose tests consumer close
func TestConsumerClose(t *testing.T) {
	consumer := NewConsumer(nil, "test-consumer")

	err := consumer.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

// TestNATSConfig tests config defaults
func TestNATSConfig(t *testing.T) {
	cfg := queue.NATSConfig{
		URL:        "nats://localhost:4222",
		StreamName: "DISPATCH",
	}

	if cfg.URL != "nats://localhost:4222" {
		t.Errorf("Expected URL 'nats://localhost:4222', got '%s'", cfg.URL)
	}

	if cfg.StreamName != "DISPATCH" {
		t.Errorf("Expected StreamName 'DISPATCH', got '%s'", cfg.StreamName)
	}
}

// TestNATSConfigDefaults tests empty config handling
func TestNATSConfigDefaults(t *testing.T) {
	cfg := queue.NATSConfig{}

	if cfg.URL != "" {
		t.Errorf("Expected empty URL, got '%s'", cfg.URL)
	}

	if cfg.AckWait != 0 {
		t.Errorf("Expected 0 AckWait, got %v", cfg.AckWait)
	}

	if cfg.MaxDeliver != 0 {
		t.Errorf("Expected 0 MaxDeliver, got %d", cfg.MaxDeliver)
	}
}

// TestMessageBuilderIntegration tests MessageBuilder with NATS headers
func TestMessageBuilderIntegration(t *testing.T) {
	builder := queue.NewMessageBuilder("dispatch.jobs").
		WithData([]byte(`{"event": "test"}`)).
		WithMessageGroup("group-1").
		WithDeduplicationID("dedup-123").
		WithMetadata("priority", "high")

	if builder.Subject() != "dispatch.jobs" {
		t.Errorf("Expected subject 'dispatch.jobs', got '%s'", builder.Subject())
	}

	if builder.MessageGroup() != "group-1" {
		t.Errorf("Expected message group 'group-1', got '%s'", builder.MessageGroup())
	}

	if builder.DeduplicationID() != "dedup-123" {
		t.Errorf("Expected deduplication ID 'dedup-123', got '%s'", builder.DeduplicationID())
	}

	metadata := builder.Metadata()
	if metadata["priority"] != "high" {
		t.Errorf("Expected priority 'high', got '%s'", metadata["priority"])
	}
}

// TestDispatchMessageContentTypes tests various content types
func TestDispatchMessageContentTypes(t *testing.T) {
	contentTypes := []string{
		"application/json",
		"application/xml",
		"text/plain",
		"application/x-www-form-urlencoded",
	}

	for _, ct := range contentTypes {
		t.Run(ct, func(t *testing.T) {
			msg := &DispatchMessage{
				JobID:       "job-ct",
				ContentType: ct,
			}

			encoded, _ := msg.Encode()
			decoded, _ := DecodeDispatchMessage(encoded)

			if decoded.ContentType != ct {
				t.Errorf("ContentType mismatch: got '%s', want '%s'", decoded.ContentType, ct)
			}
		})
	}
}

// TestDispatchMessageRetrySettings tests retry configuration
func TestDispatchMessageRetrySettings(t *testing.T) {
	msg := &DispatchMessage{
		JobID:         "job-retry",
		MaxRetries:    5,
		AttemptNumber: 2,
	}

	encoded, _ := msg.Encode()
	decoded, _ := DecodeDispatchMessage(encoded)

	if decoded.MaxRetries != 5 {
		t.Errorf("MaxRetries mismatch: got %d, want 5", decoded.MaxRetries)
	}

	if decoded.AttemptNumber != 2 {
		t.Errorf("AttemptNumber mismatch: got %d, want 2", decoded.AttemptNumber)
	}
}

// TestDispatchMessageTimeout tests timeout settings
func TestDispatchMessageTimeout(t *testing.T) {
	timeouts := []int{0, 10, 30, 60, 300}

	for _, timeout := range timeouts {
		t.Run(string(rune(timeout)), func(t *testing.T) {
			msg := &DispatchMessage{
				JobID:          "job-timeout",
				TimeoutSeconds: timeout,
			}

			encoded, _ := msg.Encode()
			decoded, _ := DecodeDispatchMessage(encoded)

			if decoded.TimeoutSeconds != timeout {
				t.Errorf("TimeoutSeconds mismatch: got %d, want %d", decoded.TimeoutSeconds, timeout)
			}
		})
	}
}

// Helper for string containment
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark for encoding
func BenchmarkDispatchMessageEncode(b *testing.B) {
	msg := &DispatchMessage{
		JobID:          "job-bench",
		DispatchPoolID: "pool-bench",
		MessageGroup:   "group-1",
		BatchID:        "batch-1",
		Sequence:       1,
		TargetURL:      "http://example.com/webhook",
		Headers:        map[string]string{"Authorization": "Bearer token"},
		Payload:        `{"event": "benchmark"}`,
		ContentType:    "application/json",
		TimeoutSeconds: 30,
		MaxRetries:     3,
		AttemptNumber:  1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.Encode()
	}
}

// Benchmark for decoding
func BenchmarkDispatchMessageDecode(b *testing.B) {
	msg := &DispatchMessage{
		JobID:          "job-bench",
		DispatchPoolID: "pool-bench",
		MessageGroup:   "group-1",
		Payload:        `{"event": "benchmark"}`,
	}
	encoded, _ := msg.Encode()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecodeDispatchMessage(encoded)
	}
}
