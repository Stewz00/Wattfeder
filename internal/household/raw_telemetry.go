package household

import (
	"errors"
	"time"
)

// RawTelemetry is one source observation whose measurements may be absent.
// Validate converts it to concrete Telemetry, distinguishing an absent measurement
// from one that is present but fails domain validation.
type RawTelemetry struct {
	EventID           EventID
	EventTime         time.Time
	DeviceID          string
	PVPowerKW         *float64
	LoadPowerKW       *float64
	BatterySOCPercent *float64
	PriceEURPerKWh    *float64
}

// Validate reports a missing-measurement error before validating any present values,
// then delegates to Telemetry.Validate for domain range and format checks.
func (r RawTelemetry) Validate() (Telemetry, error) {
	switch {
	case r.PVPowerKW == nil:
		return Telemetry{}, errors.New("PV power measurement is missing")
	case r.LoadPowerKW == nil:
		return Telemetry{}, errors.New("load power measurement is missing")
	case r.BatterySOCPercent == nil:
		return Telemetry{}, errors.New("battery SOC measurement is missing")
	case r.PriceEURPerKWh == nil:
		return Telemetry{}, errors.New("electricity price measurement is missing")
	}

	event := Telemetry{
		EventID:           r.EventID,
		EventTime:         r.EventTime,
		DeviceID:          r.DeviceID,
		PVPowerKW:         *r.PVPowerKW,
		LoadPowerKW:       *r.LoadPowerKW,
		BatterySOCPercent: *r.BatterySOCPercent,
		PriceEURPerKWh:    *r.PriceEURPerKWh,
	}
	if err := event.Validate(); err != nil {
		return Telemetry{}, err
	}

	return event, nil
}
