// Package outbox implements the Outbox Pattern for reliable message publishing.
// It polls customer databases for pending outbox items and sends them to FlowCatalyst APIs.
//
// Architecture (single-poller, status-based):
//  1. Single poller fetches items WHERE status = 0 (PENDING)
//  2. Items are marked status = 9 (IN_PROGRESS) before buffering
//  3. Distributor routes items to message group processors (maintains FIFO per group)
//  4. On completion, status is updated to reflect outcome (1=success, 2-6=error types)
//  5. Crash recovery: on startup, reset status = 9 back to 0
//
// This approach avoids row locking (FOR UPDATE SKIP LOCKED) and works
// identically across PostgreSQL, MySQL, and MongoDB.
package outbox

import (
	"time"
)

// OutboxStatus represents the processing status of an outbox item.
// Using integers for efficient storage and cross-database compatibility.
type OutboxStatus int

const (
	// StatusPending - item is waiting to be processed
	StatusPending OutboxStatus = 0

	// StatusSuccess - item was processed successfully
	StatusSuccess OutboxStatus = 1

	// StatusBadRequest - API returned 400 Bad Request (permanent failure)
	StatusBadRequest OutboxStatus = 2

	// StatusInternalError - API returned 500 Internal Server Error
	StatusInternalError OutboxStatus = 3

	// StatusUnauthorized - API returned 401 Unauthorized
	StatusUnauthorized OutboxStatus = 4

	// StatusForbidden - API returned 403 Forbidden
	StatusForbidden OutboxStatus = 5

	// StatusGatewayError - API returned 502/503/504 Gateway Error
	StatusGatewayError OutboxStatus = 6

	// StatusInProgress - item is currently being processed
	// Used to prevent re-polling; reset to 0 on crash recovery
	StatusInProgress OutboxStatus = 9
)

// String returns a human-readable status name
func (s OutboxStatus) String() string {
	switch s {
	case StatusPending:
		return "PENDING"
	case StatusSuccess:
		return "SUCCESS"
	case StatusBadRequest:
		return "BAD_REQUEST"
	case StatusInternalError:
		return "INTERNAL_ERROR"
	case StatusUnauthorized:
		return "UNAUTHORIZED"
	case StatusForbidden:
		return "FORBIDDEN"
	case StatusGatewayError:
		return "GATEWAY_ERROR"
	case StatusInProgress:
		return "IN_PROGRESS"
	default:
		return "UNKNOWN"
	}
}

// IsTerminal returns true if this status represents a final state
func (s OutboxStatus) IsTerminal() bool {
	return s == StatusSuccess || s == StatusBadRequest || s == StatusForbidden
}

// IsRetryable returns true if this status should be retried
func (s OutboxStatus) IsRetryable() bool {
	return s == StatusInternalError || s == StatusGatewayError || s == StatusUnauthorized
}

// OutboxItemType defines the type of outbox item
type OutboxItemType string

const (
	OutboxItemTypeEvent       OutboxItemType = "EVENT"
	OutboxItemTypeDispatchJob OutboxItemType = "DISPATCH_JOB"
)

// OutboxItem represents an item in the outbox table
type OutboxItem struct {
	// ID is the unique identifier (TSID format, 13-char Crockford Base32)
	ID string `bson:"_id" json:"id"`

	// Type is the type of outbox item (EVENT or DISPATCH_JOB)
	Type OutboxItemType `bson:"type" json:"type"`

	// MessageGroup is the optional message group for FIFO ordering within the group
	MessageGroup string `bson:"messageGroup,omitempty" json:"messageGroup,omitempty"`

	// Payload is the JSON payload to send to the API
	Payload string `bson:"payload" json:"payload"`

	// Status is the current processing status (integer)
	Status OutboxStatus `bson:"status" json:"status"`

	// RetryCount is the number of retry attempts made
	RetryCount int `bson:"retryCount" json:"retryCount"`

	// CreatedAt is when the item was created
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`

	// UpdatedAt is when the item was last updated
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`

	// ErrorMessage is the error message if the item failed
	ErrorMessage string `bson:"errorMessage,omitempty" json:"errorMessage,omitempty"`
}

// IsPending returns true if the item is pending
func (i *OutboxItem) IsPending() bool {
	return i.Status == StatusPending
}

// IsInProgress returns true if the item is being processed
func (i *OutboxItem) IsInProgress() bool {
	return i.Status == StatusInProgress
}

// IsSuccess returns true if the item was successfully processed
func (i *OutboxItem) IsSuccess() bool {
	return i.Status == StatusSuccess
}

// GetEffectiveMessageGroup returns the message group or "default" if empty
func (i *OutboxItem) GetEffectiveMessageGroup() string {
	if i.MessageGroup == "" {
		return "default"
	}
	return i.MessageGroup
}

// StatusFromHTTPCode converts an HTTP status code to OutboxStatus
func StatusFromHTTPCode(code int) OutboxStatus {
	switch {
	case code >= 200 && code < 300:
		return StatusSuccess
	case code == 400:
		return StatusBadRequest
	case code == 401:
		return StatusUnauthorized
	case code == 403:
		return StatusForbidden
	case code >= 500 && code < 600:
		if code == 502 || code == 503 || code == 504 {
			return StatusGatewayError
		}
		return StatusInternalError
	default:
		return StatusInternalError
	}
}

// DatabaseType defines the type of database for outbox storage
type DatabaseType string

const (
	DatabaseTypePostgreSQL DatabaseType = "POSTGRESQL"
	DatabaseTypeMySQL      DatabaseType = "MYSQL"
	DatabaseTypeMongoDB    DatabaseType = "MONGODB"
)
