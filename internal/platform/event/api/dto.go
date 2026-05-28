// dto.go contains the wire-format types for the event API.
package api

import (
	"encoding/json"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// ContextEntryDTO mirrors event.ContextEntry.
type ContextEntryDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// EventResponse mirrors event.Event. Used for the single-event detail
// view (GET /api/events/{id}); the SPA's EventDetail extends EventRead
// with the extra envelope fields below.
type EventResponse struct {
	ID              string            `json:"id"`
	SpecVersion     string            `json:"specVersion"`
	Type            string            `json:"type"`
	Source          string            `json:"source"`
	Subject         string            `json:"subject"`
	Time            httpcompat.Time   `json:"time"`
	Data            json.RawMessage   `json:"data,omitempty"`
	Context         []ContextEntryDTO `json:"context,omitempty"`
	DeduplicationID string            `json:"deduplicationId"`
	ClientID        *string           `json:"clientId,omitempty"`
	MessageGroup    *string           `json:"messageGroup,omitempty"`
	CorrelationID   *string           `json:"correlationId,omitempty"`
	CausationID     *string           `json:"causationId,omitempty"`
	Application     *string           `json:"application,omitempty"`
	Subdomain       *string           `json:"subdomain,omitempty"`
	Aggregate       *string           `json:"aggregate,omitempty"`
	ProjectedAt     *httpcompat.Time  `json:"projectedAt,omitempty"`
	CreatedAt       httpcompat.Time   `json:"createdAt"`
}

func fromEntity(e *event.Event) EventResponse {
	var ctx []ContextEntryDTO
	if len(e.Context) > 0 {
		ctx = make([]ContextEntryDTO, 0, len(e.Context))
		for _, c := range e.Context {
			ctx = append(ctx, ContextEntryDTO{Key: c.Key, Value: c.Value})
		}
	}
	var projected *httpcompat.Time
	if e.ProjectedAt != nil {
		v := jsontime.New(*e.ProjectedAt)
		projected = &v
	}
	return EventResponse{
		ID:              e.ID,
		SpecVersion:     e.SpecVersion,
		Type:            e.Type,
		Source:          e.Source,
		Subject:         e.Subject,
		Time:            jsontime.New(e.Time),
		Data:            e.Data,
		Context:         ctx,
		DeduplicationID: e.DeduplicationID,
		ClientID:        e.ClientID,
		MessageGroup:    e.MessageGroup,
		CorrelationID:   e.CorrelationID,
		CausationID:     e.CausationID,
		Application:     e.Application,
		Subdomain:       e.Subdomain,
		Aggregate:       e.Aggregate,
		ProjectedAt:     projected,
		CreatedAt:       jsontime.New(e.CreatedAt),
	}
}

// EventRead is the slim read-projection wire shape for the list endpoints
// (GET /api/events, /bff/events). Matches the SPA's `EventRead` interface
// in frontend/src/api/events.ts: top-level `type` (not eventType) and a
// non-optional `projectedAt`. Mirrors Rust's `event::entity::EventRead`.
type EventRead struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Source        string          `json:"source"`
	Subject       *string         `json:"subject,omitempty"`
	Time          httpcompat.Time `json:"time"`
	Application   *string         `json:"application,omitempty"`
	Subdomain     *string         `json:"subdomain,omitempty"`
	Aggregate     *string         `json:"aggregate,omitempty"`
	MessageGroup  *string         `json:"messageGroup,omitempty"`
	CorrelationID *string         `json:"correlationId,omitempty"`
	ClientID      *string         `json:"clientId,omitempty"`
	ProjectedAt   httpcompat.Time `json:"projectedAt"`
}

func readFromEntity(e *event.Event) EventRead {
	var subject *string
	if e.Subject != "" {
		s := e.Subject
		subject = &s
	}
	projected := e.CreatedAt
	if e.ProjectedAt != nil {
		projected = *e.ProjectedAt
	}
	return EventRead{
		ID:            e.ID,
		Type:          e.Type,
		Source:        e.Source,
		Subject:       subject,
		Time:          jsontime.New(e.Time),
		Application:   e.Application,
		Subdomain:     e.Subdomain,
		Aggregate:     e.Aggregate,
		MessageGroup:  e.MessageGroup,
		CorrelationID: e.CorrelationID,
		ClientID:      e.ClientID,
		ProjectedAt:   jsontime.New(projected),
	}
}

// RawEventResponse is the debug raw-event wire shape for GET
// /bff/debug/events. Distinct from EventRead: the Type column is named
// `eventType` here (the SPA RawEventListPage binds field="eventType" and
// field="deduplicationId"). Mirrors Rust's shared/debug_api.rs
// RawEventResponse.
type RawEventResponse struct {
	ID              string            `json:"id"`
	SpecVersion     string            `json:"specVersion"`
	EventType       string            `json:"eventType"`
	Source          string            `json:"source"`
	Subject         *string           `json:"subject,omitempty"`
	Time            httpcompat.Time   `json:"time"`
	Data            json.RawMessage   `json:"data,omitempty"`
	MessageGroup    *string           `json:"messageGroup,omitempty"`
	CorrelationID   *string           `json:"correlationId,omitempty"`
	CausationID     *string           `json:"causationId,omitempty"`
	DeduplicationID *string           `json:"deduplicationId,omitempty"`
	ContextData     []ContextEntryDTO `json:"contextData,omitempty"`
	ClientID        *string           `json:"clientId,omitempty"`
}

func rawFromEntity(e *event.Event) RawEventResponse {
	var subject, dedup *string
	if e.Subject != "" {
		s := e.Subject
		subject = &s
	}
	if e.DeduplicationID != "" {
		s := e.DeduplicationID
		dedup = &s
	}
	var ctx []ContextEntryDTO
	if len(e.Context) > 0 {
		ctx = make([]ContextEntryDTO, 0, len(e.Context))
		for _, c := range e.Context {
			ctx = append(ctx, ContextEntryDTO{Key: c.Key, Value: c.Value})
		}
	}
	return RawEventResponse{
		ID:              e.ID,
		SpecVersion:     e.SpecVersion,
		EventType:       e.Type,
		Source:          e.Source,
		Subject:         subject,
		Time:            jsontime.New(e.Time),
		Data:            e.Data,
		MessageGroup:    e.MessageGroup,
		CorrelationID:   e.CorrelationID,
		CausationID:     e.CausationID,
		DeduplicationID: dedup,
		ContextData:     ctx,
		ClientID:        e.ClientID,
	}
}

// BatchEventItem is one item in the batch ingest body.
type BatchEventItem struct {
	ID              string          `json:"id,omitempty"`
	Type            string          `json:"type"`
	Source          string          `json:"source"`
	Subject         string          `json:"subject"`
	Data            json.RawMessage `json:"data"`
	DeduplicationID string          `json:"deduplicationId,omitempty"`
	ClientID        *string         `json:"clientId,omitempty"`
	CorrelationID   *string         `json:"correlationId,omitempty"`
	CausationID     *string         `json:"causationId,omitempty"`
}

// BatchRequest is the wire body for POST /api/events/batch.
type BatchRequest struct {
	Items []BatchEventItem `json:"items"`
}

// BatchResponse reports how many items were accepted.
type BatchResponse struct {
	Accepted int `json:"accepted"`
}

// EventFilterOption is one {value,label} pair for the SPA's cascading
// filter dropdowns.
type EventFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// EventFilterOptionsResponse is the wire shape for GET
// /api/events/filter-options. Matches the SPA's `EventFilterOptions`
// interface (events.ts:44): {applications, subdomains, eventTypes} where
// each is a {value,label}[]. We don't have separate display labels for
// these distinct values, so label == value (noted in the report).
type EventFilterOptionsResponse struct {
	Applications []EventFilterOption `json:"applications"`
	Subdomains   []EventFilterOption `json:"subdomains"`
	EventTypes   []EventFilterOption `json:"eventTypes"`
}

// toFilterOptions maps raw distinct string values to {value,label} pairs.
func toFilterOptions(values []string) []EventFilterOption {
	out := make([]EventFilterOption, 0, len(values))
	for _, v := range values {
		out = append(out, EventFilterOption{Value: v, Label: v})
	}
	return out
}
