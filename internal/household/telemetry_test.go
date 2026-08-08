package household

import (
	"math"
	"testing"
	"time"
)

func TestTelemetryValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Telemetry)
		wantErr bool
	}{
		{name: "valid telemetry"},
		{name: "event ID is empty", modify: func(event *Telemetry) { event.EventID = "" }, wantErr: true},
		{name: "event ID contains only whitespace", modify: func(event *Telemetry) {
			event.EventID = " \t\n"
		}, wantErr: true},
		{name: "event ID has surrounding whitespace", modify: func(event *Telemetry) {
			event.EventID = " event-001 "
		}, wantErr: true},
		{name: "timestamp before Unix epoch is valid", modify: func(event *Telemetry) {
			event.Timestamp = time.Date(1960, time.January, 1, 0, 0, 0, 0, time.UTC)
		}},
		{name: "device ID with surrounding whitespace is valid", modify: func(event *Telemetry) {
			event.DeviceID = " home-001 "
		}},
		{name: "zero PV power is valid", modify: func(event *Telemetry) { event.PVPowerKW = 0 }},
		{name: "zero load power is valid", modify: func(event *Telemetry) { event.LoadPowerKW = 0 }},
		{name: "zero SOC is valid", modify: func(event *Telemetry) { event.BatterySOCPercent = 0 }},
		{name: "full SOC is valid", modify: func(event *Telemetry) { event.BatterySOCPercent = 100 }},
		{name: "smallest positive price is valid", modify: func(event *Telemetry) {
			event.PriceEURPerKWh = math.SmallestNonzeroFloat64
		}},
		{name: "timestamp is zero", modify: func(event *Telemetry) { event.Timestamp = time.Time{} }, wantErr: true},
		{name: "device ID is empty", modify: func(event *Telemetry) { event.DeviceID = "" }, wantErr: true},
		{name: "device ID contains only whitespace", modify: func(event *Telemetry) {
			event.DeviceID = " \t\n"
		}, wantErr: true},
		{name: "PV power is negative", modify: func(event *Telemetry) { event.PVPowerKW = -1 }, wantErr: true},
		{name: "PV power is NaN", modify: func(event *Telemetry) { event.PVPowerKW = math.NaN() }, wantErr: true},
		{name: "PV power is positive infinity", modify: func(event *Telemetry) {
			event.PVPowerKW = math.Inf(1)
		}, wantErr: true},
		{name: "PV power is negative infinity", modify: func(event *Telemetry) {
			event.PVPowerKW = math.Inf(-1)
		}, wantErr: true},
		{name: "load power is negative", modify: func(event *Telemetry) { event.LoadPowerKW = -1 }, wantErr: true},
		{name: "load power is NaN", modify: func(event *Telemetry) { event.LoadPowerKW = math.NaN() }, wantErr: true},
		{name: "load power is positive infinity", modify: func(event *Telemetry) {
			event.LoadPowerKW = math.Inf(1)
		}, wantErr: true},
		{name: "load power is negative infinity", modify: func(event *Telemetry) {
			event.LoadPowerKW = math.Inf(-1)
		}, wantErr: true},
		{name: "SOC is below minimum", modify: func(event *Telemetry) { event.BatterySOCPercent = -1 }, wantErr: true},
		{name: "SOC is above maximum", modify: func(event *Telemetry) { event.BatterySOCPercent = 101 }, wantErr: true},
		{name: "SOC is NaN", modify: func(event *Telemetry) { event.BatterySOCPercent = math.NaN() }, wantErr: true},
		{name: "SOC is positive infinity", modify: func(event *Telemetry) {
			event.BatterySOCPercent = math.Inf(1)
		}, wantErr: true},
		{name: "SOC is negative infinity", modify: func(event *Telemetry) {
			event.BatterySOCPercent = math.Inf(-1)
		}, wantErr: true},
		{name: "price is zero", modify: func(event *Telemetry) { event.PriceEURPerKWh = 0 }, wantErr: true},
		{name: "price is negative", modify: func(event *Telemetry) { event.PriceEURPerKWh = -1 }, wantErr: true},
		{name: "price is NaN", modify: func(event *Telemetry) { event.PriceEURPerKWh = math.NaN() }, wantErr: true},
		{name: "price is positive infinity", modify: func(event *Telemetry) {
			event.PriceEURPerKWh = math.Inf(1)
		}, wantErr: true},
		{name: "price is negative infinity", modify: func(event *Telemetry) {
			event.PriceEURPerKWh = math.Inf(-1)
		}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validTelemetry()
			if tt.modify != nil {
				tt.modify(&event)
			}

			err := event.Validate()
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func validTelemetry() Telemetry {
	return Telemetry{
		EventID:           "event-001",
		Timestamp:         time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC),
		DeviceID:          "home-001",
		PVPowerKW:         4.8,
		LoadPowerKW:       1.9,
		BatterySOCPercent: 61,
		PriceEURPerKWh:    0.28,
	}
}
