// Package model provides data structures for the message router
package model

// MediationType defines the type of mediation to perform
type MediationType string

const (
	// MediationTypeHTTP is HTTP-based mediation to external webhooks
	MediationTypeHTTP MediationType = "HTTP"
)

// MessagePointer contains routing and mediation information.
// This record is serialized/deserialized to/from queue messages and contains all
// information needed to route and process a message through the system.
//
// This struct exactly matches Java's tech.flowcatalyst.messagerouter.model.MessagePointer
// to ensure interoperability between Java and Go message routers.
type MessagePointer struct {
	// ID is the unique message identifier (used for deduplication)
	ID string `json:"id"`

	// PoolCode is the processing pool identifier (e.g., "POOL-HIGH", "order-service")
	PoolCode string `json:"poolCode"`

	// AuthToken is the authentication token for downstream service calls (HMAC-SHA256)
	AuthToken string `json:"authToken"`

	// MediationType is the type of mediation to perform (HTTP, etc.)
	MediationType MediationType `json:"mediationType"`

	// MediationTarget is the target endpoint URL for mediation
	MediationTarget string `json:"mediationTarget"`

	// MessageGroupID is the optional message group ID for FIFO ordering within business entities.
	// Messages with the same messageGroupId are processed sequentially,
	// while messages with different messageGroupIds are processed concurrently.
	// Examples:
	//   - "order-12345" - All events for this order process in FIFO order
	//   - "user-67890" - All events for this user process in FIFO order
	//   - empty string - Uses DEFAULT_GROUP, processes independently
	MessageGroupID string `json:"messageGroupId"`

	// --- Internal fields (not serialized to queue) ---

	// BatchID is the internal batch identifier (NOT part of external contract, populated during routing).
	// Used to track messages from the same batch for FIFO ordering enforcement.
	BatchID string `json:"-"`

	// SQSMessageID is the AWS SQS internal message ID for pipeline tracking
	SQSMessageID string `json:"-"`
}

// MediationResponse is the response from a mediation endpoint indicating whether
// the message should be acknowledged.
//
// The endpoint returns HTTP 200 with this DTO to indicate:
//   - ack: true  - Message processing is complete, ACK it and mark as success
//   - ack: false - Message is accepted but not ready to be processed yet.
//     Nack it and retry via queue visibility timeout. Optionally specify a delay.
//
// This matches Java's tech.flowcatalyst.messagerouter.model.MediationResponse
type MediationResponse struct {
	// Ack indicates whether the message should be acknowledged (true) or nacked for retry (false)
	Ack bool `json:"ack"`

	// Message is an optional message or reason (e.g., delay reason if ack=false)
	Message string `json:"message,omitempty"`

	// DelaySeconds is the optional delay in seconds before the message becomes visible again
	// (only used when ack=false). Valid range: 1-43200 (12 hours).
	// If nil or 0, uses default visibility timeout (30s).
	DelaySeconds *int `json:"delaySeconds,omitempty"`
}

// Constants for delay handling
const (
	// MaxDelaySeconds is the maximum delay allowed (12 hours = 43200 seconds, SQS limit)
	MaxDelaySeconds = 43200

	// DefaultDelaySeconds is the default delay when none specified
	DefaultDelaySeconds = 30
)

// GetEffectiveDelaySeconds returns the effective delay in seconds, clamped to valid range.
// Returns DefaultDelaySeconds if DelaySeconds is nil or 0.
func (r *MediationResponse) GetEffectiveDelaySeconds() int {
	if r.DelaySeconds == nil || *r.DelaySeconds <= 0 {
		return DefaultDelaySeconds
	}
	if *r.DelaySeconds > MaxDelaySeconds {
		return MaxDelaySeconds
	}
	return *r.DelaySeconds
}

// ProcessRequest is the request from message router to process a dispatch job.
// This matches Java's DispatchProcessingResource.ProcessRequest
type ProcessRequest struct {
	// MessageID is the dispatch job ID to process
	MessageID string `json:"messageId"`
}

// ProcessResponse is the response to message router indicating processing result.
// Aligns with MediationResponse contract:
//   - ack: true  - Remove from queue (success OR permanent error like max retries reached)
//   - ack: false - Keep on queue, retry later (transient errors, not ready yet)
//   - delaySeconds - Optional delay before message becomes visible again
type ProcessResponse struct {
	Ack          bool   `json:"ack"`
	Message      string `json:"message,omitempty"`
	DelaySeconds *int   `json:"delaySeconds,omitempty"`
}

// NewAckResponse creates a response that acknowledges (removes from queue)
func NewAckResponse(message string) *ProcessResponse {
	return &ProcessResponse{
		Ack:     true,
		Message: message,
	}
}

// NewNackResponse creates a response that does not acknowledge (keeps on queue for retry)
func NewNackResponse(message string) *ProcessResponse {
	return &ProcessResponse{
		Ack:     false,
		Message: message,
	}
}

// NewNackWithDelayResponse creates a response that does not acknowledge with a specific retry delay
func NewNackWithDelayResponse(message string, delaySeconds int) *ProcessResponse {
	return &ProcessResponse{
		Ack:          false,
		Message:      message,
		DelaySeconds: &delaySeconds,
	}
}
