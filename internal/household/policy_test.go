package household

import (
	"fmt"
	"testing"
)

func TestDecide(t *testing.T) {
	reserveReason := fmt.Sprintf(
		"Battery state of charge is at or below the %g%% reserve",
		minimumDischargeSOCPercent,
	)
	belowPriceReason := fmt.Sprintf(
		"Electricity price is below the EUR %.2f/kWh discharge threshold",
		dischargePriceEURPerKWh,
	)
	dischargeReason := fmt.Sprintf(
		"Electricity price is at or above EUR %.2f/kWh and household load exceeds PV production",
		dischargePriceEURPerKWh,
	)

	tests := []struct {
		name  string
		state State
		want  Command
	}{
		{
			name: "charges from PV surplus",
			state: State{
				PVPowerKW:         4.8,
				LoadPowerKW:       1.9,
				BatterySOCPercent: 61,
				PriceEURPerKWh:    0.28,
			},
			want: Command{
				Decision: DecisionCharge,
				PowerKW:  2.9,
				Reason:   "PV production exceeds household load",
			},
		},
		{
			name: "idles with PV surplus at full charge",
			state: State{
				PVPowerKW:         4,
				LoadPowerKW:       2,
				BatterySOCPercent: 100,
				PriceEURPerKWh:    0.30,
			},
			want: Command{
				Decision: DecisionIdle,
				Reason:   "Battery is fully charged",
			},
		},
		{
			name: "idles when production matches load",
			state: State{
				PVPowerKW:         2,
				LoadPowerKW:       2,
				BatterySOCPercent: 60,
				PriceEURPerKWh:    0.30,
			},
			want: Command{
				Decision: DecisionIdle,
				Reason:   "PV production matches household load",
			},
		},
		{
			name: "discharges at price threshold",
			state: State{
				PVPowerKW:         1,
				LoadPowerKW:       3,
				BatterySOCPercent: 50,
				PriceEURPerKWh:    0.30,
			},
			want: Command{
				Decision: DecisionDischarge,
				PowerKW:  2,
				Reason:   dischargeReason,
			},
		},
		{
			name: "idles below price threshold",
			state: State{
				PVPowerKW:         1,
				LoadPowerKW:       3,
				BatterySOCPercent: 50,
				PriceEURPerKWh:    0.299,
			},
			want: Command{
				Decision: DecisionIdle,
				Reason:   belowPriceReason,
			},
		},
		{
			name: "idles at reserve boundary",
			state: State{
				PVPowerKW:         1,
				LoadPowerKW:       3,
				BatterySOCPercent: 20,
				PriceEURPerKWh:    0.40,
			},
			want: Command{
				Decision: DecisionIdle,
				Reason:   reserveReason,
			},
		},
		{
			name: "discharges above reserve boundary",
			state: State{
				PVPowerKW:         1,
				LoadPowerKW:       3,
				BatterySOCPercent: 20.001,
				PriceEURPerKWh:    0.40,
			},
			want: Command{
				Decision: DecisionDischarge,
				PowerKW:  2,
				Reason:   dischargeReason,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.state); got != tt.want {
				t.Errorf("Decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
