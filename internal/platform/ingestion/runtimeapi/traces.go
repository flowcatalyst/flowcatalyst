package runtimeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// traceIDPrefix is the TSID prefix of a trace record (ing_trace_log.id), itr_….
// Not an internal/tsid entity type: a record is append-only and never
// addressed by a typed client.
const traceIDPrefix = "itr"

type tracesInput struct {
	Body IngestionTraceBatch
}

// wall is the platform's clock for trace decisions: the test clock, else the
// wall clock. Traces compare a runtime's `at` against the platform's own
// time, not the database's, as the function trace log does.
func (s *State) wall() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// traces serves POST /api/ingestion-runtime/v1/traces (design section 4): a
// runtime ships a batch of trace records. The platform enforces the batch
// caps, the per-ingestion rate, truncation and the acceptance rule; whatever
// it does not store is counted as dropped, never an error. Tracing must not
// fail a runtime's work, so it treats any answer but a 5xx as final.
func (s *State) traces(ctx context.Context, in *tracesInput) (*apicommon.Out[IngestionTraceAck], error) {
	if err := auth.RequireIngestionRuntime(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	batch := in.Body
	if strings.TrimSpace(batch.RuntimeID) == "" {
		return nil, usecase.Validation("RUNTIME_ID_REQUIRED", "runtimeId is required")
	}
	if len(batch.Records) > ingestion.MaxTraceBatchRecords {
		return nil, usecase.Validation("TRACE_BATCH_TOO_LARGE", "a batch holds at most 500 records")
	}
	if len(batch.Records) == 0 {
		return &apicommon.Out[IngestionTraceAck]{Body: IngestionTraceAck{}}, nil
	}

	seen := map[string]bool{}
	var named []string
	for _, r := range batch.Records {
		if !seen[r.IngestionID] {
			seen[r.IngestionID] = true
			named = append(named, r.IngestionID)
		}
	}
	rows, err := s.Repo.ListTraceWindows(ctx, named)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_trace_windows failed", err)
	}
	windows := make(map[string]ingestion.TraceWindow, len(rows))
	for _, w := range rows {
		windows[w.IngestionID] = w
	}

	now := s.wall()
	var (
		keep    = make([]ingestion.TraceLogRecord, 0, len(batch.Records))
		perIng  = map[string][]int{} // ingestion id -> indexes into keep
		dropped int
	)
	for _, r := range batch.Records {
		w, ok := windows[r.IngestionID]
		if !ok || r.WindowID != w.WindowID || r.At <= 0 || !ingestion.ValidTraceKind(r.Kind) || strings.TrimSpace(r.CaptureID) == "" {
			dropped++
			continue
		}
		at, ok := traceRecordAt(r.At, now)
		if !ok || at.After(w.HardStop.Add(ingestion.TraceAcceptSlack)) {
			dropped++
			continue
		}
		perIng[r.IngestionID] = append(perIng[r.IngestionID], len(keep))
		keep = append(keep, newTraceRecord(batch.RuntimeID, r, at, now))
	}

	// The per-ingestion rate cap: the excess counts as dropped.
	overRate := map[int]bool{}
	for id, idx := range perIng {
		allowed := s.limiter.take(id, len(idx), now)
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
		slog.DebugContext(ctx, "ingestion trace records dropped", "runtimeId", batch.RuntimeID, "accepted", len(keep), "dropped", dropped)
	}
	return &apicommon.Out[IngestionTraceAck]{Body: IngestionTraceAck{Accepted: len(keep), Dropped: dropped}}, nil
}

// traceRecordAt bounds a record's runtime-supplied time (unix ms) against the
// arrival time now: later than now+TraceAtFutureSkew is clamped to now (a
// runtime clock running ahead must not place records in the future, nor pass
// the acceptance gate after a window closed); older than now-TraceAtMaxAge is
// refused (false). Retention reads created_at, never this.
func traceRecordAt(ms int64, now time.Time) (time.Time, bool) {
	at := time.UnixMilli(ms).UTC()
	switch {
	case at.After(now.Add(ingestion.TraceAtFutureSkew)):
		return now, true
	case at.Before(now.Add(-ingestion.TraceAtMaxAge)):
		return time.Time{}, false
	}
	return at, true
}

// traceRecordIDPattern is what a runtime-supplied record id may look like (a
// UUID or ULID, or any similar opaque token).
var traceRecordIDPattern = regexp.MustCompile(`^[0-9A-Za-z_-]{16,64}$`)

// traceRecordID is the stored id: derived from the runtime's id when it sent
// a usable one (so a retried batch is stored once: the insert skips an id it
// already holds), else a fresh platform id.
func traceRecordID(runtimeRecordID string) string {
	if traceRecordIDPattern.MatchString(runtimeRecordID) {
		return traceIDPrefix + "_" + runtimeRecordID
	}
	return tsid.GenerateWithPrefix(traceIDPrefix)
}

// newTraceRecord applies the truncations of design section 4 and assigns the id.
func newTraceRecord(runtimeID string, r IngestionTraceRecord, at, now time.Time) ingestion.TraceLogRecord {
	level := r.Level
	if !ingestion.ValidTraceLevel(level) {
		level = "info"
	}
	return ingestion.TraceLogRecord{
		ID:          traceRecordID(r.ID),
		IngestionID: r.IngestionID,
		Version:     int32(min(max(r.Version, 0), 1<<30)), //nolint:gosec // clamped to a version number
		WindowID:    r.WindowID,
		CaptureID:   cutRunes(cleanText(r.CaptureID), ingestion.MaxTraceIDLength),
		RuntimeID:   cutRunes(cleanText(runtimeID), ingestion.MaxTraceIDLength),
		Kind:        r.Kind,
		Level:       level,
		Message:     cutRunes(cleanText(r.Message), ingestion.MaxTraceMessage),
		Data:        traceData(r.Data, ingestion.MaxTraceData),
		At:          at,
		CreatedAt:   now,
	}
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

// traceRateLimiter is the per-ingestion token bucket (design section 4): 50
// records a second, burst one second's worth, in memory per platform process.
// Deliberately not shared: with N platform replicas behind the load balancer
// an ingestion can store up to N*50 records a second. It bounds a runaway, it
// is not a quota. The zero value is ready to use.
type traceRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*traceBucket
}

type traceBucket struct {
	tokens float64
	last   time.Time
}

// take grants up to n records to the ingestion at now and returns how many.
func (l *traceRateLimiter) take(ingestionID string, n int, now time.Time) int {
	const rate = float64(ingestion.MaxTraceRecordsPerSec)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = map[string]*traceBucket{}
	}
	b, ok := l.buckets[ingestionID]
	if !ok {
		b = &traceBucket{tokens: rate, last: now}
		l.buckets[ingestionID] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(rate, b.tokens+elapsed*rate)
		b.last = now
	}
	granted := min(n, int(b.tokens))
	b.tokens -= float64(granted)
	return granted
}
