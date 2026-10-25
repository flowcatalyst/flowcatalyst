package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// Trace logs (docs/function-trace-design.md section 8): opt-in, time-boxed
// capture. Everything here is a cheap no-op while no window is live, and
// nothing in it may fail or block a call.

// Trace tuning (FC_FN_TRACE_QUEUE and FC_FN_TRACE_BODY_BYTES set the first two).
const (
	defaultTraceQueue     = 5000
	defaultTraceBodyBytes = 65536
	// defaultTraceQueueBytes bounds the memory of the queued records (a record
	// can carry two 64 KiB bodies, so a count alone would allow ~300 MB).
	defaultTraceQueueBytes = 32 << 20
	minTraceQueueBytes     = 1 << 20
	traceFlushEvery        = 2 * time.Second
	traceBatchAt           = 200 // queued records that trigger a flush before the tick
	traceShipTimeout       = 10 * time.Second
	traceBackoffBase       = time.Second
	traceBackoffMax        = 30 * time.Second
	traceCloseGrace        = 3 * time.Second
	// maxInvocationLogRecords and maxInvocationTraceRecords bound the log and
	// author trace records one invocation can queue, so a chatty guest cannot
	// push every other record out of the queue.
	maxInvocationLogRecords   = 500
	maxInvocationTraceRecords = 500
	redacted                  = "[redacted]"
)

// TraceShipper sends trace batches to the platform (control.Client in
// production). A control plane that does not implement it runs without tracing.
type TraceShipper interface {
	TraceLog(ctx context.Context, b control.TraceBatch) (*control.TraceAck, error)
}

// traceWindow is one live-until window of a context. seq counts the invocations
// that saw the window live, which is what the 1-in-rate sampling counts.
type traceWindow struct {
	context string
	untilMs int64
	rate    uint64
	seq     *atomic.Uint64
}

type traceWindows []traceWindow

// newTraceWindows builds the windows of a desired document, carrying the
// sampling counter over from old when a context stays enabled, so a re-render
// of desired state does not restart the 1-in-N cycle. nil when there are none.
func newTraceWindows(old *traceWindows, in []control.TraceState) *traceWindows {
	if len(in) == 0 {
		return nil
	}
	ws := make(traceWindows, 0, len(in))
	for _, s := range in {
		w := traceWindow{context: s.Context, untilMs: s.EnabledUntilMs, rate: uint64(max(s.SampleRate, 1))}
		if old != nil {
			for _, o := range *old {
				if o.context == s.Context {
					w.seq = o.seq
					break
				}
			}
		}
		if w.seq == nil {
			w.seq = new(atomic.Uint64)
		}
		ws = append(ws, w)
	}
	return &ws
}

// invTrace is one invocation's trace state: the contexts that are live for it
// (window open and sampled in). It exists only while some context is live.
type invTrace struct {
	t        *tracer
	address  string
	version  int
	invID    string
	corr     string
	live     []string
	httpLive bool
	ref      atomic.Pointer[string]
	logs     atomic.Int32
	traces   atomic.Int32
	ended    atomic.Bool
}

// traceBegin decides, once per invocation, which contexts are live for it. With
// no windows (the normal case) it is a nil check; with only expired windows it
// allocates nothing either.
func (r *Runner) traceBegin(fn *function, ver *version, invID, correlation string) *invTrace {
	ws := fn.trace.Load()
	if ws == nil || r.tracer == nil {
		return nil
	}
	now := r.now().UnixMilli()
	var tr *invTrace
	for i := range *ws {
		w := &(*ws)[i]
		if w.untilMs <= now || (w.seq.Add(1)-1)%w.rate != 0 {
			continue
		}
		if tr == nil {
			tr = &invTrace{t: r.tracer, address: fn.address, version: ver.number, invID: invID, corr: correlation}
		}
		tr.live = append(tr.live, w.context)
		if w.context == abi.TraceHTTP {
			tr.httpLive = true
		}
	}
	if tr != nil {
		r.tracer.begin()
	}
	return tr
}

// end marks the invocation's tracing finished (after its http capture), which
// a shutdown waits for. Idempotent; nil-safe.
func (i *invTrace) end() {
	if i != nil && i.ended.CompareAndSwap(false, true) {
		i.t.end()
	}
}

func (i *invTrace) isLive(context string) bool {
	return i != nil && slices.Contains(i.live, context)
}

// setReference remembers the invocation's business key (the trace.reference
// op): it is stamped on every later record of the call. Empty clears it.
func (i *invTrace) setReference(ref string) {
	if i != nil {
		i.ref.Store(&ref)
	}
}

// emit queues a record for the invocation. ref, when the call carried one,
// is that record's reference only; otherwise the remembered one applies.
func (i *invTrace) emit(context, kind, level, msg, ref string, data json.RawMessage) {
	if ref == "" {
		if p := i.ref.Load(); p != nil {
			ref = *p
		}
	}
	i.t.add(control.TraceRecord{
		ID: uuid.NewString(),
		At: i.t.now().UnixMilli(), Address: i.address, Version: i.version,
		InvocationID: i.invID, CorrelationID: i.corr, Reference: ref,
		Context: context, Kind: kind, Level: level, Message: msg, Data: data,
	})
}

// logRecord records a log op line of an http-live invocation.
func (i *invTrace) logRecord(level, msg string, attrs map[string]any) {
	if i == nil || !i.httpLive || !i.takeLogSlot() {
		return
	}
	var data json.RawMessage
	if len(attrs) > 0 {
		if b, err := json.Marshal(attrs); err == nil && len(b) <= control.MaxTraceData {
			data = b
		}
	}
	i.emit(abi.TraceHTTP, control.TraceKindLog, level, cutRunes(msg, control.MaxTraceMessage), "", data)
}

// guestOutput records lines the guest wrote to stdout or stderr.
func (i *invTrace) guestOutput(level string, p []byte) {
	if i == nil || !i.httpLive {
		return
	}
	for line := range strings.SplitSeq(string(p), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if !i.takeLogSlot() {
			return
		}
		i.emit(abi.TraceHTTP, control.TraceKindLog, level, cutRunes(line, control.MaxTraceMessage), "", nil)
	}
}

func (i *invTrace) takeLogSlot() bool {
	if i.logs.Add(1) > maxInvocationLogRecords {
		i.t.m.traceDropped.Inc()
		return false
	}
	return true
}

func (i *invTrace) takeTraceSlot() bool {
	if i.traces.Add(1) > maxInvocationTraceRecords {
		i.t.m.traceDropped.Inc()
		return false
	}
	return true
}

// cutRunes shortens s to at most n bytes without splitting a character.
func cutRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// invSink tells an instance's stdout/stderr writers which invocation (if any)
// is traced right now. An instance serves one call at a time, so one pointer
// per instance is exact.
type invSink struct{ cur atomic.Pointer[invTrace] }

// captureWriter records what the runner answered: the status, a snapshot of
// the headers at WriteHeader time and the first limit bytes of the body.
type captureWriter struct {
	http.ResponseWriter
	limit  int
	status int
	hdr    http.Header
	body   []byte
	size   int64
}

func (c *captureWriter) WriteHeader(code int) {
	if c.status == 0 {
		c.status = code
		c.hdr = c.ResponseWriter.Header().Clone()
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.WriteHeader(http.StatusOK)
	}
	if room := c.limit - len(c.body); room > 0 {
		c.body = append(c.body, p[:min(room, len(p))]...)
	}
	n, err := c.ResponseWriter.Write(p)
	c.size += int64(n)
	return n, err
}

func (c *captureWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// captureHTTP queues the kind:"http" record of a finished call. It never
// panics and never fails the call.
func (i *invTrace) captureHTTP(req *http.Request, path string, body []byte, caller *abi.Caller, cw *captureWriter, outcome string, start time.Time) {
	defer func() {
		if p := recover(); p != nil {
			i.t.log.Warn("trace capture failed", "fn.address", i.address, "panic", p)
		}
	}()
	status := cw.status
	if status == 0 {
		status = http.StatusOK
	}
	traceOutcome := traceOutcomeOf(outcome)
	level := "info"
	switch {
	case status >= 500 || traceOutcome != control.TraceOutcomeOK:
		level = "error"
	case status >= 400:
		level = "warn"
	}
	build := func(limit int) ([]byte, error) {
		rb, renc, rtrunc := encodeBody(body, int64(len(body)), limit)
		sb, senc, strunc := encodeBody(cw.body, cw.size, limit)
		data := control.TraceHTTPData{
			Request: control.TraceHTTPRequest{
				Method: req.Method, Path: path, RawQuery: redactQuery(req.URL.RawQuery),
				Headers: redactHeaders(req.Header), Body: rb, BodyEncoding: renc,
				BodyBytes: int64(len(body)), BodyTruncated: rtrunc,
			},
			Response: control.TraceHTTPResponse{
				Status: status, Headers: redactHeaders(cw.hdr), Body: sb, BodyEncoding: senc,
				BodyBytes: cw.size, BodyTruncated: strunc,
			},
			DurationMs: time.Since(start).Milliseconds(),
			Outcome:    traceOutcome,
		}
		if caller != nil {
			data.Request.Caller = map[string]any{"kind": caller.Kind, "id": caller.ID}
		}
		return json.Marshal(data)
	}
	// The platform replaces a record's data over MaxTraceHTTPData with a
	// marker, which would lose the whole capture; shrink the bodies until it fits.
	limit := i.t.bodyBytes
	data, err := build(limit)
	for err == nil && len(data) > control.MaxTraceHTTPData && limit > 0 {
		limit /= 2
		data, err = build(limit)
	}
	if err != nil {
		return
	}
	i.emit(abi.TraceHTTP, control.TraceKindHTTP, level, fmt.Sprintf("%s %s -> %d", req.Method, path, status), "", data)
}

// traceOutcomeOf maps the runner's metric outcome to the contract's.
func traceOutcomeOf(outcome string) string {
	switch outcome {
	case "2xx", "3xx", "4xx", "5xx", "ok":
		return control.TraceOutcomeOK
	case "timeout":
		return control.TraceOutcomeTimeout
	case "failed":
		return control.TraceOutcomeTrap
	case "out_of_memory":
		return control.TraceOutcomeOutOfMemory
	case "malformed":
		return control.TraceOutcomeBadResponse
	case "busy":
		return control.TraceOutcomeBusy
	}
	return control.TraceOutcomeError
}

// redactHeaders copies h with lower-cased names, hiding the values of
// credentials.
func redactHeaders(h http.Header) map[string][]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string][]string, len(h))
	for k, vs := range h {
		name := strings.ToLower(k)
		if sensitiveHeader(name) {
			masked := make([]string, len(vs))
			for i := range masked {
				masked[i] = redacted
			}
			out[name] = masked
			continue
		}
		out[name] = append([]string(nil), vs...)
	}
	return out
}

// sensitiveNameParts are the substrings that make a header or query
// parameter name a credential: authorization and proxy-authorization,
// x-forwarded-authorization, api-key, apikey, x-api-key, x-functions-key,
// ocp-apim-subscription-key, x-goog-api-key, x-auth-token, cookie and
// set-cookie, x-flowcatalyst-signature (a reader must not be able to replay a
// signed delivery), session ids, anything secret or password.
var sensitiveNameParts = []string{"auth", "key", "token", "secret", "password", "signature", "cookie", "session"}

// sensitiveHeader reports whether a (lower-cased) header name carries a
// credential.
func sensitiveHeader(name string) bool {
	for _, p := range sensitiveNameParts {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// sensitiveParam reports whether a (lower-cased) query parameter name
// carries a credential: the header patterns, OAuth's code, signed-URL
// signatures (sig, X-Amz-*), and the usual key and password names.
func sensitiveParam(name string) bool {
	switch name {
	case "code", "sig", "access_token", "token", "signature", "key", "apikey", "api_key", "password", "secret":
		return true
	}
	return strings.HasPrefix(name, "x-amz-") || sensitiveHeader(name)
}

// redactQuery replaces the values of credential-like parameters in a raw
// query string with "[redacted]", keeping the rest (and the order) as sent.
func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	changed := false
	for i, p := range parts {
		name, _, hasValue := strings.Cut(p, "=")
		decoded, err := url.QueryUnescape(name)
		if err != nil {
			decoded = name
		}
		if sensitiveParam(strings.ToLower(decoded)) && hasValue {
			parts[i] = name + "=" + redacted
			changed = true
		}
	}
	if !changed {
		return raw
	}
	return strings.Join(parts, "&")
}

// encodeBody renders at most limit bytes of b (total is the real size). The
// text is utf8 when the captured bytes are valid UTF-8, else base64.
func encodeBody(b []byte, total int64, limit int) (*string, string, bool) {
	if len(b) > limit {
		b = b[:limit]
	}
	truncated := int64(len(b)) < total
	if truncated {
		// The cut may have split a character; drop the fragment.
		for k := 0; k < utf8.UTFMax && k < len(b); k++ {
			if utf8.Valid(b[:len(b)-k]) {
				b = b[:len(b)-k]
				break
			}
		}
	}
	if utf8.Valid(b) {
		s := string(b)
		return &s, "utf8", truncated
	}
	s := base64.StdEncoding.EncodeToString(b)
	return &s, "base64", truncated
}

// tracer is the runner's bounded record queue and its batcher. The queue is
// one FIFO per function address: a full queue drops the oldest record of the
// function with the most queued, so one chatty function cannot push every
// other function's records out, and a batch takes records from the functions
// in turn.
type tracer struct {
	ship        TraceShipper
	m           *metrics
	log         *slog.Logger
	runnerID    string
	pool        string
	now         func() time.Time
	bodyBytes   int
	flushEvery  time.Duration
	batchAt     int
	backoffBase time.Duration
	backoffMax  time.Duration
	closeGrace  time.Duration

	mu       sync.Mutex
	queues   map[string]*traceQueue // by function address
	order    []string               // the addresses with queued records, round-robin order
	next     int                    // where the next take starts in order
	n        int
	cap      int
	bytes    int // encoded size of the queued records
	maxBytes int

	// active counts the invocations whose capture is still to come; close
	// waits (bounded) for it to reach zero before the final flush.
	active atomic.Int64

	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

// queuedRecord is a record with its encoded JSON size, measured once.
type queuedRecord struct {
	rec  control.TraceRecord
	size int
}

// traceQueue is one function's FIFO.
type traceQueue struct {
	items []queuedRecord
	head  int
}

func (q *traceQueue) len() int { return len(q.items) - q.head }

func (q *traceQueue) push(r queuedRecord) { q.items = append(q.items, r) }

func (q *traceQueue) pop() queuedRecord {
	r := q.items[q.head]
	q.items[q.head] = queuedRecord{}
	q.head++
	if q.head == len(q.items) {
		q.items, q.head = q.items[:0], 0
	} else if q.head > 64 && q.head*2 > len(q.items) {
		q.items = append(q.items[:0], q.items[q.head:]...)
		q.head = 0
	}
	return r
}

func newTracer(ship TraceShipper, m *metrics, log *slog.Logger, runnerID, pool string, queue, bodyBytes int, now func() time.Time) *tracer {
	return &tracer{
		ship: ship, m: m, log: log, runnerID: runnerID, pool: pool, now: now,
		bodyBytes: bodyBytes, flushEvery: traceFlushEvery, batchAt: traceBatchAt,
		backoffBase: traceBackoffBase, backoffMax: traceBackoffMax, closeGrace: traceCloseGrace,
		queues: map[string]*traceQueue{}, cap: max(queue, 1), maxBytes: defaultTraceQueueBytes,
		wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}),
	}
}

// encodedSize is the record's size as it goes on the wire (the same encoder
// the control client uses, so JSON escaping is counted exactly).
func encodedSize(rec control.TraceRecord) int {
	b, err := json.Marshal(rec)
	if err != nil {
		return 512 + len(rec.Data) + 6*len(rec.Message)
	}
	return len(b)
}

// traceBatchBudget is the encoded size a batch's records may take: the
// platform's MaxTraceBatchBytes less room for the envelope (runner id, pool,
// the record separators).
const traceBatchBudget = control.MaxTraceBatchBytes - 16<<10

// dropOldestFromLargest removes the oldest record of the function with the
// most queued records and counts it dropped. The lock is held.
func (t *tracer) dropOldestFromLargest() {
	var (
		victim string
		most   int
	)
	for addr, q := range t.queues {
		if l := q.len(); l > most || (l == most && addr < victim) {
			victim, most = addr, l
		}
	}
	if most == 0 {
		return
	}
	q := t.queues[victim]
	r := q.pop()
	t.bytes -= r.size
	t.n--
	if q.len() == 0 {
		t.forget(victim)
	}
	t.m.traceDropped.Inc()
}

// forget removes an emptied function queue. The lock is held.
func (t *tracer) forget(addr string) {
	delete(t.queues, addr)
	if i := slices.Index(t.order, addr); i >= 0 {
		t.order = slices.Delete(t.order, i, i+1)
		if t.next > i {
			t.next--
		}
	}
}

// add queues a record. It never blocks: a full queue (by count or by bytes)
// drops the oldest records of the function with the most queued.
func (t *tracer) add(rec control.TraceRecord) {
	size := encodedSize(rec)
	t.mu.Lock()
	for t.n > 0 && (t.n >= t.cap || t.bytes+size > t.maxBytes) {
		t.dropOldestFromLargest()
	}
	q := t.queues[rec.Address]
	if q == nil {
		q = &traceQueue{}
		t.queues[rec.Address] = q
		t.order = append(t.order, rec.Address)
	}
	q.push(queuedRecord{rec: rec, size: size})
	t.n++
	t.bytes += size
	full := t.n >= t.batchAt
	t.mu.Unlock()
	t.m.traceRecords.WithLabelValues(rec.Kind).Inc()
	if full {
		select {
		case t.wake <- struct{}{}:
		default:
		}
	}
}

// take removes records for one request: at most MaxTraceBatchRecords whose
// encoded size fits traceBatchBudget, one function at a time in turn (each
// function's records stay in order).
func (t *tracer) take() []control.TraceRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []control.TraceRecord
	size := 0
	for t.n > 0 && len(out) < control.MaxTraceBatchRecords {
		if t.next >= len(t.order) {
			t.next = 0
		}
		addr := t.order[t.next]
		q := t.queues[addr]
		head := q.items[q.head]
		if len(out) > 0 && size+head.size+1 > traceBatchBudget {
			break
		}
		q.pop()
		size += head.size + 1
		t.bytes -= head.size
		t.n--
		out = append(out, head.rec)
		if q.len() == 0 {
			t.forget(addr) // t.next now points at the following function
		} else {
			t.next++
		}
	}
	return out
}

func (t *tracer) queued() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.n
}

// begin and end bracket an invocation that has a live context, so close can
// wait for its capture (which runs after the call has answered).
func (t *tracer) begin() { t.active.Add(1) }
func (t *tracer) end()   { t.active.Add(-1) }

// start runs the batcher (once).
func (t *tracer) start() {
	if t == nil {
		return
	}
	t.startOnce.Do(func() { go t.run() })
}

// close stops the batcher after it has made one bounded attempt to ship what
// is queued. Call it once calls have drained: it first waits (up to the grace)
// for the captures of invocations still finishing.
func (t *tracer) close() {
	if t == nil {
		return
	}
	for deadline := time.Now().Add(t.closeGrace); t.active.Load() > 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	t.startOnce.Do(func() { close(t.done) }) // never started: nothing to wait for
	t.stopOnce.Do(func() { close(t.stop) })
	select {
	case <-t.done:
	case <-time.After(t.closeGrace + time.Second):
	}
}

func (t *tracer) run() {
	defer close(t.done)
	tick := time.NewTicker(t.flushEvery)
	defer tick.Stop()
	// pending are batches taken and not yet done with, first one next; a
	// batch the platform finds too large is split into two in its place.
	var pending [][]control.TraceRecord
	delay := time.Duration(0)
	for {
		select {
		case <-t.stop:
			t.finalFlush(pending)
			return
		case <-tick.C:
		case <-t.wake:
		}
		for {
			if len(pending) == 0 {
				b := t.take()
				if len(b) == 0 {
					break
				}
				pending = append(pending, b)
			}
			switch res, halves := t.send(pending[0], traceShipTimeout); res {
			case sendDone:
				pending, delay = pending[1:], 0
				continue
			case sendSplit:
				pending = append(halves, pending[1:]...)
				continue
			case sendRetry:
			}
			// Keep the batch and retry after a growing pause; callers go on
			// queueing (dropping the oldest when full) in the meantime.
			delay = min(max(delay*2, t.backoffBase), t.backoffMax)
			select {
			case <-t.stop:
				t.finalFlush(pending)
				return
			case <-time.After(delay):
			}
		}
	}
}

// finalFlush makes one bounded attempt to ship pending and the queue.
func (t *tracer) finalFlush(pending [][]control.TraceRecord) {
	deadline := time.Now().Add(t.closeGrace)
	for {
		if len(pending) == 0 {
			b := t.take()
			if len(b) == 0 {
				return
			}
			pending = append(pending, b)
		}
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		res, halves := t.send(pending[0], left)
		switch res {
		case sendDone:
			pending = pending[1:]
			continue
		case sendSplit:
			pending = append(halves, pending[1:]...)
			continue
		case sendRetry:
		}
		break
	}
	n := t.queued()
	for _, b := range pending {
		n += len(b)
	}
	t.m.traceDropped.Add(float64(n))
}

// sendResult is what became of one shipping attempt.
type sendResult int

const (
	sendDone  sendResult = iota // shipped, or refused for good (counted dropped)
	sendRetry                   // keep the batch and try again later
	sendSplit                   // too large: ship the two halves instead
)

// send ships one batch. A panic is treated as a refusal. A 413 splits the
// batch (a single record the platform will not take is dropped); 401 and 403
// are retried (the control client has already refreshed its token once, and a
// grant may be restored), as are 408, 429, 5xx and transport errors; any
// other 4xx is final, since retrying would wedge the queue.
func (t *tracer) send(batch []control.TraceRecord, timeout time.Duration) (res sendResult, halves [][]control.TraceRecord) {
	defer func() {
		if p := recover(); p != nil {
			t.log.Error("trace shipping panicked", "panic", p)
			t.m.traceFlushFailures.Inc()
			t.m.traceDropped.Add(float64(len(batch)))
			res, halves = sendDone, nil
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := t.ship.TraceLog(ctx, control.TraceBatch{RunnerID: t.runnerID, Pool: t.pool, Records: batch})
	if err == nil {
		return sendDone, nil
	}
	t.m.traceFlushFailures.Inc()
	if se, ok := errors.AsType[*control.ErrStatus](err); ok && se.Status >= 400 && se.Status < 500 {
		switch se.Status {
		case http.StatusRequestEntityTooLarge:
			if len(batch) > 1 {
				h := len(batch) / 2
				return sendSplit, [][]control.TraceRecord{batch[:h:h], batch[h:]}
			}
			t.log.Warn("trace record too large for the platform; dropping it", "err", err)
			t.m.traceDropped.Inc()
			return sendDone, nil
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout, http.StatusTooManyRequests:
		default:
			t.log.Warn("trace batch refused; dropping it", "records", len(batch), "err", err)
			t.m.traceDropped.Add(float64(len(batch)))
			return sendDone, nil
		}
	}
	t.log.Warn("trace batch not shipped; will retry", "records", len(batch), "err", err)
	return sendRetry, nil
}

// traceOp serves the trace op: validation first (so a typo is found while the
// context is off), then a record only when the context is live for the call.
func (c *capabilities) traceOp(meta []byte) ([]byte, []byte, *abi.Error) {
	var m struct {
		Context   string          `json:"context"`
		Level     string          `json:"level"`
		Msg       string          `json:"msg"`
		Data      json.RawMessage `json:"data"`
		Reference string          `json:"reference"`
	}
	if err := json.Unmarshal(meta, &m); err != nil {
		return nil, nil, abi.Errorf(abi.CodeBadRequest, "meta is not the expected JSON: "+err.Error())
	}
	if aerr := c.checkTraceContext(m.Context, true); aerr != nil {
		return nil, nil, aerr
	}
	switch m.Level {
	case "":
		m.Level = abi.TraceInfo
	case abi.TraceDebug, abi.TraceInfo, abi.TraceWarn, abi.TraceError:
	default:
		return nil, nil, abi.Errorf(abi.CodeBadRequest, "level must be debug, info, warn or error")
	}
	if len(m.Msg) > abi.TraceMaxMessage {
		return nil, nil, abi.Errorf(abi.CodeTooLarge, fmt.Sprintf("msg is over %d bytes", abi.TraceMaxMessage))
	}
	if len(m.Data) > abi.TraceMaxData {
		return nil, nil, abi.Errorf(abi.CodeTooLarge, fmt.Sprintf("data is over %d bytes", abi.TraceMaxData))
	}
	if d := bytes.TrimSpace(m.Data); len(d) == 0 || bytes.Equal(d, []byte("null")) {
		m.Data = nil
	} else if d[0] != '{' {
		return nil, nil, abi.Errorf(abi.CodeBadRequest, "data must be a JSON object")
	}
	if utf8.RuneCountInString(m.Reference) > abi.TraceMaxReference {
		return nil, nil, abi.Errorf(abi.CodeTooLarge, fmt.Sprintf("reference is over %d characters", abi.TraceMaxReference))
	}
	tr := c.trace
	if tr == nil {
		return nil, nil, nil
	}
	// A reference on the call is this record's only; the invocation's own is
	// set with the trace.reference op.
	if tr.isLive(m.Context) && tr.takeTraceSlot() {
		tr.emit(m.Context, control.TraceKindTrace, m.Level, m.Msg, m.Reference, m.Data)
	}
	return nil, nil, nil
}

// traceReferenceOp serves the trace.reference op: the invocation's business
// key, stamped on its later trace, http and log records. A no-op while no
// context is live for the call.
func (c *capabilities) traceReferenceOp(meta []byte) ([]byte, []byte, *abi.Error) {
	q, aerr := decode[abi.TraceReferenceCall](meta)
	if aerr != nil {
		return nil, nil, aerr
	}
	if utf8.RuneCountInString(q.Reference) > abi.TraceMaxReference {
		return nil, nil, abi.Errorf(abi.CodeTooLarge, fmt.Sprintf("reference is over %d characters", abi.TraceMaxReference))
	}
	c.trace.setReference(q.Reference)
	return nil, nil, nil
}

// tracingOp serves the tracing op: is the context live for this invocation.
func (c *capabilities) tracingOp(meta []byte) ([]byte, []byte, *abi.Error) {
	q, aerr := decode[abi.TracingCall](meta)
	if aerr != nil {
		return nil, nil, aerr
	}
	if aerr := c.checkTraceContext(q.Context, false); aerr != nil {
		return nil, nil, aerr
	}
	return encode(abi.TracingResult{Live: c.trace.isLive(q.Context)}), nil, nil
}

// checkTraceContext validates a context name. The built-in http context is
// readable (tracing) but not writable (trace) by a guest.
func (c *capabilities) checkTraceContext(name string, write bool) *abi.Error {
	switch {
	case name == "":
		return abi.Errorf(abi.CodeBadRequest, "context is required")
	case name == abi.TraceHTTP && write:
		return abi.Errorf(abi.CodeBadRequest, "the http context is built in and not writable by a function")
	case name != abi.TraceHTTP && !c.ver.describe.HasTraceContext(name):
		return abi.Errorf(abi.CodeNotDeclared, fmt.Sprintf("trace context %q is not declared by this version", name))
	}
	return nil
}
