package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"log/slog"

	"go.flowcatalyst.tech/internal/queue"
)

// Publisher publishes messages to NATS JetStream
type Publisher struct {
	js     jetstream.JetStream
	stream string
}

// NewPublisher creates a new NATS publisher
func NewPublisher(js jetstream.JetStream, streamName string) *Publisher {
	return &Publisher{
		js:     js,
		stream: streamName,
	}
}

// Publish sends a message to the specified subject
func (p *Publisher) Publish(ctx context.Context, subject string, data []byte) error {
	_, err := p.js.Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}
	return nil
}

// PublishWithGroup sends a message with a message group for ordered processing
func (p *Publisher) PublishWithGroup(ctx context.Context, subject string, data []byte, messageGroup string) error {
	// For NATS, we encode the message group in the message headers
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  make(nats.Header),
	}
	msg.Header.Set("Nats-Msg-Group", messageGroup)

	_, err := p.js.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to publish message with group: %w", err)
	}
	return nil
}

// PublishWithDeduplication sends a message with deduplication ID
func (p *Publisher) PublishWithDeduplication(ctx context.Context, subject string, data []byte, deduplicationID string) error {
	// NATS JetStream uses Nats-Msg-Id for deduplication
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  make(nats.Header),
	}
	msg.Header.Set("Nats-Msg-Id", deduplicationID)

	_, err := p.js.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to publish message with deduplication: %w", err)
	}
	return nil
}

// PublishMessage publishes a message built with MessageBuilder
func (p *Publisher) PublishMessage(ctx context.Context, builder *queue.MessageBuilder) error {
	msg := &nats.Msg{
		Subject: builder.Subject(),
		Data:    builder.Data(),
		Header:  make(nats.Header),
	}

	// Set message group if provided
	if builder.MessageGroup() != "" {
		msg.Header.Set("Nats-Msg-Group", builder.MessageGroup())
	}

	// Set deduplication ID if provided
	if builder.DeduplicationID() != "" {
		msg.Header.Set("Nats-Msg-Id", builder.DeduplicationID())
	}

	// Set metadata headers
	for k, v := range builder.Metadata() {
		msg.Header.Set("X-Meta-"+k, v)
	}

	_, err := p.js.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}
	return nil
}

// Close closes the publisher
func (p *Publisher) Close() error {
	// Nothing to close for the publisher itself
	return nil
}

// Consumer consumes messages from NATS JetStream
type Consumer struct {
	consumer jetstream.Consumer
	name     string
}

// NewConsumer creates a new NATS consumer
func NewConsumer(consumer jetstream.Consumer, name string) *Consumer {
	return &Consumer{
		consumer: consumer,
		name:     name,
	}
}

// Consume starts consuming messages and calls the handler for each
func (c *Consumer) Consume(ctx context.Context, handler func(queue.Message) error) error {
	slog.Info("Starting NATS consumer", "consumer", c.name)

	// Create a message channel consumer
	msgIter, err := c.consumer.Messages()
	if err != nil {
		return fmt.Errorf("failed to create message iterator: %w", err)
	}
	defer msgIter.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Consumer context cancelled, stopping", "consumer", c.name)
			return ctx.Err()
		default:
			// Try to get the next message with a timeout
			msg, err := msgIter.Next()
			if err != nil {
				if err == context.Canceled || err == context.DeadlineExceeded {
					return nil
				}
				slog.Error("Error getting next message", "error", err, "consumer", c.name)
				continue
			}

			// Wrap the NATS message
			wrapped := &NATSMessage{
				msg:     msg,
				subject: msg.Subject(),
			}

			// Handle the message
			if err := handler(wrapped); err != nil {
				slog.Error("Message handler error", "error", err, "consumer", c.name, "subject", msg.Subject())
				// The handler should call Nak() on the message if it fails
			}
		}
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	slog.Info("Consumer closed", "consumer", c.name)
	return nil
}

// NATSMessage wraps a NATS JetStream message
type NATSMessage struct {
	msg     jetstream.Msg
	subject string
}

// ID returns the message ID
func (m *NATSMessage) ID() string {
	if id := m.msg.Headers().Get("Nats-Msg-Id"); id != "" {
		return id
	}
	// Fall back to metadata sequence
	meta, err := m.msg.Metadata()
	if err == nil {
		return fmt.Sprintf("%s:%d", meta.Stream, meta.Sequence.Stream)
	}
	return ""
}

// Data returns the message payload
func (m *NATSMessage) Data() []byte {
	return m.msg.Data()
}

// Subject returns the message subject
func (m *NATSMessage) Subject() string {
	return m.subject
}

// MessageGroup returns the message group
func (m *NATSMessage) MessageGroup() string {
	return m.msg.Headers().Get("Nats-Msg-Group")
}

// Ack acknowledges successful processing
func (m *NATSMessage) Ack() error {
	return m.msg.Ack()
}

// Nak signals processing failure
func (m *NATSMessage) Nak() error {
	return m.msg.Nak()
}

// NakWithDelay signals failure with a delay before redelivery
func (m *NATSMessage) NakWithDelay(delay time.Duration) error {
	return m.msg.NakWithDelay(delay)
}

// InProgress extends the processing deadline
func (m *NATSMessage) InProgress() error {
	return m.msg.InProgress()
}

// Metadata returns message metadata
func (m *NATSMessage) Metadata() map[string]string {
	result := make(map[string]string)
	for k, v := range m.msg.Headers() {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// Client wraps a NATS connection and provides both publishing and consuming
type Client struct {
	conn      *nats.Conn
	js        jetstream.JetStream
	publisher *Publisher
	consumers map[string]*Consumer
	config    *queue.NATSConfig
}

// NewClient creates a new NATS client
func NewClient(cfg *queue.NATSConfig) (*Client, error) {
	if cfg.URL == "" {
		cfg.URL = "nats://localhost:4222"
	}

	conn, err := nats.Connect(cfg.URL,
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				slog.Warn("NATS disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			slog.Info("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	streamName := cfg.StreamName
	if streamName == "" {
		streamName = "DISPATCH"
	}

	return &Client{
		conn:      conn,
		js:        js,
		publisher: NewPublisher(js, streamName),
		consumers: make(map[string]*Consumer),
		config:    cfg,
	}, nil
}

// Publisher returns the client's publisher
func (c *Client) Publisher() queue.Publisher {
	return c.publisher
}

// CreateConsumer creates a new consumer for the given filter subject
func (c *Client) CreateConsumer(ctx context.Context, name, filterSubject string) (*Consumer, error) {
	ackWait := 2 * time.Minute
	if c.config.AckWait > 0 {
		ackWait = c.config.AckWait
	}

	maxDeliver := 5
	if c.config.MaxDeliver > 0 {
		maxDeliver = c.config.MaxDeliver
	}

	streamName := c.config.StreamName
	if streamName == "" {
		streamName = "DISPATCH"
	}

	consumerCfg := jetstream.ConsumerConfig{
		Name:          name,
		Durable:       name,
		FilterSubject: filterSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxDeliver:    maxDeliver,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		ReplayPolicy:  jetstream.ReplayInstantPolicy,
		MaxAckPending: 1000,
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, consumerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	wrapped := NewConsumer(consumer, name)
	c.consumers[name] = wrapped
	return wrapped, nil
}

// Close closes the client and all consumers
func (c *Client) Close() error {
	for _, consumer := range c.consumers {
		consumer.Close()
	}
	c.conn.Close()
	return nil
}

// DispatchMessage represents a dispatch job message for the queue
type DispatchMessage struct {
	JobID          string            `json:"jobId"`
	DispatchPoolID string            `json:"dispatchPoolId"`
	MessageGroup   string            `json:"messageGroup"`
	BatchID        string            `json:"batchId"`
	Sequence       int               `json:"sequence"`
	TargetURL      string            `json:"targetUrl"`
	Headers        map[string]string `json:"headers,omitempty"`
	Payload        string            `json:"payload"`
	ContentType    string            `json:"contentType"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
	MaxRetries     int               `json:"maxRetries"`
	AttemptNumber  int               `json:"attemptNumber"`
}

// Encode encodes the dispatch message to JSON
func (m *DispatchMessage) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeDispatchMessage decodes a dispatch message from JSON
func DecodeDispatchMessage(data []byte) (*DispatchMessage, error) {
	var msg DispatchMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
