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

// EventResponse mirrors event.Event.
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
		CreatedAt:       jsontime.New(e.CreatedAt),
	}
}

// EventListResponse is the wire shape for GET /api/events.
type EventListResponse struct {
	Items []EventResponse `json:"items"`
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

// EventFilterOptionsResponse is the wire shape for GET /api/events/filter-options.
type EventFilterOptionsResponse struct {
	Types     []string `json:"types"`
	Sources   []string `json:"sources"`
	ClientIDs []string `json:"clientIds"`
}
