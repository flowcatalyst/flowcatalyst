package common

// DispatchStatus is the lifecycle state of a dispatch job. Wire format
// is SCREAMING_SNAKE_CASE; matches the TypeScript SDK.
type DispatchStatus string

const (
	DispatchPending    DispatchStatus = "PENDING"
	DispatchQueued     DispatchStatus = "QUEUED"
	DispatchProcessing DispatchStatus = "PROCESSING"
	DispatchCompleted  DispatchStatus = "COMPLETED"
	DispatchFailed     DispatchStatus = "FAILED"
	DispatchCancelled  DispatchStatus = "CANCELLED"
	DispatchExpired    DispatchStatus = "EXPIRED"
)

// IsTerminal reports whether a status will not change further.
func (s DispatchStatus) IsTerminal() bool {
	switch s {
	case DispatchCompleted, DispatchFailed, DispatchCancelled, DispatchExpired:
		return true
	}
	return false
}

// IsSuccessful reports whether the status is the success terminal.
func (s DispatchStatus) IsSuccessful() bool { return s == DispatchCompleted }

// ParseDispatchStatus parses a stored/wire status value. IN_PROGRESS and
// ERROR are accepted legacy aliases of PROCESSING and FAILED — not just
// wire input, but real values `msg_dispatch_jobs.status` has held (see
// dispatchjob.GroupHoldingStatusSQL, which still matches 'ERROR' by design
// so old rows keep blocking as they always did). Returns ok=false for
// anything else — callers MUST reject on ok=false rather than coerce an
// unrecognised value to PENDING (X-06: a loud read error, never a silent
// default — a corrupted terminal status silently reappearing as PENDING
// could resurrect a job that already completed or failed). Follows the
// (T, bool) shape of ParseOutboxItemType.
func ParseDispatchStatus(s string) (DispatchStatus, bool) {
	switch s {
	case "PENDING":
		return DispatchPending, true
	case "QUEUED":
		return DispatchQueued, true
	case "PROCESSING", "IN_PROGRESS":
		return DispatchProcessing, true
	case "COMPLETED":
		return DispatchCompleted, true
	case "FAILED", "ERROR":
		return DispatchFailed, true
	case "CANCELLED":
		return DispatchCancelled, true
	case "EXPIRED":
		return DispatchExpired, true
	default:
		return "", false
	}
}
