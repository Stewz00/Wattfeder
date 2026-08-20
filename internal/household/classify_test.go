package household

import (
	"testing"
	"time"
)

var classifyStart = time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)

const classifyInterval = 15 * time.Minute

func classifyPolicy(t *testing.T) HealthPolicy {
	t.Helper()
	policy, err := NewHealthPolicy(classifyInterval, 0, 0)
	if err != nil {
		t.Fatalf("NewHealthPolicy() error = %v", err)
	}
	return policy
}

func classifyPriorState() State {
	var state State
	if err := state.ApplyTelemetry(Telemetry{
		EventID:           "event-000",
		EventTime:         classifyStart,
		DeviceID:          "home-001",
		PVPowerKW:         4.8,
		LoadPowerKW:       1.9,
		BatterySOCPercent: 61,
		PriceEURPerKWh:    0.28,
	}); err != nil {
		panic(err)
	}
	return state
}

func classifyPriorHealth() DeviceHealth {
	return DeviceHealth{
		Status:         HealthOnline,
		TransitionTime: classifyStart,
		LastContactAt:  classifyStart,
	}
}

func classifyRaw(eventTime time.Time) RawTelemetry {
	pv, load, soc, price := 5.2, 2.1, 68.0, 0.24
	return RawTelemetry{
		EventID:           "event-001",
		EventTime:         eventTime,
		DeviceID:          "home-001",
		PVPowerKW:         &pv,
		LoadPowerKW:       &load,
		BatterySOCPercent: &soc,
		PriceEURPerKWh:    &price,
	}
}

func TestClassifyAcceptsFreshStrictlyNewerEvent(t *testing.T) {
	eventTime := classifyStart.Add(classifyInterval)
	raw := classifyRaw(eventTime)
	envelope := &ObservationEnvelope{
		SourceDeviceID: "home-001",
		ReceivedAt:     eventTime,
		Telemetry:      &raw,
		Available:      true,
	}

	result := Classify(ClassifyInput{
		Envelope:    envelope,
		PriorState:  classifyPriorState(),
		PriorHealth: classifyPriorHealth(),
		Policy:      classifyPolicy(t),
		Interval:    classifyInterval,
		Now:         eventTime,
	})

	if result.Disposition != DispositionAccepted {
		t.Errorf("Disposition = %v, want %v", result.Disposition, DispositionAccepted)
	}
	if result.Telemetry == nil {
		t.Fatal("Telemetry = nil, want stored event")
	}
	if result.State == nil || result.State.UpdatedAt != eventTime {
		t.Errorf("State = %+v, want UpdatedAt %v", result.State, eventTime)
	}
	if result.SuppressCommand {
		t.Error("SuppressCommand = true, want false for a fresh accepted event")
	}
	if result.Health.Status != HealthOnline {
		t.Errorf("Health.Status = %v, want %v", result.Health.Status, HealthOnline)
	}
	if result.Health.LastContactAt != eventTime {
		t.Errorf("Health.LastContactAt = %v, want %v", result.Health.LastContactAt, eventTime)
	}
}

func TestClassifyAcceptsDelayedStrictlyNewerEvent(t *testing.T) {
	eventTime := classifyStart.Add(classifyInterval)
	receivedAt := eventTime.Add(classifyInterval + time.Minute)
	raw := classifyRaw(eventTime)
	envelope := &ObservationEnvelope{
		SourceDeviceID: "home-001",
		ReceivedAt:     receivedAt,
		Telemetry:      &raw,
		Available:      true,
	}

	result := Classify(ClassifyInput{
		Envelope:    envelope,
		PriorState:  classifyPriorState(),
		PriorHealth: classifyPriorHealth(),
		Policy:      classifyPolicy(t),
		Interval:    classifyInterval,
		Now:         receivedAt,
	})

	if result.Disposition != DispositionAccepted {
		t.Errorf("Disposition = %v, want %v", result.Disposition, DispositionAccepted)
	}
	if result.State == nil || result.State.UpdatedAt != eventTime {
		t.Errorf("State = %+v, want UpdatedAt %v", result.State, eventTime)
	}
	if !result.SuppressCommand {
		t.Error("SuppressCommand = false, want true for a delayed accepted event")
	}
	if result.Health.Status != HealthStale {
		t.Errorf("Health.Status = %v, want %v", result.Health.Status, HealthStale)
	}
}

func TestClassifyHistoryOnlyForEqualOrOlderEventTime(t *testing.T) {
	tests := []struct {
		name      string
		eventTime time.Time
	}{
		{name: "equal to latest", eventTime: classifyStart},
		{name: "older than latest", eventTime: classifyStart.Add(-time.Minute)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedAt := classifyStart.Add(time.Minute)
			raw := classifyRaw(tt.eventTime)
			envelope := &ObservationEnvelope{
				SourceDeviceID: "home-001",
				ReceivedAt:     receivedAt,
				Telemetry:      &raw,
				Available:      true,
			}

			result := Classify(ClassifyInput{
				Envelope:    envelope,
				PriorState:  classifyPriorState(),
				PriorHealth: classifyPriorHealth(),
				Policy:      classifyPolicy(t),
				Interval:    classifyInterval,
				Now:         receivedAt,
			})

			if result.Disposition != DispositionHistoryOnly {
				t.Errorf("Disposition = %v, want %v", result.Disposition, DispositionHistoryOnly)
			}
			if result.Reason == "" {
				t.Error("Reason is empty, want a structured reason")
			}
			if result.Telemetry == nil {
				t.Fatal("Telemetry = nil, want stored event")
			}
			if result.State != nil {
				t.Errorf("State = %+v, want nil (latest state preserved)", result.State)
			}
			if !result.SuppressCommand {
				t.Error("SuppressCommand = false, want true")
			}
			if result.Health.Status != HealthOnline {
				t.Errorf("Health.Status = %v, want %v", result.Health.Status, HealthOnline)
			}
		})
	}
}

func TestClassifyHistoryOnlyPreservesUnresolvedInvalidHealth(t *testing.T) {
	receivedAt := classifyStart.Add(time.Minute)
	raw := classifyRaw(classifyStart)
	envelope := &ObservationEnvelope{
		SourceDeviceID: "home-001",
		ReceivedAt:     receivedAt,
		Telemetry:      &raw,
		Available:      true,
	}
	priorHealth := DeviceHealth{
		Status:         HealthInvalid,
		Reason:         "previous event was rejected",
		TransitionTime: classifyStart,
		LastContactAt:  classifyStart,
	}

	result := Classify(ClassifyInput{
		Envelope:    envelope,
		PriorState:  classifyPriorState(),
		PriorHealth: priorHealth,
		Policy:      classifyPolicy(t),
		Interval:    classifyInterval,
		Now:         receivedAt,
	})

	if result.Disposition != DispositionHistoryOnly {
		t.Errorf("Disposition = %v, want %v", result.Disposition, DispositionHistoryOnly)
	}
	if result.Health.Status != HealthInvalid {
		t.Errorf("Health.Status = %v, want %v (must not clear unresolved invalid state)", result.Health.Status, HealthInvalid)
	}
	if result.Health.LastContactAt != receivedAt {
		t.Errorf("Health.LastContactAt = %v, want %v (historical valid telemetry still counts as contact)", result.Health.LastContactAt, receivedAt)
	}
}

func TestClassifyRejectsAndMarksHealthInvalid(t *testing.T) {
	receivedAt := classifyStart.Add(classifyInterval)
	tests := []struct {
		name   string
		modify func(*RawTelemetry)
	}{
		{name: "missing value", modify: func(raw *RawTelemetry) { raw.PVPowerKW = nil }},
		{name: "invalid measurement", modify: func(raw *RawTelemetry) {
			negative := -1.0
			raw.PVPowerKW = &negative
		}},
		{name: "device mismatch", modify: func(raw *RawTelemetry) { raw.DeviceID = "home-002" }},
		{name: "future event time", modify: func(raw *RawTelemetry) {
			raw.EventTime = receivedAt.Add(time.Minute)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := classifyRaw(classifyStart.Add(classifyInterval))
			tt.modify(&raw)
			envelope := &ObservationEnvelope{
				SourceDeviceID: "home-001",
				ReceivedAt:     receivedAt,
				Telemetry:      &raw,
				Available:      true,
			}

			result := Classify(ClassifyInput{
				Envelope:    envelope,
				PriorState:  classifyPriorState(),
				PriorHealth: classifyPriorHealth(),
				Policy:      classifyPolicy(t),
				Interval:    classifyInterval,
				Now:         receivedAt,
			})

			if result.Disposition != DispositionRejected {
				t.Errorf("Disposition = %v, want %v", result.Disposition, DispositionRejected)
			}
			if result.Telemetry != nil {
				t.Errorf("Telemetry = %+v, want nil (rejected events are not stored)", result.Telemetry)
			}
			if result.State != nil {
				t.Errorf("State = %+v, want nil", result.State)
			}
			if !result.SuppressCommand {
				t.Error("SuppressCommand = false, want true")
			}
			if result.Reason == "" {
				t.Error("Reason is empty, want a structured reason")
			}
			if result.Health.Status != HealthInvalid {
				t.Errorf("Health.Status = %v, want %v", result.Health.Status, HealthInvalid)
			}
		})
	}
}

func TestClassifyRejectsMalformedEnvelope(t *testing.T) {
	eventTime := classifyStart.Add(classifyInterval)
	receivedAt := eventTime
	now := receivedAt.Add(time.Minute)

	tests := []struct {
		name       string
		modify     func(*ObservationEnvelope)
		wantHealth time.Time
	}{
		{
			name:       "empty source device ID",
			modify:     func(envelope *ObservationEnvelope) { envelope.SourceDeviceID = "" },
			wantHealth: receivedAt,
		},
		{
			name:       "zero receive time",
			modify:     func(envelope *ObservationEnvelope) { envelope.ReceivedAt = time.Time{} },
			wantHealth: now,
		},
		{
			name: "non-UTC receive time",
			modify: func(envelope *ObservationEnvelope) {
				envelope.ReceivedAt = envelope.ReceivedAt.In(time.FixedZone("CEST", 2*60*60))
			},
			wantHealth: now,
		},
		{
			name:       "unavailable source with telemetry",
			modify:     func(envelope *ObservationEnvelope) { envelope.Available = false },
			wantHealth: receivedAt,
		},
		{
			name:       "available source without telemetry",
			modify:     func(envelope *ObservationEnvelope) { envelope.Telemetry = nil },
			wantHealth: receivedAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := classifyRaw(eventTime)
			envelope := &ObservationEnvelope{
				SourceDeviceID: "home-001",
				ReceivedAt:     receivedAt,
				Telemetry:      &raw,
				Available:      true,
			}
			tt.modify(envelope)

			result := Classify(ClassifyInput{
				Envelope:    envelope,
				PriorState:  classifyPriorState(),
				PriorHealth: classifyPriorHealth(),
				Policy:      classifyPolicy(t),
				Interval:    classifyInterval,
				Now:         now,
			})

			if result.Disposition != DispositionRejected {
				t.Errorf("Disposition = %v, want %v", result.Disposition, DispositionRejected)
			}
			if result.Reason == "" {
				t.Error("Reason is empty, want malformed-envelope reason")
			}
			if result.Telemetry != nil || result.State != nil {
				t.Errorf("Telemetry = %+v, State = %+v, want neither", result.Telemetry, result.State)
			}
			if !result.SuppressCommand {
				t.Error("SuppressCommand = false, want true")
			}
			if result.Health.Status != HealthInvalid {
				t.Errorf("Health.Status = %v, want %v", result.Health.Status, HealthInvalid)
			}
			if result.Health.TransitionTime != tt.wantHealth || result.Health.LastContactAt != tt.wantHealth {
				t.Errorf(
					"Health times = (%v, %v), want both %v",
					result.Health.TransitionTime,
					result.Health.LastContactAt,
					tt.wantHealth,
				)
			}
		})
	}
}

func TestClassifyMissingHeartbeatBecomesStaleThenOffline(t *testing.T) {
	policy := classifyPolicy(t)
	tests := []struct {
		name       string
		elapsed    time.Duration
		wantStatus DeviceHealthStatus
	}{
		{name: "before stale threshold", elapsed: policy.StaleAfter - time.Minute, wantStatus: HealthOnline},
		{name: "at stale threshold", elapsed: policy.StaleAfter, wantStatus: HealthStale},
		{name: "at offline threshold", elapsed: policy.OfflineAfter, wantStatus: HealthOffline},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := classifyStart.Add(tt.elapsed)

			result := Classify(ClassifyInput{
				Envelope:    nil,
				PriorState:  classifyPriorState(),
				PriorHealth: classifyPriorHealth(),
				Policy:      policy,
				Interval:    classifyInterval,
				Now:         now,
			})

			if result.Disposition != DispositionMissing {
				t.Errorf("Disposition = %v, want %v", result.Disposition, DispositionMissing)
			}
			if result.Telemetry != nil {
				t.Errorf("Telemetry = %+v, want nil", result.Telemetry)
			}
			if result.State != nil {
				t.Errorf("State = %+v, want nil", result.State)
			}
			if !result.SuppressCommand {
				t.Error("SuppressCommand = false, want true")
			}
			if result.Health.Status != tt.wantStatus {
				t.Errorf("Health.Status = %v, want %v", result.Health.Status, tt.wantStatus)
			}
		})
	}
}

func TestClassifyUnavailableIsImmediatelyOffline(t *testing.T) {
	receivedAt := classifyStart.Add(time.Minute)
	envelope := &ObservationEnvelope{
		SourceDeviceID: "home-001",
		ReceivedAt:     receivedAt,
		Telemetry:      nil,
		Available:      false,
	}
	priorHealth := classifyPriorHealth()

	result := Classify(ClassifyInput{
		Envelope:    envelope,
		PriorState:  classifyPriorState(),
		PriorHealth: priorHealth,
		Policy:      classifyPolicy(t),
		Interval:    classifyInterval,
		Now:         receivedAt,
	})

	if result.Disposition != DispositionUnavailable {
		t.Errorf("Disposition = %v, want %v", result.Disposition, DispositionUnavailable)
	}
	if result.Health.Status != HealthOffline {
		t.Errorf("Health.Status = %v, want %v (unavailability forces immediate offline)", result.Health.Status, HealthOffline)
	}
	if result.Health.LastContactAt != priorHealth.LastContactAt {
		t.Errorf("Health.LastContactAt = %v, want unchanged %v", result.Health.LastContactAt, priorHealth.LastContactAt)
	}
}

func TestClassifyAcceptedEventRecoversHealthFromInvalid(t *testing.T) {
	eventTime := classifyStart.Add(classifyInterval)
	raw := classifyRaw(eventTime)
	envelope := &ObservationEnvelope{
		SourceDeviceID: "home-001",
		ReceivedAt:     eventTime,
		Telemetry:      &raw,
		Available:      true,
	}
	priorHealth := DeviceHealth{
		Status:         HealthInvalid,
		Reason:         "previous event was rejected",
		TransitionTime: classifyStart,
		LastContactAt:  classifyStart,
	}

	result := Classify(ClassifyInput{
		Envelope:    envelope,
		PriorState:  classifyPriorState(),
		PriorHealth: priorHealth,
		Policy:      classifyPolicy(t),
		Interval:    classifyInterval,
		Now:         eventTime,
	})

	if result.Health.Status != HealthOnline {
		t.Errorf("Health.Status = %v, want %v (a strictly newer valid event recovers health)", result.Health.Status, HealthOnline)
	}
}
