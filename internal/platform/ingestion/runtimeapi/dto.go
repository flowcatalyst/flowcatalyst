// dto.go contains the wire-format types for the ingestion runtime API.
package runtimeapi

import (
	"encoding/json"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
)

// PollRequest is the wire body for POST /api/ingestion-runtime/v1/poll.
type PollRequest struct {
	RuntimeID       string                  `json:"runtimeId" pattern:"^[A-Za-z0-9._:-]{1,100}$" doc:"The asking runtime; unique among running runtimes"`
	ContractVersion int                     `json:"contractVersion" minimum:"0" doc:"The contract version the runtime speaks; the platform serves the current one and the one before it"`
	Kinds           []ingestion.RuntimeKind `json:"kinds,omitempty" maxItems:"100" doc:"The kinds the runtime's extensions add (and camel-route); built-in kinds are all carried unless some are named here"`
	URL             *string                 `json:"url,omitempty" pattern:"^https?://[^\\s]+$" maxLength:"500" doc:"Where the runtime can be reached; informational, the platform never calls it"`
}

// RuntimeIngestion is an ingestion in the contract's form: what a runtime
// needs to run it. The target names the pool, connection and service account
// by code, as the definition does.
type RuntimeIngestion struct {
	ID      string `json:"id"`
	Version int    `json:"version" doc:"The live version; a runtime rebuilds the route when it differs"`
	// ClientID and ClientIdentifier name the ingestion's client, which is also
	// the client of every job it creates.
	ClientID           string           `json:"clientId"`
	ClientIdentifier   string           `json:"clientIdentifier"`
	ApplicationCode    string           `json:"applicationCode"`
	Code               string           `json:"code"`
	Name               string           `json:"name"`
	Source             json.RawMessage  `json:"source"`
	Split              json.RawMessage  `json:"split"`
	Target             ingestion.Target `json:"target"`
	KeepFailedPayloads bool             `json:"keepFailedPayloads"`
}

// RuntimeTest is a "test connection" request for a runtime to try once.
type RuntimeTest struct {
	ID        string          `json:"id" doc:"Answer it in the next report under tests"`
	ExpiresAt httpcompat.Time `json:"expiresAt"`
	// Ingestion is the source to try. Its id is the saved ingestion's, or
	// ing_test for a definition not yet saved; its version is 0 either way.
	Ingestion RuntimeIngestion `json:"ingestion"`
}

// PollResponse is what a runtime should run and try.
type PollResponse struct {
	AsOf httpcompat.Time `json:"asOf" doc:"When the platform read them"`
	// LeaseFor is how long, counted from when the runtime sent the poll, it may
	// run the polled ingestions listed here without another answer (ISO 8601).
	LeaseFor   string             `json:"leaseFor" example:"PT3M"`
	Ingestions []RuntimeIngestion `json:"ingestions" doc:"Live versions only: every shared ingestion of a kind the runtime carries, and the polled ones whose lease it holds"`
	Tests      []RuntimeTest      `json:"tests" doc:"Pending test requests for kinds the runtime carries"`
}

// StateValue is a value a polled source keeps with the platform, so that it
// survives the ingestion moving to another runtime.
type StateValue struct {
	Value string `json:"value" maxLength:"4000"`
}

// RouteState is the state of one ingestion's route on the runtime.
type RouteState struct {
	IngestionID string  `json:"ingestionId"`
	Version     int     `json:"version" minimum:"0"`
	Status      string  `json:"status" enum:"STARTED,FAILING,FAILED" doc:"STARTED is running, FAILING is running but the source keeps failing, FAILED could not be built or started"`
	Error       *string `json:"error,omitempty" maxLength:"2000"`
}

// ExchangeWindow is the exchange counts of one ingestion over a window. A
// window sent again replaces the counts it sent before.
type ExchangeWindow struct {
	IngestionID  string          `json:"ingestionId"`
	From         httpcompat.Time `json:"from"`
	To           httpcompat.Time `json:"to"`
	Completed    int64           `json:"completed" minimum:"0"`
	Failed       int64           `json:"failed" minimum:"0"`
	Records      int64           `json:"records" minimum:"0"`
	DispatchJobs int64           `json:"dispatchJobs" minimum:"0"`
}

// Failure is one thing that went wrong with an ingestion. The message never
// holds a secret.
type Failure struct {
	IngestionID  string          `json:"ingestionId"`
	Version      int             `json:"version" minimum:"0"`
	At           httpcompat.Time `json:"at"`
	Kind         string          `json:"kind" enum:"REJECTED,NOT_DISPATCHED,SOURCE,AUTH"`
	Message      string          `json:"message" maxLength:"2000"`
	AfterRecords *int64          `json:"afterRecords,omitempty" minimum:"0" doc:"How many of the payload's records were read before it failed"`
	Excerpt      *string         `json:"excerpt,omitempty" maxLength:"2048" doc:"The start of what it failed on; the platform keeps it only for an ingestion that keeps failed payloads"`
}

// TestAnswer is a runtime's answer to a test request from its poll.
type TestAnswer struct {
	ID      string   `json:"id"`
	Outcome string   `json:"outcome" enum:"RESULTS,NO_RESULTS,REFUSED,FAILED,NOT_SUPPORTED"`
	Steps   []string `json:"steps" maxItems:"50" doc:"What happened, step by step, for a person; no secret or token"`
	Records *int64   `json:"records,omitempty" minimum:"0"`
	Excerpt *string  `json:"excerpt,omitempty" maxLength:"2048" doc:"The start of the answer; the platform keeps it only if the tested definition keeps failed payloads"`
}

// ReportRequest is the wire body for POST /api/ingestion-runtime/v1/report:
// the state of every route the runtime runs, the exchange counts gathered since
// its last report, what went wrong, and the answers to its tests.
type ReportRequest struct {
	RuntimeID string           `json:"runtimeId" pattern:"^[A-Za-z0-9._:-]{1,100}$"`
	Routes    []RouteState     `json:"routes,omitempty" maxItems:"2000"`
	Metrics   []ExchangeWindow `json:"metrics,omitempty" maxItems:"2000"`
	Failures  []Failure        `json:"failures,omitempty" maxItems:"500"`
	Tests     []TestAnswer     `json:"tests,omitempty" maxItems:"20"`
}
