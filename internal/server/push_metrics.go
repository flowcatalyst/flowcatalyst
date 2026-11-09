package server

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/flowcatalyst/framework/go/metrics"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
)

// frameworkGatherer exposes a framework metrics registry (the push module's fw_push_* series and
// the gateway's fc_push_* ones) to the platform's Prometheus listener: the registry renders the
// text exposition format, which is parsed back into metric families so the one /metrics handler
// serves both. A render or parse failure is an error of the gatherer, which promhttp (with
// ContinueOnError) reports without dropping the platform's own series.
type frameworkGatherer struct{ reg *metrics.Registry }

var _ prometheus.Gatherer = frameworkGatherer{}

// Gather implements prometheus.Gatherer.
func (g frameworkGatherer) Gather() ([]*dto.MetricFamily, error) {
	var buf bytes.Buffer
	if err := metrics.WritePrometheus(&buf, g.reg.Snapshot()); err != nil {
		return nil, fmt.Errorf("render the framework metrics: %w", err)
	}
	parser := expfmt.NewTextParser(model.LegacyValidation)
	byName, err := parser.TextToMetricFamilies(&buf)
	if err != nil {
		return nil, fmt.Errorf("parse the framework metrics: %w", err)
	}
	out := make([]*dto.MetricFamily, 0, len(byName))
	for _, f := range byName {
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b *dto.MetricFamily) int {
		switch {
		case a.GetName() < b.GetName():
			return -1
		case a.GetName() > b.GetName():
			return 1
		}
		return 0
	})
	return out, nil
}

// pushGatherers returns the gateway's metrics as gatherers for the metrics listener: none when the
// gateway is off.
func pushGatherers(gw *pushgw.Gateway) []prometheus.Gatherer {
	if gw == nil {
		return nil
	}
	reg := gw.Metrics()
	if reg == nil {
		return nil
	}
	return []prometheus.Gatherer{frameworkGatherer{reg: reg}}
}
