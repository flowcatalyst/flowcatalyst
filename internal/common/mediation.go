package common

// MediationResult classifies the outcome of an attempt to deliver a
// message to its target endpoint.
type MediationResult int

const (
	// MediationSuccess is a 2xx delivery — ACK.
	MediationSuccess MediationResult = iota
	// MediationErrorConfig is a 4xx — ACK to prevent infinite retries.
	MediationErrorConfig
	// MediationErrorProcess is a 5xx or timeout — NACK for retry.
	MediationErrorProcess
	// MediationErrorConnection is a connection failure — NACK for retry.
	MediationErrorConnection
	// MediationRateLimited is HTTP 429. NACK with Retry-After delay, but
	// don't count toward circuit-breaker failures: destination is
	// healthy, just throttling us.
	MediationRateLimited
	// MediationCircuitOpen means the per-endpoint breaker is open; no HTTP
	// call was attempted. DEFER (not a failure) until the breaker may probe.
	MediationCircuitOpen
	// MediationDeferred is a 2xx whose body carried {"ack": false}: the
	// target is healthy but asked us to come back later (e.g. the record is
	// blocked behind an earlier failure). Requeued on the pool's deferred
	// backoff curve, flooring at any requested delaySeconds — no in-pipeline
	// retry, no circuit-breaker impact.
	MediationDeferred
)

// MediationOutcome carries the result plus optional retry-after delay.
type MediationOutcome struct {
	Result       MediationResult
	DelaySeconds int // 0 if no delay
	StatusCode   int // 0 if not from HTTP
	ErrorMessage string

	// FlushGroup is set when a 2xx body carried {"flushGroup": true}: the
	// target is asking us to stop delivering this message's group and just
	// ACK its siblings. DelaySeconds carries how long to suppress for.
	// Honoured only alongside a successful delivery — a target that wants
	// the message back cannot also discard the group.
	FlushGroup bool
}

// Success builds a 200 outcome.
func Success() MediationOutcome {
	return MediationOutcome{Result: MediationSuccess, StatusCode: 200}
}

// ErrorConfig builds a 4xx outcome.
func ErrorConfig(status int, msg string) MediationOutcome {
	return MediationOutcome{
		Result: MediationErrorConfig, StatusCode: status, ErrorMessage: msg,
	}
}

// ErrorProcess builds a 5xx/timeout outcome with optional retry delay.
func ErrorProcess(delaySec int, msg string) MediationOutcome {
	return MediationOutcome{
		Result: MediationErrorProcess, DelaySeconds: delaySec, ErrorMessage: msg,
	}
}

// ErrorConnection builds a connection-error outcome. Default delay 30s.
func ErrorConnection(msg string) MediationOutcome {
	return MediationOutcome{
		Result: MediationErrorConnection, DelaySeconds: 30, ErrorMessage: msg,
	}
}

// Deferred builds an ack=false outcome: the target answered 2xx but told
// us to retry later ({"ack": false, "delaySeconds": N}).
func Deferred(delaySec int, msg string) MediationOutcome {
	return MediationOutcome{
		Result: MediationDeferred, DelaySeconds: delaySec, ErrorMessage: msg,
	}
}

// RateLimited builds a 429 outcome with the supplied Retry-After delay.
func RateLimited(retryAfterSec int) MediationOutcome {
	return MediationOutcome{
		Result:       MediationRateLimited,
		DelaySeconds: retryAfterSec,
		StatusCode:   429,
		ErrorMessage: "HTTP 429: Too Many Requests",
	}
}

// CircuitOpen builds a circuit-open outcome: the per-endpoint breaker is open,
// so no HTTP call was attempted. The pool DEFERS with delaySec (the breaker's
// reset timeout) and, for ordered messages, marks the batch+group failed to
// preserve FIFO — 1:1 with the prior pool circuit-open path.
func CircuitOpen(delaySec int) MediationOutcome {
	return MediationOutcome{Result: MediationCircuitOpen, DelaySeconds: delaySec}
}
