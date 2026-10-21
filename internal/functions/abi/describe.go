package abi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Endpoint auth modes. Every endpoint names one; there is no default.
const (
	AuthWebhook  = "webhook"
	AuthPlatform = "platform"
	AuthNone     = "none"
)

// Describe is what a module declares it needs (plan §5.4): the document
// fc_describe returns. It is read from the artifact at publish, validated,
// stored with the version, and drives the runner's routing and auth.
type Describe struct {
	ABI           int            `json:"abi"`
	Endpoints     []Endpoint     `json:"endpoints"`
	Subscriptions []Subscription `json:"subscriptions,omitempty"`
	Schedules     []Schedule     `json:"schedules,omitempty"`
	Config        []string       `json:"config,omitempty"`
	Secrets       []string       `json:"secrets,omitempty"`
	DB            []string       `json:"db,omitempty"`
	HTTPAllow     []string       `json:"httpAllow,omitempty"`
	Emits         []string       `json:"emits,omitempty"`
	// Dispatches are the (function, path) webhook targets the function may
	// create dispatch jobs for through the dispatch capability (plan §5.3
	// op 11). Like Emits it is a declaration the platform checks per call.
	Dispatches []DispatchTarget `json:"dispatches,omitempty"`
	// TraceContexts are the named trace points the function writes to with
	// the trace op; an operator switches each on for a bounded time
	// (docs/function-trace-design.md section 3). The built-in "http" context
	// is not declared.
	TraceContexts []TraceContext `json:"traceContexts,omitempty"`
}

// TraceContext declares one trace context.
type TraceContext struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Trace context limits.
const (
	MaxTraceContexts          = 20
	MaxTraceContextDescLength = 200
)

// HasTraceContext reports whether the function declares the context.
func (d *Describe) HasTraceContext(name string) bool {
	for _, c := range d.TraceContexts {
		if c.Name == name {
			return true
		}
	}
	return false
}

// DispatchTarget is one declared dispatch destination: the address of a
// function of the SAME application, and a literal path on it that lands on
// one of its webhook endpoints. The platform builds the delivery URL from
// it; the guest never supplies a URL.
//
// Pool, when set, is the CODE of the dispatch pool (the rate/concurrency pool
// jobs run through; not a function-runner pool) every job for this entry is
// placed in. It is the declaration's, never the call's: the guest cannot name
// a pool at call time. The platform resolves it per call (existence, status,
// client, application ownership). Absent means the client's default pool.
type DispatchTarget struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Pool   string `json:"pool,omitempty"`
	// Connection is the CODE of the connection the entry's jobs are delivered
	// through. REQUIRED (Validate refuses an entry without one): the job
	// carries it, so pausing the connection (a system down for maintenance)
	// holds the job PENDING, and the connection's service account signs the
	// delivery. Like Pool it is the declaration's, never the call's, and
	// resolved by the platform per call.
	Connection string `json:"connection,omitempty"`
}

// DispatchFor returns the dispatches entry for (target, path), whatever its
// pool. A pool is part of the declaration, not of its identity: two entries
// for one (target, path) are refused by Validate.
func (d *Describe) DispatchFor(target, path string) (DispatchTarget, bool) {
	for _, t := range d.Dispatches {
		if t.Target == target && t.Path == path {
			return t, true
		}
	}
	return DispatchTarget{}, false
}

// Endpoint is one path the function answers, and how its callers authenticate.
type Endpoint struct {
	Method       string `json:"method,omitempty"`
	Path         string `json:"path"`
	Auth         string `json:"auth"`
	MaxBodyBytes *int64 `json:"maxBodyBytes,omitempty"`
	TimeoutMs    *int64 `json:"timeoutMs,omitempty"`
	CORS         *CORS  `json:"cors,omitempty"`
}

// CORS is an endpoint's cross-origin policy; absent means the runner does
// nothing CORS-related for the endpoint.
type CORS struct {
	Origins          []string `json:"origins"`
	Methods          []string `json:"methods,omitempty"`
	Headers          []string `json:"headers,omitempty"`
	AllowCredentials bool     `json:"allowCredentials,omitempty"`
}

// Subscription asks for an event type to be delivered to a webhook endpoint.
type Subscription struct {
	EventType      string `json:"eventType"`
	Path           string `json:"path"`
	Mode           string `json:"mode,omitempty"`
	MaxRetries     *int   `json:"maxRetries,omitempty"`
	DataOnly       bool   `json:"dataOnly,omitempty"`
	TimeoutSeconds *int   `json:"timeoutSeconds,omitempty"`
}

// Schedule asks for a cron firing to be delivered to a webhook endpoint.
type Schedule struct {
	Cron     string          `json:"cron"`
	Timezone string          `json:"timezone,omitempty"`
	Path     string          `json:"path"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// Pattern is the endpoint's net/http ServeMux pattern.
func (e Endpoint) Pattern() string {
	if e.Method == "" {
		return e.Path
	}
	return e.Method + " " + e.Path
}

var (
	keyPattern    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_./-]{0,99}$`)
	dbNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	// A trace context name: lowercase, digits and dashes, at most 40.
	traceContextPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	hostPattern         = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]{1,5})?$`)
	// The platform's own rule for event-type codes: four non-empty
	// colon-separated segments (eventtype.New). Ownership — the first
	// segment is the function's application — is publish's check.
	eventTypePattern = regexp.MustCompile(`^[^:\s]+(:[^:\s]+){3}$`)
	// A dispatch pool code, as the platform stores it: the pool create/sync
	// rule (lowercase, underscore-tolerant) plus a sanity length. Existence
	// is the runtime's check; pools may be created after the publish.
	poolCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,99}$`)
	// A connection code, as the platform stores it (connection create/sync:
	// validate.CodePattern) plus a sanity length. Existence, status, client
	// and application scope are the runtime's checks.
	connectionCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,99}$`)
	methodPattern         = regexp.MustCompile(`^[A-Z]{3,10}$`)
	// A function address is {application code}.{name} (owner decision
	// §13.1). The application-code half is as loose as the platform's own
	// (publish checks it against the function's application); the name half
	// is function.NamePattern.
	addressPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,99}\.[a-z][a-z0-9-]{0,62}$`)
)

// ParseDescribe decodes a describe document strictly (unknown keys are an
// error) and validates it.
func ParseDescribe(b []byte) (*Describe, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var d Describe
	if err := dec.Decode(&d); err != nil {
		return nil, &DescribeError{Problems: []string{"not a describe document: " + err.Error()}}
	}
	if dec.More() {
		return nil, &DescribeError{Problems: []string{"trailing data after the describe document"}}
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// DescribeError lists every problem found, so a publisher fixes them in one go.
type DescribeError struct {
	Problems []string
}

func (e *DescribeError) Error() string {
	return "invalid describe document: " + strings.Join(e.Problems, "; ")
}

// Validate checks the document against the rules of plan §5.4. It does not
// check what needs the platform (event-type ownership, cron syntax); publish
// does that.
func (d *Describe) Validate() error {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	if d.ABI != Version {
		add("abi: %d is not supported (want %d)", d.ABI, Version)
	}
	if len(d.Endpoints) == 0 {
		add("endpoints: at least one endpoint is required")
	}
	for i, e := range d.Endpoints {
		at := fmt.Sprintf("endpoints[%d]", i)
		switch e.Auth {
		case AuthWebhook:
			if e.Method != "" && e.Method != http.MethodPost {
				add("%s: a webhook endpoint accepts POST only, not %s", at, e.Method)
			}
		case AuthPlatform, AuthNone:
		case "":
			add("%s: auth is required (webhook, platform or none)", at)
		default:
			add("%s: auth %q is not webhook, platform or none", at, e.Auth)
		}
		if e.Method != "" && !methodPattern.MatchString(e.Method) {
			add("%s: method %q is not an upper-case HTTP method", at, e.Method)
		}
		if !strings.HasPrefix(e.Path, "/") {
			add("%s: path %q must start with /", at, e.Path)
		}
		if e.MaxBodyBytes != nil && *e.MaxBodyBytes < 0 {
			add("%s: maxBodyBytes must not be negative", at)
		}
		if e.TimeoutMs != nil && *e.TimeoutMs <= 0 {
			add("%s: timeoutMs must be positive", at)
		}
		if e.CORS != nil && len(e.CORS.Origins) == 0 {
			add("%s: cors needs at least one origin", at)
		}
		if e.CORS != nil && e.CORS.AllowCredentials && slices.Contains(e.CORS.Origins, "*") {
			add("%s: cors cannot combine origin \"*\" with allowCredentials; list the origins", at)
		}
	}
	// Only build the router over endpoints that passed the checks above; a
	// malformed or conflicting pattern is reported, not panicked.
	var router *Router
	if len(p) == 0 {
		r, err := NewRouter(d.Endpoints)
		if err != nil {
			add("endpoints: %v", err)
		} else {
			router = r
		}
	}
	for i, s := range d.Subscriptions {
		at := fmt.Sprintf("subscriptions[%d]", i)
		if !eventTypePattern.MatchString(s.EventType) {
			add("%s: eventType %q is not app:domain:aggregate:event", at, s.EventType)
		}
		if s.Mode != "" {
			switch common.DispatchMode(s.Mode) {
			case common.DispatchImmediate, common.DispatchNextOnError, common.DispatchBlockOnError:
			default:
				add("%s: mode %q is not IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR", at, s.Mode)
			}
		}
		if s.MaxRetries != nil && *s.MaxRetries < 0 {
			add("%s: maxRetries must not be negative", at)
		}
		if s.TimeoutSeconds != nil && *s.TimeoutSeconds <= 0 {
			add("%s: timeoutSeconds must be positive", at)
		}
		if msg := webhookTarget(router, s.Path); msg != "" {
			add("%s: %s", at, msg)
		}
	}
	for i, s := range d.Schedules {
		at := fmt.Sprintf("schedules[%d]", i)
		if strings.TrimSpace(s.Cron) == "" {
			add("%s: cron is required", at)
		}
		if len(s.Payload) > 0 && !json.Valid(s.Payload) {
			add("%s: payload is not valid JSON", at)
		}
		if msg := webhookTarget(router, s.Path); msg != "" {
			add("%s: %s", at, msg)
		}
	}
	checkList := func(name string, items []string, re *regexp.Regexp) {
		seen := map[string]bool{}
		for _, k := range items {
			if !re.MatchString(k) {
				add("%s: %q is not a valid entry", name, k)
			}
			if seen[k] {
				add("%s: %q is declared twice", name, k)
			}
			seen[k] = true
		}
	}
	checkList("config", d.Config, keyPattern)
	checkList("secrets", d.Secrets, keyPattern)
	checkList("db", d.DB, dbNamePattern)
	checkList("httpAllow", d.HTTPAllow, hostPattern)
	for _, h := range d.HTTPAllow {
		if hostPattern.MatchString(h) {
			if why := hostPatternProblem(h); why != "" {
				add("httpAllow: %q %s", h, why)
			}
		}
	}
	checkList("emits", d.Emits, eventTypePattern)
	if len(d.TraceContexts) > MaxTraceContexts {
		add("traceContexts: at most %d contexts, got %d", MaxTraceContexts, len(d.TraceContexts))
	}
	seenTrace := map[string]bool{}
	for i, c := range d.TraceContexts {
		at := fmt.Sprintf("traceContexts[%d]", i)
		switch {
		case c.Name == TraceHTTP:
			add("%s: %q is the built-in capture context and cannot be declared", at, c.Name)
		case !traceContextPattern.MatchString(c.Name):
			add("%s: %q is not a valid context name (lowercase letter first, then lowercase letters, digits or -; at most 40 characters)", at, c.Name)
		}
		if len(c.Description) > MaxTraceContextDescLength {
			add("%s: description is over %d characters", at, MaxTraceContextDescLength)
		}
		if seenTrace[c.Name] {
			add("%s: %q is declared twice", at, c.Name)
		}
		seenTrace[c.Name] = true
	}
	seenDispatch := map[[2]string]bool{}
	for i, t := range d.Dispatches {
		at := fmt.Sprintf("dispatches[%d]", i)
		if !addressPattern.MatchString(t.Target) {
			add("%s: target %q is not a function address (application.name)", at, t.Target)
		}
		if msg := DispatchPathProblem(t.Path); msg != "" {
			add("%s: %s", at, msg)
		}
		if t.Pool != "" && !poolCodePattern.MatchString(t.Pool) {
			add("%s: pool %q is not a dispatch pool code (lowercase letter first, then lowercase letters, digits, - or _; at most 100 characters)", at, t.Pool)
		}
		if t.Connection == "" {
			add("%s: %s%s declares no connection; a dispatch must name the connection its jobs go through (so pausing it holds them)", at, t.Target, t.Path)
		} else if !connectionCodePattern.MatchString(t.Connection) {
			add("%s: connection %q is not a connection code (lowercase letter first, then lowercase letters, digits or -; at most 100 characters)", at, t.Connection)
		}
		key := [2]string{t.Target, t.Path}
		if seenDispatch[key] {
			add("%s: %s%s is declared twice", at, t.Target, t.Path)
		}
		seenDispatch[key] = true
	}

	if len(p) > 0 {
		return &DescribeError{Problems: p}
	}
	return nil
}

// DispatchPathProblem reports why path cannot be a dispatch target path, or
// "": it must be a literal absolute path — no pattern syntax, query,
// fragment, whitespace or dot segments — so the URL the platform builds is
// exactly the path that was declared.
func DispatchPathProblem(path string) string {
	switch {
	case !strings.HasPrefix(path, "/"):
		return fmt.Sprintf("path %q must start with /", path)
	case strings.ContainsAny(path, "{}*?#\\ \t\r\n%") || strings.Contains(path, "//"):
		return fmt.Sprintf("path %q must be a literal path (no pattern, query, fragment, escape or empty segment)", path)
	case slices.Contains(strings.Split(path, "/"), ".") || slices.Contains(strings.Split(path, "/"), ".."):
		return fmt.Sprintf("path %q must not contain dot segments", path)
	}
	return ""
}

// hostPatternProblem reports why a syntactically valid httpAllow entry is
// still unacceptable, or "". IP literals and localhost are refused (the
// runner's dial guard blocks those destinations anyway; failing at publish is
// clearer), and a wildcard must sit above at least two labels so `*.com` or
// `*.co.uk`-style sweeps cannot be declared.
func hostPatternProblem(entry string) string {
	host, _, _ := strings.Cut(entry, ":")
	wild := false
	if rest, ok := strings.CutPrefix(host, "*."); ok {
		host, wild = rest, true
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return "is an IP address; declare a host name"
	}
	labels := strings.Split(host, ".")
	if last := labels[len(labels)-1]; strings.Trim(last, "0123456789") == "" {
		return "looks like an IP address; declare a host name"
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "is a loopback name"
	}
	if wild && len(labels) < 2 {
		return "is too broad; a wildcard needs at least two labels after *."
	}
	return ""
}

// webhookTarget reports why path cannot receive deliveries, or "" when it can:
// it must be literal and a POST to it must land on a webhook endpoint.
func webhookTarget(r *Router, path string) string {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "{}*") {
		return fmt.Sprintf("path %q must be a literal path", path)
	}
	if r == nil {
		return "" // endpoint problems already reported
	}
	m, err := r.Match(http.MethodPost, path)
	if err != nil {
		return fmt.Sprintf("path %q matches no endpoint accepting POST", path)
	}
	if m.Endpoint.Auth != AuthWebhook {
		return fmt.Sprintf("path %q lands on endpoint %q whose auth is %s, not webhook", path, m.Endpoint.Pattern(), m.Endpoint.Auth)
	}
	return ""
}

// ErrNoRoute and ErrMethodNotAllowed are Router.Match's misses.
var (
	ErrNoRoute          = errors.New("abi: no endpoint matches the path")
	ErrMethodNotAllowed = errors.New("abi: the path's endpoint does not accept the method")
)
