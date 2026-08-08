package persistence

import (
	"strings"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

func TestProcessingResultValidate(t *testing.T) {
	valid := validProcessingResult(t)

	tests := []struct {
		name    string
		modify  func(*ProcessingResult)
		wantErr string
	}{
		{name: "valid result"},
		{
			name: "blank event ID",
			modify: func(result *ProcessingResult) {
				result.Telemetry.Event.EventID = ""
			},
			wantErr: "event ID",
		},
		{
			name: "non-UTC telemetry timestamp",
			modify: func(result *ProcessingResult) {
				result.Telemetry.Event.Timestamp = result.Telemetry.Event.Timestamp.In(time.FixedZone("CEST", 2*60*60))
			},
			wantErr: "telemetry timestamp must use UTC",
		},
		{
			name: "zero received time",
			modify: func(result *ProcessingResult) {
				result.Telemetry.ReceivedAt = time.Time{}
			},
			wantErr: "telemetry received time must not be zero",
		},
		{
			name: "latest state from another event",
			modify: func(result *ProcessingResult) {
				result.LatestState.LastEventID = "event-002"
			},
			wantErr: "latest state must match",
		},
		{
			name: "command linked to another event",
			modify: func(result *ProcessingResult) {
				result.Command.EventID = "event-002"
			},
			wantErr: "command event ID must match",
		},
		{
			name: "invalid command",
			modify: func(result *ProcessingResult) {
				result.Command.Command.Reason = ""
			},
			wantErr: "invalid command record",
		},
		{
			name: "non-UTC command creation time",
			modify: func(result *ProcessingResult) {
				result.Command.CreatedAt = result.Command.CreatedAt.In(time.FixedZone("CEST", 2*60*60))
			},
			wantErr: "command creation time must use UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := valid
			if tt.modify != nil {
				tt.modify(&result)
			}

			err := result.Validate()
			if tt.wantErr == "" && err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("Validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func validProcessingResult(t *testing.T) ProcessingResult {
	t.Helper()

	event := household.Telemetry{
		EventID:           "event-001",
		Timestamp:         time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC),
		DeviceID:          "home-001",
		PVPowerKW:         4.8,
		LoadPowerKW:       1.9,
		BatterySOCPercent: 61,
		PriceEURPerKWh:    0.28,
	}
	var state household.State
	if err := state.ApplyTelemetry(event); err != nil {
		t.Fatalf("ApplyTelemetry() error = %v", err)
	}

	return ProcessingResult{
		Telemetry: TelemetryRecord{
			Event:      event,
			ReceivedAt: event.Timestamp.Add(time.Second),
		},
		LatestState: state,
		Command: CommandRecord{
			EventID: event.EventID,
			Command: household.Command{
				Decision: household.DecisionCharge,
				PowerKW:  2.9,
				Reason:   "PV production exceeds household load",
			},
			CreatedAt: event.Timestamp.Add(2 * time.Second),
		},
	}
}
