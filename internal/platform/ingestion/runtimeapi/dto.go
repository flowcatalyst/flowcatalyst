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
	Kinds           []ingestion.RuntimeKind `json:"kinds,omitempty" maxItems:"100" doc:"The kinds the runtime's extensions add (and camel-route)"`
	// BuiltinKinds is additive within contract version 2: a runtime that does
	// not send it carries the v2 baseline (or, as before, exactly the built-in
	// kinds it names in kinds).
	BuiltinKinds []string `json:"builtinKinds,omitempty" maxItems:"50" doc:"The built-in source kinds the runtime carries (http, sftp, s3, sqs, rabbitmq, database-poll, api-poll); absent is all of them. Only ingestions of a kind the runtime carries are listed to it, and a polled kind's fair share counts only the runtimes that carry it."`
	URL          *string  `json:"url,omitempty" pattern:"^https?://[^\\s]+$" maxLength:"500" doc:"Where the runtime can be reached; informational, the platform never calls it"`
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
	// Traces is additive within contract version 2: an older runtime ignores
	// it. It is a list of its own, not a field of RuntimeIngestion, because a
	// runtime rebuilds a route when its ingestion changes and switching
	// tracing must not restart routes.
	Traces []RuntimeTrace `json:"traces,omitempty" doc:"The open trace windows (hardStop in the future) of the ingestions listed above, for this runtime; absent when none. A windowId seen for the first time starts the runtime's own window of windowSecs, ending no later than hardStop; the same windowId again changes nothing; a different one replaces the window; an ingestion no longer listed has none."`
}

// RuntimeTrace is the trace window of one ingestion a runtime should capture
// under (docs/ingestion-trace-design.md section 2).
type RuntimeTrace struct {
	IngestionID     string          `json:"ingestionId"`
	WindowID        string          `json:"windowId" doc:"A new id every time tracing is switched on"`
	WindowSecs      int             `json:"windowSecs" doc:"How long the runtime captures, counted from when it first sees this windowId"`
	SampleEverySecs int             `json:"sampleEverySecs" doc:"The runtime starts a capture of this ingestion at most this often"`
	HardStop        httpcompat.Time `json:"hardStop" doc:"The runtime never captures past this, and the platform refuses records dated after it (plus a short grace)"`
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
	// Polls and LastPollAt are additive within contract version 2: absent is
	// no poll. A window with polls is reported even when nothing else
	// happened in it, so a polled source that found nothing still shows.
	Polls      int64            `json:"polls,omitempty" minimum:"0" doc:"Polls finished in the window, empty ones included"`
	LastPollAt *httpcompat.Time `json:"lastPollAt,omitempty" doc:"When the last poll in the window ended"`
}

// Failure is one thing that went wrong with an ingestion. The message never
// holds a secret.
type Failure struct {
	IngestionID string          `json:"ingestionId"`
	Version     int             `json:"version" minimum:"0"`
	At          httpcompat.Time `json:"at"`
	Kind        string          `json:"kind" enum:"REJECTED,NOT_DISPATCHED,SOURCE,AUTH"`
	// Category is additive within contract version 2: absent is payload.
	Category     string  `json:"category,omitempty" enum:"payload,rejected" doc:"payload (the default): a record or payload the runtime could not take. rejected: the platform refused the runtime's jobs request permanently; the ingestion shows it as a notice until someone dismisses it."`
	Message      string  `json:"message" maxLength:"2000"`
	AfterRecords *int64  `json:"afterRecords,omitempty" minimum:"0" doc:"How many of the payload's records were read before it failed"`
	Excerpt      *string `json:"excerpt,omitempty" maxLength:"2048" doc:"The start of what it failed on; the platform keeps it only for an ingestion that keeps failed payloads"`
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

// MaxJobRecords is the most records one jobs request carries.
const MaxJobRecords = 1000

// JobRecord is one record to become a dispatch job. It carries only the
// record: where the job goes, what signs it and for which client are the
// ingestion's own target, and a field for any of them is refused.
type JobRecord struct {
	Payload            string  `json:"payload" doc:"The record, as text"`
	PayloadContentType string  `json:"payloadContentType,omitempty" doc:"Defaults to application/json; at most 100 characters"`
	MessageGroup       *string `json:"messageGroup,omitempty" doc:"Orders the jobs of a group within the target's mode; at most 200 characters"`
	ExternalID         *string `json:"externalId,omitempty" doc:"The record's idempotency key within the ingestion, at most 100 characters (the runtime sends the sha256 hex of the record's source identity, 64): sending it again answers the job it made first, and of two records in one request with the same key the first makes the job"`
	CorrelationID      *string `json:"correlationId,omitempty" doc:"At most 100 characters"`
	Subject            *string `json:"subject,omitempty" doc:"At most 500 characters"`
}

// CreateJobsRequest is the wire body for
// POST /api/ingestion-runtime/v1/ingestions/{id}/jobs.
type CreateJobsRequest struct {
	Records []JobRecord `json:"records" maxItems:"1000"`
}

// JobResult is the outcome of the record at Index: the job it is (new or the
// one an earlier request made under the same externalId), or why it was not
// made. A record that failed does not fail the others.
type JobResult struct {
	Index int     `json:"index"`
	ID    string  `json:"id,omitempty"`
	Error *string `json:"error,omitempty"`
	// Existing is true when the externalId had already made this job.
	Existing bool `json:"existing,omitempty"`
}

// CreateJobsResponse answers a jobs request, one result per record in order.
type CreateJobsResponse struct {
	Created int         `json:"created" doc:"Jobs made by this request; records answered with an existing job are not counted"`
	Results []JobResult `json:"results"`
}

// SecretValue is a revealed secret.
type SecretValue struct {
	Value     string          `json:"value"`
	UpdatedAt httpcompat.Time `json:"updatedAt"`
}

// IngestionTraceRecord is one captured record, as a runtime ships it
// (docs/ingestion-trace-design.md section 4). Data is the kind's own document
// (poll, exchange, http or error), already redacted by the runtime.
type IngestionTraceRecord struct {
	// ID is the runtime's id for the record (a random UUID), the same on every
	// retry of its batch, so the platform stores it once.
	ID          string          `json:"id,omitempty"`
	IngestionID string          `json:"ingestionId"`
	Version     int             `json:"version" minimum:"0"`
	WindowID    string          `json:"windowId"`
	CaptureID   string          `json:"captureId" doc:"Shared by the records of one capture"`
	At          int64           `json:"at" doc:"Unix milliseconds, the runtime's clock"`
	Kind        string          `json:"kind" doc:"poll, exchange, http or error"`
	Level       string          `json:"level" doc:"info, warn or error"`
	Message     string          `json:"message" doc:"One line; cut to 1000 characters"`
	Data        json.RawMessage `json:"data,omitempty" doc:"Cut to a stub over 80 KiB"`
}

// IngestionTraceBatch is the body of POST /api/ingestion-runtime/v1/traces: at most 500
// records and 1 MiB.
type IngestionTraceBatch struct {
	RuntimeID string                 `json:"runtimeId" pattern:"^[A-Za-z0-9._:-]{1,100}$"`
	Records   []IngestionTraceRecord `json:"records" maxItems:"500"`
}

// IngestionTraceAck is the answer to a IngestionTraceBatch (202): how many records were stored
// and how many were dropped (outside the ingestion's window, too old, over the
// rate cap or malformed). Dropping is never an error.
type IngestionTraceAck struct {
	Accepted int `json:"accepted"`
	Dropped  int `json:"dropped"`
}
