package abi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
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
	keyPattern       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_./-]{0,99}$`)
	dbNamePattern    = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	hostPattern      = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]{1,5})?$`)
	// The platform's own rule for event-type codes: four non-empty
	// colon-separated segments (eventtype.New). Ownership — the first
	// segment is the function's application — is publish's check.
	eventTypePattern = regexp.MustCompile(`^[^:\s]+(:[^:\s]+){3}$`)
	methodPattern    = regexp.MustCompile(`^[A-Z]{3,10}$`)
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
	checkList("emits", d.Emits, eventTypePattern)

	if len(p) > 0 {
		return &DescribeError{Problems: p}
	}
	return nil
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
