package runtimeapi

import (
	"context"
	"encoding/json"
	"mime"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// eventDedupID is the deduplication id of an event an ingestion made from a
// record with an externalId: the ingestion's id keeps two ingestions' external
// ids apart.
func eventDedupID(ingestionID, externalID string) string {
	return "ing:" + ingestionID + ":" + externalID
}

// createEvents makes an event of each record, for an ingestion whose target is
// an EVENT. The events are stored through the repository POST /api/events/batch
// inserts through (event.Repository.InsertBatch), so the stream fans them out
// to subscriptions as it does any event. A record with an externalId has the
// deduplication id ing:{ingestionId}:{externalId}: sending it again stores
// nothing and answers the event it made first (existing: true).
//
// Unlike the jobs of a DISPATCH_JOB target, events are not claimed under the
// ingestion's job keys; the deduplication id is the idempotency. The lease (a
// polled source) and the client's status are checked before the insert, not in
// one transaction with it: the event repository inserts through its own
// connection, and a request that loses its lease meanwhile can only repeat
// events the next owner makes under the same ids.
func (s *State) createEvents(ctx context.Context, in *createJobsInput, i *ingestion.Ingestion, t *jobTarget) (*apicommon.Out[CreateJobsResponse], error) {
	if _, err := s.requireRuntimeIngestion(ctx, in.ID, in.RuntimeID, true); err != nil {
		return nil, err
	}
	results := make([]JobResult, len(in.Body.Records))
	var events []event.Event
	var at []int // the record each event came from
	clientID := string(i.ClientID)
	for n, rec := range in.Body.Records {
		results[n].Index = n
		problem := ""
		if _, problem = t.item(rec, in.RuntimeID); problem == "" {
			var data json.RawMessage
			if data, problem = eventData(rec, t.target.WrapNonJson); problem == "" {
				subject := ""
				if rec.Subject != nil {
					subject = *rec.Subject
				}
				ev := event.New(t.target.Code, t.source, subject, data)
				ev.ClientID = &clientID
				ev.MessageGroup = rec.MessageGroup
				ev.CorrelationID = rec.CorrelationID
				if rec.ExternalID != nil {
					ev.DeduplicationID = eventDedupID(i.ID, *rec.ExternalID)
				}
				events, at = append(events, *ev), append(at, n)
			}
		}
		if problem != "" {
			results[n].Error = &problem
		}
	}

	created := 0
	if len(events) > 0 {
		if _, err := s.Events.InsertBatch(ctx, events); err != nil {
			return nil, usecase.Internal("REPO", "insert events failed", err)
		}
		// InsertBatch drops a repeat silently, so the row that exists may be an
		// earlier request's, or an earlier record's of this one: read the ids
		// back and answer the stored one.
		keyed := func(k int) bool { return in.Body.Records[at[k]].ExternalID != nil }
		var dedup []string
		for k, ev := range events {
			if keyed(k) {
				dedup = append(dedup, ev.DeduplicationID)
			}
		}
		stored, err := s.Events.IDsByDeduplicationID(ctx, dedup)
		if err != nil {
			return nil, usecase.Internal("REPO", "find events by deduplication id failed", err)
		}
		for k, ev := range events {
			r := &results[at[k]]
			r.ID = ev.ID
			if keyed(k) {
				if id, ok := stored[ev.DeduplicationID]; ok && id != ev.ID {
					r.ID, r.Existing = id, true
					continue
				}
			}
			created++
		}
	}
	return &apicommon.Out[CreateJobsResponse]{Body: CreateJobsResponse{Kind: ingestion.TargetEvent, Created: created, Results: results}}, nil
}

// eventData is the data of the event a record becomes: the record itself when
// its content type is JSON and it parses, else (wrapNonJson) the record as
// {"contentType", "body"}, else a reason the record cannot be an event.
func eventData(rec JobRecord, wrapNonJson bool) (json.RawMessage, string) {
	if isJSONContentType(rec.PayloadContentType) && json.Valid([]byte(rec.Payload)) {
		return json.RawMessage(rec.Payload), ""
	}
	if !wrapNonJson {
		return nil, "the record is not JSON and the target's wrapNonJson is off"
	}
	contentType := rec.PayloadContentType
	if contentType == "" {
		contentType = "text/plain"
	}
	wrapped, err := json.Marshal(struct {
		ContentType string `json:"contentType"`
		Body        string `json:"body"`
	}{contentType, rec.Payload})
	if err != nil {
		return nil, err.Error()
	}
	return wrapped, ""
}

// isJSONContentType reports whether a record's content type says JSON:
// application/json or a +json suffix, parameters ignored. An empty one is
// application/json, as the jobs request documents.
func isJSONContentType(contentType string) bool {
	if strings.TrimSpace(contentType) == "" {
		return true
	}
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		media = strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	}
	return media == "application/json" || strings.HasSuffix(media, "+json")
}
