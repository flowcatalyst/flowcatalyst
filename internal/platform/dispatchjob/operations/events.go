// Package operations holds the dispatch-job human-initiated admin use
// cases: Cancel / Complete (terminal → terminal operator overrides on a
// FAILED job) and Resend (bulk reset-to-PENDING). Per entity.go's package
// doc, these are the only dispatch-job writes that go through the use-case
// envelope — router-driven infra writes (ingest, status transitions from
// delivery) stay direct Repository calls, same as processing.go and
// reaper.go / the settled package.
//
// These three ops close the platform half of T3/A-01: without Cancel and
// Complete an operator has no way to get a FAILED BLOCK_ON_ERROR head out of
// the way (which is what GroupHoldersSQL blocks the rest of the group
// on), and Resend is how the group's siblings actually flow again once the
// head is resolved.
package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	DispatchJobCancelledType     = "platform:messaging:dispatch-job:cancelled"
	DispatchJobCompletedType     = "platform:messaging:dispatch-job:completed"
	DispatchJobPayloadEditedType = "platform:messaging:dispatch-job:payload-edited"
	DispatchJobsResentType       = "platform:messaging:dispatch-jobs:resent"
	Source                       = "platform:messaging"
)

func subjectFor(id string) string { return "platform.dispatchjob." + id }
func groupFor(id string) string   { return "platform:dispatchjob:" + id }

// commonEvent is the shape shared by the two single-resource events
// (Cancelled / Completed). Embedded by the typed events below; they wrap
// with the right EventType.
type commonEvent struct {
	Metadata      usecase.EventMetadata
	DispatchJobID string
}

func (e commonEvent) EventID() string       { return e.Metadata.EventID }
func (e commonEvent) SpecVersion() string   { return "1.0" }
func (e commonEvent) Source() string        { return Source }
func (e commonEvent) Subject() string       { return subjectFor(e.DispatchJobID) }
func (e commonEvent) Time() time.Time       { return e.Metadata.OccurredAt }
func (e commonEvent) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e commonEvent) CorrelationID() string { return e.Metadata.CorrelationID }
func (e commonEvent) CausationID() string   { return e.Metadata.CausationID }
func (e commonEvent) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e commonEvent) MessageGroup() string  { return groupFor(e.DispatchJobID) }
func (e commonEvent) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID string `json:"dispatchJobId"`
	}{e.DispatchJobID})
}

// DispatchJobCancelled is emitted when an operator overrides a FAILED job to
// CANCELLED.
type DispatchJobCancelled struct{ commonEvent }

func (DispatchJobCancelled) EventType() string { return DispatchJobCancelledType }

// DispatchJobCompleted is emitted when an operator overrides a FAILED job to
// COMPLETED (e.g. verified delivered out of band).
type DispatchJobCompleted struct{ commonEvent }

func (DispatchJobCompleted) EventType() string { return DispatchJobCompletedType }

// DispatchJobsResent is the rollup emitted by the bulk Resend operation —
// mirrors scheduledjob's SyncScheduledJobs-style rollup shape (carries the
// affected ids, not just a count) since the wire response reports the
// actual reset count and an operator wants to know which ones.
//
// The event data is only the ids reset. Redrive, Redriven and Skipped are
// not serialized: they are the use case's result for the HTTP response
// bodies; the redrive mark is recorded in the audit row's operation JSON.
type DispatchJobsResent struct {
	Metadata usecase.EventMetadata
	IDs      []string
	Redrive  bool
	Redriven []string
	Skipped  []SkippedJob
}

func (e DispatchJobsResent) EventID() string       { return e.Metadata.EventID }
func (e DispatchJobsResent) EventType() string     { return DispatchJobsResentType }
func (e DispatchJobsResent) SpecVersion() string   { return "1.0" }
func (e DispatchJobsResent) Source() string        { return Source }
func (e DispatchJobsResent) Subject() string       { return "platform.dispatchjobs.resent" }
func (e DispatchJobsResent) Time() time.Time       { return e.Metadata.OccurredAt }
func (e DispatchJobsResent) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e DispatchJobsResent) CorrelationID() string { return e.Metadata.CorrelationID }
func (e DispatchJobsResent) CausationID() string   { return e.Metadata.CausationID }
func (e DispatchJobsResent) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e DispatchJobsResent) MessageGroup() string  { return "platform:dispatchjobs:resent" }
func (e DispatchJobsResent) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IDs []string `json:"ids"`
	}{e.IDs})
}

// DispatchJobPayloadEdited is emitted when an operator rewrites a job's
// payload. The event (and so the audit row) carries sizes and SHA-256 digests
// of the old and new payload, never the payload text: a payload can hold
// personal data, and an audit log is read more widely than the job.
type DispatchJobPayloadEdited struct {
	commonEvent
	Status              string
	PreviousSize        int
	PreviousSHA256      string
	Size                int
	SHA256              string
	PreviousContentType string
	ContentType         string
}

func (DispatchJobPayloadEdited) EventType() string { return DispatchJobPayloadEditedType }
func (e DispatchJobPayloadEdited) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID                  string `json:"dispatchJobId"`
		Status              string `json:"status"`
		PreviousSize        int    `json:"previousSize"`
		PreviousSHA256      string `json:"previousSha256"`
		Size                int    `json:"size"`
		SHA256              string `json:"sha256"`
		PreviousContentType string `json:"previousContentType"`
		ContentType         string `json:"contentType"`
	}{e.DispatchJobID, e.Status, e.PreviousSize, e.PreviousSHA256, e.Size, e.SHA256, e.PreviousContentType, e.ContentType})
}
