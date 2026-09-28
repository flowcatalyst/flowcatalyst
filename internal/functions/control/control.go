// Package control is the wire contract between the platform and function
// runners (docs/function-runner-plan.md §8.3): the desired-state document a
// runner long-polls for, the heartbeat it reports back, and the emit request
// it makes on a function's behalf. The platform serves these routes; the
// runner's Client calls them.
package control

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
)

// Routes, under the platform's base URL. All require a bearer token whose
// principal holds the platform:function-runner role.
const (
	PathDesired   = "/control/functions/desired"   // GET ?pool=&wait=<seconds>; If-None-Match
	PathHeartbeat = "/control/functions/heartbeat" // POST Heartbeat
	PathArtifacts = "/control/functions/artifacts" // GET /{digest}: 302 to storage, or the bytes
	PathEvents    = "/control/functions/events"    // POST EmitRequest → EmitResponse
)

// MaxWait caps the long-poll a runner may ask for.
const MaxWait = 60 * time.Second

// Desired is everything a pool's runners should hold. It contains secrets:
// it is served only to runners and never logged.
type Desired struct {
	Pool     string `json:"pool"`
	Revision int64  `json:"revision"`
	// Auth is how the runner verifies platform bearer tokens on
	// `auth: platform` endpoints.
	Auth      TokenAuth  `json:"auth"`
	Functions []Function `json:"functions"`
}

// TokenAuth names the issuer and audience of the platform's access tokens.
type TokenAuth struct {
	Issuer   string `json:"issuer"`
	Audience string `json:"audience"`
}

// Function is one function the pool runs.
type Function struct {
	ID            string  `json:"id"`
	Address       string  `json:"address"`
	ApplicationID string  `json:"applicationId"`
	ClientID      *string `json:"clientId,omitempty"`
	Limits        Limits  `json:"limits"`
	Warm          bool    `json:"warm"`
	// WebhookSecret verifies `auth: webhook` deliveries: the secret the
	// platform signs this function's subscription and scheduled-job
	// deliveries with.
	WebhookSecret string            `json:"webhookSecret,omitempty"`
	Config        map[string]string `json:"config,omitempty"`
	Secrets       map[string]string `json:"secrets,omitempty"`
	// DB maps a declared database name to its DSN.
	DB       map[string]string `json:"db,omitempty"`
	Versions []Version         `json:"versions"`
}

// Limits are a function's resource limits.
type Limits struct {
	MemoryMB       int   `json:"memoryMb"`
	MaxConcurrency int   `json:"maxConcurrency"`
	TimeoutMs      int64 `json:"timeoutMs"`
	MaxBodyBytes   int64 `json:"maxBodyBytes"`
}

// Version roles in Desired.
const (
	RoleLive      = "live"
	RoleCandidate = "candidate"
	RoleAlias     = "alias:" // + alias name
)

// Version is one version the runner should hold, and why.
type Version struct {
	Number int    `json:"number"`
	Digest string `json:"digest"`
	// Runtime is "wasm" (or empty) for an ABI v1 module, "js" for a script
	// run on the shared JS engine (internal/functions/runtimes).
	Runtime  string          `json:"runtime,omitempty"`
	ABI      int             `json:"abi"`
	Describe json.RawMessage `json:"describe"`
	Roles    []string        `json:"roles"`
}

// Version states a runner reports.
const (
	StateLoaded   = "LOADED"   // instantiated and answering
	StateCompiled = "COMPILED" // compiled and checked, no instance yet (lazy)
	StateFailed   = "FAILED"   // could not be prepared or loaded; Reason says why
)

// Heartbeat is a runner's report to the platform.
type Heartbeat struct {
	RunnerID  string          `json:"runnerId"`
	Pool      string          `json:"pool"`
	StartedAt time.Time       `json:"startedAt"`
	Revision  int64           `json:"revision"` // of the Desired it has applied
	Budget    BudgetReport    `json:"budget"`
	Versions  []VersionReport `json:"versions"`
}

// BudgetReport is the runner's memory picture (plan §7).
type BudgetReport struct {
	budget.Stats
	CompiledBytes int64 `json:"compiledBytes"`
	RSSBytes      int64 `json:"rssBytes,omitempty"`
	Loaded        int   `json:"loaded"`
	Evictions     int64 `json:"evictions"`
}

// VersionReport is one version's state on the runner.
type VersionReport struct {
	FunctionID string `json:"functionId"`
	Number     int    `json:"number"`
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
}

// EmitRequest asks the platform to ingest an event on a function's behalf.
// The platform checks the type is declared in the version's describe
// `emits` and owned by the function's application.
type EmitRequest struct {
	FunctionID string    `json:"functionId"`
	Version    int       `json:"version"`
	Event      abi.Event `json:"event"`
	Data       []byte    `json:"data"` // base64 on the wire
}

// EmitResponse is the platform's answer to an EmitRequest.
type EmitResponse struct {
	EventID string `json:"eventId"`
}
