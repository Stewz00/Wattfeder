package simulator

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestNewRejectsInvalidConfig(t *testing.T) {
	cfg := validSimulatorConfig()
	cfg.Interval = 0

	sim, err := New(cfg)
	if err == nil {
		t.Fatal("New() error = nil, want an invalid configuration error")
	}
	if sim != nil {
		t.Errorf("New() simulator = %#v, want nil", sim)
	}
}

func TestNewNormalizesStartToUTC(t *testing.T) {
	cfg := validSimulatorConfig()
	cfg.Start = time.Date(2026, 8, 7, 8, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	want := cfg.Start.UTC()

	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !sim.currentTime.Equal(want) || sim.currentTime.Location() != time.UTC {
		t.Errorf("New() currentTime = %v, want %v in UTC", sim.currentTime, want)
	}
}

func TestSimulatorSimulateDay(t *testing.T) {
	cfg := validSimulatorConfig()
	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	events := sim.SimulateDay()
	wantCount := int((24 * time.Hour) / cfg.Interval)
	if len(events) != wantCount {
		t.Fatalf("SimulateDay() returned %d events, want %d", len(events), wantCount)
	}

	for i, event := range events {
		wantTimestamp := cfg.Start.UTC().Add(time.Duration(i) * cfg.Interval)
		if !event.Timestamp.Equal(wantTimestamp) {
			t.Errorf("event %d timestamp = %v, want %v", i, event.Timestamp, wantTimestamp)
		}
		if event.DeviceID != cfg.DeviceID {
			t.Errorf("event %d device ID = %q, want %q", i, event.DeviceID, cfg.DeviceID)
		}
		if event.BatterySOCPercent != cfg.StartingBatterySOCPercent {
			t.Errorf(
				"event %d battery SOC = %v, want %v",
				i,
				event.BatterySOCPercent,
				cfg.StartingBatterySOCPercent,
			)
		}
	}

	wantCurrentTime := cfg.Start.UTC().Add(24 * time.Hour)
	if !sim.currentTime.Equal(wantCurrentTime) {
		t.Errorf("currentTime after SimulateDay() = %v, want %v", sim.currentTime, wantCurrentTime)
	}
}

func TestSimulatorSimulateDayAdvancesToNextDay(t *testing.T) {
	cfg := validSimulatorConfig()
	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	sim.SimulateDay()
	secondDay := sim.SimulateDay()
	if len(secondDay) == 0 {
		t.Fatal("second SimulateDay() returned no events")
	}

	wantFirstTimestamp := cfg.Start.UTC().Add(24 * time.Hour)
	if !secondDay[0].Timestamp.Equal(wantFirstTimestamp) {
		t.Errorf("second day starts at %v, want %v", secondDay[0].Timestamp, wantFirstTimestamp)
	}

	wantCurrentTime := cfg.Start.UTC().Add(48 * time.Hour)
	if !sim.currentTime.Equal(wantCurrentTime) {
		t.Errorf("currentTime after two days = %v, want %v", sim.currentTime, wantCurrentTime)
	}
}

func TestSimulatorIsDeterministicForSameConfig(t *testing.T) {
	cfg := validSimulatorConfig()
	first, err := New(cfg)
	if err != nil {
		t.Fatalf("New(first) error = %v", err)
	}
	second, err := New(cfg)
	if err != nil {
		t.Fatalf("New(second) error = %v", err)
	}

	firstDay := first.SimulateDay()
	secondDay := second.SimulateDay()
	if !reflect.DeepEqual(firstDay, secondDay) {
		t.Error("SimulateDay() results differ for identical configurations")
	}
}

func TestSimulatorPVProfile(t *testing.T) {
	cfg := validSimulatorConfig()
	cfg.Start = time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)

	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	events := sim.SimulateDay()
	for i, event := range events {
		hour := float64(event.Timestamp.Hour()) + float64(event.Timestamp.Minute())/60
		isDaylight := hour > 6 && hour < 18

		if !isDaylight && event.PVPowerKW != 0 {
			t.Errorf("event %d PV power outside daylight = %v, want 0", i, event.PVPowerKW)
		}
		if isDaylight && (math.IsNaN(event.PVPowerKW) || math.IsInf(event.PVPowerKW, 0) || event.PVPowerKW <= 0 || event.PVPowerKW > cfg.PVPeakPowerKW) {
			t.Errorf(
				"event %d PV power during daylight = %v, want within (0, %v]",
				i,
				event.PVPowerKW,
				cfg.PVPeakPowerKW,
			)
		}
	}

	morning := events[eventIndexAt(cfg, 8*time.Hour)].PVPowerKW
	noon := events[eventIndexAt(cfg, 12*time.Hour)].PVPowerKW
	afternoon := events[eventIndexAt(cfg, 16*time.Hour)].PVPowerKW
	if noon <= morning || noon <= afternoon {
		t.Errorf("PV profile powers at 08:00, 12:00, and 16:00 = %v, %v, %v; want midday highest", morning, noon, afternoon)
	}
}

func TestSimulatorPVProfileVariesBySeed(t *testing.T) {
	firstConfig := validSimulatorConfig()
	secondConfig := firstConfig
	secondConfig.Seed++

	first, err := New(firstConfig)
	if err != nil {
		t.Fatalf("New(first) error = %v", err)
	}
	second, err := New(secondConfig)
	if err != nil {
		t.Fatalf("New(second) error = %v", err)
	}

	firstDay := first.SimulateDay()
	secondDay := second.SimulateDay()
	for i := range firstDay {
		if firstDay[i].PVPowerKW != secondDay[i].PVPowerKW {
			return
		}
	}

	t.Error("PV profiles are identical for different seeds")
}

func eventIndexAt(cfg Config, sinceStart time.Duration) int {
	return int(sinceStart / cfg.Interval)
}

func validSimulatorConfig() Config {
	return Config{
		Seed:                      42,
		Start:                     time.Date(2026, 8, 7, 8, 0, 0, 0, time.UTC),
		Interval:                  15 * time.Minute,
		DeviceID:                  "home-001",
		BatteryCapacityKWh:        10,
		StartingBatterySOCPercent: 50,
		PVPeakPowerKW:             6,
	}
}
