package observability

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
)

// allHealthStatuses lists every DeviceHealthStatus so the health gauge can hold every label at
// zero except the currently active one.
var allHealthStatuses = []household.DeviceHealthStatus{
	household.HealthOnline, household.HealthStale, household.HealthOffline, household.HealthInvalid,
}

// Metrics is an application.Observer that exports the wattfeder_* series on a private Prometheus
// registry — never the global default, so more than one Metrics instance never collides.
type Metrics struct {
	registry *prometheus.Registry

	telemetryReceived  prometheus.Counter
	telemetryProcessed *prometheus.CounterVec
	commandsCreated    *prometheus.CounterVec
	deviceHealth       *prometheus.GaugeVec
	processingDuration prometheus.Histogram
	eventLag           prometheus.Gauge
}

// NewMetrics registers the wattfeder_* collector set on a fresh, private registry.
func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		telemetryReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "wattfeder_telemetry_received_total",
			Help: "Total telemetry envelopes that arrived.",
		}),
		telemetryProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "wattfeder_telemetry_processed_total",
			Help: "Total intervals processed, one per interval, labeled by disposition.",
		}, []string{"disposition"}),
		commandsCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "wattfeder_commands_created_total",
			Help: "Total commands created, labeled by decision.",
		}, []string{"decision"}),
		deviceHealth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "wattfeder_device_health",
			Help: "1 on the active health status, 0 elsewhere.",
		}, []string{"status"}),
		processingDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "wattfeder_processing_duration_seconds",
			Help: "Interval processing duration: source, classify, commit, apply, write.",
		}),
		eventLag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "wattfeder_event_lag_seconds",
			Help: "Receive time minus event time for the most recently timestamped telemetry.",
		}),
	}

	m.registry.MustRegister(
		m.telemetryReceived, m.telemetryProcessed, m.commandsCreated,
		m.deviceHealth, m.processingDuration, m.eventLag,
	)
	for _, status := range allHealthStatuses {
		m.deviceHealth.WithLabelValues(string(status)).Set(0)
	}

	return m
}

// Registry exposes the private registry so the ops server can serve it.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// BeginInterval starts timing one interval; the returned EndInterval updates every collector
// from the resulting Record.
func (m *Metrics) BeginInterval(ctx context.Context) (context.Context, application.EndInterval) {
	start := time.Now()
	return ctx, func(record application.Record, _ error) {
		m.observe(record, time.Since(start))
	}
}

func (m *Metrics) observe(record application.Record, duration time.Duration) {
	// An interval that produced no record has no disposition to label a series with, and no
	// duration worth comparing against intervals that did the full work. Its failure, if it
	// failed, is reported by the log line and the span instead.
	if record.IsZero() {
		return
	}

	if record.Disposition != household.DispositionMissing {
		m.telemetryReceived.Inc()
	}
	m.telemetryProcessed.WithLabelValues(string(record.Disposition)).Inc()
	if record.Decision != "" {
		m.commandsCreated.WithLabelValues(string(record.Decision)).Inc()
	}
	m.processingDuration.Observe(duration.Seconds())

	if record.HealthStatus != "" {
		for _, status := range allHealthStatuses {
			value := 0.0
			if status == record.HealthStatus {
				value = 1
			}
			m.deviceHealth.WithLabelValues(string(status)).Set(value)
		}
	}

	if record.Timestamp != nil {
		m.eventLag.Set(record.ReceivedAt.Sub(*record.Timestamp).Seconds())
	}
}
