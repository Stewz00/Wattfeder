package simulator

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Config struct {
	Seed                      int64
	Start                     time.Time
	Interval                  time.Duration
	DeviceID                  string
	BatteryCapacityKWh        float64
	StartingBatterySOCPercent float64
	// TODO: PV generation later
	// TODO: load profile later
}

func (c Config) Validate() error {
	if c.Start.IsZero() {
		return errors.New("start must not be zero")
	}

	if c.Interval <= 0 {
		return fmt.Errorf("interval must be positive, got %s", c.Interval)
	}

	const simulationDuration = 24 * time.Hour

	// A simulated day ends cleanly -> no partial interval sneaking in at the end
	if simulationDuration%c.Interval != 0 {
		return fmt.Errorf("interval must divide %s evenly, got %s", simulationDuration, c.Interval)
	}

	// NaN bypasses ordinary range comparisons
	if math.IsNaN(c.BatteryCapacityKWh) || math.IsInf(c.BatteryCapacityKWh, 0) || c.BatteryCapacityKWh <= 0 {
		return errors.New("battery capacity must be finite and greater than 0")
	}

	if math.IsNaN(c.StartingBatterySOCPercent) || math.IsInf(c.StartingBatterySOCPercent, 0) || c.StartingBatterySOCPercent < 0 || c.StartingBatterySOCPercent > 100 {
		return errors.New("starting battery SOC must be finite and between 0 and 100")
	}

	if strings.TrimSpace(c.DeviceID) == "" {
		return errors.New("device ID must not be empty")
	}

	return nil
}
