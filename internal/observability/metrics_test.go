package observability

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
)

func TestMetricsExportsTheDocumentedSeries(t *testing.T) {
	m := NewMetrics()

	eventTime := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	receivedAt := eventTime.Add(3 * time.Second)

	_, end := m.BeginInterval(t.Context())
	end(application.Record{
		Disposition:  household.DispositionAccepted,
		Decision:     household.DecisionCharge,
		HealthStatus: household.HealthOnline,
		Timestamp:    &eventTime,
		ReceivedAt:   receivedAt,
	}, nil)

	want := `
# HELP wattfeder_commands_created_total Total commands created, labeled by decision.
# TYPE wattfeder_commands_created_total counter
wattfeder_commands_created_total{decision="charge"} 1
# HELP wattfeder_device_health 1 on the active health status, 0 elsewhere.
# TYPE wattfeder_device_health gauge
wattfeder_device_health{status="invalid"} 0
wattfeder_device_health{status="offline"} 0
wattfeder_device_health{status="online"} 1
wattfeder_device_health{status="stale"} 0
# HELP wattfeder_event_lag_seconds Receive time minus event time for the most recently timestamped telemetry.
# TYPE wattfeder_event_lag_seconds gauge
wattfeder_event_lag_seconds 3
# HELP wattfeder_telemetry_processed_total Total intervals processed, one per interval, labeled by disposition.
# TYPE wattfeder_telemetry_processed_total counter
wattfeder_telemetry_processed_total{disposition="accepted"} 1
# HELP wattfeder_telemetry_received_total Total telemetry envelopes that arrived.
# TYPE wattfeder_telemetry_received_total counter
wattfeder_telemetry_received_total 1
`
	if err := testutil.GatherAndCompare(m.Registry(), strings.NewReader(want),
		"wattfeder_commands_created_total", "wattfeder_device_health", "wattfeder_event_lag_seconds",
		"wattfeder_telemetry_processed_total", "wattfeder_telemetry_received_total",
	); err != nil {
		t.Errorf("metrics mismatch: %v", err)
	}
}

func TestMetricsDeviceHealthSetsExactlyOneStatusToOne(t *testing.T) {
	m := NewMetrics()
	_, end := m.BeginInterval(t.Context())
	end(application.Record{Disposition: household.DispositionAccepted, HealthStatus: household.HealthStale}, nil)

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, family := range families {
		if family.GetName() != "wattfeder_device_health" {
			continue
		}
		activeCount := 0
		for _, metric := range family.GetMetric() {
			if metric.GetGauge().GetValue() == 1 {
				activeCount++
				var status string
				for _, label := range metric.GetLabel() {
					if label.GetName() == "status" {
						status = label.GetValue()
					}
				}
				if status != "stale" {
					t.Errorf("active status = %q, want %q", status, "stale")
				}
			}
		}
		if activeCount != 1 {
			t.Errorf("active statuses = %d, want exactly 1", activeCount)
		}
	}
}

func TestMetricsSkipsTelemetryReceivedForMissingHeartbeat(t *testing.T) {
	m := NewMetrics()
	_, end := m.BeginInterval(t.Context())
	end(application.Record{Disposition: household.DispositionMissing}, nil)

	count := testutil.ToFloat64(m.telemetryReceived)
	if count != 0 {
		t.Errorf("wattfeder_telemetry_received_total = %v, want 0 (a missing heartbeat means no envelope arrived)", count)
	}
}

func TestMetricsEventLagUntouchedWithoutTelemetry(t *testing.T) {
	m := NewMetrics()
	_, end := m.BeginInterval(t.Context())
	end(application.Record{Disposition: household.DispositionMissing}, nil)

	if got := testutil.ToFloat64(m.eventLag); got != 0 {
		t.Errorf("wattfeder_event_lag_seconds = %v, want the default zero value untouched", got)
	}
}

func TestMetricsDurationMeasuresTheWholeInterval(t *testing.T) {
	m := NewMetrics()
	_, end := m.BeginInterval(t.Context())
	time.Sleep(5 * time.Millisecond)
	end(application.Record{Disposition: household.DispositionAccepted}, nil)

	if got := durationObservations(t, m); got != 1 {
		t.Fatalf("processing duration observations = %d, want 1", got)
	}
}

func TestMetricsIgnoreAnIntervalThatProducedNoRecord(t *testing.T) {
	m := NewMetrics()
	_, end := m.BeginInterval(t.Context())
	end(application.Record{}, nil)

	if got := testutil.ToFloat64(m.telemetryReceived); got != 0 {
		t.Errorf("wattfeder_telemetry_received_total = %v, want 0 (no envelope arrived)", got)
	}
	if got := testutil.CollectAndCount(m.telemetryProcessed); got != 0 {
		t.Errorf("wattfeder_telemetry_processed_total series = %d, want 0 (an empty disposition is not a series)", got)
	}
	if got := durationObservations(t, m); got != 0 {
		t.Errorf("processing duration observations = %d, want 0", got)
	}
}

// durationObservations reports how many intervals the duration histogram actually observed.
func durationObservations(t *testing.T, m *Metrics) uint64 {
	t.Helper()
	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, family := range families {
		if family.GetName() == "wattfeder_processing_duration_seconds" {
			return family.GetMetric()[0].GetHistogram().GetSampleCount()
		}
	}
	t.Fatal("wattfeder_processing_duration_seconds is not registered")
	return 0
}
