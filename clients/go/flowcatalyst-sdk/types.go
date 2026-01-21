package flowcatalyst

import "time"

// EventType represents an event type definition
type EventType struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Version   string                 `json:"version"`
	Schema    map[string]interface{} `json:"schema"`
	CreatedAt time.Time              `json:"createdAt"`
}

// CreateEventTypeRequest is the request body for creating an event type
type CreateEventTypeRequest struct {
	Name    string                 `json:"name"`
	Version string                 `json:"version"`
	Schema  map[string]interface{} `json:"schema"`
}

// Subscription represents an event subscription
type Subscription struct {
	ID          string    `json:"id"`
	EventTypeID string    `json:"eventTypeId"`
	Endpoint    string    `json:"endpoint"`
	Status      string    `json:"status"` // active, paused, failed
	CreatedAt   time.Time `json:"createdAt"`
}

// CreateSubscriptionRequest is the request body for creating a subscription
type CreateSubscriptionRequest struct {
	EventTypeID string `json:"eventTypeId"`
	Endpoint    string `json:"endpoint"`
	Status      string `json:"status"`
}

// DispatchJob represents a dispatch job
type DispatchJob struct {
	ID             string     `json:"id"`
	EventID        string     `json:"eventId"`
	SubscriptionID string     `json:"subscriptionId"`
	Status         string     `json:"status"` // pending, processing, completed, failed
	Attempts       int        `json:"attempts"`
	CreatedAt      time.Time  `json:"createdAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
}
