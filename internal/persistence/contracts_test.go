package persistence

import (
	"strings"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

func TestObservationResultValidateAcceptedResult(t *testing.T) {
	result := validAcceptedResult(t)

	if err := result.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestObservationResultValidateHistoryOnlyResult(t *testing.T) {
	result := validHistoryOnlyResult(t)

	if err := result.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestObservationResultValidateHealthOnlyResult(t *testing.T) {
	result := validHealthOnlyResult()

	if err := result.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestObservationResultValidateRejects(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ObservationResult)
		wantErr string
	}{
		{
			name:    "blank device ID",
			modify:  func(result *ObservationResult) { result.DeviceID = "" },
			wantErr: "device ID must not be empty",
		},
		{
			name: "invalid telemetry event",
			modify: func(result *ObservationResult) {
				result.Telemetry.Event.EventID = ""
			},
			wantErr: "invalid telemetry record",
		},
		{
			name: "non-UTC telemetry event time",
			modify: func(result *ObservationResult) {
				result.Telemetry.Event.EventTime = result.Telemetry.Event.EventTime.In(time.FixedZone("CEST", 2*60*60))
			},
			wantErr: "telemetry event time must use UTC",
		},
		{
			name: "zero telemetry received time",
			modify: func(result *ObservationResult) {
				result.Telemetry.ReceivedAt = time.Time{}
			},
			wantErr: "telemetry received time must not be zero",
		},
		{
			name: "telemetry device mismatch",
			modify: func(result *ObservationResult) {
				result.Telemetry.Event.DeviceID = "home-002"
			},
			wantErr: "telemetry device ID must match",
		},
		{
			name: "unknown telemetry disposition",
			modify: func(result *ObservationResult) {
				result.Telemetry.Disposition = household.DispositionDuplicate
			},
			wantErr: "telemetry disposition must be accepted or history_only",
		},
		{
			name: "latest state without accepted telemetry",
			modify: func(result *ObservationResult) {
				result.Telemetry.Disposition = household.DispositionHistoryOnly
			},
			wantErr: "latest state requires an accepted telemetry record",
		},
		{
			name: "latest state mismatched with telemetry",
			modify: func(result *ObservationResult) {
				result.LatestState.LastEventID = "event-002"
			},
			wantErr: "latest state must match",
		},
		{
			name: "command without latest state",
			modify: func(result *ObservationResult) {
				result.LatestState = nil
			},
			wantErr: "command requires a latest state update",
		},
		{
			name: "command linked to another event",
			modify: func(result *ObservationResult) {
				result.Command.EventID = "event-002"
			},
			wantErr: "command event ID must match",
		},
		{
			name: "invalid command",
			modify: func(result *ObservationResult) {
				result.Command.Command.Reason = ""
			},
			wantErr: "invalid command record",
		},
		{
			name: "non-UTC command creation time",
			modify: func(result *ObservationResult) {
				result.Command.CreatedAt = result.Command.CreatedAt.In(time.FixedZone("CEST", 2*60*60))
			},
			wantErr: "command creation time must use UTC",
		},
		{
			name: "unknown health status",
			modify: func(result *ObservationResult) {
				result.Health.Status = "unknown"
			},
			wantErr: "health status must be a known value",
		},
		{
			name: "zero health transition time",
			modify: func(result *ObservationResult) {
				result.Health.TransitionTime = time.Time{}
			},
			wantErr: "health transition time must not be zero",
		},
		{
			name: "non-UTC health transition time",
			modify: func(result *ObservationResult) {
				result.Health.TransitionTime = result.Health.TransitionTime.In(time.FixedZone("CEST", 2*60*60))
			},
			wantErr: "health transition time must use UTC",
		},
		{
			name: "non-UTC health last contact time",
			modify: func(result *ObservationResult) {
				result.Health.LastContactAt = result.Health.LastContactAt.In(time.FixedZone("CEST", 2*60*60))
			},
			wantErr: "health last contact time must use UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validAcceptedResult(t)
			tt.modify(&result)

			err := result.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func validTelemetryEvent() household.Telemetry {
	return household.Telemetry{
		EventID:           "event-001",
		EventTime:         time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC),
		DeviceID:          "home-001",
		PVPowerKW:         4.8,
		LoadPowerKW:       1.9,
		BatterySOCPercent: 61,
		PriceEURPerKWh:    0.28,
	}
}

func validAcceptedResult(t *testing.T) ObservationResult {
	t.Helper()

	event := validTelemetryEvent()
	var state household.State
	if err := state.ApplyTelemetry(event); err != nil {
		t.Fatalf("ApplyTelemetry() error = %v", err)
	}

	return ObservationResult{
		DeviceID: event.DeviceID,
		Telemetry: &TelemetryRecord{
			Event:             event,
			ReceivedAt:        event.EventTime.Add(time.Second),
			Disposition:       household.DispositionAccepted,
			DispositionReason: "",
		},
		LatestState: &state,
		Command: &CommandRecord{
			EventID: event.EventID,
			Command: household.Command{
				Decision: household.DecisionCharge,
				PowerKW:  2.9,
				Reason:   "PV production exceeds household load",
			},
			CreatedAt: event.EventTime.Add(2 * time.Second),
		},
		Health: household.DeviceHealth{
			Status:         household.HealthOnline,
			TransitionTime: event.EventTime.Add(time.Second),
			LastContactAt:  event.EventTime.Add(time.Second),
		},
	}
}

func validHistoryOnlyResult(t *testing.T) ObservationResult {
	t.Helper()

	event := validTelemetryEvent()

	return ObservationResult{
		DeviceID: event.DeviceID,
		Telemetry: &TelemetryRecord{
			Event:             event,
			ReceivedAt:        event.EventTime.Add(time.Second),
			Disposition:       household.DispositionHistoryOnly,
			DispositionReason: "event time is not strictly newer than the latest state",
		},
		Health: household.DeviceHealth{
			Status:         household.HealthOnline,
			TransitionTime: event.EventTime.Add(time.Second),
			LastContactAt:  event.EventTime.Add(time.Second),
		},
	}
}

func validHealthOnlyResult() ObservationResult {
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)

	return ObservationResult{
		DeviceID: "home-001",
		Health: household.DeviceHealth{
			Status:         household.HealthInvalid,
			Reason:         "future event time",
			TransitionTime: now,
			LastContactAt:  now,
		},
	}
}
