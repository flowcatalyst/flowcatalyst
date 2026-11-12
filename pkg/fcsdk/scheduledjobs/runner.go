// Package scheduledjobs implements the consumer-side dispatcher for
// platform-fired scheduled-job webhooks.
//
// Mount Runner.ProcessSigned on whatever HTTP framework you use at the URL
// you set as target_url on the job definition. Each platform → SDK
// firing arrives as a JSON envelope signed with the X-FlowCatalyst-Signature
// and X-FlowCatalyst-Timestamp headers; ProcessSigned verifies the signature
// over the raw body, validates the envelope, kicks off the handler in the
// background, and returns a Result immediately so you can map to HTTP status
// without blocking on the handler.
//
// The signing secret is the value given to Builder.SigningSecret, else the
// FLOWCATALYST_SIGNING_SECRET environment variable. With neither, firings run
// unverified (one warning is logged per Runner) unless Builder.RequireSignature
// is set. Runner.Process takes no headers and so cannot verify; keep it for
// trusted in-process callers and tests.
//
// Concurrency control is via an injected lock.Provider. For
// concurrent: false jobs (the platform doesn't enforce this; the envelope's
// concurrent flag decides, unless EnforceLock overrides), the lock key defaults to "scheduled-job:{jobCode}";
// contended fires return immediately without invoking the handler,
// and if the job tracks completion, a Failure with
// reason "lock-held" is reported back.
//
// # Example
//
//	r := scheduledjobs.NewBuilder(apiClient, lockProvider).
//	    Handler("daily-rollup", func(ctx context.Context, hctx *scheduledjobs.HandlerContext) (json.RawMessage, error) {
//	        hctx.Log(ctx, "starting", nil)
//	        // … work …
//	        return json.RawMessage(`{"processed":42}`), nil
//	    }).
//	    Build()
//
//	http.HandleFunc("/webhooks/scheduled-job", func(w http.ResponseWriter, req *http.Request) {
//	    body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 1<<20))
//	    if err != nil { http.Error(w, "bad body", http.StatusBadRequest); return }
//	    res := r.ProcessSigned(body,
//	        req.Header.Get(webhook.SignatureHeader),
//	        req.Header.Get(webhook.TimestampHeader))
//	    switch res.Kind {
//	        case scheduledjobs.ResultAccepted:     w.WriteHeader(http.StatusAccepted)
//	        case scheduledjobs.ResultUnauthorized: http.Error(w, res.Message, http.StatusUnauthorized)
//	        case scheduledjobs.ResultNotFound:     http.Error(w, res.Message, http.StatusNotFound)
//	        case scheduledjobs.ResultBadRequest:   http.Error(w, res.Message, http.StatusBadRequest)
//	    }
//	})
package scheduledjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/flowcatalyst/go-sdk/client"
	"github.com/flowcatalyst/go-sdk/lock"
	"github.com/flowcatalyst/go-sdk/webhook"
)

// DefaultLockTTL is how long a job's serialisation lock is held when
// not overridden via Builder.LockTTL. Should comfortably exceed your
// expected critical-section duration; the lock self-releases on
// handler completion.
const DefaultLockTTL = 10 * time.Minute

// MaxResultBytes caps the size of completion result JSON forwarded to
// the platform; oversized payloads are replaced with a preview.
const MaxResultBytes = 10_000

// TriggerKind identifies how a firing was started.
type TriggerKind string

const (
	TriggerCron   TriggerKind = "CRON"
	TriggerManual TriggerKind = "MANUAL"
)

// Envelope is the JSON body POSTed by the platform's scheduler
// dispatcher. Mirrors ScheduledJobEnvelope in the TS SDK.
type Envelope struct {
	JobID            string          `json:"jobId"`
	JobCode          string          `json:"jobCode"`
	InstanceID       string          `json:"instanceId"`
	ScheduledFor     string          `json:"scheduledFor,omitempty"`
	FiredAt          string          `json:"firedAt"`
	TriggerKind      TriggerKind     `json:"triggerKind"`
	CorrelationID    string          `json:"correlationId,omitempty"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	TracksCompletion bool            `json:"tracksCompletion"`
	TimeoutSeconds   *uint32         `json:"timeoutSeconds,omitempty"`
	// Concurrent echoes the job definition's concurrent attribute. When
	// true the runner skips its serialisation lock (unless EnforceLock(true)
	// was set explicitly). Absent on older platforms, which reads as false
	// (lock).
	Concurrent bool `json:"concurrent,omitempty"`
}

// HandlerFunc is invoked once per accepted firing. Return a non-nil
// JSON value on success (stored as completion_result on the
// instance); return a non-nil error on failure (the error message is
// stored). Nil JSON + nil error → empty success.
type HandlerFunc func(ctx context.Context, hctx *HandlerContext) (json.RawMessage, error)

// HandlerContext is passed to user handlers. Wraps the envelope and a
// best-effort Log callback that posts a log line back to the
// platform.
type HandlerContext struct {
	Envelope Envelope

	api        *client.FlowCatalystClient
	instanceID string
	jobCode    string
	onError    func(err error, env *Envelope)
}

// Log appends a structured log entry to this instance. Best-effort —
// errors are forwarded to the runner's OnError hook (if configured)
// and then swallowed; logging failures never fail the handler.
func (h *HandlerContext) Log(ctx context.Context, message string, opts *LogOptions) {
	req := &client.InstanceLogRequest{Message: message, Level: client.LogLevelInfo}
	if opts != nil {
		if opts.Level != "" {
			req.Level = client.LogLevel(opts.Level)
		}
		req.Metadata = opts.Metadata
	}
	if err := h.api.ScheduledJobs().LogForInstance(ctx, h.instanceID, req); err != nil {
		if h.onError != nil {
			h.onError(&RunnerError{Kind: ErrCallbackFailed, Cause: err}, &h.Envelope)
		}
	}
}

// LogOptions configures HandlerContext.Log. Level defaults to INFO
// when nil or empty.
type LogOptions struct {
	Level    string
	Metadata json.RawMessage
}

// ResultKind classifies the outcome of Runner.Process.
type ResultKind int

const (
	// ResultAccepted — envelope recognised; handler dispatched in
	// the background. Respond with HTTP 202.
	ResultAccepted ResultKind = iota
	// ResultBadRequest — envelope malformed. Respond with HTTP 400.
	ResultBadRequest
	// ResultNotFound — envelope well-formed but no handler is
	// registered for the job code. Respond with HTTP 404.
	ResultNotFound
	// ResultUnauthorized — the firing's signature is missing or invalid
	// (a signing secret is configured), or a signature is required and
	// cannot be checked. Respond with HTTP 401.
	ResultUnauthorized
)

// Result is the return value of Runner.Process. Map to your
// framework's HTTP response type.
type Result struct {
	Kind    ResultKind
	Message string
}

// ErrorKind classifies runner-internal failures surfaced via the
// OnError hook.
type ErrorKind int

const (
	// ErrCallbackFailed — log or complete API call failed.
	ErrCallbackFailed ErrorKind = iota
	// ErrLockFailed — lock acquire or release failed.
	ErrLockFailed
	// ErrHandlerPanicked — the user handler returned an error or
	// panicked (recovered).
	ErrHandlerPanicked
)

// RunnerError wraps a runner-internal failure for the OnError hook.
type RunnerError struct {
	Kind  ErrorKind
	Cause error
}

func (e *RunnerError) Error() string {
	prefix := "scheduledjobs: "
	switch e.Kind {
	case ErrCallbackFailed:
		prefix += "callback failed"
	case ErrLockFailed:
		prefix += "lock failed"
	case ErrHandlerPanicked:
		prefix += "handler panicked"
	}
	if e.Cause != nil {
		return prefix + ": " + e.Cause.Error()
	}
	return prefix
}

func (e *RunnerError) Unwrap() error { return e.Cause }

// OnErrorHook is fired on every uncaught handler / callback / lock
// error. The hook runs synchronously inside the background goroutine;
// if you need async reporting, dispatch from here to your own
// channel.
type OnErrorHook func(err error, env *Envelope)

// LockKeyFunc derives the lock key from an envelope. The default is
// "scheduled-job:{jobCode}".
type LockKeyFunc func(env *Envelope) string

// Runner is the registry-of-handlers + envelope dispatcher.
type Runner struct {
	api          *client.FlowCatalystClient
	lockProvider lock.Provider
	handlers     map[string]HandlerFunc
	opts         runnerOpts

	wg sync.WaitGroup // tracks background goroutines for Shutdown

	warnUnverified sync.Once // one "firings are not verified" warning per Runner
}

type runnerOpts struct {
	// enforceLock nil = follow the envelope (lock unless concurrent).
	enforceLock      *bool
	signingSecret    string
	requireSignature bool
	lockTTL          time.Duration
	lockKey          LockKeyFunc
	onError          OnErrorHook
	background       func(fn func()) // test seam: defaults to go fn()
}

// Builder configures a Runner. Start one with NewBuilder.
type Builder struct {
	api          *client.FlowCatalystClient
	lockProvider lock.Provider
	handlers     map[string]HandlerFunc
	opts         runnerOpts
}

// NewBuilder starts a builder. Pass the API client used for
// log/complete callbacks and a lock.Provider (use lock.NewNoOp() if
// you don't need concurrency control).
func NewBuilder(api *client.FlowCatalystClient, lockProvider lock.Provider) *Builder {
	return &Builder{
		api:          api,
		lockProvider: lockProvider,
		handlers:     map[string]HandlerFunc{},
		opts: runnerOpts{
			lockTTL: DefaultLockTTL,
			lockKey: func(e *Envelope) string { return "scheduled-job:" + e.JobCode },
		},
	}
}

// Handler registers a handler keyed by the job's code.
func (b *Builder) Handler(code string, h HandlerFunc) *Builder {
	b.handlers[code] = h
	return b
}

// EnforceLock forces locking on or off for every firing, overriding the
// envelope's concurrent flag. By default (never called) the runner locks
// every firing whose envelope says concurrent: false and skips the lock
// when concurrent: true.
func (b *Builder) EnforceLock(enforce bool) *Builder { b.opts.enforceLock = &enforce; return b }

// SigningSecret sets the firing signing secret explicitly. With a secret set,
// firings must arrive through [Runner.ProcessSigned] carrying a valid
// X-FlowCatalyst-Signature/Timestamp pair; [Runner.Process] (which has no
// headers) then rejects everything with ResultUnauthorized.
//
// When SigningSecret is not called (or given ""), [Runner.ProcessSigned] falls
// back to the FLOWCATALYST_SIGNING_SECRET environment variable (the one
// [webhook.ValidatorFromEnv] reads) and verifies with it. With no secret from
// either source, firings run unverified and a warning is logged once per
// Runner, unless [Builder.RequireSignature] is set.
func (b *Builder) SigningSecret(secret string) *Builder { b.opts.signingSecret = secret; return b }

// RequireSignature makes the runner refuse to run any firing it cannot
// verify: with no signing secret resolvable (neither SigningSecret nor
// FLOWCATALYST_SIGNING_SECRET), [Runner.Process] and [Runner.ProcessSigned]
// return ResultUnauthorized. [Runner.Process] can never verify (it has no
// headers), so with RequireSignature set it always refuses. Default off.
func (b *Builder) RequireSignature() *Builder { b.opts.requireSignature = true; return b }

// LockTTL overrides DefaultLockTTL.
func (b *Builder) LockTTL(ttl time.Duration) *Builder { b.opts.lockTTL = ttl; return b }

// LockKey overrides the default key function.
func (b *Builder) LockKey(f LockKeyFunc) *Builder { b.opts.lockKey = f; return b }

// OnError sets the failure hook.
func (b *Builder) OnError(h OnErrorHook) *Builder { b.opts.onError = h; return b }

// Build finalises the Runner.
func (b *Builder) Build() *Runner {
	return &Runner{
		api:          b.api,
		lockProvider: b.lockProvider,
		handlers:     b.handlers,
		opts:         b.opts,
	}
}

// RegisteredCodes returns the set of registered handler codes
// (diagnostics only).
func (r *Runner) RegisteredCodes() []string {
	out := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		out = append(out, k)
	}
	return out
}

// Process is the inbound dispatch entry point for firings whose signature
// the caller has already checked (or that arrive in-process). Validates raw
// as an Envelope, kicks off the handler in the background, and returns a
// Result immediately. The actual handler + lock + completion callback run
// on a goroutine.
//
// Process has no headers and cannot verify a signature. It returns
// ResultUnauthorized when a secret was given explicitly through
// Builder.SigningSecret, or when Builder.RequireSignature is set; otherwise
// it runs the firing unverified and logs a one-time warning. A secret that
// comes only from the environment does not make Process refuse. Mount
// [Runner.ProcessSigned] on HTTP endpoints instead.
func (r *Runner) Process(raw json.RawMessage) Result {
	if r.opts.signingSecret != "" {
		return Result{Kind: ResultUnauthorized, Message: "firing signature required: use ProcessSigned with the signature and timestamp headers"}
	}
	if r.opts.requireSignature {
		return Result{Kind: ResultUnauthorized, Message: "firing signature required: Process cannot verify; use ProcessSigned with the signature and timestamp headers"}
	}
	r.warnUnverifiedOnce()
	return r.process(raw)
}

// ProcessSigned is Process plus signature verification. body MUST be the
// raw request bytes (HMAC is computed over them); signature and timestamp
// are the X-FlowCatalyst-Signature and X-FlowCatalyst-Timestamp header
// values.
//
// The secret is Builder.SigningSecret, else FLOWCATALYST_SIGNING_SECRET. With
// a secret, any verification failure returns ResultUnauthorized before the
// envelope is parsed: no handler runs and no lock is taken. With no secret,
// ResultUnauthorized is returned if Builder.RequireSignature is set;
// otherwise the headers are ignored, the firing runs, and a warning is
// logged once per Runner that firings are not being verified.
func (r *Runner) ProcessSigned(body []byte, signature, timestamp string) Result {
	secret := r.opts.signingSecret
	if secret == "" {
		secret = webhook.SigningSecretFromEnv()
	}
	if secret == "" {
		if r.opts.requireSignature {
			return Result{Kind: ResultUnauthorized, Message: "firing signature required but no signing secret is configured"}
		}
		r.warnUnverifiedOnce()
		return r.process(body)
	}
	if err := webhook.NewVerifier(secret).Verify(body, signature, timestamp); err != nil {
		return Result{Kind: ResultUnauthorized, Message: err.Error()}
	}
	return r.process(body)
}

func (r *Runner) warnUnverifiedOnce() {
	r.warnUnverified.Do(func() {
		slog.Default().Warn("scheduledjobs: running scheduled-job firings WITHOUT verifying their signature; "+
			"set Builder.SigningSecret or "+webhook.SigningSecretEnvVar+" and mount Runner.ProcessSigned, "+
			"or call Builder.RequireSignature to refuse unverified firings",
			"envVar", webhook.SigningSecretEnvVar)
	})
}

func (r *Runner) process(raw json.RawMessage) Result {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Result{Kind: ResultBadRequest, Message: err.Error()}
	}
	if env.TriggerKind != TriggerCron && env.TriggerKind != TriggerManual {
		return Result{Kind: ResultBadRequest, Message: "invalid triggerKind"}
	}
	handler, ok := r.handlers[env.JobCode]
	if !ok {
		return Result{Kind: ResultNotFound, Message: fmt.Sprintf("no handler registered for code %q", env.JobCode)}
	}

	r.wg.Add(1)
	dispatch := r.opts.background
	if dispatch == nil {
		dispatch = func(fn func()) { go fn() }
	}
	dispatch(func() {
		defer r.wg.Done()
		r.runInBackground(env, handler)
	})
	return Result{Kind: ResultAccepted}
}

// Wait blocks until all in-flight background dispatches finish.
// Intended for tests + graceful shutdown.
func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) runInBackground(env Envelope, handler HandlerFunc) {
	ctx := context.Background()
	if env.TimeoutSeconds != nil && *env.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*env.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	var held lock.Handle
	wantLock := !env.Concurrent
	if r.opts.enforceLock != nil {
		wantLock = *r.opts.enforceLock
	}
	if wantLock && r.lockProvider != nil {
		key := r.opts.lockKey(&env)
		h, err := r.lockProvider.Acquire(ctx, key, r.opts.lockTTL)
		if err != nil {
			r.fireOnError(&RunnerError{Kind: ErrLockFailed, Cause: err}, &env)
			return
		}
		if h == nil {
			// Lock contention — skip. If the job tracks completion,
			// mark Failure with reason "lock-held" so the operator
			// can see the firing was skipped, not silently lost.
			if env.TracksCompletion {
				r.reportSkipped(ctx, &env)
			}
			return
		}
		held = h
	}
	defer func() {
		if held != nil {
			if err := held.Release(ctx); err != nil {
				r.fireOnError(&RunnerError{Kind: ErrLockFailed, Cause: err}, &env)
			}
		}
	}()

	hctx := &HandlerContext{
		Envelope:   env,
		api:        r.api,
		instanceID: env.InstanceID,
		jobCode:    env.JobCode,
		onError:    r.opts.onError,
	}

	value, handlerErr := r.invokeHandler(ctx, handler, hctx, &env)

	if env.TracksCompletion {
		req := &client.InstanceCompleteRequest{}
		if handlerErr == nil {
			req.Status = client.CompletionStatusSuccess
			req.Result = sanitiseResult(value)
		} else {
			req.Status = client.CompletionStatusFailure
			req.Result = json.RawMessage(fmt.Sprintf(`{"error":%q}`, handlerErr.Error()))
		}
		if err := r.api.ScheduledJobs().CompleteInstance(ctx, env.InstanceID, req); err != nil {
			r.fireOnError(&RunnerError{Kind: ErrCallbackFailed, Cause: err}, &env)
		}
	}

	if handlerErr != nil {
		r.fireOnError(&RunnerError{Kind: ErrHandlerPanicked, Cause: handlerErr}, &env)
	}
}

func (r *Runner) invokeHandler(
	ctx context.Context,
	handler HandlerFunc,
	hctx *HandlerContext,
	env *Envelope,
) (value json.RawMessage, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			switch v := rec.(type) {
			case error:
				err = fmt.Errorf("handler panic: %w", v)
			default:
				err = fmt.Errorf("handler panic: %v", v)
			}
			_ = env // captured for parity with hook callers; nothing else to do here
		}
	}()
	return handler(ctx, hctx)
}

func (r *Runner) reportSkipped(ctx context.Context, env *Envelope) {
	req := &client.InstanceCompleteRequest{
		Status: client.CompletionStatusFailure,
		Result: json.RawMessage(`{"skipped":true,"reason":"lock-held"}`),
	}
	if err := r.api.ScheduledJobs().CompleteInstance(ctx, env.InstanceID, req); err != nil {
		r.fireOnError(&RunnerError{Kind: ErrCallbackFailed, Cause: err}, env)
	}
}

func (r *Runner) fireOnError(err *RunnerError, env *Envelope) {
	if r.opts.onError != nil {
		r.opts.onError(err, env)
	}
}

func sanitiseResult(v json.RawMessage) json.RawMessage {
	if len(v) <= MaxResultBytes {
		return v
	}
	preview := string(v[:MaxResultBytes])
	return json.RawMessage(fmt.Sprintf(`{"truncated":true,"preview":%q}`, preview))
}
