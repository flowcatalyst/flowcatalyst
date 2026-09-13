// Package dispatch holds how the platform names and addresses the dispatch
// queues it advertises to the router, and how it builds the router-config
// document describing them.
//
// The router is a general message router, not a FlowCatalyst component: it
// merges several config documents, of which the platform's is one. So the
// platform describes its own queues and pools here, and the router learns them
// through the config URL it already polls — no new router setting.
package dispatch

import (
	"fmt"
	"net/url"
	"strings"
)

// Settings is how this platform names and addresses the dispatch queues it
// advertises. Resolved once at startup. No queue is created or checked to
// exist here: queues are created lazily, on first publish.
type Settings struct {
	// SQS is true for FC_DISPATCH_QUEUE_TYPE=SQS (matched ignoring case).
	// Every other value, including unset, names Postgres-backed queues — what
	// fcdev runs. It alone decides whether composed names are .fifo-suffixed
	// and length-capped, so a dev/prod disagreement about queue TYPE can never
	// become a disagreement about queue IDENTITY.
	SQS bool
	// Prefix is FC_DISPATCH_QUEUE_PREFIX, e.g. "FC-staging". Required when SQS
	// is true; may be blank for a Postgres (dev) deployment.
	Prefix string
	// DatabaseURL is the queue URI every Postgres-backed queue in the document
	// shares, scheme-normalised to postgres://. Blank when SQS is true.
	DatabaseURL string
	// SQSAccountID is the AWS account parsed from FC_DISPATCH_QUEUE_URL's path.
	SQSAccountID string
	// SQSRegion is FC_DISPATCH_QUEUE_REGION when set, else the region parsed
	// from FC_DISPATCH_QUEUE_URL.
	SQSRegion string
}

// ResolveSettings resolves the dispatch queue settings, refusing to start on a
// misconfigured deployment rather than composing meaningless queue names at
// runtime.
//
// queueURL is read ONLY to derive the AWS account and region to compose this
// platform's own per-tenant queue URLs with; the single queue that URL names is
// itself unused.
func ResolveSettings(queueType, queueURL, queueRegion, prefix, databaseURL string) (Settings, error) {
	sqs := strings.EqualFold(strings.TrimSpace(queueType), "SQS")
	if !sqs {
		return Settings{SQS: false, Prefix: prefix, DatabaseURL: normalisePostgresScheme(databaseURL)}, nil
	}
	// Without a prefix the queues would be named literally "FC-{env}-…".
	if strings.TrimSpace(prefix) == "" {
		return Settings{}, fmt.Errorf(
			"FC_DISPATCH_QUEUE_PREFIX is required when FC_DISPATCH_QUEUE_TYPE=SQS: without it, dispatch " +
				"queues would be named literally \"FC-{env}-...\" instead of a real per-deployment prefix")
	}
	region := strings.TrimSpace(queueRegion)
	if region == "" {
		region = regionFromSQSURL(queueURL)
	}
	account := accountFromSQSURL(queueURL)
	// Both are interpolated straight into each composed queue URL, so a blank
	// one yields "https://sqs..amazonaws.com//FC-staging-acme-DEFAULT.fifo".
	// The router still recognises that as an SQS endpoint (the host starts
	// "sqs." and contains ".amazonaws."), so it would build a consumer against
	// a nonsense address and fail at poll time rather than at boot.
	if region == "" || account == "" {
		return Settings{}, fmt.Errorf(
			"FC_DISPATCH_QUEUE_TYPE=SQS needs an account id and region to compose queue URLs "+
				"(account=%q, region=%q, FC_DISPATCH_QUEUE_URL=%q)", account, region, queueURL)
	}
	return Settings{SQS: true, Prefix: prefix, SQSAccountID: account, SQSRegion: region}, nil
}

// QueueURIFor is the URI a composed queue name resolves to: the composed SQS
// queue URL, or the shared database URL for a Postgres-backed queue.
//
// Several Postgres queue NAMES sharing one URI is the ordinary shape — one
// database, many queue_name values — and is exactly what the router's own
// Postgres backend expects.
func (s Settings) QueueURIFor(name string) string {
	if s.SQS {
		return "https://sqs." + s.SQSRegion + ".amazonaws.com/" + s.SQSAccountID + "/" + name
	}
	return s.DatabaseURL
}

// normalisePostgresScheme rewrites postgresql:// to postgres://. The database
// URL carries the former; the queue registry only ever registers the latter,
// so without this every Postgres-backed queue in the served document would
// carry a scheme the router refuses outright — it would log "no consumer
// registered for scheme" and consume nothing at all.
func normalisePostgresScheme(databaseURL string) string {
	if after, ok := strings.CutPrefix(databaseURL, "postgresql://"); ok {
		return "postgres://" + after
	}
	return databaseURL
}

// regionFromSQSURL extracts the region from an SQS queue URL whose host is
// sqs.<region>.amazonaws.com (or sqs-fips.<region>.amazonaws.com[.cn]).
func regionFromSQSURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	parts := strings.Split(u.Host, ".")
	if len(parts) >= 4 && strings.HasPrefix(parts[0], "sqs") && parts[2] == "amazonaws" {
		return parts[1]
	}
	return ""
}

// accountFromSQSURL is the first path segment of an SQS queue URL
// (https://sqs.<region>.amazonaws.com/<account>/<queue>).
func accountFromSQSURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	for seg := range strings.SplitSeq(u.Path, "/") {
		if strings.TrimSpace(seg) != "" {
			return seg
		}
	}
	return ""
}
