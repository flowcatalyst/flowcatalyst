// Package nats provides NATS JetStream queue implementation
package nats

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"log/slog"

	"go.flowcatalyst.tech/internal/queue"
)

// EmbeddedServer wraps an embedded NATS server with JetStream
type EmbeddedServer struct {
	server    *server.Server
	conn      *nats.Conn
	js        jetstream.JetStream
	dataDir   string
	port      int
	publisher *Publisher
	consumer  *Consumer
}

// EmbeddedConfig holds configuration for the embedded NATS server
type EmbeddedConfig struct {
	// DataDir is the directory for JetStream data persistence
	DataDir string

	// Host is the bind address (default: 127.0.0.1)
	Host string

	// Port is the server port (default: 4222)
	Port int

	// StreamName is the JetStream stream name
	StreamName string

	// Subjects is the list of subjects for the stream
	Subjects []string

	// MaxAge is the maximum age of messages in the stream
	MaxAge time.Duration

	// ConsumerName is the durable consumer name
	ConsumerName string
}

// DefaultEmbeddedConfig returns default embedded server configuration
func DefaultEmbeddedConfig() *EmbeddedConfig {
	return &EmbeddedConfig{
		DataDir:      "./data/nats",
		Host:         "127.0.0.1",
		Port:         4222,
		StreamName:   "DISPATCH",
		Subjects:     []string{"dispatch.>"},
		MaxAge:       24 * time.Hour,
		ConsumerName: "flowcatalyst-router",
	}
}

// NewEmbeddedServer creates and starts a new embedded NATS server
func NewEmbeddedServer(cfg *EmbeddedConfig) (*EmbeddedServer, error) {
	if cfg == nil {
		cfg = DefaultEmbeddedConfig()
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Configure NATS server
	opts := &server.Options{
		Host:      cfg.Host,
		Port:      cfg.Port,
		JetStream: true,
		StoreDir:  cfg.DataDir,
		NoLog:     true,
		NoSigs:    true,
	}

	// Create and start server
	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS server: %w", err)
	}

	// Start the server
	go ns.Start()

	// Wait for server to be ready
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		return nil, fmt.Errorf("NATS server failed to start within timeout")
	}

	slog.Info("Embedded NATS server started", "host", cfg.Host, "port", cfg.Port, "dataDir", cfg.DataDir)

	// Connect to the server
	url := fmt.Sprintf("nats://%s:%d", cfg.Host, cfg.Port)
	conn, err := nats.Connect(url,
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
		ns.Shutdown()
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Get JetStream context
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		ns.Shutdown()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	embedded := &EmbeddedServer{
		server:  ns,
		conn:    conn,
		js:      js,
		dataDir: cfg.DataDir,
		port:    cfg.Port,
	}

	// Create or update the stream
	if err := embedded.ensureStream(context.Background(), cfg); err != nil {
		embedded.Close()
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// Create publisher
	embedded.publisher = &Publisher{
		js:     js,
		stream: cfg.StreamName,
	}

	slog.Info("JetStream stream configured", "stream", cfg.StreamName, "subjects", cfg.Subjects)

	return embedded, nil
}

// ensureStream creates or updates the JetStream stream
func (e *EmbeddedServer) ensureStream(ctx context.Context, cfg *EmbeddedConfig) error {
	streamCfg := jetstream.StreamConfig{
		Name:      cfg.StreamName,
		Subjects:  cfg.Subjects,
		Storage:   jetstream.FileStorage,
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    cfg.MaxAge,
		Replicas:  1, // Single node for embedded
		Discard:   jetstream.DiscardOld,
		MaxMsgs:   -1, // Unlimited
		MaxBytes:  -1, // Unlimited
		NoAck:     false,
	}

	// Try to get existing stream
	_, err := e.js.Stream(ctx, cfg.StreamName)
	if err != nil {
		// Stream doesn't exist, create it
		_, err = e.js.CreateStream(ctx, streamCfg)
		if err != nil {
			return fmt.Errorf("failed to create stream: %w", err)
		}
		slog.Info("Created JetStream stream", "stream", cfg.StreamName)
	} else {
		// Stream exists, update it
		_, err = e.js.UpdateStream(ctx, streamCfg)
		if err != nil {
			return fmt.Errorf("failed to update stream: %w", err)
		}
		slog.Info("Updated JetStream stream", "stream", cfg.StreamName)
	}

	return nil
}

// CreateConsumer creates a consumer for the given subject pattern
func (e *EmbeddedServer) CreateConsumer(ctx context.Context, name, filterSubject string, cfg *queue.NATSConfig) (*Consumer, error) {
	ackWait := 2 * time.Minute
	if cfg != nil && cfg.AckWait > 0 {
		ackWait = cfg.AckWait
	}

	maxDeliver := 5
	if cfg != nil && cfg.MaxDeliver > 0 {
		maxDeliver = cfg.MaxDeliver
	}

	consumerCfg := jetstream.ConsumerConfig{
		Name:           name,
		Durable:        name,
		FilterSubject:  filterSubject,
		AckPolicy:      jetstream.AckExplicitPolicy,
		AckWait:        ackWait,
		MaxDeliver:     maxDeliver,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
		ReplayPolicy:   jetstream.ReplayInstantPolicy,
		MaxAckPending:  1000,
	}

	streamName := "DISPATCH"
	if cfg != nil && cfg.StreamName != "" {
		streamName = cfg.StreamName
	}

	stream, err := e.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, consumerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return &Consumer{
		consumer: consumer,
		name:     name,
	}, nil
}

// Publisher returns the embedded server's publisher
func (e *EmbeddedServer) Publisher() queue.Publisher {
	return e.publisher
}

// JetStream returns the JetStream context
func (e *EmbeddedServer) JetStream() jetstream.JetStream {
	return e.js
}

// Connection returns the NATS connection
func (e *EmbeddedServer) Connection() *nats.Conn {
	return e.conn
}

// Server returns the underlying NATS server
func (e *EmbeddedServer) Server() *server.Server {
	return e.server
}

// DataDir returns the data directory
func (e *EmbeddedServer) DataDir() string {
	return e.dataDir
}

// Port returns the server port
func (e *EmbeddedServer) Port() int {
	return e.port
}

// Close shuts down the embedded server
func (e *EmbeddedServer) Close() error {
	slog.Info("Shutting down embedded NATS server")

	if e.conn != nil {
		e.conn.Close()
	}

	if e.server != nil {
		e.server.Shutdown()
		e.server.WaitForShutdown()
	}

	// Clean up lock file if it exists
	lockFile := filepath.Join(e.dataDir, "jetstream", "lock.lck")
	if _, err := os.Stat(lockFile); err == nil {
		os.Remove(lockFile)
	}

	slog.Info("Embedded NATS server shut down")
	return nil
}
