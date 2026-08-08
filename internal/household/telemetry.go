package household

import (
	"errors"
	"math"
	"strings"
	"time"
)

// EventID is the producer-assigned identity of one telemetry event.
type EventID string

// Telemetry contains one household measurement at a point in time.
// Power values use kW, battery state of charge uses percent, and electricity price uses EUR/kWh.
type Telemetry struct {
	EventID           EventID
	Timestamp         time.Time
	DeviceID          string
	PVPowerKW         float64
	LoadPowerKW       float64
	BatterySOCPercent float64
	PriceEURPerKWh    float64
}

// Validate reports whether the telemetry contains a usable household measurement.
func (t Telemetry) Validate() error {
	eventID := string(t.EventID)
	if strings.TrimSpace(eventID) == "" {
		return errors.New("event ID must not be empty")
	}
	if strings.TrimSpace(eventID) != eventID {
		return errors.New("event ID must not have surrounding whitespace")
	}

	if t.Timestamp.IsZero() {
		return errors.New("timestamp must not be zero")
	}

	if strings.TrimSpace(t.DeviceID) == "" {
		return errors.New("device ID must not be empty")
	}

	if !isFinite(t.PVPowerKW) || t.PVPowerKW < 0 {
		return errors.New("PV power must be finite and non-negative")
	}

	if !isFinite(t.LoadPowerKW) || t.LoadPowerKW < 0 {
		return errors.New("load power must be finite and non-negative")
	}

	if !isFinite(t.BatterySOCPercent) || t.BatterySOCPercent < 0 || t.BatterySOCPercent > 100 {
		return errors.New("battery SOC must be finite and between 0 and 100")
	}

	if !isFinite(t.PriceEURPerKWh) || t.PriceEURPerKWh <= 0 {
		return errors.New("electricity price must be finite and greater than 0")
	}

	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
