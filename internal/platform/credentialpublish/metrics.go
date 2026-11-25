package credentialpublish

import "github.com/prometheus/client_golang/prometheus"

// MetricsRegistry holds the publisher's series, in a registry of its own like
// every other subsystem's; the server mounts it on the metrics listener.
//
//	fc_credential_publish_puts_total      parameters written
//	fc_credential_publish_deletes_total   parameters deleted (or found gone, or left in place as foreign)
//	fc_credential_publish_failures_total{code}  failed attempts, by AWS error code
//	fc_credential_publish_pending         items waiting to be written or deleted
//
// Labels never carry a path, an owner or a value.
var MetricsRegistry = prometheus.NewRegistry()

var (
	putsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "fc_credential_publish_puts_total", Help: "Credential parameters written.",
	})
	deletesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "fc_credential_publish_deletes_total", Help: "Credential parameters deleted.",
	})
	failuresTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fc_credential_publish_failures_total", Help: "Failed publish attempts, by AWS error code.",
	}, []string{"code"})
	pendingGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "fc_credential_publish_pending", Help: "Publication items waiting to be written or deleted.",
	})
)

func init() {
	MetricsRegistry.MustRegister(putsTotal, deletesTotal, failuresTotal, pendingGauge)
}
