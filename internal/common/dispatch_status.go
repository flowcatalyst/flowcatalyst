package common

// DispatchStatus is the lifecycle state of a dispatch job. Wire format
// is SCREAMING_SNAKE_CASE; matches TS and Rust.
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

// ParseDispatchStatus is the lenient parser. Accepts legacy aliases
// (IN_PROGRESS → PROCESSING, ERROR → FAILED). Defaults to PENDING.
func ParseDispatchStatus(s string) DispatchStatus {
	switch s {
	case "PENDING":
		return DispatchPending
	case "QUEUED":
		return DispatchQueued
	case "PROCESSING", "IN_PROGRESS":
		return DispatchProcessing
	case "COMPLETED":
		return DispatchCompleted
	case "FAILED", "ERROR":
		return DispatchFailed
	case "CANCELLED":
		return DispatchCancelled
	case "EXPIRED":
		return DispatchExpired
	default:
		return DispatchPending
	}
}
