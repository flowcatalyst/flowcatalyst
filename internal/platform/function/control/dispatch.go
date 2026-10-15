package control

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Limits on what a function may put on a job. A job is a message, not a
// transfer.
const (
	maxDispatchPayloadBytes = 256 << 10
	maxDispatchMetadata     = 50
	maxMetadataKeyLen       = 100
	maxMetadataValueLen     = 1000
	maxCorrelationIDLen     = 100
	// reservedMetadataPrefix is the platform's: every function-created job
	// carries fn.* provenance entries the guest cannot forge or shadow.
	reservedMetadataPrefix = "fn."
)

// dispatch serves POST /control/functions/dispatch-jobs (the `dispatch` host
// call, ABI op 11): it creates a dispatch job on a function's behalf, so the
// function holds no platform credential and writes only what its describe
// declares. Nothing the guest sends decides where the job goes or who signs
// it:
//
//  1. (target, path) must be an entry of THIS version's `dispatches` (the
//     version the runner invoked, as emit checks `emits`);
//  2. the target function must be in the function's own application (and own
//     client), so the job is delivered, signed and billed inside one tenant's
//     application and never on another application's credential;
//  3. the target must exist and have a live version with a webhook endpoint
//     taking POST at the path, so a job is never created for a URL that cannot
//     verify its signature;
//  4. the URL is built here, from the runner URL template, the target's pool,
//     its address and the declared path;
//  5. the job code's first segment is the function's application, which is
//     what delivery-credential resolution keys on for a job with no
//     subscription: the application's signing account, the same one the
//     target's `webhook` endpoints verify.
//
// The job is built through sdk.NewJobFromItem, the SDK ingest routes' own
// mapping, so defaults and validation are the platform's (dataOnly aside,
// which defaults to true for a function target). It records its origin as
// kind TASK, source "function:<address>", and fn.* metadata.
func (s *State) dispatch(ctx context.Context, in *apicommon.In[fncontrol.DispatchRequest]) (*apicommon.Out[fncontrol.DispatchResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if err := auth.CanControlFunctionRunner(ac); err != nil {
		return nil, err
	}
	req := in.Body
	d := req.Dispatch
	if strings.TrimSpace(req.FunctionID) == "" {
		return nil, usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
	}
	if strings.TrimSpace(d.Target) == "" || strings.TrimSpace(d.Path) == "" {
		return nil, usecase.Validation("DISPATCH_TARGET_REQUIRED", "dispatch.target and dispatch.path are required")
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
	desc, err := abi.ParseDescribe(ver.Describe)
	if err != nil {
		return nil, usecase.Internal("CORRUPT_DESCRIBE",
			"function "+fn.Address+" version "+strconv.Itoa(req.Version)+" has an unparseable describe document", err)
	}

	// 1. Declared by THIS version.
	if !slices.Contains(desc.Dispatches, abi.DispatchTarget{Target: d.Target, Path: d.Path}) {
		return nil, usecase.Authorization("DISPATCH_NOT_DECLARED",
			"dispatch target "+d.Target+d.Path+" is not declared in this version's dispatches")
	}

	// 2. Same application (the address prefix is the code; the ids below make
	// it airtight against a renamed code) and same client.
	if app, _, _ := strings.Cut(d.Target, "."); app != fn.ApplicationCode {
		return nil, usecase.Authorization("DISPATCH_TARGET_NOT_OWNED",
			"dispatch target "+d.Target+" is not a function of application "+fn.ApplicationCode)
	}
	target, err := s.Repo.FindByAddress(ctx, d.Target)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_address failed", err)
	}
	if target == nil {
		return nil, usecase.NotFound("DISPATCH_TARGET_NOT_FOUND", "no function "+d.Target+" exists")
	}
	if target.ApplicationID != fn.ApplicationID {
		return nil, usecase.Authorization("DISPATCH_TARGET_NOT_OWNED",
			"dispatch target "+d.Target+" is not a function of application "+fn.ApplicationCode)
	}
	if !sameClient(fn.ClientID, target.ClientID) {
		return nil, usecase.Authorization("DISPATCH_TARGET_CLIENT_MISMATCH",
			"dispatch target "+d.Target+" belongs to a different client than "+fn.Address)
	}

	// 3. A live version that takes a signed POST at the path.
	if msg, err := s.targetProblem(ctx, target, d.Path); err != nil {
		return nil, err
	} else if msg != "" {
		return nil, usecase.BusinessRule("DISPATCH_TARGET_UNAVAILABLE", msg)
	}

	// The job's own fields. Everything below is validated here or by
	// NewJobFromItem; none of it can change the destination.
	if len(req.Payload) > maxDispatchPayloadBytes {
		return nil, usecase.Validation("DISPATCH_PAYLOAD_TOO_LARGE", "the dispatch payload is over 256 KiB")
	}
	if !utf8.Valid(req.Payload) {
		return nil, usecase.Validation("DISPATCH_PAYLOAD_NOT_TEXT", "the dispatch payload must be UTF-8 text")
	}
	contentType := strings.TrimSpace(d.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	if isJSONType(contentType) && !json.Valid(req.Payload) {
		return nil, usecase.Validation("DISPATCH_PAYLOAD_NOT_JSON", "the dispatch payload is not valid JSON for content type "+contentType)
	}
	code := strings.TrimSpace(d.Code)
	if code == "" {
		code = fn.ApplicationCode + ":function:" + fn.Name + ":dispatch"
	}
	if first, _, _ := strings.Cut(code, ":"); first != fn.ApplicationCode || strings.ContainsAny(code, " \t\r\n") || len(code) > 255 {
		return nil, usecase.Validation("DISPATCH_CODE_NOT_OWNED",
			"code must start with the function's application ("+fn.ApplicationCode+":) and contain no whitespace")
	}
	if d.Mode != "" {
		switch common.DispatchMode(d.Mode) {
		case common.DispatchImmediate, common.DispatchNextOnError, common.DispatchBlockOnError:
		default:
			return nil, usecase.Validation("DISPATCH_MODE_INVALID", "mode must be IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR")
		}
	}
	if d.MaxRetries != nil && *d.MaxRetries < 0 {
		return nil, usecase.Validation("DISPATCH_MAX_RETRIES_INVALID", "maxRetries must not be negative")
	}
	if d.TimeoutSeconds != nil && *d.TimeoutSeconds <= 0 {
		return nil, usecase.Validation("DISPATCH_TIMEOUT_INVALID", "timeoutSeconds must be positive")
	}
	meta, err := jobMetadata(d.Metadata, fn, req)
	if err != nil {
		return nil, err
	}
	correlation := firstNonBlank(d.CorrelationID, req.RequestCorrelationID)
	if len(correlation) > maxCorrelationIDLen {
		return nil, usecase.Validation("DISPATCH_CORRELATION_TOO_LONG", "correlationId is over 100 characters")
	}

	dataOnly := true
	if d.DataOnly != nil {
		dataOnly = *d.DataOnly
	}
	payload := string(req.Payload)
	source := "function:" + fn.Address
	item := sdk.BatchItem{
		Kind:               string(dispatchjob.KindTask),
		Code:               code,
		Source:             &source,
		Subject:            nonEmpty(d.Subject),
		TargetURL:          function.TargetURL(s.RunnerURLTemplate, target.RunnerPool(), target.Address, d.Path),
		Payload:            &payload,
		PayloadContentType: contentType,
		DataOnly:           dataOnly,
		CorrelationID:      nonEmpty(correlation),
		ClientID:           fn.ClientID,
		MessageGroup:       nonEmpty(d.MessageGroup),
		Mode:               d.Mode,
		Descriptor:         nonEmpty(d.Descriptor),
		Metadata:           meta,
	}
	if d.TimeoutSeconds != nil {
		item.TimeoutSeconds = uint32(*d.TimeoutSeconds)
	}
	if d.MaxRetries != nil {
		item.MaxRetries = uint32(*d.MaxRetries)
	}
	job, err := sdk.NewJobFromItem(item)
	if err != nil {
		return nil, err // httperror: INVALID_TARGET_URL and friends
	}
	if d.MaxRetries != nil && *d.MaxRetries == 0 {
		job.MaxRetries = 0 // jobFromItem reads 0 as "default"; an explicit 0 means no retries
	}
	if d.CausationID != "" {
		// A job has no causation column; the link travels as metadata.
		job.Metadata = append(job.Metadata, dispatchjob.Metadata{Key: "causationId", Value: d.CausationID})
	}

	if s.Jobs == nil {
		return nil, usecase.Internal("DISPATCH_UNWIRED", "the dispatch-job repository is not configured", nil)
	}
	if err := s.Jobs.InsertBatch(ctx, []dispatchjob.DispatchJob{job}); err != nil {
		return nil, usecase.Internal("REPO", "insert failed", err)
	}
	return &apicommon.Out[fncontrol.DispatchResponse]{Body: fncontrol.DispatchResponse{JobID: job.ID}}, nil
}

// targetProblem reports why target cannot take a signed delivery at path, or
// "": it needs a live alias onto a version whose describe has a webhook
// endpoint accepting POST at the literal path.
func (s *State) targetProblem(ctx context.Context, target *function.Function, path string) (string, error) {
	aliases, err := s.Repo.ListAliases(ctx, target.ID)
	if err != nil {
		return "", usecase.Internal("REPO", "list_aliases failed", err)
	}
	var liveVersionID string
	for _, a := range aliases {
		if a.Name == function.LiveAlias {
			liveVersionID = a.VersionID
		}
	}
	var tv *function.Version
	if liveVersionID != "" {
		if tv, err = s.Repo.GetVersionByID(ctx, liveVersionID); err != nil {
			return "", usecase.Internal("REPO", "get_version_by_id failed", err)
		}
	}
	if tv == nil {
		return "function " + target.Address + " has no live version to deliver to", nil
	}
	td, err := abi.ParseDescribe(tv.Describe)
	if err != nil {
		return "", usecase.Internal("CORRUPT_DESCRIBE", "function "+target.Address+"'s live version has an unparseable describe document", err)
	}
	router, err := abi.NewRouter(td.Endpoints)
	if err != nil {
		return "", usecase.Internal("CORRUPT_DESCRIBE", "function "+target.Address+"'s live version has conflicting endpoints", err)
	}
	m, err := router.Match("POST", path)
	if err != nil {
		return "function " + target.Address + "'s live version has no endpoint accepting POST " + path, nil
	}
	if m.Endpoint.Auth != abi.AuthWebhook {
		return "function " + target.Address + "'s endpoint " + m.Endpoint.Pattern() + " is not a webhook endpoint, so it cannot verify the delivery's signature", nil
	}
	return "", nil
}

// jobMetadata validates the guest's entries and appends the platform's fn.*
// provenance, which the guest cannot supply.
func jobMetadata(in []abi.KV, fn *function.Function, req fncontrol.DispatchRequest) ([]dispatchjob.Metadata, error) {
	if len(in) > maxDispatchMetadata {
		return nil, usecase.Validation("DISPATCH_METADATA_TOO_MANY", "metadata is over 50 entries")
	}
	out := make([]dispatchjob.Metadata, 0, len(in)+3)
	seen := map[string]bool{}
	for _, kv := range in {
		k := strings.TrimSpace(kv.Key)
		switch {
		case k == "":
			return nil, usecase.Validation("DISPATCH_METADATA_INVALID", "a metadata key is empty")
		case len(k) > maxMetadataKeyLen || len(kv.Value) > maxMetadataValueLen:
			return nil, usecase.Validation("DISPATCH_METADATA_INVALID", "metadata key "+k+": key over 100 or value over 1000 characters")
		case strings.HasPrefix(strings.ToLower(k), reservedMetadataPrefix):
			return nil, usecase.Validation("DISPATCH_METADATA_RESERVED", "metadata keys starting with "+reservedMetadataPrefix+" are the platform's")
		case seen[k]:
			return nil, usecase.Validation("DISPATCH_METADATA_INVALID", "metadata key "+k+" appears twice")
		}
		seen[k] = true
		out = append(out, dispatchjob.Metadata{Key: k, Value: kv.Value})
	}
	out = append(out,
		dispatchjob.Metadata{Key: reservedMetadataPrefix + "address", Value: fn.Address},
		dispatchjob.Metadata{Key: reservedMetadataPrefix + "version", Value: strconv.Itoa(req.Version)},
	)
	if req.InvocationID != "" {
		out = append(out, dispatchjob.Metadata{Key: reservedMetadataPrefix + "invocation", Value: req.InvocationID})
	}
	return out, nil
}

func sameClient(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func isJSONType(ct string) bool {
	mt, _, _ := strings.Cut(strings.ToLower(ct), ";")
	mt = strings.TrimSpace(mt)
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

func nonEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func firstNonBlank(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
