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
	Context         []ContextEntryDTO `json:"contextData,omitempty"`
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

// ── singular create (POST /api/events) ──────────────────────────────────

// CreateEventRequest is the wire body for POST /api/events, the singular
// SDK ingest. 1:1 with Rust event/api.rs CreateEventRequest and the
// Laravel SDK's Model\CreateEventRequest: required eventType/source/data,
// the rest optional.
type CreateEventRequest struct {
	EventType       string            `json:"eventType" doc:"Event type code (e.g., \"orders:fulfillment:shipment:shipped\")"`
	Source          string            `json:"source" doc:"Event source URI"`
	Subject         string            `json:"subject,omitempty" doc:"Event subject (optional context)"`
	Data            json.RawMessage   `json:"data" doc:"Event payload data"`
	MessageGroup    *string           `json:"messageGroup,omitempty" doc:"Message group for FIFO ordering"`
	CorrelationID   *string           `json:"correlationId,omitempty" doc:"Correlation ID for request tracing"`
	CausationID     *string           `json:"causationId,omitempty" doc:"Causation ID - the event that caused this event"`
	DeduplicationID string            `json:"deduplicationId,omitempty" doc:"Deduplication ID for exactly-once delivery"`
	ClientID        *string           `json:"clientId,omitempty" doc:"Client ID (optional, defaults to caller's client)"`
	ContextData     []ContextEntryDTO `json:"contextData,omitempty" doc:"Context data for filtering/searching"`
}

// CreatedEvent is the event envelope inside CreateEventResponse. It
// mirrors Rust's EventResponse (event/api.rs) — note `eventType`, NOT the
// `type` key used by this package's read-side EventResponse — because the
// Laravel SDK's Model\EventResponse decodes `eventType`. A distinct Go
// type so the OpenAPI schema name can't collide with the existing
// EventResponse component.
type CreatedEvent struct {
	ID              string            `json:"id"`
	SpecVersion     string            `json:"specVersion"`
	EventType       string            `json:"eventType"`
	Source          string            `json:"source"`
	Subject         string            `json:"subject,omitempty"`
	Time            httpcompat.Time   `json:"time"`
	Data            json.RawMessage   `json:"data"`
	MessageGroup    *string           `json:"messageGroup,omitempty"`
	CorrelationID   *string           `json:"correlationId,omitempty"`
	CausationID     *string           `json:"causationId,omitempty"`
	DeduplicationID string            `json:"deduplicationId,omitempty"`
	ClientID        *string           `json:"clientId,omitempty"`
	ContextData     []ContextEntryDTO `json:"contextData,omitempty"`
	CreatedAt       httpcompat.Time   `json:"createdAt"`
}

// CreateEventResponse is the wire body for POST /api/events. 1:1 with
// Rust CreateEventResponse {event, dispatchJobCount, isDuplicate} and the
// SDK's Model\CreateEventResponse.
type CreateEventResponse struct {
	Event            CreatedEvent `json:"event"`
	DispatchJobCount int          `json:"dispatchJobCount" doc:"Number of dispatch jobs created for matching subscriptions"`
	IsDuplicate      bool         `json:"isDuplicate" doc:"True if this was a deduplicated request (event already existed)"`
}

func createdFromEntity(e *event.Event) CreatedEvent {
	var ctx []ContextEntryDTO
	if len(e.Context) > 0 {
		ctx = make([]ContextEntryDTO, 0, len(e.Context))
		for _, c := range e.Context {
			ctx = append(ctx, ContextEntryDTO{Key: c.Key, Value: c.Value})
		}
	}
	return CreatedEvent{
		ID:              e.ID,
		SpecVersion:     e.SpecVersion,
		EventType:       e.Type,
		Source:          e.Source,
		Subject:         e.Subject,
		Time:            jsontime.New(e.Time),
		Data:            e.Data,
		MessageGroup:    e.MessageGroup,
		CorrelationID:   e.CorrelationID,
		CausationID:     e.CausationID,
		DeduplicationID: e.DeduplicationID,
		ClientID:        e.ClientID,
		ContextData:     ctx,
		CreatedAt:       jsontime.New(e.CreatedAt),
	}
}

// BatchEventItem is one item in the batch ingest body. Fields are optional in
// the generated schema so the endpoint accepts both the camelCase API form (the
// SPA fan-out) and the snake_case SDK-outbox payload form (event_type,
// correlation_id, …); the alias coalescing in UnmarshalJSON is 1:1 with Rust
// shared/batch_api.rs BatchEventItem's serde aliases.
type BatchEventItem struct {
	ID              string          `json:"id,omitempty"`
	SpecVersion     string          `json:"specVersion,omitempty"`
	Type            string          `json:"type,omitempty"`
	Source          string          `json:"source,omitempty"`
	Subject         string          `json:"subject,omitempty"`
	Data            json.RawMessage `json:"data,omitempty"`
	DeduplicationID string          `json:"deduplicationId,omitempty"`
	ClientID        *string         `json:"clientId,omitempty"`
	// ClientCode resolves to a client_id at ingest (client-centric linkage). The
	// SDK outbox sends this; an explicit ClientID still takes precedence.
	ClientCode    *string `json:"clientCode,omitempty"`
	MessageGroup  *string `json:"messageGroup,omitempty"`
	CorrelationID *string `json:"correlationId,omitempty"`
	CausationID   *string `json:"causationId,omitempty"`
	// Context (principalId / executionId / aggregateType, …) — the SDK outbox
	// sends these as `contextData`; mirrors the single-event create + the event
	// entity's context array (stored in context_data).
	Context []ContextEntryDTO `json:"contextData,omitempty"`
}

// UnmarshalJSON accepts both the camelCase API keys and the snake_case SDK
// outbox-payload keys (event_type, spec_version, correlation_id, causation_id,
// deduplication_id, message_group, client_id). Mirrors the serde aliases on the
// Rust BatchEventItem so the platform ingests whatever a deployed outbox sends.
func (b *BatchEventItem) UnmarshalJSON(data []byte) error {
	var r struct {
		ID                 string            `json:"id"`
		SpecVersion        string            `json:"specVersion"`
		SpecVersionAlt     string            `json:"spec_version"`
		Type               string            `json:"type"`
		TypeAlt            string            `json:"event_type"`
		Source             string            `json:"source"`
		Subject            string            `json:"subject"`
		Data               json.RawMessage   `json:"data"`
		DeduplicationID    string            `json:"deduplicationId"`
		DeduplicationIDAlt string            `json:"deduplication_id"`
		ClientID           *string           `json:"clientId"`
		ClientIDAlt        *string           `json:"client_id"`
		ClientCode         *string           `json:"clientCode"`
		ClientCodeAlt      *string           `json:"client_code"`
		MessageGroup       *string           `json:"messageGroup"`
		MessageGroupAlt    *string           `json:"message_group"`
		CorrelationID      *string           `json:"correlationId"`
		CorrelationIDAlt   *string           `json:"correlation_id"`
		CausationID        *string           `json:"causationId"`
		CausationIDAlt     *string           `json:"causation_id"`
		ContextData        []ContextEntryDTO `json:"contextData"`
		ContextDataAlt     []ContextEntryDTO `json:"context_data"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	b.ID = r.ID
	b.SpecVersion = coalesceStr(r.SpecVersion, r.SpecVersionAlt)
	b.Type = coalesceStr(r.Type, r.TypeAlt)
	b.Source = r.Source
	b.Subject = r.Subject
	b.Data = r.Data
	b.DeduplicationID = coalesceStr(r.DeduplicationID, r.DeduplicationIDAlt)
	b.ClientID = coalescePtr(r.ClientID, r.ClientIDAlt)
	b.ClientCode = coalescePtr(r.ClientCode, r.ClientCodeAlt)
	b.MessageGroup = coalescePtr(r.MessageGroup, r.MessageGroupAlt)
	b.CorrelationID = coalescePtr(r.CorrelationID, r.CorrelationIDAlt)
	b.CausationID = coalescePtr(r.CausationID, r.CausationIDAlt)
	b.Context = r.ContextData
	if b.Context == nil {
		b.Context = r.ContextDataAlt
	}
	return nil
}

func coalesceStr(primary, alt string) string {
	if primary != "" {
		return primary
	}
	return alt
}

func coalescePtr(primary, alt *string) *string {
	if primary != nil {
		return primary
	}
	return alt
}

// BatchRequest is the wire body for POST /api/events/batch.
type BatchRequest struct {
	Items []BatchEventItem `json:"items"`
}

// BatchResultItem is one per-item outcome in the batch-ingest response. The
// status is the SCREAMING_SNAKE OutboxStatus the outbox dispatcher parses
// (SUCCESS / BAD_REQUEST / INTERNAL_ERROR / …).
type BatchResultItem struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// BatchResponse is the wire body for POST /api/events/batch: a per-item result
// list, 1:1 with Rust BatchResponse {results:[…]} and the contract the outbox
// dispatcher (fc-outbox/http_dispatcher.rs) requires on a 2xx.
type BatchResponse struct {
	Results []BatchResultItem `json:"results"`
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
