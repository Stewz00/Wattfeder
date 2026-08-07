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
	PVPeakPowerKW             float64
	LoadBasePowerKW           float64
	PriceBaseEURPerKWh        float64
}

const simulationDuration = 24 * time.Hour

func (c Config) Validate() error {
	if c.Start.IsZero() {
		return errors.New("start must not be zero")
	}

	if c.Interval <= 0 {
		return fmt.Errorf("interval must be positive, got %s", c.Interval)
	}

	// A simulated day ends cleanly -> no partial interval sneaking in at the end
	if simulationDuration%c.Interval != 0 {
		return fmt.Errorf("interval must divide %s evenly, got %s", simulationDuration, c.Interval)
	}

	// Reject non-finite values before applying ordinary range checks
	if !isFinite(c.BatteryCapacityKWh) || c.BatteryCapacityKWh <= 0 {
		return errors.New("battery capacity must be finite and greater than 0")
	}

	if !isFinite(c.StartingBatterySOCPercent) || c.StartingBatterySOCPercent < 0 || c.StartingBatterySOCPercent > 100 {
		return errors.New("starting battery SOC must be finite and between 0 and 100")
	}

	if !isFinite(c.PVPeakPowerKW) || c.PVPeakPowerKW <= 0 {
		return errors.New("PV peak power must be finite and greater than 0")
	}

	if !isFinite(c.LoadBasePowerKW) || c.LoadBasePowerKW <= 0 {
		return errors.New("load base power must be finite and greater than 0")
	}

	if !isFinite(c.PriceBaseEURPerKWh) || c.PriceBaseEURPerKWh <= 0 {
		return errors.New("base electricity price must be finite and greater than 0")
	}

	if strings.TrimSpace(c.DeviceID) == "" {
		return errors.New("device ID must not be empty")
	}

	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
