package ingestion

// Delivery modes of a target's jobs, within one payload's records.
const (
	ModeImmediate    = "IMMEDIATE"      // no order
	ModeNextOnError  = "NEXT_ON_ERROR"  // in order; a failed job is retried later and the rest go on
	ModeBlockOnError = "BLOCK_ON_ERROR" // in order; a failed job holds the rest
)

// What a target makes of each record.
const (
	TargetDispatchJob = "DISPATCH_JOB" // a dispatch job, delivered to the target URL (the default)
	TargetEvent       = "EVENT"        // a platform event, delivered by the subscriptions to its type
)

// Target is what each record becomes: a dispatch job (Kind DISPATCH_JOB, the
// default) or an event (Kind EVENT). The job's client is the
// ingestion's client. Pool, Connection and ServiceAccount are codes, resolved
// to ids at save by a Resolver; a definition file therefore ports between
// environments. Connection and ServiceAccount are exclusive: a connection's
// own service account signs, and neither means unsigned delivery. TargetURL is
// empty only in a definition whose source names a config entry that gives it.
//
// An event target carries a Code (an existing event type's), BatchSize and
// WrapNonJson only: the subscriptions decide where the event goes. A dispatch
// job target may set DataOnly, to deliver the record as it came rather than in
// the platform envelope.
type Target struct {
	Kind           string  `json:"kind,omitempty" enum:"DISPATCH_JOB,EVENT" doc:"What each record becomes; DISPATCH_JOB when absent"`
	Code           string  `json:"code" doc:"A dispatch job's code, application:subdomain:aggregate:action; for an EVENT target, the code of an existing event type"`
	TargetURL      string  `json:"targetUrl,omitempty" doc:"The delivery URL. May be left out only when the source names a config entry whose value gives it (exactly one of the two). Not for an EVENT target"`
	Pool           string  `json:"pool,omitempty" doc:"Not for an EVENT target"`
	Connection     *string `json:"connection,omitempty"`
	ServiceAccount *string `json:"serviceAccount,omitempty"`
	Mode           string  `json:"mode,omitempty" doc:"Not for an EVENT target"`
	BatchSize      int     `json:"batchSize"`
	DataOnly       bool    `json:"dataOnly,omitempty" doc:"DISPATCH_JOB only: deliver the record as it is, with its own content type, instead of the platform envelope"`
	WrapNonJson    bool    `json:"wrapNonJson,omitempty" doc:"EVENT only: deliver a record that is not JSON as {contentType, body}. Required unless the records are known to be JSON"`
}

// EffectiveKind is the kind the target runs as: a target without a kind (every
// definition stored before kinds) is a dispatch job target.
func (t Target) EffectiveKind() string {
	if t.Kind == "" {
		return TargetDispatchJob
	}
	return t.Kind
}

// IsEvent reports whether the target makes events.
func (t Target) IsEvent() bool { return t.Kind == TargetEvent }
