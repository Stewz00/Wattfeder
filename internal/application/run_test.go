package application

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/simulator"
)

func TestRunDayConnectsTelemetryPolicyCommandAndOutput(t *testing.T) {
	sim, cfg := newTestSimulator(t)

	var records []Record
	if err := RunDay(context.Background(), sim, func(record Record) error {
		records = append(records, record)
		return nil
	}); err != nil {
		t.Fatalf("RunDay() error = %v", err)
	}

	if len(records) != sim.IntervalsPerDay() {
		t.Fatalf("record count = %d, want %d", len(records), sim.IntervalsPerDay())
	}

	for i, record := range records {
		if record.DeviceID != cfg.DeviceID {
			t.Errorf("record %d device ID = %q, want %q", i, record.DeviceID, cfg.DeviceID)
		}
		if record.Decision == "" || record.Reason == "" {
			t.Errorf("record %d decision = %q, reason = %q, want both populated", i, record.Decision, record.Reason)
		}
	}

	for i, record := range records[:len(records)-1] {
		var batteryPowerKW float64
		switch record.Decision {
		case household.DecisionCharge:
			batteryPowerKW = record.CommandPowerKW
		case household.DecisionDischarge:
			batteryPowerKW = -record.CommandPowerKW
		case household.DecisionIdle:
			batteryPowerKW = 0
		default:
			t.Errorf("record %d decision = %q, want charge, discharge, or idle", i, record.Decision)
			continue
		}

		wantSOCPercent := nextSOCPercent(record.BatterySOCPercent, batteryPowerKW, cfg)
		if math.Abs(records[i+1].BatterySOCPercent-wantSOCPercent) > 1e-12 {
			t.Errorf(
				"battery SOC after record %d = %v, want %v from %q command",
				i,
				records[i+1].BatterySOCPercent,
				wantSOCPercent,
				record.Decision,
			)
		}
	}
}

func TestRunDayStopsAfterCancellation(t *testing.T) {
	sim, _ := newTestSimulator(t)

	ctx, cancel := context.WithCancel(context.Background())
	writeCount := 0
	err := RunDay(ctx, sim, func(Record) error {
		writeCount++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RunDay() error = %v, want context cancellation", err)
	}
	if writeCount != 1 {
		t.Errorf("write count = %d, want 1 before cancellation", writeCount)
	}
}

func TestRunDayReturnsOutputError(t *testing.T) {
	sim, _ := newTestSimulator(t)
	wantErr := errors.New("output unavailable")

	err := RunDay(context.Background(), sim, func(Record) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("RunDay() error = %v, want wrapped output error %v", err, wantErr)
	}
}

func newTestSimulator(t *testing.T) (*simulator.Simulator, simulator.Config) {
	t.Helper()

	cfg := simulator.Config{
		Seed:                      42,
		Start:                     time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		Interval:                  time.Hour,
		DeviceID:                  "home-001",
		BatteryCapacityKWh:        10,
		StartingBatterySOCPercent: 50,
		PVPeakPowerKW:             6,
		LoadBasePowerKW:           0.4,
		PriceBaseEURPerKWh:        0.30,
	}
	sim, err := simulator.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return sim, cfg
}

func nextSOCPercent(currentSOCPercent, batteryPowerKW float64, cfg simulator.Config) float64 {
	currentEnergyKWh := currentSOCPercent / 100 * cfg.BatteryCapacityKWh
	nextEnergyKWh := currentEnergyKWh + batteryPowerKW*cfg.Interval.Hours()
	nextEnergyKWh = math.Max(0, math.Min(cfg.BatteryCapacityKWh, nextEnergyKWh))

	return nextEnergyKWh / cfg.BatteryCapacityKWh * 100
}
