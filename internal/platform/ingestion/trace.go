package ingestion

import (
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
)

// This file is the platform side of ingestion traces
// (docs/ingestion-trace-design.md): the window an operator switches on for an
// ingestion, the records the runtimes ship, and the limits around both.

// Trace limits and defaults.
const (
	// TraceWindowSecs60, TraceWindowSecs300 and TraceWindowSecs900 are the
	// window lengths an operator may choose: 1, 5 and 15 minutes. The longest
	// is the cap (TraceMaxWindowSecs).
	TraceWindowSecs60  = 60
	TraceWindowSecs300 = 300
	TraceWindowSecs900 = 900
	TraceMaxWindowSecs = TraceWindowSecs900

	// DefaultTraceWindowSecs and DefaultTraceSampleEverySecs apply when a
	// request names neither.
	DefaultTraceWindowSecs      = TraceWindowSecs60
	DefaultTraceSampleEverySecs = 5
	// MinTraceSampleEverySecs and MaxTraceSampleEverySecs bound
	// sampleEverySecs: a runtime starts a capture of an ingestion at most this
	// often.
	MinTraceSampleEverySecs = 1
	MaxTraceSampleEverySecs = 60

	// DefaultTracePickupGrace is what is added to a window's hard stop for a
	// runtime to learn of it (FC_INGESTION_TRACE_PICKUP_GRACE): it covers one
	// production sync, the runtime's capture counting from the moment it first
	// sees the window.
	DefaultTracePickupGrace = 2 * time.Minute
	// DefaultTraceRetentionDays is how long records are kept
	// (FC_INGESTION_TRACE_RETENTION_DAYS).
	DefaultTraceRetentionDays = 7

	// TraceAcceptSlack is how long after a window's hard stop a record is
	// still accepted: records captured just before the stop and still queued
	// in a runtime.
	TraceAcceptSlack = 10 * time.Second
	// TraceAtFutureSkew and TraceAtMaxAge bound a record's runtime-supplied
	// `at` against the platform's clock when it arrives: a later `at` is
	// clamped to the arrival time (a runtime clock running ahead), an older
	// one than TraceAtMaxAge is dropped (a clock far behind, or a record held
	// through a long outage). Retention never reads `at`.
	TraceAtFutureSkew = time.Minute
	TraceAtMaxAge     = time.Hour

	// The limits of one POST /traces batch and of one record in it.
	MaxTraceBatchRecords  = 500
	MaxTraceBatchBytes    = 1 << 20
	MaxTraceRecordsPerSec = 50 // per ingestion, per platform process
	MaxTraceMessage       = 1000
	MaxTraceData          = 80 << 10
	// MaxTraceIDLength cuts a runtime-supplied window, capture or runtime id.
	MaxTraceIDLength = 100

	// DefaultTraceLogLimit and MaxTraceLogLimit are the page of a trace-log read.
	DefaultTraceLogLimit = 100
	MaxTraceLogLimit     = 500

	// TracePruneChunk is how many rows one retention DELETE removes.
	TracePruneChunk = 5000
)

// Trace record kinds and levels (the contract's, section 4).
const (
	TraceKindPoll     = "poll"
	TraceKindExchange = "exchange"
	TraceKindHTTP     = "http"
	TraceKindError    = "error"
)

// TraceKinds are the kinds a runtime ships.
var TraceKinds = []string{TraceKindPoll, TraceKindExchange, TraceKindHTTP, TraceKindError}

// ValidTraceKind reports whether k is a kind a runtime may ship.
func ValidTraceKind(k string) bool { return slices.Contains(TraceKinds, k) }

// ValidTraceLevel reports whether l is info, warn or error.
func ValidTraceLevel(l string) bool { return l == "info" || l == "warn" || l == "error" }

// ValidTraceWindowSecs reports whether n is one of the window lengths.
func ValidTraceWindowSecs(n int) bool {
	return n == TraceWindowSecs60 || n == TraceWindowSecs300 || n == TraceWindowSecs900
}

// TracePickupGrace is FC_INGESTION_TRACE_PICKUP_GRACE (a Go duration such as
// 2m), or DefaultTracePickupGrace when unset, unparseable or not positive.
func TracePickupGrace() time.Duration {
	if d, err := time.ParseDuration(envutil.Or("FC_INGESTION_TRACE_PICKUP_GRACE", "")); err == nil && d > 0 {
		return d
	}
	return DefaultTracePickupGrace
}

// TraceRetention is how long trace records are kept:
// FC_INGESTION_TRACE_RETENTION_DAYS, or DefaultTraceRetentionDays when unset,
// unparseable or not positive.
func TraceRetention() time.Duration {
	days := envutil.Int("FC_INGESTION_TRACE_RETENTION_DAYS", DefaultTraceRetentionDays)
	if days <= 0 {
		days = DefaultTraceRetentionDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// TraceWindow is the row of ing_trace_windows: the window an operator last
// switched on for an ingestion.
type TraceWindow struct {
	IngestionID     string
	WindowID        string
	WindowSecs      int
	SampleEverySecs int
	// HardStop is when no runtime captures any more: the enable time plus the
	// window plus the pickup grace, or the switch-off time once switched off.
	HardStop  time.Time
	EnabledBy *string
	EnabledAt time.Time
	UpdatedAt time.Time
}

// Live reports whether the window is open at now. Compared when read: an
// expired window needs no cleanup.
func (w TraceWindow) Live(now time.Time) bool { return w.HardStop.After(now) }

// TraceLogRecord is one row of ing_trace_log.
type TraceLogRecord struct {
	ID          string
	IngestionID string
	Version     int32
	WindowID    string
	CaptureID   string
	RuntimeID   string
	Kind        string
	Level       string
	Message     string
	Data        []byte // JSON; nil when none
	At          time.Time
	CreatedAt   time.Time
}

// TraceLogFilter drives QueryTraceLog. Zero fields do not filter.
type TraceLogFilter struct {
	IngestionID string
	Kind        string
	RuntimeID   string
	CaptureID   string
	Since       *time.Time
	Until       *time.Time
	// After is the keyset cursor: only rows strictly older than (At, ID).
	After *TraceCursor
	Limit int
}

// TraceCursor is the keyset position (at, id), newest first.
type TraceCursor struct {
	At time.Time
	ID string
}

// WithholdTraceDetail strips what a reader without
// platform:ingestion:trace:view-bodies may not see, and reports whether
// anything was withheld (design section 7). Such a reader sees an http
// record's client, method, URL path (never host or query), status, sizes,
// duration, attempt and failure; an exchange record's counts and outcome; a
// poll record whole but for its error; an error record's metadata only. No
// headers, query strings, bodies, source headers, payload excerpts or error
// text. The message of an http, exchange or error record is rebuilt from what
// stays, because the runtime's one-line message can carry the query string,
// a file name or the error text.
func WithholdTraceDetail(r *TraceLogRecord) bool {
	switch r.Kind {
	case TraceKindHTTP:
		var path, method string
		var status int
		r.Data, method, path, status = httpTraceSummary(r.Data)
		r.Message = strings.TrimSpace(method + " " + path)
		if status != 0 {
			r.Message += " -> " + strconv.Itoa(status)
		}
		return true
	case TraceKindExchange:
		var outcome string
		r.Data, outcome = exchangeTraceSummary(r.Data)
		r.Message = "exchange"
		if outcome != "" {
			r.Message += ": " + outcome
		}
		return true
	case TraceKindPoll:
		var withheld bool
		r.Data, withheld = pollTraceSummary(r.Data)
		return withheld
	default:
		r.Message, r.Data = "", nil
		return true
	}
}

// httpTraceSummary keeps the non-sensitive fields of an http record's data.
func httpTraceSummary(data []byte) (out []byte, method, path string, status int) {
	var in struct {
		Client  string `json:"client"`
		Request struct {
			Method        string `json:"method"`
			URL           string `json:"url"`
			BodyBytes     int64  `json:"bodyBytes"`
			BodyTruncated bool   `json:"bodyTruncated"`
		} `json:"request"`
		Response *struct {
			Status        int   `json:"status"`
			BodyBytes     int64 `json:"bodyBytes"`
			BodyTruncated bool  `json:"bodyTruncated"`
		} `json:"response"`
		DurationMs int64   `json:"durationMs"`
		Attempt    int     `json:"attempt"`
		Failure    *string `json:"failure"`
	}
	if len(data) == 0 || json.Unmarshal(data, &in) != nil {
		return nil, "", "", 0
	}
	path = urlPathOnly(in.Request.URL)
	type request struct {
		Method        string `json:"method"`
		URL           string `json:"url"`
		BodyBytes     int64  `json:"bodyBytes"`
		BodyTruncated bool   `json:"bodyTruncated"`
	}
	type response struct {
		Status        int   `json:"status"`
		BodyBytes     int64 `json:"bodyBytes"`
		BodyTruncated bool  `json:"bodyTruncated"`
	}
	kept := struct {
		Client     string    `json:"client,omitempty"`
		Request    request   `json:"request"`
		Response   *response `json:"response"`
		DurationMs int64     `json:"durationMs"`
		Attempt    int       `json:"attempt"`
		Failure    *string   `json:"failure,omitempty"`
	}{
		Client:     in.Client,
		Request:    request{Method: in.Request.Method, URL: path, BodyBytes: in.Request.BodyBytes, BodyTruncated: in.Request.BodyTruncated},
		DurationMs: in.DurationMs, Attempt: in.Attempt, Failure: in.Failure,
	}
	if in.Response != nil {
		kept.Response = &response{Status: in.Response.Status, BodyBytes: in.Response.BodyBytes, BodyTruncated: in.Response.BodyTruncated}
		status = in.Response.Status
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return nil, "", "", 0
	}
	return out, in.Request.Method, path, status
}

// urlPathOnly is the path of a URL (scheme, host, user info and query
// dropped); a value that does not parse as a URL is cut at the first ? or #.
func urlPathOnly(raw string) string {
	if u, err := url.Parse(raw); err == nil && (u.Scheme != "" || u.Host != "" || strings.HasPrefix(raw, "/")) {
		return u.EscapedPath()
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

// exchangeTraceSummary keeps an exchange record's counts and outcome.
func exchangeTraceSummary(data []byte) (out []byte, outcome string) {
	var in struct {
		PayloadBytes int64  `json:"payloadBytes"`
		Records      int64  `json:"records"`
		Jobs         int64  `json:"jobs"`
		Outcome      string `json:"outcome"`
	}
	if len(data) == 0 || json.Unmarshal(data, &in) != nil {
		return nil, ""
	}
	out, err := json.Marshal(in)
	if err != nil {
		return nil, ""
	}
	return out, in.Outcome
}

// pollTraceSummary keeps a poll record's data but for its error, and reports
// whether there was an error to withhold.
func pollTraceSummary(data []byte) ([]byte, bool) {
	var in map[string]json.RawMessage
	if len(data) == 0 || json.Unmarshal(data, &in) != nil {
		return nil, false
	}
	_, had := in["error"]
	delete(in, "error")
	out, err := json.Marshal(in)
	if err != nil {
		return nil, had
	}
	return out, had
}
