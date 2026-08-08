package simulator

import (
	"math"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{name: "valid configuration"},
		{name: "zero SOC is valid", modify: func(cfg *Config) { cfg.StartingBatterySOCPercent = 0 }},
		{name: "full SOC is valid", modify: func(cfg *Config) { cfg.StartingBatterySOCPercent = 100 }},
		{name: "SOC below minimum", modify: func(cfg *Config) { cfg.StartingBatterySOCPercent = -1 }, wantErr: true},
		{name: "SOC above maximum", modify: func(cfg *Config) { cfg.StartingBatterySOCPercent = 101 }, wantErr: true},
		{name: "SOC is NaN", modify: func(cfg *Config) { cfg.StartingBatterySOCPercent = math.NaN() }, wantErr: true},
		{name: "SOC is infinite", modify: func(cfg *Config) { cfg.StartingBatterySOCPercent = math.Inf(1) }, wantErr: true},
		{name: "capacity is zero", modify: func(cfg *Config) { cfg.BatteryCapacityKWh = 0 }, wantErr: true},
		{name: "capacity is negative", modify: func(cfg *Config) { cfg.BatteryCapacityKWh = -1 }, wantErr: true},
		{name: "capacity is NaN", modify: func(cfg *Config) { cfg.BatteryCapacityKWh = math.NaN() }, wantErr: true},
		{name: "capacity is infinite", modify: func(cfg *Config) { cfg.BatteryCapacityKWh = math.Inf(1) }, wantErr: true},
		{name: "PV peak power is zero", modify: func(cfg *Config) { cfg.PVPeakPowerKW = 0 }, wantErr: true},
		{name: "PV peak power is negative", modify: func(cfg *Config) { cfg.PVPeakPowerKW = -1 }, wantErr: true},
		{name: "PV peak power is NaN", modify: func(cfg *Config) { cfg.PVPeakPowerKW = math.NaN() }, wantErr: true},
		{name: "PV peak power is infinite", modify: func(cfg *Config) { cfg.PVPeakPowerKW = math.Inf(1) }, wantErr: true},
		{name: "load base power is zero", modify: func(cfg *Config) { cfg.LoadBasePowerKW = 0 }, wantErr: true},
		{name: "load base power is negative", modify: func(cfg *Config) { cfg.LoadBasePowerKW = -1 }, wantErr: true},
		{name: "load base power is NaN", modify: func(cfg *Config) { cfg.LoadBasePowerKW = math.NaN() }, wantErr: true},
		{name: "load base power is infinite", modify: func(cfg *Config) { cfg.LoadBasePowerKW = math.Inf(1) }, wantErr: true},
		{name: "base electricity price is zero", modify: func(cfg *Config) { cfg.PriceBaseEURPerKWh = 0 }, wantErr: true},
		{name: "base electricity price is negative", modify: func(cfg *Config) { cfg.PriceBaseEURPerKWh = -1 }, wantErr: true},
		{name: "base electricity price is NaN", modify: func(cfg *Config) { cfg.PriceBaseEURPerKWh = math.NaN() }, wantErr: true},
		{name: "base electricity price is infinite", modify: func(cfg *Config) { cfg.PriceBaseEURPerKWh = math.Inf(1) }, wantErr: true},
		{name: "one day interval is valid", modify: func(cfg *Config) { cfg.Interval = SimulationDuration }},
		{name: "interval does not divide one day", modify: func(cfg *Config) { cfg.Interval = 7 * time.Minute }, wantErr: true},
		{name: "interval is zero", modify: func(cfg *Config) { cfg.Interval = 0 }, wantErr: true},
		{name: "interval is negative", modify: func(cfg *Config) { cfg.Interval = -time.Minute }, wantErr: true},
		{name: "device ID is empty", modify: func(cfg *Config) { cfg.DeviceID = "" }, wantErr: true},
		{name: "device ID contains only whitespace", modify: func(cfg *Config) { cfg.DeviceID = " \t\n" }, wantErr: true},
		{name: "start is zero", modify: func(cfg *Config) { cfg.Start = time.Time{} }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validSimulatorConfig()
			if tt.modify != nil {
				tt.modify(&cfg)
			}

			err := cfg.Validate()
			gotErr := err != nil

			if gotErr != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
