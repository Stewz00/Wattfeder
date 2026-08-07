package simulator

import (
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
}

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

// SimulateDay emits one day of telemetry and evolves battery state from uncontrolled net power.
// It returns an error if another telemetry event is awaiting a command.
func (s *Simulator) SimulateDay() ([]household.Telemetry, error) {
	eventCount := s.IntervalsPerDay()
	events := make([]household.Telemetry, 0, eventCount)

	for range eventCount {
		event, err := s.NextTelemetry()
		if err != nil {
			return nil, fmt.Errorf("get telemetry: %w", err)
		}

		if err := s.ApplyCommand(passiveCommand(event)); err != nil {
			return nil, fmt.Errorf("apply passive command: %w", err)
		}
		events = append(events, event)
	}

	return events, nil
}

// IntervalsPerDay returns the number of telemetry events in one simulated day.
func (s *Simulator) IntervalsPerDay() int {
	return int(simulationDuration / s.cfg.Interval)
}

// NextTelemetry reports the current battery state and awaits one control command.
func (s *Simulator) NextTelemetry() (household.Telemetry, error) {
	if s.pendingTelemetry {
		return household.Telemetry{}, fmt.Errorf(
			"control command is required for telemetry at %s",
			s.currentTime.Format(time.RFC3339),
		)
	}

	if !s.currentTime.Before(s.dayEnd) {
		s.startDay()
	}

	event := household.Telemetry{
		Timestamp:         s.currentTime,
		DeviceID:          s.cfg.DeviceID,
		PVPowerKW:         s.pvPowerKW(s.currentTime, s.dailyPVFactor),
		LoadPowerKW:       s.loadPowerKW(s.currentTime, s.dailyLoadFactor),
		BatterySOCPercent: s.batterySOCPercent,
		PriceEURPerKWh:    s.priceEURPerKWh(s.currentTime, s.dailyPriceFactor),
	}
	s.pendingTelemetry = true

	return event, nil
}

// ApplyCommand evolves the battery over the pending telemetry interval.
func (s *Simulator) ApplyCommand(command household.Command) error {
	if !s.pendingTelemetry {
		return errors.New("telemetry is required before applying a control command")
	}

	if err := command.Validate(); err != nil {
		return fmt.Errorf("invalid control command: %w", err)
	}

	batteryPowerKW := commandBatteryPowerKW(command)
	s.batterySOCPercent = nextBatterySOCPercent(
		s.batterySOCPercent,
		batteryPowerKW,
		s.cfg.Interval,
		s.cfg.BatteryCapacityKWh,
	)
	s.currentTime = s.currentTime.Add(s.cfg.Interval)
	s.pendingTelemetry = false

	return nil
}

func (s *Simulator) startDay() {
	s.dayEnd = s.currentTime.Add(simulationDuration)
	s.dailyPVFactor = s.randomFactor(pvDailyFactorMin, pvDailyFactorMax)
	s.dailyLoadFactor = s.randomFactor(loadDailyFactorMin, loadDailyFactorMax)
	s.dailyPriceFactor = s.randomFactor(priceDailyFactorMin, priceDailyFactorMax)
}

func passiveCommand(event household.Telemetry) household.Command {
	const reason = "Follow uncontrolled net power"

	powerKW := event.PVPowerKW - event.LoadPowerKW
	if powerKW > 0 {
		return household.Command{Decision: household.DecisionCharge, PowerKW: powerKW, Reason: reason}
	}
	if powerKW < 0 {
		return household.Command{Decision: household.DecisionDischarge, PowerKW: -powerKW, Reason: reason}
	}

	return household.Command{Decision: household.DecisionIdle, Reason: reason}
}

func commandBatteryPowerKW(command household.Command) float64 {
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
	normalizedDistance := distance / widthHours
	return math.Exp(-0.5 * normalizedDistance * normalizedDistance)
}
