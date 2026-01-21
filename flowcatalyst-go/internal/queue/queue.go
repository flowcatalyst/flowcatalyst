// Package queue provides abstractions for message queue operations
package queue

import (
	"context"
	"time"
)

// Message represents a message from a queue
type Message interface {
	// ID returns the unique message identifier
	ID() string

	// Data returns the message payload
	Data() []byte

	// Subject returns the message subject/topic
	Subject() string

	// MessageGroup returns the message group for ordered processing
	MessageGroup() string

	// Ack acknowledges successful processing
	Ack() error

	// Nak signals processing failure (will be redelivered)
	Nak() error

	// NakWithDelay signals failure with a delay before redelivery
	NakWithDelay(delay time.Duration) error

	// InProgress extends the processing deadline
	InProgress() error

	// Metadata returns message metadata
	Metadata() map[string]string
}

// ReceiptHandleUpdatable is an optional interface for messages that support
// receipt handle updates. This is used for SQS messages where the receipt
// handle may expire and need to be updated when a message is redelivered
// while the original is still being processed.
//
// This mirrors Java's tech.flowcatalyst.messagerouter.callback.ReceiptHandleUpdatable
type ReceiptHandleUpdatable interface {
	// UpdateReceiptHandle updates the receipt handle to a new value.
	// Called when a redelivery of the same message is detected.
	UpdateReceiptHandle(newReceiptHandle string)

	// GetReceiptHandle returns the current receipt handle.
	GetReceiptHandle() string
}

// Publisher publishes messages to a queue
type Publisher interface {
	// Publish sends a message to the specified subject
	Publish(ctx context.Context, subject string, data []byte) error

	// PublishWithGroup sends a message with a message group for ordered processing
	PublishWithGroup(ctx context.Context, subject string, data []byte, messageGroup string) error

	// PublishWithDeduplication sends a message with deduplication ID
	PublishWithDeduplication(ctx context.Context, subject string, data []byte, deduplicationID string) error

	// Close closes the publisher
	Close() error
}

// Consumer consumes messages from a queue
type Consumer interface {
	// Consume starts consuming messages and calls the handler for each
	// This blocks until the context is cancelled or an error occurs
	Consume(ctx context.Context, handler func(Message) error) error

	// Close closes the consumer
	Close() error
}

// Queue combines Publisher and Consumer interfaces
type Queue interface {
	Publisher
	Consumer
}

// Config holds queue configuration
type Config struct {
	// Type is the queue implementation type: "embedded", "nats", "sqs"
	Type string

	// DataDir is the data directory for embedded NATS
	DataDir string

	// NATS specific configuration
	NATS NATSConfig

	// SQS specific configuration
	SQS SQSConfig
}

// NATSConfig holds NATS-specific configuration
type NATSConfig struct {
	// URL is the NATS server URL (e.g., "nats://localhost:4222")
	URL string

	// StreamName is the JetStream stream name
	StreamName string

	// ConsumerName is the durable consumer name
	ConsumerName string

	// Subjects is the list of subjects to subscribe to
	Subjects []string

	// MaxPending is the maximum number of pending messages
	MaxPending int

	// AckWait is the time to wait for message acknowledgment
	AckWait time.Duration

	// MaxDeliver is the maximum number of delivery attempts
	MaxDeliver int

	// MaxAge is the maximum age of messages in the stream
	MaxAge time.Duration
}

// SQSConfig holds AWS SQS-specific configuration
type SQSConfig struct {
	// QueueURL is the SQS queue URL
	QueueURL string

	// Region is the AWS region
	Region string

	// WaitTimeSeconds is the long-polling wait time (max 20)
	WaitTimeSeconds int32

	// VisibilityTimeout is the visibility timeout in seconds
	VisibilityTimeout int32

	// MaxNumberOfMessages is the max messages per receive (1-10)
	MaxNumberOfMessages int32

	// MetricsPollIntervalSeconds is the interval for polling queue metrics (default 300)
	MetricsPollIntervalSeconds int32
}

// MessageBuilder helps construct messages for publishing
type MessageBuilder struct {
	subject         string
	data            []byte
	messageGroup    string
	deduplicationID string
	metadata        map[string]string
}

// NewMessageBuilder creates a new message builder
func NewMessageBuilder(subject string) *MessageBuilder {
	return &MessageBuilder{
		subject:  subject,
		metadata: make(map[string]string),
	}
}

// WithData sets the message payload
func (b *MessageBuilder) WithData(data []byte) *MessageBuilder {
	b.data = data
	return b
}

// WithMessageGroup sets the message group for ordered processing
func (b *MessageBuilder) WithMessageGroup(group string) *MessageBuilder {
	b.messageGroup = group
	return b
}

// WithDeduplicationID sets the deduplication ID
func (b *MessageBuilder) WithDeduplicationID(id string) *MessageBuilder {
	b.deduplicationID = id
	return b
}

// WithMetadata adds metadata to the message
func (b *MessageBuilder) WithMetadata(key, value string) *MessageBuilder {
	b.metadata[key] = value
	return b
}

// Subject returns the subject
func (b *MessageBuilder) Subject() string {
	return b.subject
}

// Data returns the data
func (b *MessageBuilder) Data() []byte {
	return b.data
}

// MessageGroup returns the message group
func (b *MessageBuilder) MessageGroup() string {
	return b.messageGroup
}

// DeduplicationID returns the deduplication ID
func (b *MessageBuilder) DeduplicationID() string {
	return b.deduplicationID
}

// Metadata returns the metadata
func (b *MessageBuilder) Metadata() map[string]string {
	return b.metadata
}
