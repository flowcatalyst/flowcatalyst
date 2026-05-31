// Package queue provides a multi-backend message queue abstraction.
// Mirrors the Rust fc-queue crate: a Consumer interface, a Publisher
// interface, and per-backend implementations registered at runtime.
//
// Phase 1 ships:
//   - Postgres backend (used by the embedded-queue dev path and as the
//     prod fallback when SQS is unavailable)
//   - SQLite backend (fc-dev embedded mode)
//   - SQS backend (production)
//
// Future backends (NATS, AMQP) plug in via the same Register pattern.
package queue

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"strings"
	"sync"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// ErrNotImplemented is returned when a backend doesn't support an optional method.
var ErrNotImplemented = errors.New("queue: not implemented for this backend")

// Metrics captures queue health snapshot.
type Metrics struct {
	QueueIdentifier  string
	PendingMessages  uint64
	InFlightMessages uint64
	TotalPolled      uint64
	TotalAcked       uint64
	TotalNacked      uint64
	TotalDeferred    uint64
}

// Consumer is the trait every queue backend implements for the consume side.
type Consumer interface {
	// Identifier returns a stable name for this consumer (typically the queue URL).
	Identifier() string
	// Poll fetches up to maxMessages messages.
	Poll(ctx context.Context, maxMessages uint32) ([]common.QueuedMessage, error)
	// Ack deletes the message from the queue.
	Ack(ctx context.Context, receipt string) error
	// Nack marks the message visible again after delay. Counts as a failure.
	Nack(ctx context.Context, receipt string, delaySeconds *uint32) error
	// Defer marks the message visible again without counting as a failure.
	// Use for backpressure / rate-limiting; default falls back to Nack.
	Defer(ctx context.Context, receipt string, delaySeconds *uint32) error
	// ExtendVisibility prolongs the visibility timeout for in-flight messages.
	ExtendVisibility(ctx context.Context, receipt string, seconds uint32) error
	// Healthy reports liveness.
	Healthy() bool
	// Stop signals the consumer to wind down.
	Stop()
	// Metrics returns the broker-side metrics. May call the broker; nil if unavailable.
	Metrics(ctx context.Context) (*Metrics, error)
	// Counters returns process-local counters (atomic reads). Nil if unsupported.
	Counters() *Metrics
}

// Publisher is the produce side.
type Publisher interface {
	Identifier() string
	Publish(ctx context.Context, m common.Message) (string, error)
	PublishBatch(ctx context.Context, msgs []common.Message) ([]string, error)
}

// Embedded combines Consumer + Publisher for in-process queue backends.
type Embedded interface {
	Consumer
	Publisher
	// InitSchema creates the broker's required tables / topics.
	InitSchema(ctx context.Context) error
}

// ConsumerFactory builds a Consumer from a backend-specific URI.
type ConsumerFactory func(ctx context.Context, cfg common.QueueConfig) (Consumer, error)

// PublisherFactory builds a Publisher from a backend-specific URI.
type PublisherFactory func(ctx context.Context, cfg common.QueueConfig) (Publisher, error)

var (
	registryMu  sync.RWMutex
	consumerFs  = make(map[string]ConsumerFactory)
	publisherFs = make(map[string]PublisherFactory)
)

// RegisterConsumer registers a backend-specific consumer factory. Backends
// call this in init() so that NewConsumer can route by scheme.
func RegisterConsumer(scheme string, f ConsumerFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	consumerFs[scheme] = f
}

// RegisterPublisher registers a backend-specific publisher factory.
func RegisterPublisher(scheme string, f PublisherFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	publisherFs[scheme] = f
}

// NewConsumer builds a Consumer for the supplied URI. The URI scheme
// (e.g. "sqs", "postgres", "sqlite") determines the backend.
func NewConsumer(ctx context.Context, cfg common.QueueConfig) (Consumer, error) {
	scheme := backendScheme(cfg.URI)
	registryMu.RLock()
	f, ok := consumerFs[scheme]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("queue: no consumer registered for scheme %q", scheme)
	}
	return f(ctx, cfg)
}

// NewPublisher builds a Publisher for the supplied URI.
func NewPublisher(ctx context.Context, cfg common.QueueConfig) (Publisher, error) {
	scheme := backendScheme(cfg.URI)
	registryMu.RLock()
	f, ok := publisherFs[scheme]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("queue: no publisher registered for scheme %q", scheme)
	}
	return f(ctx, cfg)
}

func schemeOf(uri string) string {
	for i := range len(uri) - 2 {
		if uri[i] == ':' && uri[i+1] == '/' && uri[i+2] == '/' {
			return uri[:i]
		}
	}
	return uri
}

// backendScheme maps a queue URI to the registered backend key. It's normally
// just the URI scheme, but AWS SQS queue URLs are full https endpoints
// (https://sqs.<region>.amazonaws.com/<account>/<queue>) — the platform's
// config-sync hands these out verbatim — so an http(s) URL pointing at an SQS
// endpoint is routed to the "sqs" backend rather than a nonexistent "https" one.
func backendScheme(uri string) string {
	scheme := schemeOf(uri)
	if (scheme == "https" || scheme == "http") && isSQSEndpoint(uri) {
		return "sqs"
	}
	return scheme
}

// isSQSEndpoint reports whether uri's host is an AWS SQS service endpoint
// (sqs.<region>.amazonaws.com or sqs-fips.<region>.amazonaws.com[.cn]).
func isSQSEndpoint(uri string) bool {
	u, err := neturl.Parse(uri)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return (strings.HasPrefix(host, "sqs.") || strings.HasPrefix(host, "sqs-fips.")) &&
		strings.Contains(host, ".amazonaws.")
}
