package simulator

import (
	"testing"

	"github.com/Stewz00/wattfeder/internal/household"
)

func TestSimulatorNextReturnsAnObservationWithoutANominalTime(t *testing.T) {
	cfg := validSimulatorConfig()
	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	envelope, err := sim.Next(t.Context())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if envelope == nil || envelope.Telemetry == nil {
		t.Fatal("Next() returned no telemetry for a fault-free interval")
	}
	if envelope.SourceDeviceID != cfg.DeviceID {
		t.Errorf("Next() SourceDeviceID = %q, want %q", envelope.SourceDeviceID, cfg.DeviceID)
	}
}

func TestSimulatorApplyAdvancesTheSimulatedInterval(t *testing.T) {
	cfg := validSimulatorConfig()
	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := sim.Next(t.Context()); err != nil {
		t.Fatalf("Next() error = %v", err)
	}

	command := household.Command{Decision: household.DecisionIdle, Reason: "test idle"}
	if err := sim.Apply(t.Context(), &command); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if _, err := sim.Next(t.Context()); err != nil {
		t.Errorf("second Next() error = %v, want nil after Apply completed the pending interval", err)
	}
}

func TestSimulatorApplyRequiresAPendingObservation(t *testing.T) {
	cfg := validSimulatorConfig()
	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := sim.Apply(t.Context(), nil); err == nil {
		t.Error("Apply() error = nil, want an error when no observation is pending")
	}
}
