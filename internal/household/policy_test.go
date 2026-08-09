package household

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// Floating-point calculations can miss an expected result by a tiny representation error
const floatingPointTolerance = 1e-12

func TestNewPolicy(t *testing.T) {
	tests := []struct {
		name               string
		batteryCapacityKWh float64
		interval           time.Duration
		wantErr            bool
	}{
		{name: "valid policy", batteryCapacityKWh: 10, interval: time.Hour},
		{name: "zero capacity", interval: time.Hour, wantErr: true},
		{name: "negative capacity", batteryCapacityKWh: -1, interval: time.Hour, wantErr: true},
		{name: "NaN capacity", batteryCapacityKWh: math.NaN(), interval: time.Hour, wantErr: true},
		{name: "infinite capacity", batteryCapacityKWh: math.Inf(1), interval: time.Hour, wantErr: true},
		{name: "zero interval", batteryCapacityKWh: 10, wantErr: true},
		{name: "negative interval", batteryCapacityKWh: 10, interval: -time.Hour, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPolicy(tt.batteryCapacityKWh, tt.interval)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPolicyInterval(t *testing.T) {
	policy, err := NewPolicy(10, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	if got := policy.Interval(); got != 15*time.Minute {
		t.Errorf("Interval() = %v, want %v", got, 15*time.Minute)
	}
}

func TestPolicyDecide(t *testing.T) {
	policy := newTestPolicy(t, 10, time.Hour)
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
	limitedReason := fmt.Sprintf(
		"High electricity price favors discharge, but power is limited to keep the battery at or above the %g%% reserve",
		minimumDischargeSOCPercent,
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
			name: "idles below reserve boundary",
			state: State{
				PVPowerKW:         1,
				LoadPowerKW:       3,
				BatterySOCPercent: 19.999,
				PriceEURPerKWh:    0.40,
			},
			want: Command{
				Decision: DecisionIdle,
				Reason:   reserveReason,
			},
		},
		{
			name: "limits discharge just above reserve boundary",
			state: State{
				PVPowerKW:         1,
				LoadPowerKW:       3,
				BatterySOCPercent: 20.001,
				PriceEURPerKWh:    0.40,
			},
			want: Command{
				Decision: DecisionDischarge,
				PowerKW:  0.0001,
				Reason:   limitedReason,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.Decide(tt.state)
			if got.Decision != tt.want.Decision || got.Reason != tt.want.Reason ||
				math.Abs(got.PowerKW-tt.want.PowerKW) > floatingPointTolerance {
				t.Errorf("Decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPolicyDecideAccountsForIntervalLength(t *testing.T) {
	state := State{
		LoadPowerKW:       4,
		BatterySOCPercent: 30,
		PriceEURPerKWh:    0.40,
	}

	tests := []struct {
		name        string
		interval    time.Duration
		wantPowerKW float64
	}{
		{name: "one hour", interval: time.Hour, wantPowerKW: 1},
		{name: "half hour", interval: 30 * time.Minute, wantPowerKW: 2},
		{name: "quarter hour", interval: 15 * time.Minute, wantPowerKW: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newTestPolicy(t, 10, tt.interval)
			command := policy.Decide(state)
			if command.Decision != DecisionDischarge || command.PowerKW != tt.wantPowerKW {
				t.Errorf("Decide() = %+v, want discharge at %v kW", command, tt.wantPowerKW)
			}
		})
	}
}

func newTestPolicy(t *testing.T, batteryCapacityKWh float64, interval time.Duration) Policy {
	t.Helper()

	policy, err := NewPolicy(batteryCapacityKWh, interval)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	return policy
}
