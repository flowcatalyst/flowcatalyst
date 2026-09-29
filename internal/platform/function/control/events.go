package control

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// emit serves POST /control/functions/events (plan §6.3, §8.3): a runner's
// `event.emit` host call, made on a function's behalf. The runner never
// holds an application credential; the platform checks the event type
// against the invoking version's own describe document instead.
//
// Ingestion goes through event.Repository — the same repository
// POST /api/events uses — so dedup-by-deduplicationId and the insert path
// are identical; only the source default ("function:<address>") and the
// describe-driven ownership checks are specific to this route.
func (s *State) emit(ctx context.Context, in *apicommon.In[fncontrol.EmitRequest]) (*apicommon.Out[fncontrol.EmitResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if err := auth.CanControlFunctionRunner(ac); err != nil {
		return nil, err
	}
	req := in.Body
	if strings.TrimSpace(req.FunctionID) == "" {
		return nil, usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
	}
	if strings.TrimSpace(req.Event.Type) == "" {
		return nil, usecase.Validation("EVENT_TYPE_REQUIRED", "event.type is required")
	}

	fn, err := s.Repo.FindByID(ctx, req.FunctionID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if fn == nil {
		return nil, httperror.NotFound("Function", req.FunctionID)
	}
	ver, err := s.Repo.GetVersionByNumber(ctx, fn.ID, int32(req.Version))
	if err != nil {
		return nil, usecase.Internal("REPO", "get_version_by_number failed", err)
	}
	if ver == nil {
		return nil, httperror.NotFound("Function version", fn.Address+"@v"+strconv.Itoa(req.Version))
	}
	d, err := abi.ParseDescribe(ver.Describe)
	if err != nil {
		return nil, usecase.Internal("CORRUPT_DESCRIBE",
			"function "+fn.Address+" version "+strconv.Itoa(req.Version)+" has an unparseable describe document", err)
	}

	// The type must be declared in THIS version's emits (not just any
	// version of the function) — describe is per-version, and a runner
	// only ever invokes the version it loaded.
	if !slices.Contains(d.Emits, req.Event.Type) {
		return nil, usecase.Authorization("EVENT_NOT_DECLARED",
			"event type "+req.Event.Type+" is not declared in this version's emits")
	}
	owner, _, _ := strings.Cut(req.Event.Type, ":")
	if owner != fn.ApplicationCode {
		return nil, usecase.Authorization("EVENT_TYPE_NOT_OWNED",
			"event type "+req.Event.Type+" is not owned by application "+fn.ApplicationCode)
	}

	source := strings.TrimSpace(req.Event.Source)
	if source == "" {
		source = "function:" + fn.Address
	}
	ev := event.New(req.Event.Type, source, req.Event.Subject, req.Data)
	if req.Event.DedupID != "" {
		ev.DeduplicationID = req.Event.DedupID
	}
	ev.ClientID = fn.ClientID
	if req.Event.CorrelationID != "" {
		v := req.Event.CorrelationID
		ev.CorrelationID = &v
	}
	if req.Event.CausationID != "" {
		v := req.Event.CausationID
		ev.CausationID = &v
	}
	if req.Event.MessageGroup != "" {
		v := req.Event.MessageGroup
		ev.MessageGroup = &v
	}

	if _, err := s.Events.InsertBatch(ctx, []event.Event{*ev}); err != nil {
		return nil, usecase.Internal("REPO", "insert_batch failed", err)
	}
	// Read back by dedup id rather than trusting ev.ID: InsertBatch drops a
	// duplicate silently (idempotent-on-dedupId, matching POST /api/events),
	// so the row that actually exists may be an earlier request's, not this
	// one's — this is what makes retried emits with the same dedupId
	// idempotent at the response level too.
	stored, err := s.Events.FindByDeduplicationID(ctx, ev.DeduplicationID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_deduplication_id failed", err)
	}
	if stored == nil {
		return nil, usecase.Internal("EVENT_INSERT_MISSING", "event insert did not produce a row", nil)
	}
	return &apicommon.Out[fncontrol.EmitResponse]{Body: fncontrol.EmitResponse{EventID: stored.ID}}, nil
}
