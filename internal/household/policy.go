package household

import (
	"errors"
	"fmt"
	"time"
)

const (
	minimumDischargeSOCPercent = 20.0
	dischargePriceEURPerKWh    = 0.30
)

// Policy makes deterministic battery decisions for a configured battery and telemetry interval.
type Policy struct {
	batteryCapacityKWh float64
	interval           time.Duration
}

// NewPolicy returns a policy configured to keep interval commands within the battery reserve.
func NewPolicy(batteryCapacityKWh float64, interval time.Duration) (Policy, error) {
	if !isFinite(batteryCapacityKWh) || batteryCapacityKWh <= 0 {
		return Policy{}, errors.New("battery capacity must be finite and greater than 0")
	}
	if interval <= 0 {
		return Policy{}, errors.New("interval must be positive")
	}

	return Policy{batteryCapacityKWh: batteryCapacityKWh, interval: interval}, nil
}

// Interval returns the telemetry interval this policy was configured for.
func (p Policy) Interval() time.Duration {
	return p.interval
}

// Decide returns the battery command for the latest valid household state.
func (p Policy) Decide(state State) Command {
	netPowerKW := state.PVPowerKW - state.LoadPowerKW

	if netPowerKW > 0 {
		if state.BatterySOCPercent >= 100 {
			return Command{
				Decision: DecisionIdle,
				Reason:   "Battery is fully charged",
			}
		}

		return Command{
			Decision: DecisionCharge,
			PowerKW:  netPowerKW,
			Reason:   "PV production exceeds household load",
		}
	}

	if netPowerKW == 0 {
		return Command{
			Decision: DecisionIdle,
			Reason:   "PV production matches household load",
		}
	}

	if state.BatterySOCPercent <= minimumDischargeSOCPercent {
		// %% emits the literal percent sign after the formatted reserve value
		return Command{
			Decision: DecisionIdle,
			Reason: fmt.Sprintf(
				"Battery state of charge is at or below the %g%% reserve",
				minimumDischargeSOCPercent,
			),
		}
	}

	if state.PriceEURPerKWh < dischargePriceEURPerKWh {
		return Command{
			Decision: DecisionIdle,
			Reason: fmt.Sprintf(
				"Electricity price is below the EUR %.2f/kWh discharge threshold",
				dischargePriceEURPerKWh,
			),
		}
	}

	dischargePowerKW := -netPowerKW
	maximumDischargePowerKW := p.maximumDischargePowerKW(state.BatterySOCPercent)
	reason := fmt.Sprintf(
		"Electricity price is at or above EUR %.2f/kWh and household load exceeds PV production",
		dischargePriceEURPerKWh,
	)
	if dischargePowerKW > maximumDischargePowerKW {
		dischargePowerKW = maximumDischargePowerKW
		reason = fmt.Sprintf(
			"High electricity price favors discharge, but power is limited to keep the battery at or above the %g%% reserve",
			minimumDischargeSOCPercent,
		)
	}

	return Command{
		Decision: DecisionDischarge,
		PowerKW:  dischargePowerKW,
		Reason:   reason,
	}
}

func (p Policy) maximumDischargePowerKW(socPercent float64) float64 {
	// Convert the percentage above reserve to energy, then spread that energy across the interval as power
	energyAboveReserveKWh := (socPercent - minimumDischargeSOCPercent) / 100 * p.batteryCapacityKWh
	return energyAboveReserveKWh / p.interval.Hours()
}
