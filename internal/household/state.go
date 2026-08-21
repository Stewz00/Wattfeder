// Package household holds the core domain model for one household's telemetry and control:
// observations, classification, device health, and the commands dispatched in response.
package household

import (
	"fmt"
	"time"
)

// State contains the latest accepted telemetry values for one household device.
type State struct {
	LastEventID       EventID
	DeviceID          string
	UpdatedAt         time.Time
	PVPowerKW         float64
	LoadPowerKW       float64
	BatterySOCPercent float64
	PriceEURPerKWh    float64
}

// ApplyTelemetry validates an event and replaces the latest state for the same device
// when the event's EventTime is strictly newer than the state's current UpdatedAt.
// An event with an equal or older EventTime is still validated but leaves the state unchanged.
func (s *State) ApplyTelemetry(event Telemetry) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid telemetry: %w", err)
	}

	if s.DeviceID != "" && event.DeviceID != s.DeviceID {
		return fmt.Errorf("telemetry device ID %q does not match state device ID %q", event.DeviceID, s.DeviceID)
	}

	if !s.UpdatedAt.IsZero() && !event.EventTime.After(s.UpdatedAt) {
		return nil
	}

	s.LastEventID = event.EventID
	s.DeviceID = event.DeviceID
	s.UpdatedAt = event.EventTime
	s.PVPowerKW = event.PVPowerKW
	s.LoadPowerKW = event.LoadPowerKW
	s.BatterySOCPercent = event.BatterySOCPercent
	s.PriceEURPerKWh = event.PriceEURPerKWh

	return nil
}
