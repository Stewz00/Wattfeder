package application

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence"
	"github.com/Stewz00/wattfeder/internal/simulator"
)

// Floating-point calculations can miss an expected result by a tiny representation error
// This tolerance ignores that noise without accepting a meaningful SOC or power difference
const floatingPointTolerance = 1e-12

// The application should enforce the reserve defined by the household policy
const policyReserveSOCPercent = 20.0

func TestRunDayConnectsTelemetryPolicyCommandAndOutput(t *testing.T) {
	sim, cfg := newTestSimulator(t)
	policy := newTestPolicy(t, cfg)

	var records []Record
	if err := RunDay(context.Background(), sim, policy, func(record Record) error {
		records = append(records, record)
		return nil
	}); err != nil {
		t.Fatalf("RunDay() error = %v", err)
	}

	if len(records) != sim.IntervalsPerDay() {
		t.Fatalf("record count = %d, want %d", len(records), sim.IntervalsPerDay())
	}

	for i, record := range records {
		if record.EventID == "" {
			t.Errorf("record %d event ID is empty", i)
		}
		if record.DeviceID != cfg.DeviceID {
			t.Errorf("record %d device ID = %q, want %q", i, record.DeviceID, cfg.DeviceID)
		}
		if record.Decision == "" || record.Reason == "" {
			t.Errorf("record %d decision = %q, reason = %q, want both populated", i, record.Decision, record.Reason)
		}
		if record.BatterySOCPercent < policyReserveSOCPercent-floatingPointTolerance {
			// %% escapes the percent sign in the formatted failure message
			t.Errorf("record %d battery SOC = %v, want at least the 20%% reserve", i, record.BatterySOCPercent)
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
		if math.Abs(records[i+1].BatterySOCPercent-wantSOCPercent) > floatingPointTolerance {
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
	sim, cfg := newTestSimulator(t)
	policy := newTestPolicy(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	writeCount := 0
	err := RunDay(ctx, sim, policy, func(Record) error {
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
	sim, cfg := newTestSimulator(t)
	policy := newTestPolicy(t, cfg)
	wantErr := errors.New("output unavailable")

	err := RunDay(context.Background(), sim, policy, func(Record) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("RunDay() error = %v, want wrapped output error %v", err, wantErr)
	}
}

func TestRunDayReturnsSimulationErrors(t *testing.T) {
	policy, err := household.NewPolicy(10, time.Hour)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	wantErr := errors.New("simulator unavailable")

	tests := []struct {
		name string
		sim  *stubSimulation
	}{
		{
			name: "telemetry error",
			sim:  &stubSimulation{nextErr: wantErr},
		},
		{
			name: "command error",
			sim: &stubSimulation{
				event:    validApplicationTelemetry(),
				applyErr: wantErr,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunDay(context.Background(), tt.sim, policy, func(Record) error { return nil })
			if !errors.Is(err, wantErr) {
				t.Errorf("RunDay() error = %v, want wrapped error %v", err, wantErr)
			}
		})
	}
}

func TestRunDayRejectsInvalidSimulatedTelemetry(t *testing.T) {
	policy, err := household.NewPolicy(10, time.Hour)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	event := validApplicationTelemetry()
	event.PriceEURPerKWh = 0

	err = RunDay(
		context.Background(),
		&stubSimulation{event: event},
		policy,
		func(Record) error { return nil },
	)
	if err == nil {
		t.Fatal("RunDay() error = nil, want invalid telemetry error")
	}
}

func TestRunPersistentDayCommitsBeforeApplyingCommand(t *testing.T) {
	policy, err := household.NewPolicy(10, time.Hour)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	sim := &stubSimulation{event: validApplicationTelemetry()}
	repository := &stubRepository{commitStatus: persistence.CommitStored}
	repository.beforeCommit = func() {
		if sim.applyCalls != 0 {
			t.Fatalf("ApplyCommand() calls before commit = %d, want 0", sim.applyCalls)
		}
	}

	writeCalls := 0
	if err := RunPersistentDay(t.Context(), sim, policy, repository, func(Record) error {
		writeCalls++
		return nil
	}); err != nil {
		t.Fatalf("RunPersistentDay() error = %v", err)
	}

	if repository.commitCalls != 1 {
		t.Errorf("CommitProcessing() calls = %d, want 1", repository.commitCalls)
	}
	if sim.applyCalls != 1 {
		t.Errorf("ApplyCommand() calls = %d, want 1", sim.applyCalls)
	}
	if writeCalls != 1 {
		t.Errorf("write calls = %d, want 1", writeCalls)
	}
	if err := repository.result.Validate(); err != nil {
		t.Errorf("committed processing result is invalid: %v", err)
	}
	if repository.result.Telemetry.Event.EventID != sim.event.EventID {
		t.Errorf(
			"committed event ID = %q, want %q",
			repository.result.Telemetry.Event.EventID,
			sim.event.EventID,
		)
	}
}

func TestRunPersistentDayStopsBeforeCommandWhenCommitFails(t *testing.T) {
	policy, err := household.NewPolicy(10, time.Hour)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	wantErr := errors.New("persistence unavailable")
	sim := &stubSimulation{event: validApplicationTelemetry()}
	repository := &stubRepository{commitErr: wantErr}

	writeCalls := 0
	err = RunPersistentDay(t.Context(), sim, policy, repository, func(Record) error {
		writeCalls++
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("RunPersistentDay() error = %v, want wrapped persistence error %v", err, wantErr)
	}
	if sim.applyCalls != 0 {
		t.Errorf("ApplyCommand() calls = %d, want 0 after persistence failure", sim.applyCalls)
	}
	if writeCalls != 0 {
		t.Errorf("write calls = %d, want 0 after persistence failure", writeCalls)
	}
}

func TestRunPersistentDayStopsWithoutRedeliveringDuplicate(t *testing.T) {
	policy, err := household.NewPolicy(10, time.Hour)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	sim := &stubSimulation{event: validApplicationTelemetry()}
	repository := &stubRepository{commitStatus: persistence.CommitDuplicate}

	writeCalls := 0
	if err := RunPersistentDay(t.Context(), sim, policy, repository, func(Record) error {
		writeCalls++
		return nil
	}); err != nil {
		t.Fatalf("RunPersistentDay() error = %v", err)
	}
	if sim.applyCalls != 0 {
		t.Errorf("ApplyCommand() calls = %d, want 0 for duplicate event", sim.applyCalls)
	}
	if writeCalls != 0 {
		t.Errorf("write calls = %d, want 0 for duplicate event", writeCalls)
	}
}

func TestRunPersistentDayRejectsNilRepository(t *testing.T) {
	policy, err := household.NewPolicy(10, time.Hour)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	err = RunPersistentDay(
		t.Context(),
		&stubSimulation{event: validApplicationTelemetry()},
		policy,
		nil,
		func(Record) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "repository must not be nil") {
		t.Errorf("RunPersistentDay() error = %v, want nil-repository error", err)
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

func newTestPolicy(t *testing.T, cfg simulator.Config) household.Policy {
	t.Helper()

	policy, err := household.NewPolicy(cfg.BatteryCapacityKWh, cfg.Interval)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	return policy
}

// stubSimulation lets tests force results and failures that the real simulator cannot produce on demand
type stubSimulation struct {
	event      household.Telemetry
	nextErr    error
	applyErr   error
	applyCalls int
}

func (s *stubSimulation) IntervalsPerDay() int {
	return 1
}

func (s *stubSimulation) NextTelemetry() (household.Telemetry, error) {
	return s.event, s.nextErr
}

func (s *stubSimulation) ApplyCommand(household.Command) error {
	s.applyCalls++
	return s.applyErr
}

type stubRepository struct {
	commitStatus persistence.CommitStatus
	commitErr    error
	commitCalls  int
	result       persistence.ProcessingResult
	beforeCommit func()
}

func (r *stubRepository) Migrate(context.Context) error {
	return nil
}

func (r *stubRepository) LatestState(context.Context, string) (household.State, bool, error) {
	return household.State{}, false, nil
}

func (r *stubRepository) CommitProcessing(
	_ context.Context,
	result persistence.ProcessingResult,
) (persistence.CommitStatus, error) {
	r.commitCalls++
	r.result = result
	if r.beforeCommit != nil {
		r.beforeCommit()
	}
	return r.commitStatus, r.commitErr
}

func validApplicationTelemetry() household.Telemetry {
	return household.Telemetry{
		EventID:           "event-001",
		Timestamp:         time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		DeviceID:          "home-001",
		LoadPowerKW:       1,
		BatterySOCPercent: 50,
		PriceEURPerKWh:    0.40,
	}
}

// nextSOCPercent calculates the expected result independently from the simulator under test
func nextSOCPercent(currentSOCPercent, batteryPowerKW float64, cfg simulator.Config) float64 {
	currentEnergyKWh := currentSOCPercent / 100 * cfg.BatteryCapacityKWh
	nextEnergyKWh := currentEnergyKWh + batteryPowerKW*cfg.Interval.Hours()
	nextEnergyKWh = math.Max(0, math.Min(cfg.BatteryCapacityKWh, nextEnergyKWh))

	return nextEnergyKWh / cfg.BatteryCapacityKWh * 100
}
