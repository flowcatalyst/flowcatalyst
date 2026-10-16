package fn

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Job is a dispatch job to create via Dispatch (plan §5.3 op 11,
// dispatch.create). Target and Path name a function of this function's own
// application and a literal webhook path on it; both must be declared with
// Dispatches. The function never supplies a URL or a credential: the platform
// builds the delivery URL and signs it as the application.
type Job struct {
	Target string
	Path   string
	// Payload is delivered as the request body (see DataOnly).
	Payload []byte
	// Code defaults to <application>:function:<name>:dispatch; it must start
	// with the function's application.
	Code    string
	Subject string
	// Metadata travels as the platform's array of {key, value} (sorted by key
	// here). Keys starting with "fn." belong to the platform and are refused.
	Metadata map[string]string
	// MetadataEntries, when set, is sent as given (order preserved) and
	// Metadata is ignored.
	MetadataEntries []MetadataEntry
	// MessageGroup orders delivery per Mode.
	MessageGroup string
	// Mode is IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR; empty is the
	// platform default.
	Mode string
	// CorrelationID defaults to the calling request's X-Correlation-Id.
	CorrelationID string
	CausationID   string
	// ContentType defaults to application/json.
	ContentType string
	// Descriptor is what the job is, in words, for the dispatch-jobs grid.
	Descriptor string
	// MaxRetries and TimeoutSeconds override the platform defaults when non-nil.
	MaxRetries     *int
	TimeoutSeconds *int
	// DataOnly delivers the payload alone (the default, when nil) or wrapped
	// in the platform's envelope (false).
	DataOnly *bool
}

// MetadataEntry is one metadata entry.
type MetadataEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type dispatchMeta struct {
	Target         string          `json:"target"`
	Path           string          `json:"path"`
	Code           string          `json:"code,omitempty"`
	Subject        string          `json:"subject,omitempty"`
	MessageGroup   string          `json:"messageGroup,omitempty"`
	Mode           string          `json:"mode,omitempty"`
	CorrelationID  string          `json:"correlationId,omitempty"`
	CausationID    string          `json:"causationId,omitempty"`
	ContentType    string          `json:"contentType,omitempty"`
	Descriptor     string          `json:"descriptor,omitempty"`
	MaxRetries     *int            `json:"maxRetries,omitempty"`
	TimeoutSeconds *int            `json:"timeoutSeconds,omitempty"`
	DataOnly       *bool           `json:"dataOnly,omitempty"`
	Metadata       []MetadataEntry `json:"metadata,omitempty"`
}

type dispatchResult struct {
	JobID string `json:"jobId"`
}

// metadataEntries is the array form of j's metadata, or nil.
func (j Job) metadataEntries() []MetadataEntry {
	if len(j.MetadataEntries) > 0 {
		return append([]MetadataEntry(nil), j.MetadataEntries...)
	}
	if len(j.Metadata) == 0 {
		return nil
	}
	keys := make([]string, 0, len(j.Metadata))
	for k := range j.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]MetadataEntry, 0, len(keys))
	for _, k := range keys {
		out = append(out, MetadataEntry{Key: k, Value: j.Metadata[k]})
	}
	return out
}

// Dispatch creates the job and returns its id. Errors wrap *Error;
// errors.Is(err, ErrRetryable) is true for UNAVAILABLE (platform trouble)
// and false for a permanent refusal: NOT_DECLARED (Target/Path not declared),
// NOT_ALLOWED (the platform will not deliver there: other application, not
// live, no webhook endpoint at the path) or BAD_REQUEST (an invalid field).
//
// Dispatch is NOT idempotent: the platform does not de-duplicate jobs, so
// retrying after an ambiguous failure can create a second job.
func Dispatch(ctx context.Context, j Job) (jobID string, err error) {
	meta := dispatchMeta{
		Target: j.Target, Path: j.Path, Code: j.Code, Subject: j.Subject,
		MessageGroup: j.MessageGroup, Mode: j.Mode,
		CorrelationID: j.CorrelationID, CausationID: j.CausationID,
		ContentType: j.ContentType, Descriptor: j.Descriptor,
		MaxRetries: j.MaxRetries, TimeoutSeconds: j.TimeoutSeconds, DataOnly: j.DataOnly,
		Metadata: j.metadataEntries(),
	}
	mb, merr := json.Marshal(meta)
	if merr != nil {
		return "", merr
	}
	rm, _, hostErr, err := currentHost.call(opDispatch, mb, j.Payload)
	if err != nil {
		return "", err
	}
	if hostErr != nil {
		return "", hostErr
	}
	var out dispatchResult
	if len(rm) > 0 {
		if uerr := json.Unmarshal(rm, &out); uerr != nil {
			return "", uerr
		}
	}
	return out.JobID, nil
}

// DispatchOption configures a Dispatches declaration.
type DispatchOption func(*dispatchJSON)

// DispatchPool places every job for the declaration in the dispatch pool with
// this code (the rate/concurrency pool for dispatch jobs, e.g.
// "yard-process-pool"; not a function-runner pool). The platform resolves it
// on every Dispatch: the pool must exist, be ACTIVE, belong to this
// function's client (or none) and application (or none); otherwise Dispatch
// returns NOT_ALLOWED / BAD_REQUEST. Without it the client's default pool
// is used. The pool is the declaration's: a Job cannot name one.
func DispatchPool(code string) DispatchOption {
	return func(d *dispatchJSON) { d.Pool = code }
}

// DispatchConnection names the connection (by code) the declaration's jobs
// go through. REQUIRED: Dispatches panics without it. The job carries the
// connection, so pausing it (a system down for maintenance) holds the jobs
// PENDING until it is reactivated, and the connection's service account signs
// their delivery. The platform resolves it on every Dispatch: it must exist
// and be visible to this function's client and application; a PAUSED
// connection does not refuse the call, the job is created and waits.
func DispatchConnection(code string) DispatchOption {
	return func(d *dispatchJSON) { d.Connection = code }
}

// Dispatches declares a (function, path) this function may dispatch jobs to
// with Dispatch. The target must be a function of the same application and
// path a literal webhook path on it. DispatchConnection is required (it
// panics at registration, so build, describe and deploy all fail with the
// message); DispatchPool is optional (default: the client's default pool). A
// platform that predates them refuses the publish rather than ignoring them.
func Dispatches(target, path string, opts ...DispatchOption) {
	d := dispatchJSON{Target: target, Path: path}
	for _, o := range opts {
		o(&d)
	}
	if d.Connection == "" {
		panic(fmt.Sprintf("fn: Dispatches(%q, %q) declares no connection; add fn.DispatchConnection(\"<connection code>\") so the jobs go through a connection that can be paused", target, path))
	}
	reg.mu.Lock()
	reg.dispatches = append(reg.dispatches, d)
	reg.mu.Unlock()
}
