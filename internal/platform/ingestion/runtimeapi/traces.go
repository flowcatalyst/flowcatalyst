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

	"github.com/jackc/pgx/v5"

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
	if err := s.requireRuntime(ctx, batch.RuntimeID, TracesOperationID, ""); err != nil {
		return nil, err
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
		candidates = make([]ingestion.TraceLogRecord, 0, len(batch.Records))
		withRecord []string // the ingestions candidates are of
		dropped    int
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
		withRecord = append(withRecord, r.IngestionID)
		candidates = append(candidates, newTraceRecord(batch.RuntimeID, r, at, now))
	}
	if len(candidates) == 0 {
		return &apicommon.Out[IngestionTraceAck]{Body: IngestionTraceAck{Dropped: dropped}}, nil
	}

	// The runtime must be one that may post for the ingestion, as the jobs
	// call checks it (owner's decision D2): live, of a kind it carries, its
	// lease held for a polled source, its client active. Checked, and the
	// records stored, in one transaction that holds the polled ingestions'
	// lease rows, so a lease cannot pass to another runtime in between. A
	// record of an ingestion the runtime may not post for counts as dropped.
	var (
		keep    []ingestion.TraceLogRecord
		refused map[string]int // refusal code -> records dropped for it
	)
	err = s.Repo.WithTraceAccess(ctx, batch.RuntimeID, withRecord, s.now(), func(ctx context.Context, tx pgx.Tx, codes map[string]string) error {
		keep, refused = make([]ingestion.TraceLogRecord, 0, len(candidates)), map[string]int{}
		perIng := map[string][]int{} // ingestion id -> indexes into keep
		for _, rec := range candidates {
			if code, no := codes[rec.IngestionID]; no {
				refused[code]++
				continue
			}
			perIng[rec.IngestionID] = append(perIng[rec.IngestionID], len(keep))
			keep = append(keep, rec)
		}
		keep = s.capRate(keep, perIng, now)
		return s.Repo.InsertTraceRecordsTx(ctx, tx, keep)
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "insert_trace_records failed", err)
	}
	// What is not kept was refused for its ingestion or over the rate.
	dropped += len(candidates) - len(keep)
	if dropped > 0 {
		slog.DebugContext(ctx, "ingestion trace records dropped", "runtimeId", batch.RuntimeID, "accepted", len(keep),
			"dropped", dropped, "refused", refused)
	}
	return &apicommon.Out[IngestionTraceAck]{Body: IngestionTraceAck{Accepted: len(keep), Dropped: dropped}}, nil
}

// capRate applies the per-ingestion rate cap to keep (perIng: the indexes of
// each ingestion's records in it) and returns what is within it; the excess
// counts as dropped.
func (s *State) capRate(keep []ingestion.TraceLogRecord, perIng map[string][]int, now time.Time) []ingestion.TraceLogRecord {
	overRate := map[int]bool{}
	for id, idx := range perIng {
		allowed := s.limiter.take(id, len(idx), now)
		for _, i := range idx[allowed:] {
			overRate[i] = true
		}
	}
	if len(overRate) == 0 {
		return keep
	}
	kept := keep[:0]
	for i, rec := range keep {
		if !overRate[i] {
			kept = append(kept, rec)
		}
	}
	return kept
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
	return replaceNULEscapes(raw)
}

// nulEscape is the JSON escape of NUL, which Postgres refuses in jsonb.
var nulEscape = []byte(`\u0000`)

// replaceNULEscapes replaces each escape of NUL in raw, which is valid JSON,
// with U+FFFD, the replacement character, and leaves everything else as it
// is (owner's decision D11, 2026-11-22). It reads the escapes as JSON does: in
// valid JSON a backslash occurs only in a string and always starts an escape,
// a backslash and one character, or \u and four hex digits, so walking from
// escape to escape never takes the second half of an escaped backslash for
// the start of one. The text \\u0000 (an escaped backslash, then u0000) is
// therefore kept, where replacing the text \u0000 wherever it occurred left
// an invalid escape that made Postgres refuse the whole batch.
func replaceNULEscapes(raw []byte) []byte {
	if !bytes.Contains(raw, nulEscape) {
		return raw
	}
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 >= len(raw) {
			out = append(out, raw[i])
			continue
		}
		if bytes.HasPrefix(raw[i:], nulEscape) {
			out = append(out, "\uFFFD"...)
			i += len(nulEscape) - 1
			continue
		}
		// Any other escape is copied whole: its second character is never
		// the start of another.
		out = append(out, raw[i], raw[i+1])
		i++
	}
	return out
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
