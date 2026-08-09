package simulator

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

// sampleRawTelemetryEventTime is an arbitrary fixed, non-zero time used only to build a
// throwaway sample for measurement-validity checks.
var sampleRawTelemetryEventTime = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// FaultKind names one deliberate delivery anomaly a fault schedule can inject.
type FaultKind string

const (
	FaultDuplicate          FaultKind = "duplicate"
	FaultOutOfOrder         FaultKind = "out_of_order"
	FaultDelay              FaultKind = "delay"
	FaultMissingValue       FaultKind = "missing_value"
	FaultInvalidMeasurement FaultKind = "invalid_measurement"
	FaultMissingHeartbeat   FaultKind = "missing_heartbeat"
	FaultUnavailable        FaultKind = "unavailable"
)

// Measurement names one telemetry field a fault can target.
type Measurement string

const (
	MeasurementPVPower    Measurement = "pv_power_kw"
	MeasurementLoadPower  Measurement = "load_power_kw"
	MeasurementBatterySOC Measurement = "battery_soc_percent"
	MeasurementPrice      Measurement = "price_eur_per_kwh"
)

func (m Measurement) valid() bool {
	switch m {
	case MeasurementPVPower, MeasurementLoadPower, MeasurementBatterySOC, MeasurementPrice:
		return true
	}
	return false
}

// rejects reports whether substituting value for m fails domain validation, reusing
// household validation instead of duplicating its range rules.
func (m Measurement) rejects(value float64) bool {
	raw := sampleRawTelemetry()
	switch m {
	case MeasurementPVPower:
		raw.PVPowerKW = &value
	case MeasurementLoadPower:
		raw.LoadPowerKW = &value
	case MeasurementBatterySOC:
		raw.BatterySOCPercent = &value
	case MeasurementPrice:
		raw.PriceEURPerKWh = &value
	}
	_, err := raw.Validate()
	return err != nil
}

// Fault configures one deterministic delivery anomaly starting at a one-based simulated
// interval step and optionally repeating across consecutive steps.
type Fault struct {
	// Step is the one-based simulated interval at which the fault first applies.
	Step int
	// Repeat is the number of consecutive steps the fault applies to; zero means one step.
	Repeat int
	Kind   FaultKind

	// EventTimeOffset applies to FaultOutOfOrder and must be negative.
	EventTimeOffset time.Duration
	// EventID applies to FaultOutOfOrder and must be a unique, stable, non-empty event ID.
	EventID string
	// Delay applies to FaultDelay and must be positive.
	Delay time.Duration
	// Measurement applies to FaultMissingValue and FaultInvalidMeasurement.
	Measurement Measurement
	// Value applies to FaultInvalidMeasurement together with Measurement.
	Value float64
}

func (f Fault) repeatCount() int {
	if f.Repeat <= 0 {
		return 1
	}
	return f.Repeat
}

func (f Fault) stepRange() (first, last int) {
	first = f.Step
	last = f.Step + f.repeatCount() - 1
	return first, last
}

func (f Fault) overlaps(other Fault) bool {
	firstA, lastA := f.stepRange()
	firstB, lastB := other.stepRange()
	return firstA <= lastB && firstB <= lastA
}

// Validate reports whether one fault's parameters are well-formed for its kind.
func (f Fault) Validate() error {
	if f.Step < 1 {
		return errors.New("step must be at least 1")
	}
	if f.Repeat < 0 {
		return errors.New("repeat must not be negative")
	}

	switch f.Kind {
	case FaultDuplicate:
		if f.Step == 1 {
			return errors.New("duplicate cannot apply at step 1; there is no prior observation to duplicate")
		}
		return nil
	case FaultMissingHeartbeat, FaultUnavailable:
		return nil
	case FaultOutOfOrder:
		if f.EventTimeOffset >= 0 {
			return errors.New("out_of_order event time offset must be negative")
		}
		if strings.TrimSpace(f.EventID) == "" {
			return errors.New("out_of_order requires a unique, stable event ID")
		}
		return nil
	case FaultDelay:
		if f.Delay <= 0 {
			return errors.New("delay must be positive")
		}
		return nil
	case FaultMissingValue:
		if !f.Measurement.valid() {
			return fmt.Errorf("missing_value requires a known measurement, got %q", f.Measurement)
		}
		return nil
	case FaultInvalidMeasurement:
		if !f.Measurement.valid() {
			return fmt.Errorf("invalid_measurement requires a known measurement, got %q", f.Measurement)
		}
		if !f.Measurement.rejects(f.Value) {
			return fmt.Errorf("invalid_measurement value %v does not fail validation for %q", f.Value, f.Measurement)
		}
		return nil
	default:
		return fmt.Errorf("unknown fault kind %q", f.Kind)
	}
}

// FaultSchedule is an ordered set of deterministic faults applied across simulated steps.
type FaultSchedule []Fault

// Validate reports whether every fault is individually valid and no two faults' step ranges overlap.
func (s FaultSchedule) Validate() error {
	for i, fault := range s {
		if err := fault.Validate(); err != nil {
			return fmt.Errorf("fault %d: %w", i, err)
		}
	}

	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[i].overlaps(s[j]) {
				return fmt.Errorf("fault %d and fault %d have overlapping step ranges", i, j)
			}
		}
	}

	return nil
}

// at returns the fault active at the given one-based step, if any.
func (s FaultSchedule) at(step int) (Fault, bool) {
	for _, fault := range s {
		first, last := fault.stepRange()
		if step >= first && step <= last {
			return fault, true
		}
	}
	return Fault{}, false
}

func sampleRawTelemetry() household.RawTelemetry {
	pv, load, soc, price := 1.0, 1.0, 50.0, 0.30
	return household.RawTelemetry{
		EventID:           "fault-check",
		EventTime:         sampleRawTelemetryEventTime,
		DeviceID:          "fault-check",
		PVPowerKW:         &pv,
		LoadPowerKW:       &load,
		BatterySOCPercent: &soc,
		PriceEURPerKWh:    &price,
	}
}
