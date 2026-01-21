package queue

import (
	"context"
	"fmt"
	"time"
)

// QueueType defines the type of queue implementation
type QueueType string

const (
	QueueTypeEmbedded QueueType = "embedded" // Embedded NATS for dev
	QueueTypeNATS     QueueType = "nats"     // External NATS
	QueueTypeSQS      QueueType = "sqs"      // AWS SQS
)

// Factory creates queue implementations
type Factory struct {
	config *Config
}

// NewFactory creates a new queue factory
func NewFactory(cfg *Config) *Factory {
	return &Factory{config: cfg}
}

// Type returns the configured queue type
func (f *Factory) Type() QueueType {
	return QueueType(f.config.Type)
}

// IsEmbedded returns true if using embedded NATS
func (f *Factory) IsEmbedded() bool {
	return f.config.Type == "embedded" || f.config.Type == ""
}

// IsNATS returns true if using external NATS
func (f *Factory) IsNATS() bool {
	return f.config.Type == "nats"
}

// IsSQS returns true if using AWS SQS
func (f *Factory) IsSQS() bool {
	return f.config.Type == "sqs"
}

// Config returns the queue configuration
func (f *Factory) Config() *Config {
	return f.config
}

// DefaultConfig returns default queue configuration
func DefaultConfig() *Config {
	return &Config{
		Type:    "embedded",
		DataDir: "./data/nats",
		NATS: NATSConfig{
			StreamName:   "DISPATCH",
			ConsumerName: "flowcatalyst-router",
			Subjects:     []string{"dispatch.>"},
		},
		SQS: SQSConfig{
			WaitTimeSeconds:     20,
			VisibilityTimeout:   120,
			MaxNumberOfMessages: 10,
		},
	}
}

// QueueManager provides a unified interface for queue operations
type QueueManager interface {
	// Publisher returns the queue publisher
	Publisher() Publisher

	// CreateConsumer creates a consumer for the given pool
	CreateConsumer(ctx context.Context, poolCode string) (Consumer, error)

	// Close closes the queue manager
	Close() error
}

// Ensure consistent types across queue implementations
var (
	_ Message   = (*messageAdapter)(nil)
	_ Publisher = (*publisherAdapter)(nil)
	_ Consumer  = (*consumerAdapter)(nil)
)

// messageAdapter adapts any message implementation to the Message interface
type messageAdapter struct {
	id           string
	data         []byte
	subject      string
	messageGroup string
	metadata     map[string]string
	ackFn        func() error
	nakFn        func() error
	nakDelayFn   func(delay time.Duration) error
	inProgressFn func() error
}

func (m *messageAdapter) ID() string                              { return m.id }
func (m *messageAdapter) Data() []byte                            { return m.data }
func (m *messageAdapter) Subject() string                         { return m.subject }
func (m *messageAdapter) MessageGroup() string                    { return m.messageGroup }
func (m *messageAdapter) Metadata() map[string]string             { return m.metadata }
func (m *messageAdapter) Ack() error                              { return m.ackFn() }
func (m *messageAdapter) Nak() error                              { return m.nakFn() }
func (m *messageAdapter) NakWithDelay(d time.Duration) error      { return m.nakDelayFn(d) }
func (m *messageAdapter) InProgress() error                       { return m.inProgressFn() }

// publisherAdapter adapts any publisher implementation
type publisherAdapter struct {
	publishFn      func(ctx context.Context, subject string, data []byte) error
	publishGroupFn func(ctx context.Context, subject string, data []byte, group string) error
	publishDedupFn func(ctx context.Context, subject string, data []byte, dedupID string) error
	closeFn        func() error
}

func (p *publisherAdapter) Publish(ctx context.Context, subject string, data []byte) error {
	return p.publishFn(ctx, subject, data)
}
func (p *publisherAdapter) PublishWithGroup(ctx context.Context, subject string, data []byte, group string) error {
	if p.publishGroupFn == nil {
		return fmt.Errorf("publish with group not supported")
	}
	return p.publishGroupFn(ctx, subject, data, group)
}
func (p *publisherAdapter) PublishWithDeduplication(ctx context.Context, subject string, data []byte, dedupID string) error {
	if p.publishDedupFn == nil {
		return fmt.Errorf("publish with deduplication not supported")
	}
	return p.publishDedupFn(ctx, subject, data, dedupID)
}
func (p *publisherAdapter) Close() error { return p.closeFn() }

// consumerAdapter adapts any consumer implementation
type consumerAdapter struct {
	consumeFn func(ctx context.Context, handler func(Message) error) error
	closeFn   func() error
}

func (c *consumerAdapter) Consume(ctx context.Context, handler func(Message) error) error {
	return c.consumeFn(ctx, handler)
}
func (c *consumerAdapter) Close() error { return c.closeFn() }
