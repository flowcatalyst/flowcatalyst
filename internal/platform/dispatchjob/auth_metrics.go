package dispatchjob

import "github.com/prometheus/client_golang/prometheus"

// Endpoints whose job tokens are counted by ObserveCallbackAuth.
const (
	AuthEndpointProcess = "process"
	AuthEndpointSettled = "settled"
)

// Dispatch callback authentication series, on MetricsRegistry:
//
//	fc_dispatch_job_auth_rejected_total{endpoint}      job tokens that verified under no configured key
//	fc_dispatch_job_auth_previous_key_total{endpoint}  job tokens that verified only under a previous key
//
// A rejection answers 401 and the router does not ACK, so the message comes
// back by redelivery and stale recovery, again and again, with nothing else
// to show for it. A sustained rate means the processes disagree about
// FLOWCATALYST_APP_KEY: a tier deployed with another key, or a key rotated
// without the old one in FLOWCATALYST_APP_KEY_PREVIOUS. The previous-key
// counter is the rotation's progress: once it stays at zero for longer than a
// job can sit on the queue, no in-flight token needs the old key.
var (
	authRejected    = newAuthCounter("fc_dispatch_job_auth_rejected_total", "Dispatch callback job tokens that verified under no configured key (answered 401).")
	authPreviousKey = newAuthCounter("fc_dispatch_job_auth_previous_key_total", "Dispatch callback job tokens that verified only under a previous FLOWCATALYST_APP_KEY.")
)

func newAuthCounter(name, help string) *prometheus.CounterVec {
	v := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, []string{"endpoint"})
	// Materialise every label so a zero shows up in a scrape.
	v.WithLabelValues(AuthEndpointProcess)
	v.WithLabelValues(AuthEndpointSettled)
	MetricsRegistry.MustRegister(v)
	return v
}

// ObserveCallbackAuth counts one job-token verification on a dispatch
// callback endpoint: a rejection, or a token only a previous key accepted.
func ObserveCallbackAuth(endpoint string, ok, previousKey bool) {
	switch {
	case !ok:
		authRejected.WithLabelValues(endpoint).Inc()
	case previousKey:
		authPreviousKey.WithLabelValues(endpoint).Inc()
	}
}
