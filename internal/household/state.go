package household

import (
	"fmt"
	"time"
)

// State contains the latest accepted telemetry values for one household device.
type State struct {
	DeviceID          string
	UpdatedAt         time.Time
	PVPowerKW         float64
	LoadPowerKW       float64
	BatterySOCPercent float64
	PriceEURPerKWh    float64
}

// ApplyTelemetry validates an event and replaces the latest state for the same device.
func (s *State) ApplyTelemetry(event Telemetry) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid telemetry: %w", err)
	}

	if s.DeviceID != "" && event.DeviceID != s.DeviceID {
		return fmt.Errorf("telemetry device ID %q does not match state device ID %q", event.DeviceID, s.DeviceID)
	}

	s.DeviceID = event.DeviceID
	s.UpdatedAt = event.Timestamp
	s.PVPowerKW = event.PVPowerKW
	s.LoadPowerKW = event.LoadPowerKW
	s.BatterySOCPercent = event.BatterySOCPercent
	s.PriceEURPerKWh = event.PriceEURPerKWh

	return nil
}
