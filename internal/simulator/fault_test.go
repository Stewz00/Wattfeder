package simulator

import (
	"testing"
	"time"
)

func TestFaultValidateAccepts(t *testing.T) {
	tests := []struct {
		name  string
		fault Fault
	}{
		{name: "duplicate", fault: Fault{Step: 2, Kind: FaultDuplicate}},
		{name: "duplicate with repeat", fault: Fault{Step: 2, Repeat: 3, Kind: FaultDuplicate}},
		{name: "out_of_order", fault: Fault{
			Step: 3, Kind: FaultOutOfOrder, EventTimeOffset: -30 * time.Minute, EventID: "fault-ooo-1",
		}},
		{name: "delay", fault: Fault{Step: 4, Kind: FaultDelay, Delay: 45 * time.Minute}},
		{name: "delay with repeat", fault: Fault{Step: 4, Repeat: 2, Kind: FaultDelay, Delay: 45 * time.Minute}},
		{name: "missing_value pv_power_kw", fault: Fault{Step: 5, Kind: FaultMissingValue, Measurement: MeasurementPVPower}},
		{name: "missing_value load_power_kw", fault: Fault{Step: 5, Kind: FaultMissingValue, Measurement: MeasurementLoadPower}},
		{name: "missing_value battery_soc_percent", fault: Fault{Step: 5, Kind: FaultMissingValue, Measurement: MeasurementBatterySOC}},
		{name: "missing_value price_eur_per_kwh", fault: Fault{Step: 5, Kind: FaultMissingValue, Measurement: MeasurementPrice}},
		{name: "invalid_measurement pv_power_kw", fault: Fault{
			Step: 6, Kind: FaultInvalidMeasurement, Measurement: MeasurementPVPower, Value: -1,
		}},
		{name: "invalid_measurement load_power_kw", fault: Fault{
			Step: 6, Kind: FaultInvalidMeasurement, Measurement: MeasurementLoadPower, Value: -1,
		}},
		{name: "invalid_measurement battery_soc_percent", fault: Fault{
			Step: 6, Kind: FaultInvalidMeasurement, Measurement: MeasurementBatterySOC, Value: -1,
		}},
		{name: "invalid_measurement price_eur_per_kwh", fault: Fault{
			Step: 6, Kind: FaultInvalidMeasurement, Measurement: MeasurementPrice, Value: -1,
		}},
		{name: "missing_heartbeat", fault: Fault{Step: 7, Kind: FaultMissingHeartbeat}},
		{name: "unavailable", fault: Fault{Step: 8, Kind: FaultUnavailable}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fault.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestFaultValidateRejects(t *testing.T) {
	tests := []struct {
		name  string
		fault Fault
	}{
		{name: "step below 1", fault: Fault{Step: 0, Kind: FaultMissingHeartbeat}},
		{name: "negative repeat", fault: Fault{Step: 1, Repeat: -1, Kind: FaultMissingHeartbeat}},
		{name: "unknown kind", fault: Fault{Step: 1, Kind: "bogus"}},
		{name: "out_of_order with non-negative offset", fault: Fault{
			Step: 2, Kind: FaultOutOfOrder, EventTimeOffset: 0, EventID: "fault-1",
		}},
		{name: "out_of_order without event ID", fault: Fault{
			Step: 2, Kind: FaultOutOfOrder, EventTimeOffset: -time.Minute, EventID: "",
		}},
		{name: "delay with non-positive duration", fault: Fault{Step: 3, Kind: FaultDelay, Delay: 0}},
		{name: "missing_value with unknown measurement", fault: Fault{
			Step: 4, Kind: FaultMissingValue, Measurement: "bogus",
		}},
		{name: "invalid_measurement with unknown measurement", fault: Fault{
			Step: 5, Kind: FaultInvalidMeasurement, Measurement: "bogus", Value: -1,
		}},
		{name: "invalid_measurement with a value that is actually valid", fault: Fault{
			Step: 5, Kind: FaultInvalidMeasurement, Measurement: MeasurementPVPower, Value: 3,
		}},
		{name: "duplicate at step 1", fault: Fault{Step: 1, Kind: FaultDuplicate}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fault.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
		})
	}
}

func TestFaultScheduleValidateRejectsOverlappingRanges(t *testing.T) {
	tests := []struct {
		name     string
		schedule FaultSchedule
	}{
		{
			name: "identical steps",
			schedule: FaultSchedule{
				{Step: 3, Kind: FaultMissingHeartbeat},
				{Step: 3, Kind: FaultUnavailable},
			},
		},
		{
			name: "repeat range overlaps a later single step",
			schedule: FaultSchedule{
				{Step: 2, Repeat: 3, Kind: FaultMissingHeartbeat},
				{Step: 4, Kind: FaultUnavailable},
			},
		},
		{
			name: "repeat ranges overlap each other",
			schedule: FaultSchedule{
				{Step: 1, Repeat: 4, Kind: FaultMissingHeartbeat},
				{Step: 3, Repeat: 2, Kind: FaultUnavailable},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.schedule.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want overlapping-range error")
			}
		})
	}
}

func TestFaultScheduleValidateAcceptsNonOverlappingRanges(t *testing.T) {
	schedule := FaultSchedule{
		{Step: 1, Repeat: 2, Kind: FaultMissingHeartbeat},
		{Step: 3, Kind: FaultUnavailable},
		{Step: 4, Repeat: 2, Kind: FaultDelay, Delay: time.Minute},
	}

	if err := schedule.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestFaultScheduleValidateRejectsDuplicateAtStepOne(t *testing.T) {
	schedule := FaultSchedule{{Step: 1, Kind: FaultDuplicate}}

	if err := schedule.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error (nothing precedes step 1 to duplicate)")
	}
}

func TestFaultScheduleValidatePropagatesFaultError(t *testing.T) {
	schedule := FaultSchedule{{Step: 0, Kind: FaultMissingHeartbeat}}

	if err := schedule.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want propagated fault validation error")
	}
}

func TestFaultScheduleAtReturnsTheActiveFault(t *testing.T) {
	schedule := FaultSchedule{
		{Step: 2, Repeat: 3, Kind: FaultMissingHeartbeat},
		{Step: 10, Kind: FaultUnavailable},
	}

	tests := []struct {
		step      int
		wantFound bool
		wantKind  FaultKind
	}{
		{step: 1, wantFound: false},
		{step: 2, wantFound: true, wantKind: FaultMissingHeartbeat},
		{step: 3, wantFound: true, wantKind: FaultMissingHeartbeat},
		{step: 4, wantFound: true, wantKind: FaultMissingHeartbeat},
		{step: 5, wantFound: false},
		{step: 10, wantFound: true, wantKind: FaultUnavailable},
	}

	for _, tt := range tests {
		fault, found := schedule.at(tt.step)
		if found != tt.wantFound {
			t.Errorf("at(%d) found = %v, want %v", tt.step, found, tt.wantFound)
			continue
		}
		if found && fault.Kind != tt.wantKind {
			t.Errorf("at(%d) kind = %v, want %v", tt.step, fault.Kind, tt.wantKind)
		}
	}
}
