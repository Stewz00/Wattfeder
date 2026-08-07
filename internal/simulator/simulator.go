package simulator

import (
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
)

// Simulator owns the timeline and random stream for one reproducible household run.
type Simulator struct {
	cfg         Config
	currentTime time.Time
	rng         *rand.Rand
}

func New(cfg Config) (*Simulator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid simulator config: %w", err)
	}

	// Keep every run on one canonical clock
	cfg.Start = cfg.Start.UTC()

	rng := rand.New(rand.NewSource(cfg.Seed))

	return &Simulator{
		cfg:         cfg,
		currentTime: cfg.Start,
		rng:         rng,
	}, nil
}

func (s *Simulator) SimulateDay() []household.Telemetry {
	// Config validation guarantees that Interval divides 24 hours exactly
	eventCount := int(simulationDuration / s.cfg.Interval)
	events := make([]household.Telemetry, 0, eventCount)

	destinationTime := s.currentTime.Add(simulationDuration)
	// Keep each profile's shape stable within the simulated day while varying its overall level reproducibly between days
	dailyPVFactor := s.randomFactor(pvDailyFactorMin, pvDailyFactorMax)
	dailyLoadFactor := s.randomFactor(loadDailyFactorMin, loadDailyFactorMax)
	dailyPriceFactor := s.randomFactor(priceDailyFactorMin, priceDailyFactorMax)

	// Emit the exact half-open window [start, start+24h), one event per interval
	for s.currentTime.Before(destinationTime) {
		event := household.Telemetry{
			Timestamp:         s.currentTime,
			DeviceID:          s.cfg.DeviceID,
			PVPowerKW:         s.pvPowerKW(s.currentTime, dailyPVFactor),
			LoadPowerKW:       s.loadPowerKW(s.currentTime, dailyLoadFactor),
			BatterySOCPercent: s.cfg.StartingBatterySOCPercent,
			PriceEURPerKWh:    s.priceEURPerKWh(s.currentTime, dailyPriceFactor),
		}
		s.currentTime = s.currentTime.Add(s.cfg.Interval)
		events = append(events, event)
	}

	return events
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
