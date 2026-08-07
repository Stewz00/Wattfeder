package simulator

import (
	"math"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	validConfig := Config{
		Seed:                      42,
		Start:                     time.Date(2026, 8, 7, 8, 0, 0, 0, time.UTC),
		Interval:                  15 * time.Minute,
		DeviceID:                  "home-001",
		BatteryCapacityKWh:        10,
		StartingBatterySOCPercent: 50,
	}

	zeroSOC := validConfig
	zeroSOC.StartingBatterySOCPercent = 0

	fullSOC := validConfig
	fullSOC.StartingBatterySOCPercent = 100

	negativeSOC := validConfig
	negativeSOC.StartingBatterySOCPercent = -1

	aboveMaximumSOC := validConfig
	aboveMaximumSOC.StartingBatterySOCPercent = 101

	nanSOC := validConfig
	nanSOC.StartingBatterySOCPercent = math.NaN()

	infiniteSOC := validConfig
	infiniteSOC.StartingBatterySOCPercent = math.Inf(1)

	zeroCapacity := validConfig
	zeroCapacity.BatteryCapacityKWh = 0

	negativeCapacity := validConfig
	negativeCapacity.BatteryCapacityKWh = -1

	nanCapacity := validConfig
	nanCapacity.BatteryCapacityKWh = math.NaN()

	infiniteCapacity := validConfig
	infiniteCapacity.BatteryCapacityKWh = math.Inf(1)

	oneDayInterval := validConfig
	oneDayInterval.Interval = 24 * time.Hour

	unevenInterval := validConfig
	unevenInterval.Interval = 7 * time.Minute

	zeroInterval := validConfig
	zeroInterval.Interval = 0

	negativeInterval := validConfig
	negativeInterval.Interval = -time.Minute

	emptyDeviceID := validConfig
	emptyDeviceID.DeviceID = ""

	blankDeviceID := validConfig
	blankDeviceID.DeviceID = " \t\n"

	zeroStart := validConfig
	zeroStart.Start = time.Time{}

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "valid configuration", cfg: validConfig},
		{name: "zero SOC is valid", cfg: zeroSOC},
		{name: "full SOC is valid", cfg: fullSOC},
		{name: "SOC below minimum", cfg: negativeSOC, wantErr: true},
		{name: "SOC above maximum", cfg: aboveMaximumSOC, wantErr: true},
		{name: "SOC is NaN", cfg: nanSOC, wantErr: true},
		{name: "SOC is infinite", cfg: infiniteSOC, wantErr: true},
		{name: "capacity is zero", cfg: zeroCapacity, wantErr: true},
		{name: "capacity is negative", cfg: negativeCapacity, wantErr: true},
		{name: "capacity is NaN", cfg: nanCapacity, wantErr: true},
		{name: "capacity is infinite", cfg: infiniteCapacity, wantErr: true},
		{name: "one day interval is valid", cfg: oneDayInterval},
		{name: "interval does not divide one day", cfg: unevenInterval, wantErr: true},
		{name: "interval is zero", cfg: zeroInterval, wantErr: true},
		{name: "interval is negative", cfg: negativeInterval, wantErr: true},
		{name: "device ID is empty", cfg: emptyDeviceID, wantErr: true},
		{name: "device ID contains only whitespace", cfg: blankDeviceID, wantErr: true},
		{name: "start is zero", cfg: zeroStart, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			gotErr := err != nil

			if gotErr != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
