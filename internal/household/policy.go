package household

import "fmt"

const (
	minimumDischargeSOCPercent = 20.0
	dischargePriceEURPerKWh    = 0.30
)

// Decide returns the battery command for the latest household state.
func Decide(state State) Command {
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

	return Command{
		Decision: DecisionDischarge,
		PowerKW:  -netPowerKW,
		Reason: fmt.Sprintf(
			"Electricity price is at or above EUR %.2f/kWh and household load exceeds PV production",
			dischargePriceEURPerKWh,
		),
	}
}
