package household

import (
	"testing"
)

func TestRawTelemetryValidateConvertsCompleteValidMeasurement(t *testing.T) {
	raw := validRawTelemetry()

	event, err := raw.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	want := validTelemetry()
	if event != want {
		t.Errorf("Validate() event = %+v, want %+v", event, want)
	}
}

func TestRawTelemetryValidateReportsMissingMeasurement(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*RawTelemetry)
	}{
		{name: "PV power is missing", modify: func(raw *RawTelemetry) { raw.PVPowerKW = nil }},
		{name: "load power is missing", modify: func(raw *RawTelemetry) { raw.LoadPowerKW = nil }},
		{name: "battery SOC is missing", modify: func(raw *RawTelemetry) { raw.BatterySOCPercent = nil }},
		{name: "price is missing", modify: func(raw *RawTelemetry) { raw.PriceEURPerKWh = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := validRawTelemetry()
			tt.modify(&raw)

			_, err := raw.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
		})
	}
}

func TestRawTelemetryValidateReportsInvalidMeasurement(t *testing.T) {
	negative := -1.0

	raw := validRawTelemetry()
	raw.PVPowerKW = &negative

	_, err := raw.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func validRawTelemetry() RawTelemetry {
	event := validTelemetry()
	pv, load, soc, price := event.PVPowerKW, event.LoadPowerKW, event.BatterySOCPercent, event.PriceEURPerKWh

	return RawTelemetry{
		EventID:           event.EventID,
		EventTime:         event.EventTime,
		DeviceID:          event.DeviceID,
		PVPowerKW:         &pv,
		LoadPowerKW:       &load,
		BatterySOCPercent: &soc,
		PriceEURPerKWh:    &price,
	}
}
