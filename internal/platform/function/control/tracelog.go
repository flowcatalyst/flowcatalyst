package control

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// traceIDPrefix is the TSID prefix of a trace-log record (fng_trace_log.id).
// Not an internal/tsid entity type: a record is append-only and never
// addressed by a typed client.
const traceIDPrefix = "ftl"

type traceLogInput struct {
	Body fncontrol.TraceBatch
}

// TraceLogOperationID is the OpenAPI operation id of the trace-log ingest. Its
// request body is relaxed all the way down (httpcompat.RelaxRequestBodiesDeep)
// so a newer runner may add fields to a batch or a record.
const TraceLogOperationID = "postFunctionRunnerTraceLog"

// registerTraceLog mounts POST /control/functions/trace-log. Registered with
// huma directly (not apiroute) to pin MaxBodyBytes to the design's 1 MiB batch
// cap rather than rely on huma's default.
func registerTraceLog(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   TraceLogOperationID,
		Method:        http.MethodPost,
		Path:          fncontrol.PathTraceLog,
		Summary:       "Ship trace records captured by a function runner (function-runner role only)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusAccepted,
		MaxBodyBytes:  fncontrol.MaxTraceBatchBytes,
	}, s.traceLog)
}

// traceLog serves POST /control/functions/trace-log (design section 5): a
// runner ships a batch of trace records. The platform enforces the batch caps,
// the per-function rate, truncation, and the acceptance rule; whatever it does
// not store is counted as dropped, never an error. Tracing must not fail a
// call, so a runner treats any answer but a 5xx as final.
func (s *State) traceLog(ctx context.Context, in *traceLogInput) (*apicommon.Out[fncontrol.TraceAck], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if err := auth.CanControlFunctionRunner(ac); err != nil {
		return nil, err
	}
	batch := in.Body
	if strings.TrimSpace(batch.RunnerID) == "" {
		return nil, usecase.Validation("RUNNER_ID_REQUIRED", "runnerId is required")
	}
	if len(batch.Records) > fncontrol.MaxTraceBatchRecords {
		return nil, usecase.Validation("TRACE_BATCH_TOO_LARGE",
			"a batch holds at most 500 records")
	}
	if len(batch.Records) == 0 {
		return &apicommon.Out[fncontrol.TraceAck]{Body: fncontrol.TraceAck{}}, nil
	}

	addresses := make([]string, 0, len(batch.Records))
	for _, r := range batch.Records {
		addresses = append(addresses, r.Address)
	}
	idByAddress, err := s.Repo.FunctionIDsByAddress(ctx, addresses)
	if err != nil {
		return nil, usecase.Internal("REPO", "function_ids_by_address failed", err)
	}
	functionIDs := make([]string, 0, len(idByAddress))
	for _, id := range idByAddress {
		functionIDs = append(functionIDs, id)
	}
	rows, err := s.Repo.ListTraceContextsFor(ctx, functionIDs)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_trace_contexts_for failed", err)
	}
	until := make(map[[2]string]time.Time, len(rows)) // (function, context) -> enabled_until
	for _, c := range rows {
		if c.EnabledUntil != nil {
			until[[2]string{c.FunctionID, c.Context}] = *c.EnabledUntil
		}
	}

	now := time.Now().UTC()
	slack := time.Duration(fncontrol.TraceAcceptSlackMillis) * time.Millisecond
	var (
		keep     = make([]function.TraceLogRecord, 0, len(batch.Records))
		perFnIdx = map[string][]int{} // function id -> indexes into keep
		dropped  int
	)
	for _, r := range batch.Records {
		fid, ok := idByAddress[r.Address]
		if !ok || r.At <= 0 || !validTraceKind(r.Kind) || strings.TrimSpace(r.Context) == "" {
			dropped++
			continue
		}
		at, ok := traceRecordAt(r.At, now)
		if !ok {
			dropped++
			continue
		}
		enabledUntil, ok := until[[2]string{fid, r.Context}]
		if !ok || at.After(enabledUntil.Add(slack)) {
			dropped++
			continue
		}
		perFnIdx[fid] = append(perFnIdx[fid], len(keep))
		keep = append(keep, s.newTraceRecord(fid, batch.RunnerID, r, at, now))
	}

	// The per-function rate cap: the excess counts as dropped.
	overRate := map[int]bool{}
	for fid, idx := range perFnIdx {
		allowed := s.traceLimiter().take(fid, len(idx), now)
		for _, i := range idx[allowed:] {
			overRate[i] = true
		}
	}
	if len(overRate) > 0 {
		kept := keep[:0]
		for i, rec := range keep {
			if overRate[i] {
				dropped++
				continue
			}
			kept = append(kept, rec)
		}
		keep = kept
	}

	if err := s.Repo.InsertTraceRecords(ctx, keep); err != nil {
		return nil, usecase.Internal("REPO", "insert_trace_records failed", err)
	}
	if dropped > 0 {
		slog.Debug("function trace records dropped", "runnerId", batch.RunnerID, "accepted", len(keep), "dropped", dropped)
	}
	return &apicommon.Out[fncontrol.TraceAck]{Body: fncontrol.TraceAck{Accepted: len(keep), Dropped: dropped}}, nil
}

func validTraceKind(k string) bool {
	switch k {
	case fncontrol.TraceKindTrace, fncontrol.TraceKindHTTP, fncontrol.TraceKindLog:
		return true
	}
	return false
}

func validTraceLevel(l string) bool {
	switch l {
	case "debug", "info", "warn", "error":
		return true
	}
	return false
}

// traceRecordAt bounds a record's runner-supplied time (unix ms) against the
// arrival time now: later than now+TraceAtFutureSkew is clamped to now (a
// runner clock running ahead must not place records in the future, nor pass
// the acceptance gate after a window closed); older than now-TraceAtMaxAge is
// refused (false). Retention reads created_at, never this.
func traceRecordAt(ms int64, now time.Time) (time.Time, bool) {
	at := time.UnixMilli(ms).UTC()
	switch {
	case at.After(now.Add(function.TraceAtFutureSkew)):
		return now, true
	case at.Before(now.Add(-function.TraceAtMaxAge)):
		return time.Time{}, false
	}
	return at, true
}

// traceRecordIDPattern is what a runner-supplied record id may look like (a
// UUID or ULID, or any similar opaque token).
var traceRecordIDPattern = regexp.MustCompile(`^[0-9A-Za-z_-]{16,64}$`)

// traceRecordID is the stored id: derived from the runner's id when it sent a
// usable one (so a retried batch is stored once: the insert skips an id it
// already holds), else a fresh platform id (an older runner).
func traceRecordID(runnerRecordID string) string {
	if traceRecordIDPattern.MatchString(runnerRecordID) {
		return traceIDPrefix + "_r" + runnerRecordID
	}
	return tsid.GenerateWithPrefix(traceIDPrefix)
}

// newTraceRecord applies the truncations of design section 5 and assigns the id.
func (s *State) newTraceRecord(functionID, runnerID string, r fncontrol.TraceRecord, at, now time.Time) function.TraceLogRecord {
	level := r.Level
	if !validTraceLevel(level) {
		level = "info"
	}
	maxData := fncontrol.MaxTraceHTTPData
	if r.Kind == fncontrol.TraceKindTrace {
		maxData = fncontrol.MaxTraceData
	}
	rec := function.TraceLogRecord{
		ID:           traceRecordID(r.ID),
		FunctionID:   functionID,
		Version:      int32(r.Version), //nolint:gosec // a version number
		InvocationID: cleanText(r.InvocationID),
		Context:      cleanText(r.Context),
		Kind:         r.Kind,
		Level:        level,
		Message:      cutRunes(cleanText(r.Message), fncontrol.MaxTraceMessage),
		Data:         traceData(r.Data, maxData),
		At:           at,
		CreatedAt:    now,
	}
	if v := cutRunes(cleanText(r.CorrelationID), function.MaxTraceCorrelation); v != "" {
		rec.CorrelationID = &v
	}
	if v := cutRunes(cleanText(r.Reference), function.MaxTraceReference); v != "" {
		rec.Reference = &v
	}
	if runnerID != "" {
		v := cleanText(runnerID)
		rec.RunnerID = &v
	}
	return rec
}

// traceData is the record's data as stored: nil when absent or not JSON, the
// stub {"truncated":true,"bytes":N} when over max bytes.
func traceData(raw json.RawMessage, maxBytes int) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || !json.Valid(raw) {
		return nil
	}
	if len(raw) > maxBytes {
		b, _ := json.Marshal(struct {
			Truncated bool `json:"truncated"`
			Bytes     int  `json:"bytes"`
		}{true, len(raw)})
		return b
	}
	// Postgres refuses a NUL in jsonb text.
	return bytes.ReplaceAll(raw, []byte(`\u0000`), []byte(`�`))
}

// cleanText drops NULs (Postgres rejects them in text) and invalid UTF-8.
func cleanText(s string) string {
	if strings.ContainsRune(s, 0) {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	return s
}

// cutRunes cuts s to at most n characters.
func cutRunes(s string, n int) string {
	if len(s) <= n { // bytes >= runes, so this is a safe fast path
		return s
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// traceRateLimiter is the per-function token bucket (design section 5): 200
// records a second, burst one second's worth, in memory per platform process.
// Deliberately not shared: with N platform replicas behind the load balancer
// a function can store up to N*200 records a second. It bounds a runaway, it
// is not a quota.
type traceRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*traceBucket
}

type traceBucket struct {
	tokens float64
	last   time.Time
}

func newTraceRateLimiter() *traceRateLimiter {
	return &traceRateLimiter{buckets: map[string]*traceBucket{}}
}

// take grants up to n records to the function at now and returns how many.
func (l *traceRateLimiter) take(functionID string, n int, now time.Time) int {
	const rate = float64(fncontrol.MaxTraceRecordsPerSec)
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[functionID]
	if !ok {
		b = &traceBucket{tokens: rate, last: now}
		l.buckets[functionID] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(rate, b.tokens+elapsed*rate)
		b.last = now
	}
	granted := min(n, int(b.tokens))
	b.tokens -= float64(granted)
	return granted
}
