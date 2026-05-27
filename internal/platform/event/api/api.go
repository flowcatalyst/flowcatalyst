// Package api wires the event HTTP endpoints via huma.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *event.Repository
}

const tag = "events"

// Register mounts the event endpoints.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "batchIngestEvents",
		Method:        http.MethodPost,
		Path:          "/api/events/batch",
		Summary:       "Ingest a batch of events (SDK)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.batchIngest)

	huma.Register(api, huma.Operation{
		OperationID:   "eventFilterOptions",
		Method:        http.MethodGet,
		Path:          "/api/events/filter-options",
		Summary:       "Distinct event types/sources/clients for filter UI",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.filterOptions)

	huma.Register(api, huma.Operation{
		OperationID:   "listEventsRaw",
		Method:        http.MethodGet,
		Path:          "/api/events/list-raw",
		Summary:       "List events with raw JSONB rows",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listRaw)

	huma.Register(api, huma.Operation{
		OperationID:   "listEvents",
		Method:        http.MethodGet,
		Path:          "/api/events",
		Summary:       "List events with filters",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "getEvent",
		Method:        http.MethodGet,
		Path:          "/api/events/{id}",
		Summary:       "Get an event by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)
}

// ── batch ingest ─────────────────────────────────────────────────────────

type batchInput struct {
	Body BatchRequest
}

type batchOutput struct {
	Body BatchResponse
}

func (s *State) batchIngest(ctx context.Context, in *batchInput) (*batchOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:batch:events-write"); err != nil {
		return nil, err
	}
	if len(in.Body.Items) > 1000 {
		return nil, httperror.BadRequest("BATCH_TOO_LARGE", "max 1000 items per batch")
	}
	events := make([]event.Event, 0, len(in.Body.Items))
	for _, it := range in.Body.Items {
		ev := event.New(it.Type, it.Source, it.Subject, it.Data)
		if it.ID != "" {
			ev.ID = it.ID
		}
		if it.DeduplicationID != "" {
			ev.DeduplicationID = it.DeduplicationID
		}
		ev.ClientID = it.ClientID
		ev.CorrelationID = it.CorrelationID
		ev.CausationID = it.CausationID
		events = append(events, *ev)
	}
	n, err := s.Repo.InsertBatch(ctx, events)
	if err != nil {
		return nil, usecase.Internal("REPO", "insert batch failed", err)
	}
	return &batchOutput{Body: BatchResponse{Accepted: n}}, nil
}

// ── list / detail ────────────────────────────────────────────────────────

type listInput struct {
	Type          string `query:"type"`
	Source        string `query:"source"`
	Subject       string `query:"subject"`
	ClientID      string `query:"clientId"`
	PrincipalID   string `query:"principalId"`
	CorrelationID string `query:"correlationId"`
	Since         string `query:"since" doc:"RFC3339 timestamp"`
	Until         string `query:"until" doc:"RFC3339 timestamp"`
	Limit         int    `query:"limit"`
	Offset        int    `query:"offset"`
}

func (in *listInput) toFilters() event.FilterParams {
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		s := v
		return &s
	}
	ts := func(v string) *time.Time {
		if v == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return &t
		}
		return nil
	}
	return event.FilterParams{
		Type:          str(in.Type),
		Source:        str(in.Source),
		Subject:       str(in.Subject),
		ClientID:      str(in.ClientID),
		PrincipalID:   str(in.PrincipalID),
		CorrelationID: str(in.CorrelationID),
		Since:         ts(in.Since),
		Until:         ts(in.Until),
		Limit:         in.Limit,
		Offset:        in.Offset,
	}
}

type listOutput struct {
	Body EventListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view"); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, in.toFilters())
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := make([]EventResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: EventListResponse{Items: out}}, nil
}

func (s *State) listRaw(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view-raw"); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, in.toFilters())
	if err != nil {
		return nil, usecase.Internal("REPO", "find_raw failed", err)
	}
	out := make([]EventResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: EventListResponse{Items: out}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body EventResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view"); err != nil {
		return nil, err
	}
	ev, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if ev == nil {
		return nil, httperror.NotFound("Event", in.ID)
	}
	return &getOutput{Body: fromEntity(ev)}, nil
}

// ── filter-options ───────────────────────────────────────────────────────

type emptyInput struct{}

type filterOptionsOutput struct {
	Body EventFilterOptionsResponse
}

func (s *State) filterOptions(ctx context.Context, _ *emptyInput) (*filterOptionsOutput, error) {
	q := func(col string) []string {
		out, _ := s.Repo.DistinctValues(ctx, col, 200)
		return out
	}
	return &filterOptionsOutput{Body: EventFilterOptionsResponse{
		Types:     q("type"),
		Sources:   q("source"),
		ClientIDs: q("client_id"),
	}}, nil
}
