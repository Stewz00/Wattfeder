package simulator

import (
	"math"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

func TestSimulatorNextObservationAppliesMissingHeartbeat(t *testing.T) {
	sim := newFaultedSimulator(t, Fault{Step: 2, Kind: FaultMissingHeartbeat})
	completeStep(t, sim) // step 1: normal

	envelope, _, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if envelope != nil {
		t.Fatalf("NextObservation() = %+v, want nil (missing heartbeat)", envelope)
	}

	socBefore := sim.batterySOCPercent
	timeBefore := sim.currentTime
	if err := sim.Complete(nil); err != nil {
		t.Fatalf("Complete(nil) error = %v", err)
	}
	if sim.batterySOCPercent != socBefore {
		t.Errorf("SOC after missing heartbeat = %v, want unchanged %v (idle battery)", sim.batterySOCPercent, socBefore)
	}
	if !sim.currentTime.Equal(timeBefore.Add(sim.cfg.Interval)) {
		t.Errorf("currentTime after missing heartbeat = %v, want %v", sim.currentTime, timeBefore.Add(sim.cfg.Interval))
	}
}

func TestSimulatorNextObservationAppliesUnavailable(t *testing.T) {
	sim := newFaultedSimulator(t, Fault{Step: 2, Kind: FaultUnavailable})
	completeStep(t, sim)

	envelope, _, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if envelope == nil {
		t.Fatal("NextObservation() = nil, want an unavailable envelope")
	}
	if envelope.Available {
		t.Error("envelope.Available = true, want false")
	}
	if envelope.Telemetry != nil {
		t.Errorf("envelope.Telemetry = %+v, want nil", envelope.Telemetry)
	}
	if err := envelope.Validate(); err != nil {
		t.Errorf("envelope.Validate() error = %v", err)
	}
}

func TestSimulatorNextObservationAppliesDuplicate(t *testing.T) {
	sim := newFaultedSimulator(t, Fault{Step: 2, Kind: FaultDuplicate})
	first := completeStep(t, sim)

	envelope, _, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if envelope == nil || envelope.Telemetry == nil {
		t.Fatal("NextObservation() returned no telemetry for the duplicate fault")
	}
	if envelope.Telemetry.EventID != first.EventID {
		t.Errorf("duplicate event ID = %q, want the prior event ID %q", envelope.Telemetry.EventID, first.EventID)
	}
	if !envelope.Telemetry.EventTime.Equal(first.EventTime) {
		t.Errorf("duplicate event time = %v, want the prior event time %v", envelope.Telemetry.EventTime, first.EventTime)
	}
	if *envelope.Telemetry.PVPowerKW != first.PVPowerKW {
		t.Errorf("duplicate PV power = %v, want the prior value %v", *envelope.Telemetry.PVPowerKW, first.PVPowerKW)
	}
	if envelope.ReceivedAt.Equal(first.EventTime) {
		t.Error("duplicate receive time equals the prior event time, want the current interval's own receive time")
	}
}

func TestSimulatorNextObservationAppliesOutOfOrder(t *testing.T) {
	offset := -30 * time.Minute
	sim := newFaultedSimulator(t, Fault{
		Step: 2, Kind: FaultOutOfOrder, EventTimeOffset: offset, EventID: "fault-ooo-1",
	})
	completeStep(t, sim)
	nominalEventTime := sim.currentTime

	envelope, _, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if envelope == nil || envelope.Telemetry == nil {
		t.Fatal("NextObservation() returned no telemetry for the out_of_order fault")
	}
	if envelope.Telemetry.EventID != "fault-ooo-1" {
		t.Errorf("out_of_order event ID = %q, want %q", envelope.Telemetry.EventID, "fault-ooo-1")
	}
	wantEventTime := nominalEventTime.Add(offset)
	if !envelope.Telemetry.EventTime.Equal(wantEventTime) {
		t.Errorf("out_of_order event time = %v, want %v", envelope.Telemetry.EventTime, wantEventTime)
	}
	if !envelope.ReceivedAt.Equal(nominalEventTime) {
		t.Errorf("out_of_order receive time = %v, want the nominal interval time %v", envelope.ReceivedAt, nominalEventTime)
	}
}

func TestSimulatorNextObservationAppliesDelay(t *testing.T) {
	delay := 45 * time.Minute
	sim := newFaultedSimulator(t, Fault{Step: 2, Kind: FaultDelay, Delay: delay})
	completeStep(t, sim)
	nominalEventTime := sim.currentTime

	envelope, _, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if envelope == nil || envelope.Telemetry == nil {
		t.Fatal("NextObservation() returned no telemetry for the delay fault")
	}
	if !envelope.Telemetry.EventTime.Equal(nominalEventTime) {
		t.Errorf("delayed event time = %v, want the nominal interval time %v", envelope.Telemetry.EventTime, nominalEventTime)
	}
	wantReceivedAt := nominalEventTime.Add(delay)
	if !envelope.ReceivedAt.Equal(wantReceivedAt) {
		t.Errorf("delayed receive time = %v, want %v", envelope.ReceivedAt, wantReceivedAt)
	}
}

func TestSimulatorNextObservationAppliesMissingValue(t *testing.T) {
	sim := newFaultedSimulator(t, Fault{Step: 2, Kind: FaultMissingValue, Measurement: MeasurementPVPower})
	completeStep(t, sim)

	envelope, _, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if envelope == nil || envelope.Telemetry == nil {
		t.Fatal("NextObservation() returned no telemetry for the missing_value fault")
	}
	if envelope.Telemetry.PVPowerKW != nil {
		t.Errorf("PVPowerKW = %v, want nil (missing)", *envelope.Telemetry.PVPowerKW)
	}
	if envelope.Telemetry.LoadPowerKW == nil {
		t.Error("LoadPowerKW = nil, want present (only the named measurement is missing)")
	}

	if _, err := envelope.Telemetry.Validate(); err == nil {
		t.Error("Validate() error = nil, want a missing-measurement error")
	}
}

func TestSimulatorNextObservationAppliesInvalidMeasurement(t *testing.T) {
	sim := newFaultedSimulator(t, Fault{
		Step: 2, Kind: FaultInvalidMeasurement, Measurement: MeasurementBatterySOC, Value: -5,
	})
	completeStep(t, sim)

	envelope, _, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if envelope == nil || envelope.Telemetry == nil {
		t.Fatal("NextObservation() returned no telemetry for the invalid_measurement fault")
	}
	if envelope.Telemetry.BatterySOCPercent == nil || *envelope.Telemetry.BatterySOCPercent != -5 {
		t.Errorf("BatterySOCPercent = %v, want -5", envelope.Telemetry.BatterySOCPercent)
	}
	if _, err := envelope.Telemetry.Validate(); err == nil {
		t.Error("Validate() error = nil, want a validation error")
	}
}

func TestSimulatorNextObservationRepeatsFaultAcrossConsecutiveSteps(t *testing.T) {
	sim := newFaultedSimulator(t, Fault{Step: 2, Repeat: 2, Kind: FaultMissingHeartbeat})
	completeStep(t, sim) // step 1: normal

	for step := 2; step <= 3; step++ {
		envelope, _, err := sim.NextObservation()
		if err != nil {
			t.Fatalf("step %d: NextObservation() error = %v", step, err)
		}
		if envelope != nil {
			t.Errorf("step %d: NextObservation() = %+v, want nil (repeated missing heartbeat)", step, envelope)
		}
		if err := sim.Complete(nil); err != nil {
			t.Fatalf("step %d: Complete(nil) error = %v", step, err)
		}
	}

	// step 4 must be back to normal delivery
	envelope, _, err := sim.NextObservation()
	if err != nil {
		t.Fatalf("step 4: NextObservation() error = %v", err)
	}
	if envelope == nil || envelope.Telemetry == nil {
		t.Error("step 4: NextObservation() returned no telemetry, want normal delivery after the repeated fault ends")
	}
}

func TestSimulatorContinuesAdvancingAcrossEveryFaultKind(t *testing.T) {
	kinds := []Fault{
		{Step: 2, Kind: FaultDuplicate},
		{Step: 3, Kind: FaultOutOfOrder, EventTimeOffset: -time.Minute, EventID: "fault-advance-1"},
		{Step: 4, Kind: FaultDelay, Delay: time.Minute},
		{Step: 5, Kind: FaultMissingValue, Measurement: MeasurementPVPower},
		{Step: 6, Kind: FaultInvalidMeasurement, Measurement: MeasurementPVPower, Value: -1},
		{Step: 7, Kind: FaultMissingHeartbeat},
		{Step: 8, Kind: FaultUnavailable},
	}
	sim := newFaultedSimulator(t, kinds...)
	start := sim.currentTime

	for step := 1; step <= len(kinds)+1; step++ {
		if _, _, err := sim.NextObservation(); err != nil {
			t.Fatalf("step %d: NextObservation() error = %v", step, err)
		}
		if err := sim.Complete(nil); err != nil {
			t.Fatalf("step %d: Complete(nil) error = %v", step, err)
		}
	}

	wantCurrentTime := start.Add(time.Duration(len(kinds)+1) * sim.cfg.Interval)
	if !sim.currentTime.Equal(wantCurrentTime) {
		t.Errorf("currentTime after %d faulted intervals = %v, want %v", len(kinds)+1, sim.currentTime, wantCurrentTime)
	}
}

func TestSimulatorCompleteWithNilCommandHoldsBatteryIdle(t *testing.T) {
	sim := newFaultedSimulator(t, Fault{Step: 1, Kind: FaultUnavailable})
	socBefore := sim.batterySOCPercent

	if _, _, err := sim.NextObservation(); err != nil {
		t.Fatalf("NextObservation() error = %v", err)
	}
	if err := sim.Complete(nil); err != nil {
		t.Fatalf("Complete(nil) error = %v", err)
	}

	if math.Abs(sim.batterySOCPercent-socBefore) > floatingPointTolerance {
		t.Errorf("SOC after suppressed command = %v, want unchanged %v", sim.batterySOCPercent, socBefore)
	}
}

// newFaultedSimulator returns a deterministic simulator configured with the given faults.
func newFaultedSimulator(t *testing.T, faults ...Fault) *Simulator {
	t.Helper()
	cfg := validSimulatorConfig()
	cfg.Interval = time.Hour
	cfg.Faults = FaultSchedule(faults)

	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return sim
}

// completeStep drives one normal interval to completion with a passive command and returns
// its telemetry.
func completeStep(t *testing.T, sim *Simulator) household.Telemetry {
	t.Helper()
	event := nextTelemetry(t, sim)
	command := passiveCommand(event)
	if err := sim.Complete(&command); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	return event
}
