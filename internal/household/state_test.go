package household

import (
	"testing"
	"time"
)

func TestStateApplyTelemetryInitializesState(t *testing.T) {
	event := validTelemetry()
	var state State

	if err := state.ApplyTelemetry(event); err != nil {
		t.Fatalf("ApplyTelemetry() error = %v", err)
	}

	want := stateFromTelemetry(event)
	if state != want {
		t.Errorf("ApplyTelemetry() state = %+v, want %+v", state, want)
	}
}

func TestStateApplyTelemetryRejectsInvalidInitialization(t *testing.T) {
	event := validTelemetry()
	event.PriceEURPerKWh = 0
	var state State

	if err := state.ApplyTelemetry(event); err == nil {
		t.Fatal("ApplyTelemetry() error = nil, want error")
	}
	if state != (State{}) {
		t.Errorf("ApplyTelemetry() state = %+v, want zero state", state)
	}
}

func TestStateApplyTelemetryReplacesLatestValues(t *testing.T) {
	first := validTelemetry()
	second := Telemetry{
		EventID:           "event-002",
		Timestamp:         first.Timestamp.Add(15 * time.Minute),
		DeviceID:          first.DeviceID,
		PVPowerKW:         5.2,
		LoadPowerKW:       2.1,
		BatterySOCPercent: 68,
		PriceEURPerKWh:    0.24,
	}
	var state State

	if err := state.ApplyTelemetry(first); err != nil {
		t.Fatalf("first ApplyTelemetry() error = %v", err)
	}
	if err := state.ApplyTelemetry(second); err != nil {
		t.Fatalf("second ApplyTelemetry() error = %v", err)
	}

	want := stateFromTelemetry(second)
	if state != want {
		t.Errorf("ApplyTelemetry() state = %+v, want %+v", state, want)
	}
}

func TestStateApplyTelemetryRejectsEventWithoutChangingState(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Telemetry)
	}{
		{name: "invalid telemetry", modify: func(event *Telemetry) {
			event.PriceEURPerKWh = 0
		}},
		{name: "different device", modify: func(event *Telemetry) {
			event.DeviceID = "home-002"
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := validTelemetry()
			var state State
			if err := state.ApplyTelemetry(first); err != nil {
				t.Fatalf("initial ApplyTelemetry() error = %v", err)
			}
			before := state

			next := first
			next.Timestamp = next.Timestamp.Add(15 * time.Minute)
			tt.modify(&next)

			if err := state.ApplyTelemetry(next); err == nil {
				t.Fatal("ApplyTelemetry() error = nil, want error")
			}
			if state != before {
				t.Errorf("ApplyTelemetry() changed state to %+v, want %+v", state, before)
			}
		})
	}
}

func stateFromTelemetry(event Telemetry) State {
	return State{
		LastEventID:       event.EventID,
		DeviceID:          event.DeviceID,
		UpdatedAt:         event.Timestamp,
		PVPowerKW:         event.PVPowerKW,
		LoadPowerKW:       event.LoadPowerKW,
		BatterySOCPercent: event.BatterySOCPercent,
		PriceEURPerKWh:    event.PriceEURPerKWh,
	}
}
