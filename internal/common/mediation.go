package common

// MediationResult classifies the outcome of an attempt to deliver a
// message to its target endpoint.
type MediationResult int

const (
	// MediationSuccess is a 2xx delivery — ACK.
	MediationSuccess MediationResult = iota
	// MediationErrorConfig is a 4xx — ACK to prevent infinite retries.
	MediationErrorConfig
	// MediationErrorProcess is a 502/503/504 ("target unavailable") or a
	// transport-level timeout — NACK for retry. Every other 5xx means the
	// app was reached and answered, and classifies as MediationErrorConfig
	// instead (permanent, like a 4xx).
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

	// PreFlight marks an outcome decided BEFORE any request went out — an
	// unsupported mediation type, an unusable target URL, a payload that would
	// not marshal. Such an outcome is no evidence about the target's health in
	// either direction, so the circuit breaker must ignore it. Recording one as
	// a success actively masks a failing endpoint: a misconfigured URL would
	// hold the breaker closed on a host that is down.
	//
	// Set only by PreFlightError, so it cannot be forgotten at a call site.
	PreFlight bool
}

// Success builds a successful outcome carrying the status the target actually
// returned.
//
// The status is a parameter rather than a hard-coded 200 because it used to be
// hard-coded, and every success — 201, 202, 204 — was recorded as 200. The
// error and ack=false paths beside it copied the real status all along, so the
// logs were confidently wrong about exactly one class of response. Taking it as
// an argument means the compiler asks the question at each new call site; the
// flushGroup branch was added later and silently inherited the wrong answer.
func Success(status int) MediationOutcome {
	return MediationOutcome{Result: MediationSuccess, StatusCode: status}
}

// ErrorConfig builds a 4xx outcome.
func ErrorConfig(status int, msg string) MediationOutcome {
	return MediationOutcome{
		Result: MediationErrorConfig, StatusCode: status, ErrorMessage: msg,
	}
}

// PreFlightError builds a permanent rejection for a message that never reached
// the network. It is an ErrorConfig — the message is unusable as it stands and
// retrying it unchanged cannot help — flagged so the breaker stays out of it.
func PreFlightError(msg string) MediationOutcome {
	return MediationOutcome{
		Result: MediationErrorConfig, ErrorMessage: msg, PreFlight: true,
	}
}

// ErrorProcess builds a 502/503/504-or-timeout ("target unavailable")
// outcome with optional retry delay. Every other 5xx is ErrorConfig instead
// — see MediationErrorProcess.
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
