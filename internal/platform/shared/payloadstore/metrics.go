package payloadstore

import "github.com/prometheus/client_golang/prometheus"

// MetricsRegistry holds the payload store's Prometheus series
// (docs/payload-offload-design.md, section 13), in a registry of its own like
// every other subsystem's; the server mounts it on the metrics listener's
// /metrics.
//
//	fc_payload_offloaded_total                 payloads written to the store
//	fc_payload_offloaded_bytes{form}           their bytes: raw (as sent) and stored (compressed)
//	fc_payload_store_put_seconds               segment writes
//	fc_payload_store_get_seconds               range reads
//	fc_payload_store_errors_total{operation}   store failures: put, get
//	fc_payload_inline_fallback_total           payloads kept in the database because the store could not be written
//	fc_payload_cache_hits_total                resolutions served by the cache
//	fc_payload_missing_total                   payloads that could not be produced (ErrMissing)
//
// Labels never carry a key, an id or a payload.
var MetricsRegistry = prometheus.NewRegistry()

// The values of the form and operation labels.
const (
	formRaw    = "raw"
	formStored = "stored"
	opPut      = "put"
	opGet      = "get"
)

var (
	offloadedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "fc_payload_offloaded_total",
		Help: "Payloads written to the payload store.",
	})
	offloadedBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fc_payload_offloaded_bytes",
		Help: "Bytes of the payloads written to the payload store: raw (as sent) and stored (compressed).",
	}, []string{"form"})
	storePutSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "fc_payload_store_put_seconds",
		Help:    "Time to write one segment to the payload store.",
		Buckets: prometheus.DefBuckets,
	})
	storeGetSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "fc_payload_store_get_seconds",
		Help:    "Time to read one payload's range from the payload store.",
		Buckets: prometheus.DefBuckets,
	})
	storeErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fc_payload_store_errors_total",
		Help: "Payload store operations that failed, by operation (put, get). A missing object is counted in fc_payload_missing_total instead.",
	}, []string{"operation"})
	inlineFallbackTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "fc_payload_inline_fallback_total",
		Help: "Payloads over the offload threshold kept in the database because the payload store could not be written.",
	})
	cacheHitsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "fc_payload_cache_hits_total",
		Help: "Offloaded payloads resolved from the in-process cache.",
	})
	missingTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "fc_payload_missing_total",
		Help: "Stored payloads that could not be produced: row or object gone, or the bytes did not match the reference.",
	})
)

func init() {
	for _, f := range []string{formRaw, formStored} {
		offloadedBytes.WithLabelValues(f)
	}
	for _, op := range []string{opPut, opGet} {
		storeErrors.WithLabelValues(op)
	}
	MetricsRegistry.MustRegister(offloadedTotal, offloadedBytes, storePutSeconds, storeGetSeconds,
		storeErrors, inlineFallbackTotal, cacheHitsTotal, missingTotal)
}
