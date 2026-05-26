package common

// OutboxStatus stores the integer-coded status used by Java's outbox
// implementation. Wire string is SCREAMING_SNAKE_CASE.
type OutboxStatus int

const (
	OutboxPending       OutboxStatus = 0
	OutboxSuccess       OutboxStatus = 1
	OutboxBadRequest    OutboxStatus = 2
	OutboxInternalError OutboxStatus = 3
	OutboxUnauthorized  OutboxStatus = 4
	OutboxForbidden     OutboxStatus = 5
	OutboxGatewayError  OutboxStatus = 6
	OutboxInProgress    OutboxStatus = 9
)

// Code returns the integer code (for DB storage).
func (s OutboxStatus) Code() int { return int(s) }

// FromOutboxCode maps an integer to OutboxStatus; unknown codes default to PENDING.
func FromOutboxCode(c int) OutboxStatus {
	switch c {
	case 0, 1, 2, 3, 4, 5, 6, 9:
		return OutboxStatus(c)
	default:
		return OutboxPending
	}
}

// IsRetryable reports whether this status will be retried by the outbox processor.
func (s OutboxStatus) IsRetryable() bool {
	switch s {
	case OutboxInternalError, OutboxUnauthorized, OutboxGatewayError, OutboxInProgress:
		return true
	}
	return false
}

// IsTerminal reports whether this status will not be retried.
func (s OutboxStatus) IsTerminal() bool {
	switch s {
	case OutboxSuccess, OutboxBadRequest, OutboxForbidden:
		return true
	}
	return false
}

// String returns the SCREAMING_SNAKE_CASE wire representation.
func (s OutboxStatus) String() string {
	switch s {
	case OutboxPending:
		return "PENDING"
	case OutboxSuccess:
		return "SUCCESS"
	case OutboxBadRequest:
		return "BAD_REQUEST"
	case OutboxInternalError:
		return "INTERNAL_ERROR"
	case OutboxUnauthorized:
		return "UNAUTHORIZED"
	case OutboxForbidden:
		return "FORBIDDEN"
	case OutboxGatewayError:
		return "GATEWAY_ERROR"
	case OutboxInProgress:
		return "IN_PROGRESS"
	default:
		return "PENDING"
	}
}

// OutboxItemType is the kind of payload an outbox row carries.
type OutboxItemType string

const (
	OutboxItemEvent       OutboxItemType = "EVENT"
	OutboxItemDispatchJob OutboxItemType = "DISPATCH_JOB"
	OutboxItemAuditLog    OutboxItemType = "AUDIT_LOG"
)

// AllOutboxItemTypes is the iterable set for type-aware polling loops.
var AllOutboxItemTypes = []OutboxItemType{OutboxItemEvent, OutboxItemDispatchJob, OutboxItemAuditLog}

// APIPath returns the platform endpoint that consumes this item type.
func (t OutboxItemType) APIPath() string {
	switch t {
	case OutboxItemEvent:
		return "/api/events/batch"
	case OutboxItemDispatchJob:
		return "/api/dispatch-jobs/batch"
	case OutboxItemAuditLog:
		return "/api/audit-logs/batch"
	}
	return ""
}

// ParseOutboxItemType accepts case-insensitive plus hyphenated forms.
// Returns ok=false on unknown input.
func ParseOutboxItemType(s string) (OutboxItemType, bool) {
	switch s {
	case "EVENT":
		return OutboxItemEvent, true
	case "DISPATCH_JOB", "DISPATCHJOB", "DISPATCH-JOB":
		return OutboxItemDispatchJob, true
	case "AUDIT_LOG", "AUDITLOG", "AUDIT-LOG":
		return OutboxItemAuditLog, true
	}
	return "", false
}
