package outbox

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Dispatch-job kinds, modes and queues the batch endpoint accepts.
const (
	DispatchKindEvent = "EVENT"
	DispatchKindTask  = "TASK"

	DispatchModeImmediate    = "IMMEDIATE"
	DispatchModeNextOnError  = "NEXT_ON_ERROR"
	DispatchModeBlockOnError = "BLOCK_ON_ERROR"

	DispatchQueueDefault      = "DEFAULT"
	DispatchQueueHighPriority = "HIGH_PRIORITY"
)

var (
	dispatchCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:.+$`)
	platformIDPattern   = regexp.MustCompile(`^[0-9A-Za-z_-]{1,17}$`)
)

// MetadataEntry is one dispatch-job metadata pair; the batch endpoint reads
// metadata as an array of {"key","value"}, in order.
type MetadataEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DispatchJob is a job for POST /api/dispatch-jobs/batch, written to the
// outbox in the same transaction as the work that asks for it. Build it with
// NewDispatchJob and the With* methods, then Validate (the writers validate
// before writing, so a row the platform would refuse terminally is never
// stored). Zero values mean "not set" and are left out of the payload.
type DispatchJob struct {
	// ID is the job id, [0-9A-Za-z_-]{1,17}; generated (a 13-character TSID)
	// when empty. It is in the payload, so a batch the processor sends again
	// creates a job with the same id.
	ID        string
	Kind      string // EVENT or TASK; default TASK
	Code      string // {application}:{rest}, at most 255 characters
	Source    string // default: the code's application segment
	Subject   string
	TargetURL string // absolute http(s) URL with a host and no user info

	// Payload is the body delivered to TargetURL, with PayloadContentType.
	// Default: {} as application/json.
	Payload            string
	PayloadContentType string
	DataOnly           bool

	ExternalID    string
	EventID       string
	CorrelationID string

	// PlatformClientID is the platform's client id (never a slug). ClientCode
	// is the client's identifier slug, which the platform resolves at ingest
	// (an unknown code refuses the whole batch: 400 for an anchor, 403
	// otherwise). Set at most one; without either the job has no client.
	PlatformClientID string
	ClientCode       string

	SubscriptionID   string
	ServiceAccountID string
	DispatchPoolID   string
	MessageGroup     string
	Mode             string // IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR
	// Sequence, TimeoutSeconds and MaxRetries are at least 1 when set (the
	// platform turns 0 into its default, so 0 cannot be sent); nil leaves
	// them to the platform (99, 30 and 3).
	Sequence       *int
	TimeoutSeconds *int
	MaxRetries     *int
	Metadata       []MetadataEntry
	Queue          string // DEFAULT or HIGH_PRIORITY
	Descriptor     string

	payloadSet bool
}

// NewDispatchJob starts a job for code delivered to targetURL.
func NewDispatchJob(code, targetURL string) DispatchJob {
	return DispatchJob{Code: code, TargetURL: targetURL}
}

func (j DispatchJob) WithID(id string) DispatchJob            { j.ID = id; return j }
func (j DispatchJob) WithKind(kind string) DispatchJob        { j.Kind = kind; return j }
func (j DispatchJob) WithSource(source string) DispatchJob    { j.Source = source; return j }
func (j DispatchJob) WithSubject(subject string) DispatchJob  { j.Subject = subject; return j }
func (j DispatchJob) DataOnlyEnabled() DispatchJob            { j.DataOnly = true; return j }
func (j DispatchJob) WithExternalID(id string) DispatchJob    { j.ExternalID = id; return j }
func (j DispatchJob) WithEventID(id string) DispatchJob       { j.EventID = id; return j }
func (j DispatchJob) WithCorrelationID(id string) DispatchJob { j.CorrelationID = id; return j }
func (j DispatchJob) WithPlatformClientID(id string) DispatchJob {
	j.PlatformClientID = id
	return j
}
func (j DispatchJob) WithClientCode(code string) DispatchJob     { j.ClientCode = code; return j }
func (j DispatchJob) WithSubscriptionID(id string) DispatchJob   { j.SubscriptionID = id; return j }
func (j DispatchJob) WithServiceAccountID(id string) DispatchJob { j.ServiceAccountID = id; return j }
func (j DispatchJob) WithDispatchPoolID(id string) DispatchJob   { j.DispatchPoolID = id; return j }
func (j DispatchJob) WithMessageGroup(g string) DispatchJob      { j.MessageGroup = g; return j }
func (j DispatchJob) WithMode(mode string) DispatchJob           { j.Mode = mode; return j }
func (j DispatchJob) WithSequence(n int) DispatchJob             { j.Sequence = &n; return j }
func (j DispatchJob) WithTimeoutSeconds(n int) DispatchJob       { j.TimeoutSeconds = &n; return j }
func (j DispatchJob) WithMaxRetries(n int) DispatchJob           { j.MaxRetries = &n; return j }
func (j DispatchJob) WithQueue(q string) DispatchJob             { j.Queue = q; return j }
func (j DispatchJob) WithDescriptor(d string) DispatchJob        { j.Descriptor = d; return j }

// WithMetadata appends one metadata pair.
func (j DispatchJob) WithMetadata(key, value string) DispatchJob {
	j.Metadata = append(append([]MetadataEntry(nil), j.Metadata...), MetadataEntry{Key: key, Value: value})
	return j
}

// WithPayload sets the delivered body and its content type.
func (j DispatchJob) WithPayload(text, contentType string) DispatchJob {
	j.Payload, j.PayloadContentType, j.payloadSet = text, contentType, true
	return j
}

// WithJSONPayload sets the delivered body to v serialised as JSON
// (application/json).
func (j DispatchJob) WithJSONPayload(v any) (DispatchJob, error) {
	b, err := marshal(v)
	if err != nil {
		return j, fmt.Errorf("dispatch job payload: %w", err)
	}
	return j.WithPayload(string(b), "application/json"), nil
}

// Validate applies the batch endpoint's rules (framework write-path 7a).
func (j DispatchJob) Validate() error {
	if !dispatchCodePattern.MatchString(j.Code) || utf8.RuneCountInString(j.Code) > 255 {
		return fmt.Errorf("dispatch job code must be {application}:{rest} (application [a-z][a-z0-9-]*), at most 255 characters: %s", j.Code)
	}
	if !validTargetURL(j.TargetURL) {
		return errors.New("dispatch job targetUrl must be an absolute http(s) URL with a host and no user info")
	}
	if j.ID != "" && !platformIDPattern.MatchString(j.ID) {
		return errors.New("id must match [0-9A-Za-z_-]{1,17}")
	}
	if j.Kind != "" && j.Kind != DispatchKindEvent && j.Kind != DispatchKindTask {
		return errors.New("kind must be EVENT or TASK")
	}
	for name, v := range map[string]string{
		"eventId": j.EventID, "platformClientId": j.PlatformClientID, "subscriptionId": j.SubscriptionID,
		"serviceAccountId": j.ServiceAccountID, "dispatchPoolId": j.DispatchPoolID,
	} {
		if v != "" && !platformIDPattern.MatchString(v) {
			return fmt.Errorf("%s must match [0-9A-Za-z_-]{1,17}", name)
		}
	}
	if j.PlatformClientID != "" && j.ClientCode != "" {
		return errors.New("set platformClientId or clientCode, not both (the platform ignores the code when an id is given)")
	}
	if j.ClientCode != "" && (strings.TrimSpace(j.ClientCode) == "" || utf8.RuneCountInString(j.ClientCode) > 100) {
		return errors.New("clientCode must be 1 to 100 characters")
	}
	for _, n := range []struct {
		name string
		v    *int
	}{{"sequence", j.Sequence}, {"timeoutSeconds", j.TimeoutSeconds}, {"maxRetries", j.MaxRetries}} {
		if n.v != nil && *n.v < 1 {
			return fmt.Errorf("%s must be at least 1 (the platform turns 0 into its default)", n.name)
		}
	}
	if j.Source != "" && (strings.TrimSpace(j.Source) == "" || utf8.RuneCountInString(j.Source) > 255) {
		return errors.New("source must be 1 to 255 characters")
	}
	for name, v := range map[string]string{
		"messageGroup": j.MessageGroup, "externalId": j.ExternalID, "subject": j.Subject, "descriptor": j.Descriptor,
	} {
		if v != "" && (strings.TrimSpace(v) == "" || utf8.RuneCountInString(v) > 255) {
			return fmt.Errorf("%s must be 1 to 255 characters", name)
		}
	}
	if j.Mode != "" && j.Mode != DispatchModeImmediate && j.Mode != DispatchModeNextOnError && j.Mode != DispatchModeBlockOnError {
		return errors.New("mode must be IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR")
	}
	if j.Queue != "" && j.Queue != DispatchQueueDefault && j.Queue != DispatchQueueHighPriority {
		return errors.New("queue must be DEFAULT or HIGH_PRIORITY")
	}
	if len(j.Metadata) > 100 {
		return errors.New("metadata must have at most 100 entries")
	}
	for _, m := range j.Metadata {
		if strings.TrimSpace(m.Key) == "" {
			return errors.New("metadata keys must not be blank")
		}
	}
	if j.payloadSet && (j.PayloadContentType == "" || utf8.RuneCountInString(j.PayloadContentType) > 100) {
		return errors.New("payloadContentType must be 1 to 100 characters")
	}
	return nil
}

func validTargetURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Hostname() != "" && u.User == nil
}

// dispatchJobPayload is the batch item, members in the framework's order.
type dispatchJobPayload struct {
	ID                 string          `json:"id"`
	Kind               string          `json:"kind"`
	Code               string          `json:"code"`
	Source             string          `json:"source"`
	Subject            string          `json:"subject,omitempty"`
	TargetURL          string          `json:"targetUrl"`
	Payload            string          `json:"payload"`
	PayloadContentType string          `json:"payloadContentType"`
	DataOnly           bool            `json:"dataOnly"`
	ExternalID         string          `json:"externalId,omitempty"`
	EventID            string          `json:"eventId,omitempty"`
	CorrelationID      string          `json:"correlationId,omitempty"`
	ClientID           string          `json:"clientId,omitempty"`
	ClientCode         string          `json:"clientCode,omitempty"`
	SubscriptionID     string          `json:"subscriptionId,omitempty"`
	ServiceAccountID   string          `json:"serviceAccountId,omitempty"`
	DispatchPoolID     string          `json:"dispatchPoolId,omitempty"`
	MessageGroup       string          `json:"messageGroup,omitempty"`
	Mode               string          `json:"mode,omitempty"`
	Sequence           *int            `json:"sequence,omitempty"`
	TimeoutSeconds     *int            `json:"timeoutSeconds,omitempty"`
	MaxRetries         *int            `json:"maxRetries,omitempty"`
	Metadata           []MetadataEntry `json:"metadata,omitempty"`
	Queue              string          `json:"queue,omitempty"`
	Descriptor         string          `json:"descriptor,omitempty"`
}

// Resolved returns the job with its defaults applied (id generated, kind
// TASK, source from the code, payload {} as application/json), after
// validating it.
func (j DispatchJob) Resolved() (DispatchJob, error) {
	if err := j.Validate(); err != nil {
		return j, err
	}
	if j.ID == "" {
		j.ID = newRowID()
	}
	if j.Kind == "" {
		j.Kind = DispatchKindTask
	}
	if j.Source == "" {
		j.Source, _, _ = strings.Cut(j.Code, ":")
	}
	if !j.payloadSet {
		j.Payload, j.PayloadContentType = "{}", "application/json"
	}
	return j, nil
}

// DispatchJobPayload validates the job, applies its defaults and serialises
// it as POST /api/dispatch-jobs/batch's item. It returns the resolved job
// (its id is the row id).
func DispatchJobPayload(job DispatchJob) (DispatchJob, []byte, error) {
	j, err := job.Resolved()
	if err != nil {
		return job, nil, err
	}
	b, err := marshal(dispatchJobPayload{
		ID: j.ID, Kind: j.Kind, Code: j.Code, Source: j.Source, Subject: j.Subject,
		TargetURL: j.TargetURL, Payload: j.Payload, PayloadContentType: j.PayloadContentType,
		DataOnly: j.DataOnly, ExternalID: j.ExternalID, EventID: j.EventID,
		CorrelationID: j.CorrelationID, ClientID: j.PlatformClientID, ClientCode: strings.TrimSpace(j.ClientCode),
		SubscriptionID: j.SubscriptionID, ServiceAccountID: j.ServiceAccountID,
		DispatchPoolID: j.DispatchPoolID, MessageGroup: j.MessageGroup, Mode: j.Mode,
		Sequence: j.Sequence, TimeoutSeconds: j.TimeoutSeconds, MaxRetries: j.MaxRetries,
		Metadata: j.Metadata, Queue: j.Queue, Descriptor: j.Descriptor,
	})
	if err != nil {
		return job, nil, err
	}
	return j, b, nil
}
