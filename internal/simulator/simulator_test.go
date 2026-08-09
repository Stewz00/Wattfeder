package simulator

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

// Floating-point calculations can miss an expected result by a tiny representation error
const floatingPointTolerance = 1e-12

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

	events := simulateDay(t, sim)
	wantCount := int((24 * time.Hour) / cfg.Interval)
	if len(events) != wantCount {
		t.Fatalf("SimulateDay() returned %d events, want %d", len(events), wantCount)
	}

	wantBatteryEnergyKWh := cfg.StartingBatterySOCPercent / 100 * cfg.BatteryCapacityKWh
	seenEventIDs := make(map[household.EventID]struct{}, len(events))
	for i, event := range events {
		wantTimestamp := cfg.Start.UTC().Add(time.Duration(i) * cfg.Interval)
		if !event.EventTime.Equal(wantTimestamp) {
			t.Errorf("event %d timestamp = %v, want %v", i, event.EventTime, wantTimestamp)
		}
		if event.DeviceID != cfg.DeviceID {
			t.Errorf("event %d device ID = %q, want %q", i, event.DeviceID, cfg.DeviceID)
		}
		if event.EventID == "" {
			t.Errorf("event %d event ID is empty", i)
		}
		if _, exists := seenEventIDs[event.EventID]; exists {
			t.Errorf("event %d reuses event ID %q", i, event.EventID)
		}
		seenEventIDs[event.EventID] = struct{}{}
		if event.BatterySOCPercent < minimumBatterySOCPercent || event.BatterySOCPercent > maximumBatterySOCPercent {
			t.Errorf("event %d battery SOC = %v, want within [0, 100]", i, event.BatterySOCPercent)
		}

		wantSOCPercent := wantBatteryEnergyKWh / cfg.BatteryCapacityKWh * 100
		if math.Abs(event.BatterySOCPercent-wantSOCPercent) > floatingPointTolerance {
			t.Errorf("event %d battery SOC = %v, want %v from prior interval energy", i, event.BatterySOCPercent, wantSOCPercent)
		}

		intervalEnergyKWh := (event.PVPowerKW - event.LoadPowerKW) * cfg.Interval.Hours()
		wantBatteryEnergyKWh += intervalEnergyKWh
		wantBatteryEnergyKWh = math.Max(0, math.Min(cfg.BatteryCapacityKWh, wantBatteryEnergyKWh))
	}

	if events[0].BatterySOCPercent != cfg.StartingBatterySOCPercent {
		t.Errorf(
			"first event battery SOC = %v, want starting SOC %v",
			events[0].BatterySOCPercent,
			cfg.StartingBatterySOCPercent,
		)
	}

	batteryStateChanged := false
	for _, event := range events[1:] {
		if event.BatterySOCPercent != cfg.StartingBatterySOCPercent {
			batteryStateChanged = true
			break
		}
	}
	if !batteryStateChanged {
		t.Error("battery SOC stayed at its starting value for the entire day, want interval energy flows to change it")
	}

	wantCurrentTime := cfg.Start.UTC().Add(24 * time.Hour)
	if !sim.currentTime.Equal(wantCurrentTime) {
		t.Errorf("currentTime after SimulateDay() = %v, want %v", sim.currentTime, wantCurrentTime)
	}
}

func TestSimulatedEventIDIsStableForSourceEvent(t *testing.T) {
	timestamp := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	want := simulatedEventID("home-001", timestamp)

	if got := simulatedEventID("home-001", timestamp.In(time.FixedZone("CEST", 2*60*60))); got != want {
		t.Errorf("simulatedEventID() = %q in another timezone, want stable ID %q", got, want)
	}
	if got := simulatedEventID("home-002", timestamp); got == want {
		t.Errorf("simulatedEventID() = %q for another device, want a distinct ID", got)
	}
	if got := simulatedEventID("home-001", timestamp.Add(time.Minute)); got == want {
		t.Errorf("simulatedEventID() = %q for another timestamp, want a distinct ID", got)
	}
}

func TestSimulatorSimulateDayAdvancesToNextDay(t *testing.T) {
	cfg := validSimulatorConfig()
	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	firstDay := simulateDay(t, sim)
	secondDay := simulateDay(t, sim)
	if len(secondDay) == 0 {
		t.Fatal("second SimulateDay() returned no events")
	}

	wantFirstTimestamp := cfg.Start.UTC().Add(24 * time.Hour)
	if !secondDay[0].EventTime.Equal(wantFirstTimestamp) {
		t.Errorf("second day starts at %v, want %v", secondDay[0].EventTime, wantFirstTimestamp)
	}

	lastEvent := firstDay[len(firstDay)-1]
	wantFirstSOC := nextBatterySOCPercent(
		lastEvent.BatterySOCPercent,
		lastEvent.PVPowerKW-lastEvent.LoadPowerKW,
		cfg.Interval,
		cfg.BatteryCapacityKWh,
	)
	if math.Abs(secondDay[0].BatterySOCPercent-wantFirstSOC) > floatingPointTolerance {
		t.Errorf("second day starts with battery SOC %v, want carried state %v", secondDay[0].BatterySOCPercent, wantFirstSOC)
	}

	wantCurrentTime := cfg.Start.UTC().Add(48 * time.Hour)
	if !sim.currentTime.Equal(wantCurrentTime) {
		t.Errorf("currentTime after two days = %v, want %v", sim.currentTime, wantCurrentTime)
	}
}

func TestNextBatterySOCPercent(t *testing.T) {
	tests := []struct {
		name              string
		currentSOCPercent float64
		batteryPowerKW    float64
		interval          time.Duration
		capacityKWh       float64
		wantSOCPercent    float64
	}{
		{
			name:              "positive power charges",
			currentSOCPercent: 50,
			batteryPowerKW:    2,
			interval:          30 * time.Minute,
			capacityKWh:       10,
			wantSOCPercent:    60,
		},
		{
			name:              "negative power discharges",
			currentSOCPercent: 50,
			batteryPowerKW:    -2,
			interval:          30 * time.Minute,
			capacityKWh:       10,
			wantSOCPercent:    40,
		},
		{
			name:              "duration converts power to energy",
			currentSOCPercent: 50,
			batteryPowerKW:    4,
			interval:          15 * time.Minute,
			capacityKWh:       10,
			wantSOCPercent:    60,
		},
		{
			name:              "charge is capped at capacity",
			currentSOCPercent: 95,
			batteryPowerKW:    2,
			interval:          time.Hour,
			capacityKWh:       10,
			wantSOCPercent:    100,
		},
		{
			name:              "discharge is capped at empty",
			currentSOCPercent: 5,
			batteryPowerKW:    -2,
			interval:          time.Hour,
			capacityKWh:       10,
			wantSOCPercent:    0,
		},
		{
			name:              "zero power leaves state unchanged",
			currentSOCPercent: 37,
			batteryPowerKW:    0,
			interval:          time.Hour,
			capacityKWh:       10,
			wantSOCPercent:    37,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextBatterySOCPercent(
				tt.currentSOCPercent,
				tt.batteryPowerKW,
				tt.interval,
				tt.capacityKWh,
			)
			if math.Abs(got-tt.wantSOCPercent) > floatingPointTolerance {
				t.Errorf("nextBatterySOCPercent() = %v, want %v", got, tt.wantSOCPercent)
			}
		})
	}
}

func TestPassiveCommand(t *testing.T) {
	tests := []struct {
		name  string
		event household.Telemetry
		want  household.Command
	}{
		{
			name:  "surplus charges",
			event: household.Telemetry{PVPowerKW: 3, LoadPowerKW: 1},
			want: household.Command{
				Decision: household.DecisionCharge,
				PowerKW:  2,
				Reason:   "Follow uncontrolled net power",
			},
		},
		{
			name:  "deficit discharges",
			event: household.Telemetry{PVPowerKW: 1, LoadPowerKW: 3},
			want: household.Command{
				Decision: household.DecisionDischarge,
				PowerKW:  2,
				Reason:   "Follow uncontrolled net power",
			},
		},
		{
			name:  "balance idles",
			event: household.Telemetry{PVPowerKW: 2, LoadPowerKW: 2},
			want: household.Command{
				Decision: household.DecisionIdle,
				Reason:   "Follow uncontrolled net power",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := passiveCommand(tt.event); got != tt.want {
				t.Errorf("passiveCommand() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSimulatorAppliesControlCommandsToBatteryState(t *testing.T) {
	tests := []struct {
		name           string
		command        household.Command
		wantSOCPercent float64
	}{
		{
			name: "charge",
			command: household.Command{
				Decision: household.DecisionCharge,
				PowerKW:  2,
				Reason:   "Store surplus power",
			},
			wantSOCPercent: 70,
		},
		{
			name: "discharge",
			command: household.Command{
				Decision: household.DecisionDischarge,
				PowerKW:  2,
				Reason:   "Serve household demand",
			},
			wantSOCPercent: 30,
		},
		{
			name: "idle",
			command: household.Command{
				Decision: household.DecisionIdle,
				Reason:   "Use grid power",
			},
			wantSOCPercent: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validSimulatorConfig()
			cfg.Interval = time.Hour
			sim, err := New(cfg)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			first := nextTelemetry(t, sim)
			if err := sim.Complete(&tt.command); err != nil {
				t.Fatalf("Complete() error = %v", err)
			}

			second := nextTelemetry(t, sim)
			if math.Abs(second.BatterySOCPercent-tt.wantSOCPercent) > floatingPointTolerance {
				t.Errorf("SOC after %s command = %v, want %v", tt.name, second.BatterySOCPercent, tt.wantSOCPercent)
			}
			if !second.EventTime.Equal(first.EventTime.Add(cfg.Interval)) {
				t.Errorf("timestamp after command = %v, want %v", second.EventTime, first.EventTime.Add(cfg.Interval))
			}
		})
	}
}

func TestSimulatorRequiresOneValidCommandPerTelemetryEvent(t *testing.T) {
	sim, err := New(validSimulatorConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	validCommand := household.Command{
		Decision: household.DecisionIdle,
		Reason:   "Use grid power",
	}
	if err := sim.Complete(&validCommand); err == nil {
		t.Error("Complete() before an observation error = nil, want an error")
	}

	if _, err := sim.NextObservation(); err != nil {
		t.Fatalf("first NextObservation() error = %v", err)
	}
	if _, err := sim.NextObservation(); err == nil {
		t.Error("second NextObservation() error = nil, want pending-command error")
	}

	invalidCommand := household.Command{Decision: household.DecisionIdle}
	if err := sim.Complete(&invalidCommand); err == nil {
		t.Error("Complete(invalid) error = nil, want validation error")
	}
	if _, err := sim.NextObservation(); err == nil {
		t.Error("NextObservation() after invalid command error = nil, want pending-command error")
	}
	if _, err := sim.SimulateDay(); err == nil {
		t.Error("SimulateDay() with pending telemetry error = nil, want pending-command error")
	}

	if err := sim.Complete(&validCommand); err != nil {
		t.Fatalf("Complete(valid) error = %v", err)
	}
	if err := sim.Complete(&validCommand); err == nil {
		t.Error("second Complete() error = nil, want missing-observation error")
	}
	if _, err := sim.NextObservation(); err != nil {
		t.Errorf("NextObservation() after valid command error = %v", err)
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

	firstDay := simulateDay(t, first)
	secondDay := simulateDay(t, second)
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

	events := simulateDay(t, sim)
	for i, event := range events {
		hour := float64(event.EventTime.Hour()) + float64(event.EventTime.Minute())/60
		isDaylight := hour > 6 && hour < 18
		hasValidDaylightPower := !math.IsNaN(event.PVPowerKW) &&
			!math.IsInf(event.PVPowerKW, 0) &&
			event.PVPowerKW > 0 &&
			event.PVPowerKW <= cfg.PVPeakPowerKW

		if !isDaylight && event.PVPowerKW != 0 {
			t.Errorf("event %d PV power outside daylight = %v, want 0", i, event.PVPowerKW)
		}
		if isDaylight && !hasValidDaylightPower {
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

func TestSimulatorProfilesVaryBySeed(t *testing.T) {
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

	firstDay := simulateDay(t, first)
	secondDay := simulateDay(t, second)
	if len(firstDay) != len(secondDay) {
		t.Fatalf("profile lengths = %d and %d, want equal lengths", len(firstDay), len(secondDay))
	}

	profiles := []struct {
		name  string
		value func(household.Telemetry) float64
	}{
		{name: "PV", value: func(event household.Telemetry) float64 { return event.PVPowerKW }},
		{name: "load", value: func(event household.Telemetry) float64 { return event.LoadPowerKW }},
		{name: "price", value: func(event household.Telemetry) float64 { return event.PriceEURPerKWh }},
	}

	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			for i := range firstDay {
				if profile.value(firstDay[i]) != profile.value(secondDay[i]) {
					return
				}
			}

			t.Errorf("%s profiles are identical for different seeds", profile.name)
		})
	}
}

func TestSimulatorLoadProfile(t *testing.T) {
	cfg := validSimulatorConfig()
	cfg.Start = time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)

	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	events := simulateDay(t, sim)
	for i, event := range events {
		if math.IsNaN(event.LoadPowerKW) || math.IsInf(event.LoadPowerKW, 0) || event.LoadPowerKW < 0 {
			t.Errorf("event %d load power = %v, want a finite non-negative value", i, event.LoadPowerKW)
		}
	}

	overnight := events[eventIndexAt(cfg, 2*time.Hour)].LoadPowerKW
	morning := events[eventIndexAt(cfg, 7*time.Hour)].LoadPowerKW
	midday := events[eventIndexAt(cfg, 12*time.Hour)].LoadPowerKW
	evening := events[eventIndexAt(cfg, 19*time.Hour)].LoadPowerKW
	if morning <= overnight || morning <= midday {
		t.Errorf(
			"load powers overnight, morning, and midday = %v, %v, %v; want a distinct morning peak",
			overnight,
			morning,
			midday,
		)
	}
	if evening <= morning || evening <= midday {
		t.Errorf(
			"load powers morning, midday, and evening = %v, %v, %v; want evening highest",
			morning,
			midday,
			evening,
		)
	}
}

func TestSimulatorLoadProfileIsContinuousAtMidnight(t *testing.T) {
	cfg := validSimulatorConfig()
	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	beforeMidnight := time.Date(2026, 8, 7, 23, 59, 0, 0, time.UTC)
	afterMidnight := time.Date(2026, 8, 8, 0, 1, 0, 0, time.UTC)
	difference := math.Abs(sim.loadPowerKW(beforeMidnight, 1) - sim.loadPowerKW(afterMidnight, 1))
	// A two-minute step should change the smooth profile by no more than 2% of the configured base load
	maximumDifference := cfg.LoadBasePowerKW * 0.02
	if difference > maximumDifference {
		t.Errorf("load power changes by %v across midnight, want at most %v", difference, maximumDifference)
	}
}

func TestSimulatorPriceProfile(t *testing.T) {
	cfg := validSimulatorConfig()
	cfg.Start = time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)

	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	events := simulateDay(t, sim)
	for i, event := range events {
		if math.IsNaN(event.PriceEURPerKWh) || math.IsInf(event.PriceEURPerKWh, 0) || event.PriceEURPerKWh <= 0 {
			t.Errorf("event %d price = %v, want a finite positive value", i, event.PriceEURPerKWh)
		}
	}

	morning := events[eventIndexAt(cfg, 7*time.Hour)].PriceEURPerKWh
	midday := events[eventIndexAt(cfg, 13*time.Hour)].PriceEURPerKWh
	evening := events[eventIndexAt(cfg, 19*time.Hour)].PriceEURPerKWh
	if midday >= morning || midday >= evening {
		t.Errorf(
			"prices in the morning, at midday, and in the evening = %v, %v, %v; want midday lowest",
			morning,
			midday,
			evening,
		)
	}
	if evening <= morning {
		t.Errorf("morning and evening prices = %v and %v; want evening highest", morning, evening)
	}
}

func eventIndexAt(cfg Config, sinceStart time.Duration) int {
	return int(sinceStart / cfg.Interval)
}

func nextTelemetry(t *testing.T, sim *Simulator) household.Telemetry {
	t.Helper()

	envelope, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if envelope == nil || envelope.Telemetry == nil {
		t.Fatal("NextObservation() returned no telemetry")
	}
	event, err := envelope.Telemetry.Validate()
	if err != nil {
		t.Fatalf("validate observed telemetry: %v", err)
	}
	return event
}

func simulateDay(t *testing.T, sim *Simulator) []household.Telemetry {
	t.Helper()

	events, err := sim.SimulateDay()
	if err != nil {
		t.Fatalf("SimulateDay() error = %v", err)
	}

	return events
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
		LoadBasePowerKW:           0.4,
		PriceBaseEURPerKWh:        0.30,
	}
}
