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
	// PathDispatchJobs: POST DispatchRequest → DispatchResponse.
	PathDispatchJobs = "/control/functions/dispatch-jobs"
	// PathTraceLog: POST TraceBatch → TraceAck (docs/function-trace-design.md section 5).
	PathTraceLog = "/control/functions/trace-log"
)

// MaxWait caps the long-poll a runner may ask for.
const MaxWait = 60 * time.Second

// ProtocolV1 is the control protocol version this package speaks (the Go v1
// protocol: long-poll desired state with an ETag and revision fence, state
// heartbeats). Advertised in Capabilities.Protocol.
const ProtocolV1 = 1

// QueryRuntimes is the desired-state query parameter naming the runtimes a
// runner can run, comma separated ("wasm,js"). The platform returns only
// versions of those runtimes. Absent or empty means DefaultRuntimes: a runner
// that predates the parameter is a wasm/js runner.
const QueryRuntimes = "runtimes"

// DefaultRuntimes are the runtimes assumed for a runner that does not say.
var DefaultRuntimes = []string{"wasm", "js"}

// Desired is everything a pool's runners should hold. It contains secrets:
// it is served only to runners and never logged.
type Desired struct {
	Pool     string `json:"pool"`
	Revision int64  `json:"revision"`
	// Auth is how the runner verifies platform bearer tokens on
	// `auth: platform` endpoints.
	Auth      TokenAuth  `json:"auth"`
	Functions []Function `json:"functions"`
	// Routes are the pool's public routes: requests on the runner's public
	// entry are matched by Host and path prefix.
	Routes []Route `json:"routes,omitempty"`
}

// Route maps a public hostname and path prefix to a function. The prefix is
// stripped before the function's own endpoint matching.
type Route struct {
	Hostname   string `json:"hostname"`
	PathPrefix string `json:"pathPrefix"`
	Address    string `json:"address"`
	Alias      string `json:"alias,omitempty"` // "" = live
	// AliasPrefixes opts the route into alias-prefixed hostnames: for each
	// prefix p, host "p-<hostname>" serves alias p. Empty = exact host only.
	AliasPrefixes []string `json:"aliasPrefixes,omitempty"`
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
	DB map[string]string `json:"db,omitempty"`
	// Trace lists the function's trace windows that are live when the
	// document is rendered (docs/function-trace-design.md section 4). The
	// runner compares EnabledUntilMs with its own clock on every use.
	Trace    []TraceState `json:"trace,omitempty"`
	Versions []Version    `json:"versions"`
}

// TraceState is one live trace window of a function.
type TraceState struct {
	Context        string `json:"context"`
	EnabledUntilMs int64  `json:"enabledUntilMs"` // unix milliseconds
	SampleRate     int    `json:"sampleRate"`     // capture one invocation in N; 1 = all
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
	// StateRetrying: still PREPARING, retrying a transient failure (control
	// plane 5xx, network, storage). Informational: the platform ignores it
	// (the version stays PUBLISHED); an older platform ignores unknown states
	// the same way. Reason says what is being retried.
	StateRetrying = "RETRYING"
)

// Heartbeat is a runner's report to the platform.
type Heartbeat struct {
	RunnerID  string          `json:"runnerId"`
	Pool      string          `json:"pool"`
	StartedAt time.Time       `json:"startedAt"`
	Revision  int64           `json:"revision"` // of the Desired it has applied
	Budget    BudgetReport    `json:"budget"`
	Versions  []VersionReport `json:"versions"`
	// Capabilities says what this runner can run. Absent from a runner that
	// predates it, which the platform treats as a DefaultRuntimes runner.
	Capabilities *RunnerCapabilities `json:"capabilities,omitempty"`
}

// RunnerCapabilities is what a runner advertises in every heartbeat. The platform
// uses Runtimes to refuse promoting a version no live runner of the pool can
// run (POOL_LACKS_RUNTIME); the rest is informational and stored with the
// heartbeat report.
type RunnerCapabilities struct {
	// Protocol is the control protocol version the runner speaks (ProtocolV1).
	Protocol int `json:"protocol"`
	// Runtimes are the runtime tags the runner can run, and the same set it
	// sends as the desired-state `runtimes` filter.
	Runtimes []string `json:"runtimes"`
	// RunnerKind names the implementation ("go", "java", "echo"...).
	RunnerKind string `json:"runnerKind,omitempty"`
	// Version is the runner's own release version.
	Version string `json:"version,omitempty"`
	// Extra carries runner-specific details (for instance a JVM's version).
	Extra map[string]any `json:"extra,omitempty"`
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

// DispatchRequest asks the platform to create a dispatch job on a function's
// behalf (the `dispatch` host call, ABI op 11). The platform checks the
// (target, path) pair against the invoking version's describe `dispatches`,
// the target's ownership and the rest of the dispatch-job rules; the guest
// never supplies a URL or a credential.
type DispatchRequest struct {
	FunctionID string `json:"functionId"`
	Version    int    `json:"version"`
	// InvocationID is the calling invocation (recorded in the job's
	// metadata). RequestCorrelationID is the inbound request's
	// X-Correlation-Id when it carried one: the job's correlationId unless
	// the guest set its own.
	InvocationID         string       `json:"invocationId,omitempty"`
	RequestCorrelationID string       `json:"requestCorrelationId,omitempty"`
	Dispatch             abi.Dispatch `json:"dispatch"`
	Payload              []byte       `json:"payload"` // base64 on the wire
}

// DispatchResponse is the platform's answer to a DispatchRequest.
type DispatchResponse struct {
	JobID string `json:"jobId"`
}

// Trace record kinds.
const (
	TraceKindTrace = "trace" // written by the function with the trace op
	TraceKindHTTP  = "http"  // the built-in request/response capture
	TraceKindLog   = "log"   // stdout/stderr and log calls during an http-live invocation
)

// Trace record outcomes (TraceHTTPData.Outcome).
const (
	TraceOutcomeOK          = "ok"
	TraceOutcomeTimeout     = "timeout"
	TraceOutcomeTrap        = "trap"
	TraceOutcomeOutOfMemory = "out_of_memory"
	TraceOutcomeBadResponse = "bad_response"
	TraceOutcomeBusy        = "busy"
	TraceOutcomeError       = "error"
)

// TraceRecord is one captured record, as a runner ships it and as the
// platform reads it back (the platform adds the id and creation time).
type TraceRecord struct {
	At            int64           `json:"at"` // unix milliseconds, the runner's clock
	Address       string          `json:"address"`
	Version       int             `json:"version"`
	InvocationID  string          `json:"invocationId"`
	CorrelationID string          `json:"correlationId,omitempty"`
	Reference     string          `json:"reference,omitempty"`
	Context       string          `json:"context"`
	Kind          string          `json:"kind"`
	Level         string          `json:"level"`
	Message       string          `json:"message"`
	Data          json.RawMessage `json:"data,omitempty"`
}

// TraceBatch is the body of POST PathTraceLog. The platform accepts at most
// MaxTraceBatchRecords records and MaxTraceBatchBytes bytes per batch.
type TraceBatch struct {
	RunnerID string        `json:"runnerId"`
	Pool     string        `json:"pool"`
	Records  []TraceRecord `json:"records"`
}

// TraceAck is the answer to a TraceBatch (202).
type TraceAck struct {
	Accepted int `json:"accepted"`
	Dropped  int `json:"dropped"`
}

// Trace limits (docs/function-trace-design.md section 5).
const (
	MaxTraceBatchRecords   = 500
	MaxTraceBatchBytes     = 1 << 20
	MaxTraceMessage        = 4096  // characters; longer is cut
	MaxTraceHTTPData       = 65536 // bytes of a http record's data
	MaxTraceData           = 32768 // bytes of a trace record's data
	MaxTraceRecordsPerSec  = 200   // per function, platform side
	TraceAcceptSlackMillis = 10_000
)

// TraceHTTPData is the data of a TraceKindHTTP record.
type TraceHTTPData struct {
	Request    TraceHTTPRequest  `json:"request"`
	Response   TraceHTTPResponse `json:"response"`
	DurationMs int64             `json:"durationMs"`
	Outcome    string            `json:"outcome"`
}

// TraceHTTPRequest is the captured request.
type TraceHTTPRequest struct {
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	RawQuery      string              `json:"rawQuery,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	Body          *string             `json:"body"`         // null when withheld from the reader
	BodyEncoding  string              `json:"bodyEncoding"` // "utf8" or "base64"
	BodyBytes     int64               `json:"bodyBytes"`    // the real size
	BodyTruncated bool                `json:"bodyTruncated"`
	Caller        map[string]any      `json:"caller,omitempty"` // kind and id
}

// TraceHTTPResponse is the captured response.
type TraceHTTPResponse struct {
	Status        int                 `json:"status"`
	Headers       map[string][]string `json:"headers,omitempty"`
	Body          *string             `json:"body"`
	BodyEncoding  string              `json:"bodyEncoding"`
	BodyBytes     int64               `json:"bodyBytes"`
	BodyTruncated bool                `json:"bodyTruncated"`
}
