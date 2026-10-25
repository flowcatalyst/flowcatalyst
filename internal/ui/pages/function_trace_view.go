package pages

import (
	"bytes"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The trace log's filters live in the URL (/ui/functions/{id}/traces?kind=http
// &reference=TRIP-1), as the other lists' state does.

// Filter choices (the SPA's KIND_CHOICES, LEVEL_CHOICES, SINCE_CHOICES).
var (
	traceKinds  = []string{"trace", "http", "log"}
	traceLevels = []string{"debug", "info", "warn", "error"}
	// traceSinceChoices are "last N minutes"; the empty value is any time.
	traceSinceChoices = []kit.Option{
		{Value: "", Label: "Any time"},
		{Value: "5", Label: "Last 5 minutes"},
		{Value: "60", Label: "Last hour"},
		{Value: "1440", Label: "Last 24 hours"},
	}
)

type traceQuery struct {
	Context, Kind, Level, Since            string
	Reference, CorrelationID, InvocationID string
	Paused                                 bool
}

// parseTraceQuery reads the filters; anything unknown is no filter (a
// hand-edited URL still shows a log). The pause button and the resume button
// are the form's submitter: they override the carried state.
func parseTraceQuery(v url.Values) traceQuery {
	q := traceQuery{
		Context:       v.Get("context"),
		Reference:     traceText(v.Get("reference")),
		CorrelationID: traceText(v.Get("correlationId")),
		InvocationID:  traceText(v.Get("invocationId")),
		Paused:        v.Get("paused") == "1",
	}
	if q.Context != function.TraceHTTPContext && !function.TraceContextPattern.MatchString(q.Context) {
		q.Context = ""
	}
	if k := v.Get("kind"); slices.Contains(traceKinds, k) {
		q.Kind = k
	}
	if l := v.Get("level"); slices.Contains(traceLevels, l) {
		q.Level = l
	}
	if s := v.Get("since"); slices.ContainsFunc(traceSinceChoices, func(o kit.Option) bool { return o.Value == s }) {
		q.Since = s
	}
	if v.Get("pause") == "1" {
		q.Paused = true
	}
	if v.Get("resume") == "1" {
		q.Paused = false
	}
	return q
}

func traceText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxQueryLen {
		s = s[:maxQueryLen]
	}
	return s
}

// filtered reports whether a filter narrows the log (not the pause state).
func (q traceQuery) filtered() bool {
	return q.Context != "" || q.Kind != "" || q.Level != "" || q.Since != "" ||
		q.Reference != "" || q.CorrelationID != "" || q.InvocationID != ""
}

func (q traceQuery) values() url.Values {
	v := url.Values{}
	for k, val := range map[string]string{
		"context": q.Context, "kind": q.Kind, "level": q.Level, "since": q.Since,
		"reference": q.Reference, "correlationId": q.CorrelationID, "invocationId": q.InvocationID,
	} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if q.Paused {
		v.Set("paused", "1")
	}
	return v
}

// URL is the canonical address of this view of function fnID's log.
func (q traceQuery) URL(fnID string) string {
	if enc := q.values().Encode(); enc != "" {
		return traceLogPath(fnID) + "?" + enc
	}
	return traceLogPath(fnID)
}

// filter is the repository filter: the SPA's buildTraceQuery (blank fields
// left out; "since" is an instant computed from the quick choice at now).
func (q traceQuery) filter(fnID string, now time.Time) function.TraceLogFilter {
	f := function.TraceLogFilter{
		FunctionID: fnID, Context: q.Context, Kind: q.Kind, Level: q.Level,
		Reference: q.Reference, CorrelationID: q.CorrelationID, InvocationID: q.InvocationID,
	}
	if mins, err := strconv.Atoi(q.Since); err == nil && mins > 0 {
		t := now.Add(-time.Duration(mins) * time.Minute).UTC()
		f.Since = &t
	}
	return f
}

// moreURL is the Datastar address of the next page of this view.
func (q traceQuery) moreURL(fnID, cursor string) string {
	v := q.values()
	v.Del("paused")
	v.Set("cursor", cursor)
	return traceLogPath(fnID) + "/more?" + v.Encode()
}

// pageURL is the same page for a browser without Datastar.
func (q traceQuery) pageURL(fnID, cursor string) string {
	v := q.values()
	v.Set("cursor", cursor)
	return traceLogPath(fnID) + "?" + v.Encode()
}

// ── records ────────────────────────────────────────────────────────────

// traceRec is one record, ready to render.
type traceRec struct {
	DomID, ID       string
	At              time.Time
	Context, Kind   string
	Level           string
	Message         string
	Version         int32
	RunnerID        string
	Reference       string
	CorrelationID   string
	CorrelationText string
	InvocationID    string
	InvocationText  string
	// Count is the records of this invocation in the page.
	Count int
	// HTTP is set for an http record of the expected shape; Data is the
	// pretty-printed data of any other record.
	HTTP *traceHTTP
	Data string
	// Redacted is a record whose sensitive parts (an http record's headers,
	// query string and bodies, a log record's line, a trace record's data)
	// the viewer may not see.
	Redacted bool
}

// traceDetailHidden is shown on a redacted log or trace record.
const traceDetailHidden = "Details hidden: you do not have permission to view what the function logged (platform:function:trace:view-bodies)"

type traceHTTP struct {
	Summary string
	Panels  []tracePanel
}

type tracePanel struct {
	Title, Line string
	Headers     [][2]string
	Body        traceBody
	// BodyID is the element the copy button reads.
	BodyID string
}

// traceBody is how a body is shown: state "hidden" (the viewer lacks
// view-bodies), "empty" (none captured) or "shown".
type traceBody struct {
	State, Text, Label, Note string
}

// traceBodiesHidden is the SPA's BODIES_HIDDEN_MESSAGE.
const traceBodiesHidden = "Bodies, headers and query string hidden: you do not have permission to view them"

func traceRecords(recs []function.TraceLogRecord, bodies bool) []traceRec {
	counts := map[string]int{}
	for _, r := range recs {
		counts[r.InvocationID]++
	}
	out := make([]traceRec, 0, len(recs))
	for _, r := range recs {
		out = append(out, traceRecOf(r, bodies, counts[r.InvocationID]))
	}
	return out
}

func traceRecOf(r function.TraceLogRecord, bodies bool, count int) traceRec {
	rec := traceRec{
		DomID: "trace-rec-" + traceSafeID(r.ID), ID: r.ID, At: r.At, Context: r.Context, Kind: r.Kind, Level: r.Level,
		Message: r.Message, Version: r.Version, RunnerID: deref(r.RunnerID), Reference: deref(r.Reference),
		CorrelationID: deref(r.CorrelationID), InvocationID: r.InvocationID, Count: count,
	}
	rec.CorrelationText, rec.InvocationText = traceShortID(rec.CorrelationID), traceShortID(r.InvocationID)
	if !bodies {
		// The API's rule: without view-bodies an http record keeps only its
		// method, path, status, timings and sizes, a log record only its
		// metadata, a trace record its message (function.WithholdTraceDetail).
		rec.Redacted = function.WithholdTraceDetail(&r)
		rec.Message = r.Message
	}
	if r.Kind == "http" {
		// An http record is never printed raw: its data holds the bodies.
		if h := parseTraceHTTP(r.Data, r.ID, bodies); h != nil {
			rec.HTTP = h
		}
		return rec
	}
	rec.Data = traceDataText(r.Data)
	return rec
}

// traceSafeID keeps a DOM id to [A-Za-z0-9_-] (ids come from the platform and
// already are; this is for the page's sake, not for them).
func traceSafeID(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
}

// traceShortID is the SPA's shortId: the table cell shows the ends, the full
// id is the tooltip and the filter value.
func traceShortID(id string) string {
	r := []rune(id)
	if len(r) > 12 {
		return string(r[:8]) + "…" + string(r[len(r)-3:])
	}
	return id
}

// traceHeaderValue reads a header value that is a string or a list of them.
type traceHeaderValue []string

func (h *traceHeaderValue) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*h = traceHeaderValue{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*h = many
	return nil
}

type traceHTTPPart struct {
	Method        string                      `json:"method"`
	Path          string                      `json:"path"`
	RawQuery      string                      `json:"rawQuery"`
	Status        *int                        `json:"status"`
	Headers       map[string]traceHeaderValue `json:"headers"`
	Body          *string                     `json:"body"`
	BodyEncoding  string                      `json:"bodyEncoding"`
	BodyBytes     *int64                      `json:"bodyBytes"`
	BodyTruncated bool                        `json:"bodyTruncated"`
}

// parseTraceHTTP reads the data of an http record (request, response,
// durationMs, outcome) into its panels. It is nil when the data is not that
// shape. Without bodies the body fields are not even kept.
func parseTraceHTTP(data []byte, recID string, bodies bool) *traceHTTP {
	var d struct {
		Request    *traceHTTPPart `json:"request"`
		Response   *traceHTTPPart `json:"response"`
		DurationMs *float64       `json:"durationMs"`
		Outcome    string         `json:"outcome"`
	}
	if json.Unmarshal(data, &d) != nil || d.Request == nil || d.Response == nil {
		return nil
	}
	if !bodies {
		d.Request.Body, d.Response.Body = nil, nil
	}
	req, res := d.Request, d.Response
	query := ""
	if req.RawQuery != "" {
		query = "?" + req.RawQuery
	}
	h := &traceHTTP{Summary: traceHTTPSummary(req, res, d.DurationMs, d.Outcome)}
	reqLine := strings.TrimSpace(req.Method + " " + req.Path + query)
	resLine := ""
	if res.Status != nil {
		resLine = "Status " + strconv.Itoa(*res.Status)
	}
	base := "trace-body-" + traceSafeID(recID)
	h.Panels = []tracePanel{
		{Title: "Request", Line: reqLine, Headers: traceHeaderRows(req.Headers), Body: traceBodyOf(req, !bodies), BodyID: base + "-req"},
		{Title: "Response", Line: resLine, Headers: traceHeaderRows(res.Headers), Body: traceBodyOf(res, !bodies), BodyID: base + "-res"},
	}
	return h
}

// traceHTTPSummary is the headline of an http record: "POST /orders → 200 ·
// 12 ms · ok".
func traceHTTPSummary(req, res *traceHTTPPart, durationMs *float64, outcome string) string {
	method := req.Method
	if method == "" {
		method = "?"
	}
	query := ""
	if req.RawQuery != "" {
		query = "?" + req.RawQuery
	}
	head := strings.TrimSpace(method + " " + req.Path + query)
	if res.Status != nil {
		head += " → " + strconv.Itoa(*res.Status)
	}
	parts := []string{head}
	if durationMs != nil {
		parts = append(parts, strconv.FormatFloat(*durationMs, 'f', -1, 64)+" ms")
	}
	if outcome != "" {
		parts = append(parts, outcome)
	}
	return strings.Join(parts, " · ")
}

// traceHeaderRows is the header map as rows sorted by name; a header with
// several values joins them with ", ".
func traceHeaderRows(h map[string]traceHeaderValue) [][2]string {
	rows := make([][2]string, 0, len(h))
	for k, v := range h {
		rows = append(rows, [2]string{k, strings.Join(v, ", ")})
	}
	slices.SortFunc(rows, func(a, b [2]string) int { return strings.Compare(strings.ToLower(a[0]), strings.ToLower(b[0])) })
	return rows
}

// traceBodyOf is the SPA's bodyView: pretty JSON, labelled base64, the
// truncation note, and the hidden state for a viewer without view-bodies.
func traceBodyOf(p *traceHTTPPart, redacted bool) traceBody {
	if redacted {
		return traceBody{State: "hidden"}
	}
	note := ""
	if p.BodyTruncated {
		note = "truncated"
		if p.BodyBytes != nil {
			note = "truncated (" + thousands(*p.BodyBytes) + " bytes total)"
		}
	}
	if p.Body == nil || *p.Body == "" {
		return traceBody{State: "empty", Note: note}
	}
	if p.BodyEncoding == "base64" {
		return traceBody{State: "shown", Text: *p.Body, Label: "base64", Note: note}
	}
	// A truncated capture cuts the document mid-way, so it will not parse: shown raw.
	if !p.BodyTruncated && json.Valid([]byte(*p.Body)) {
		var pretty bytes.Buffer
		if json.Indent(&pretty, []byte(*p.Body), "", "  ") == nil {
			return traceBody{State: "shown", Text: pretty.String(), Label: "JSON", Note: note}
		}
	}
	return traceBody{State: "shown", Text: *p.Body, Label: "text", Note: note}
}

// traceDataText is a trace or log record's data as pretty JSON; empty when
// there is none.
func traceDataText(data []byte) string {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return ""
	}
	var s string
	if data[0] == '"' && json.Unmarshal(data, &s) == nil {
		return s
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, data, "", "  ") != nil {
		return string(data)
	}
	return pretty.String()
}

// traceLevelSeverity is the SPA's levelSeverity.
func traceLevelSeverity(level string) kit.TagSeverity {
	switch level {
	case "error":
		return kit.TagDanger
	case "warn":
		return kit.TagWarn
	case "info":
		return kit.TagInfo
	}
	return kit.TagSecondary
}

func (r traceRec) headline() string {
	if r.HTTP != nil {
		return r.HTTP.Summary
	}
	if r.Message == "" && r.Redacted {
		return "(hidden)"
	}
	return r.Message
}

func (r traceRec) versionText() string { return "v" + strconv.Itoa(int(r.Version)) }

func (r traceRec) atText() string { return r.At.UTC().Format(time.RFC3339Nano) }

func (r traceRec) dataID() string { return "trace-data-" + traceSafeID(r.ID) }

func (r traceRec) countText() string { return strconv.Itoa(max(r.Count, 1)) }

func (r traceRec) countTitle() string { return r.countText() + " records in this view" }

// contextOptions are the context filter's options: the function's contexts,
// and the chosen one when it is not among them (a URL never silently loses a
// filter).
func (v traceLogView) contextOptions() []kit.Option {
	var chosen []string
	if v.Q.Context != "" {
		chosen = []string{v.Q.Context}
	}
	return withChosen(plainOptions(v.Contexts), chosen, nil)
}

// switchesWithDialog is the switches view with the enable dialog in its slot
// (a navigation to the dialog's address renders the page with it open).
func (v traceLogView) switchesWithDialog() TraceSwitchesView {
	s := v.Switches
	s.dialog = v.Dialog
	return s
}

func (f traceForm) maxText() string { return strconv.Itoa(f.MaxMinutes) }

// traceEnableHref is the enable dialog's address; the log page adds page=traces
// so that a post comes back to it.
func traceEnableHref(fnID, name string, onPage bool) string {
	href := traceContextPath(fnID, name) + "/enable"
	if onPage {
		href += "?page=traces"
	}
	return href
}
