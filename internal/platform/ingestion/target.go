package ingestion

// Delivery modes of a target's jobs, within one payload's records.
const (
	ModeImmediate    = "IMMEDIATE"      // no order
	ModeNextOnError  = "NEXT_ON_ERROR"  // in order; a failed job is retried later and the rest go on
	ModeBlockOnError = "BLOCK_ON_ERROR" // in order; a failed job holds the rest
)

// Target is the dispatch job each record becomes. The job's client is the
// ingestion's client. Pool, Connection and ServiceAccount are codes, resolved
// to ids at save by a Resolver; a definition file therefore ports between
// environments. Connection and ServiceAccount are exclusive: a connection's
// own service account signs, and neither means unsigned delivery.
type Target struct {
	Code           string  `json:"code"`
	TargetURL      string  `json:"targetUrl"`
	Pool           string  `json:"pool"`
	Connection     *string `json:"connection,omitempty"`
	ServiceAccount *string `json:"serviceAccount,omitempty"`
	Mode           string  `json:"mode"`
	BatchSize      int     `json:"batchSize"`
}
