package router

import "fmt"

// ErrorKind classifies a RouterError. Mirrors the Rust enum
// `crates/fc-router/src/error.rs::RouterError` exactly so HealthService
// can attribute failures to the right bucket and the warning service
// can match on kind.
type ErrorKind string

const (
	// ErrPool — pool-internal failure (dispatcher panic, executor error).
	ErrPool ErrorKind = "POOL"
	// ErrQueue — queue read/write failure (Postgres or SQS).
	ErrQueue ErrorKind = "QUEUE"
	// ErrMediation — message-group serialisation failure.
	ErrMediation ErrorKind = "MEDIATION"
	// ErrConfig — invalid or stale router configuration.
	ErrConfig ErrorKind = "CONFIG"
	// ErrPoolNotFound — submit targeted a pool id with no registered pool.
	ErrPoolNotFound ErrorKind = "POOL_NOT_FOUND"
	// ErrPoolAtCapacity — submit refused because the pool's bounded
	// inflight set is full.
	ErrPoolAtCapacity ErrorKind = "POOL_AT_CAPACITY"
	// ErrRateLimited — pool token bucket exhausted.
	ErrRateLimited ErrorKind = "RATE_LIMITED"
	// ErrShutdownInProgress — server is draining; new work refused.
	ErrShutdownInProgress ErrorKind = "SHUTDOWN_IN_PROGRESS"
	// ErrDuplicateMessage — idempotency window matched a recently
	// processed message id.
	ErrDuplicateMessage ErrorKind = "DUPLICATE_MESSAGE"
	// ErrHTTP — outbound webhook delivery failed.
	ErrHTTP ErrorKind = "HTTP"
	// ErrSerialization — payload encode/decode failure.
	ErrSerialization ErrorKind = "SERIALIZATION"
)

// RouterError is the canonical error envelope routed through the
// mediator, dispatcher pool, and health/warning services. Carries a
// kind + a human-readable detail. Construct via the helpers below; the
// helpers exist so call sites stay grep-able.
type RouterError struct {
	Kind   ErrorKind
	Detail string
	// Cause is the underlying error if one was wrapped.
	Cause error
}

// Error implements error.
func (e *RouterError) Error() string {
	if e.Detail == "" {
		return string(e.Kind)
	}
	return fmt.Sprintf("%s: %s", string(e.Kind), e.Detail)
}

// Unwrap exposes the wrapped cause for errors.Is/As.
func (e *RouterError) Unwrap() error { return e.Cause }

// ── Constructors ─────────────────────────────────────────────────────────

func newRouterError(kind ErrorKind, detail string) *RouterError {
	return &RouterError{Kind: kind, Detail: detail}
}

// NewPoolError reports a pool-internal failure.
func NewPoolError(detail string) *RouterError { return newRouterError(ErrPool, detail) }

// NewQueueError reports a queue read/write failure.
func NewQueueError(detail string) *RouterError { return newRouterError(ErrQueue, detail) }

// NewMediationError reports a message-group serialisation failure.
func NewMediationError(detail string) *RouterError { return newRouterError(ErrMediation, detail) }

// NewConfigError reports invalid configuration.
func NewConfigError(detail string) *RouterError { return newRouterError(ErrConfig, detail) }

// NewPoolNotFoundError reports a submit against an unknown pool id.
func NewPoolNotFoundError(poolID string) *RouterError {
	return newRouterError(ErrPoolNotFound, poolID)
}

// NewPoolAtCapacityError reports a submit refused because the pool's
// inflight set is full.
func NewPoolAtCapacityError(poolID string) *RouterError {
	return newRouterError(ErrPoolAtCapacity, poolID)
}

// NewRateLimitedError reports a token-bucket exhaustion.
func NewRateLimitedError(detail string) *RouterError { return newRouterError(ErrRateLimited, detail) }

// NewShutdownInProgressError reports a submit refused during drain.
func NewShutdownInProgressError() *RouterError {
	return newRouterError(ErrShutdownInProgress, "")
}

// NewDuplicateMessageError reports an idempotency-window match.
func NewDuplicateMessageError(messageID string) *RouterError {
	return newRouterError(ErrDuplicateMessage, messageID)
}

// WrapHTTPError captures an outbound HTTP failure with its cause.
func WrapHTTPError(err error) *RouterError {
	if err == nil {
		return nil
	}
	return &RouterError{Kind: ErrHTTP, Detail: err.Error(), Cause: err}
}

// WrapSerializationError captures a JSON encode/decode failure.
func WrapSerializationError(err error) *RouterError {
	if err == nil {
		return nil
	}
	return &RouterError{Kind: ErrSerialization, Detail: err.Error(), Cause: err}
}

// AsRouterError extracts a RouterError from a wrapped chain. Returns
// nil + false if the error doesn't carry one.
func AsRouterError(err error) (*RouterError, bool) {
	if err == nil {
		return nil, false
	}
	var re *RouterError
	for cur := err; cur != nil; {
		if r, ok := cur.(*RouterError); ok {
			re = r
			break
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := cur.(unwrapper)
		if !ok {
			break
		}
		cur = u.Unwrap()
	}
	if re == nil {
		return nil, false
	}
	return re, true
}
