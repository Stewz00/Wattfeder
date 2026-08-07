package simulator

import (
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

func validSimulatorConfig() Config {
	return Config{
		Seed:                      42,
		Start:                     time.Date(2026, 8, 7, 8, 0, 0, 0, time.UTC),
		Interval:                  15 * time.Minute,
		DeviceID:                  "home-001",
		BatteryCapacityKWh:        10,
		StartingBatterySOCPercent: 50,
	}
}
