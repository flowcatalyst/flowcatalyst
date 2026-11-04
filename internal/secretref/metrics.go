package secretref

import "github.com/prometheus/client_golang/prometheus"

// MetricsRegistry holds the resolver's Prometheus series, in a registry of its
// own like every other subsystem's; the server mounts it on the metrics
// listener's /metrics.
//
//	fc_secret_reference_resolve_total{outcome}  hit, miss (asked AWS), stale (served past its TTL after a failed refresh)
//	fc_secret_reference_errors_total{code}      resolutions that failed, by SECRET_REFERENCE_* code
//
// Labels never carry a reference or a value.
var MetricsRegistry = prometheus.NewRegistry()

// The outcomes of a resolution that returned a value.
const (
	outcomeHit   = "hit"
	outcomeMiss  = "miss"
	outcomeStale = "stale"
)

var (
	resolves = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fc_secret_reference_resolve_total",
		Help: "Secret reference resolutions that returned a value, by outcome (hit, miss, stale).",
	}, []string{"outcome"})
	resolveErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fc_secret_reference_errors_total",
		Help: "Secret reference resolutions that failed, by SECRET_REFERENCE_* code.",
	}, []string{"code"})
)

func init() {
	for _, o := range []string{outcomeHit, outcomeMiss, outcomeStale} {
		resolves.WithLabelValues(o)
	}
	for _, c := range []string{CodeNotFound, CodeDenied, CodeKeyMissing, CodeUnavailable, CodeNotAllowed} {
		resolveErrors.WithLabelValues(c)
	}
	MetricsRegistry.MustRegister(resolves, resolveErrors)
}
