// Package simulator generates deterministic synthetic household telemetry and control runs,
// optionally injecting configured delivery faults, for exercising the rest of Wattfeder without
// real hardware.
package simulator

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

// These fixed parameters describe simple synthetic daily profiles
// Peak widths are Gaussian standard deviations in hours; scales are multipliers of each profile's baseline
const (
	hoursPerDay         = 24.0
	pvSunriseHour       = 6.0
	pvSunsetHour        = 18.0
	pvDailyFactorMin    = 0.8
	pvDailyFactorMax    = 1.0
	loadDailyFactorMin  = 0.85
	loadDailyFactorMax  = 1.15
	priceDailyFactorMin = 0.9
	priceDailyFactorMax = 1.1

	loadMorningPeakHour       = 7.0
	loadMorningPeakWidthHours = 1.5
	loadMorningPeakScale      = 1.5
	loadEveningPeakHour       = 19.0
	loadEveningPeakWidthHours = 2.0
	loadEveningPeakScale      = 2.5

	priceMorningPeakHour       = 7.0
	priceMorningPeakWidthHours = 1.5
	priceMorningPeakScale      = 0.2
	priceMiddayDipHour         = 13.0
	priceMiddayDipWidthHours   = 2.5
	priceMiddayDipScale        = 0.15
	priceEveningPeakHour       = 19.0
	priceEveningPeakWidthHours = 2.0
	priceEveningPeakScale      = 0.65

	minimumBatterySOCPercent = 0.0
	maximumBatterySOCPercent = 100.0
)

// Simulator owns the timeline and random stream for one reproducible household run.
type Simulator struct {
	cfg               Config
	currentTime       time.Time
	batterySOCPercent float64
	rng               *rand.Rand
	dayEnd            time.Time
	dailyPVFactor     float64
	dailyLoadFactor   float64
	dailyPriceFactor  float64
	pendingTelemetry  bool
	step              int

	// pending* describe the interval currently awaiting completion, independent of any fault
	// substitution applied to the envelope actually returned by NextObservation.
	pendingEventID   household.EventID
	pendingEventTime time.Time
	pendingPV        float64
	pendingLoad      float64
	pendingSOC       float64
	pendingPrice     float64

	// prior* describe the most recently completed interval's own natural values, used to
	// replay a delivery verbatim for the duplicate fault.
	priorEventID   household.EventID
	priorEventTime time.Time
	priorPV        float64
	priorLoad      float64
	priorSOC       float64
	priorPrice     float64
}

// New validates cfg and builds a Simulator ready to produce the first simulated interval.
func New(cfg Config) (*Simulator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid simulator config: %w", err)
	}

	// Keep every run on one canonical clock
	cfg.Start = cfg.Start.UTC()

	rng := rand.New(rand.NewSource(cfg.Seed))

	simulator := &Simulator{
		cfg:               cfg,
		currentTime:       cfg.Start,
		batterySOCPercent: cfg.StartingBatterySOCPercent,
		rng:               rng,
	}
	simulator.startDay()

	return simulator, nil
}

// IntervalsPerDay returns the number of telemetry events in one simulated day.
func (s *Simulator) IntervalsPerDay() int {
	return int(SimulationDuration / s.cfg.Interval)
}

// NextObservation produces the envelope for the current simulated interval, applying any
// fault configured for this step, along with that interval's nominal simulated time. A nil
// envelope with a nil error means no observation arrived for the interval at all (a missing
// heartbeat); the nominal time is still returned so a caller can classify the missing
// interval deterministically.
func (s *Simulator) NextObservation() (*household.ObservationEnvelope, time.Time, error) {
	if s.pendingTelemetry {
		return nil, time.Time{}, fmt.Errorf(
			"control command is required for telemetry at %s",
			s.currentTime.Format(time.RFC3339),
		)
	}

	if !s.currentTime.Before(s.dayEnd) {
		s.startDay()
	}

	s.step++
	eventTime := s.currentTime
	eventID := simulatedEventID(s.cfg.DeviceID, eventTime)
	pv := s.pvPowerKW(eventTime, s.dailyPVFactor)
	load := s.loadPowerKW(eventTime, s.dailyLoadFactor)
	soc := s.batterySOCPercent
	price := s.priceEURPerKWh(eventTime, s.dailyPriceFactor)

	s.pendingTelemetry = true
	s.pendingEventID, s.pendingEventTime = eventID, eventTime
	s.pendingPV, s.pendingLoad, s.pendingSOC, s.pendingPrice = pv, load, soc, price

	fault, hasFault := s.cfg.Faults.at(s.step)
	if !hasFault {
		return fullEnvelope(s.cfg.DeviceID, eventID, eventTime, eventTime, pv, load, soc, price), eventTime, nil
	}

	switch fault.Kind {
	case FaultMissingHeartbeat:
		return nil, eventTime, nil
	case FaultUnavailable:
		return &household.ObservationEnvelope{SourceDeviceID: s.cfg.DeviceID, ReceivedAt: eventTime}, eventTime, nil
	case FaultDuplicate:
		return fullEnvelope(
			s.cfg.DeviceID, s.priorEventID, s.priorEventTime, eventTime, s.priorPV, s.priorLoad, s.priorSOC, s.priorPrice,
		), eventTime, nil
	case FaultOutOfOrder:
		outOfOrderTime := eventTime.Add(fault.EventTimeOffset)
		return fullEnvelope(
			s.cfg.DeviceID, household.EventID(fault.EventID), outOfOrderTime, eventTime, pv, load, soc, price,
		), eventTime, nil
	case FaultDelay:
		return fullEnvelope(s.cfg.DeviceID, eventID, eventTime, eventTime.Add(fault.Delay), pv, load, soc, price), eventTime, nil
	case FaultMissingValue:
		return &household.ObservationEnvelope{
			SourceDeviceID: s.cfg.DeviceID,
			ReceivedAt:     eventTime,
			Telemetry:      rawTelemetryWithFault(eventID, eventTime, s.cfg.DeviceID, pv, load, soc, price, fault.Measurement, nil),
			Available:      true,
		}, eventTime, nil
	case FaultInvalidMeasurement:
		value := fault.Value
		return &household.ObservationEnvelope{
			SourceDeviceID: s.cfg.DeviceID,
			ReceivedAt:     eventTime,
			Telemetry: rawTelemetryWithFault(
				eventID, eventTime, s.cfg.DeviceID, pv, load, soc, price, fault.Measurement, &value,
			),
			Available: true,
		}, eventTime, nil
	default:
		return nil, eventTime, fmt.Errorf("unknown fault kind %q", fault.Kind)
	}
}

func fullEnvelope(
	deviceID string, eventID household.EventID, eventTime, receivedAt time.Time, pv, load, soc, price float64,
) *household.ObservationEnvelope {
	return &household.ObservationEnvelope{
		SourceDeviceID: deviceID,
		ReceivedAt:     receivedAt,
		Telemetry:      rawTelemetryWithFault(eventID, eventTime, deviceID, pv, load, soc, price, "", nil),
		Available:      true,
	}
}

// rawTelemetryWithFault builds a raw telemetry sample, optionally overriding one named
// measurement with override (nil simulates a missing value).
func rawTelemetryWithFault(
	eventID household.EventID,
	eventTime time.Time,
	deviceID string,
	pv, load, soc, price float64,
	measurement Measurement,
	override *float64,
) *household.RawTelemetry {
	pvPtr, loadPtr, socPtr, pricePtr := &pv, &load, &soc, &price
	switch measurement {
	case MeasurementPVPower:
		pvPtr = override
	case MeasurementLoadPower:
		loadPtr = override
	case MeasurementBatterySOC:
		socPtr = override
	case MeasurementPrice:
		pricePtr = override
	}

	return &household.RawTelemetry{
		EventID:           eventID,
		EventTime:         eventTime,
		DeviceID:          deviceID,
		PVPowerKW:         pvPtr,
		LoadPowerKW:       loadPtr,
		BatterySOCPercent: socPtr,
		PriceEURPerKWh:    pricePtr,
	}
}

func simulatedEventID(deviceID string, timestamp time.Time) household.EventID {
	// A stable source key makes replaying the same simulated interval a duplicate after a restart
	sourceKey := fmt.Sprintf("%d:%s:%s", len(deviceID), deviceID, timestamp.UTC().Format(time.RFC3339Nano))
	return household.EventID(fmt.Sprintf("sim-%x", sha256.Sum256([]byte(sourceKey))))
}

// Complete evolves the battery over the pending interval and advances the simulated clock.
// A nil command means the observation's command was suppressed or rejected; the interval
// still advances, with the battery held idle.
func (s *Simulator) Complete(command *household.Command) error {
	if !s.pendingTelemetry {
		return errors.New("an observation is required before completing a control command")
	}

	var batteryPowerKW float64
	if command != nil {
		if err := command.Validate(); err != nil {
			return fmt.Errorf("invalid control command: %w", err)
		}
		batteryPowerKW = commandBatteryPowerKW(*command)
	}

	s.batterySOCPercent = nextBatterySOCPercent(
		s.batterySOCPercent,
		batteryPowerKW,
		s.cfg.Interval,
		s.cfg.BatteryCapacityKWh,
	)
	s.priorEventID, s.priorEventTime = s.pendingEventID, s.pendingEventTime
	s.priorPV, s.priorLoad, s.priorSOC, s.priorPrice = s.pendingPV, s.pendingLoad, s.pendingSOC, s.pendingPrice
	s.currentTime = s.currentTime.Add(s.cfg.Interval)
	s.pendingTelemetry = false

	return nil
}

func (s *Simulator) startDay() {
	s.dayEnd = s.currentTime.Add(SimulationDuration)
	s.dailyPVFactor = s.randomFactor(pvDailyFactorMin, pvDailyFactorMax)
	s.dailyLoadFactor = s.randomFactor(loadDailyFactorMin, loadDailyFactorMax)
	s.dailyPriceFactor = s.randomFactor(priceDailyFactorMin, priceDailyFactorMax)
}

func commandBatteryPowerKW(command household.Command) float64 {
	// Commands use non-negative magnitudes; discharge becomes negative battery-relative power
	if command.Decision == household.DecisionDischarge {
		return -command.PowerKW
	}

	return command.PowerKW
}

func nextBatterySOCPercent(
	currentSOCPercent float64,
	batteryPowerKW float64,
	interval time.Duration,
	capacityKWh float64,
) float64 {
	// SOC is converted to energy because power changes stored energy rather than percentage directly
	currentEnergyKWh := currentSOCPercent / maximumBatterySOCPercent * capacityKWh
	intervalEnergyKWh := batteryPowerKW * interval.Hours()
	nextEnergyKWh := currentEnergyKWh + intervalEnergyKWh

	if nextEnergyKWh <= 0 {
		return minimumBatterySOCPercent
	}
	if nextEnergyKWh >= capacityKWh {
		return maximumBatterySOCPercent
	}

	return nextEnergyKWh / capacityKWh * maximumBatterySOCPercent
}

func (s *Simulator) pvPowerKW(at time.Time, dailyFactor float64) float64 {
	hour := hourOfDay(at)
	if hour <= pvSunriseHour || hour >= pvSunsetHour {
		return 0
	}

	// Map 06:00 - 18:00 linearly to (0, 1)
	// sin(pi*x) is zero at both boundaries, reaches one at noon, and stays positive during daylight
	daylightProgress := (hour - pvSunriseHour) / (pvSunsetHour - pvSunriseHour)
	return s.cfg.PVPeakPowerKW * dailyFactor * math.Sin(math.Pi*daylightProgress)
}

func (s *Simulator) loadPowerKW(at time.Time, dailyFactor float64) float64 {
	hour := hourOfDay(at)
	morningDemand := loadMorningPeakScale * dailyGaussian(hour, loadMorningPeakHour, loadMorningPeakWidthHours)
	eveningDemand := loadEveningPeakScale * dailyGaussian(hour, loadEveningPeakHour, loadEveningPeakWidthHours)

	return s.cfg.LoadBasePowerKW * dailyFactor * (1 + morningDemand + eveningDemand)
}

func (s *Simulator) priceEURPerKWh(at time.Time, dailyFactor float64) float64 {
	hour := hourOfDay(at)
	morningPeak := priceMorningPeakScale * dailyGaussian(hour, priceMorningPeakHour, priceMorningPeakWidthHours)
	middayDip := priceMiddayDipScale * dailyGaussian(hour, priceMiddayDipHour, priceMiddayDipWidthHours)
	eveningPeak := priceEveningPeakScale * dailyGaussian(hour, priceEveningPeakHour, priceEveningPeakWidthHours)

	return s.cfg.PriceBaseEURPerKWh * dailyFactor * (1 + morningPeak - middayDip + eveningPeak)
}

func (s *Simulator) randomFactor(minimum, maximum float64) float64 {
	return minimum + (maximum-minimum)*s.rng.Float64()
}

func hourOfDay(at time.Time) float64 {
	return float64(at.Hour()) + float64(at.Minute())/60 + float64(at.Second())/3600
}

// dailyGaussian models a gradual peak or dip instead of an abrupt time-based step
// It returns 1 at peakHour and falls toward 0; widthHours controls how broad the shape is
// Distances wrap at midnight to keep the curve continuous
func dailyGaussian(hour, peakHour, widthHours float64) float64 {
	distance := math.Abs(hour - peakHour)
	distance = math.Min(distance, hoursPerDay-distance)
	// Dividing by the width expresses distance in peak widths; the exponential turns it into a smooth bell curve
	normalizedDistance := distance / widthHours
	return math.Exp(-0.5 * normalizedDistance * normalizedDistance)
}
