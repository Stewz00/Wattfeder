package simulator

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

// Simulator owns the timeline and random stream for one reproducible household run
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
	eventCount := int((24 * time.Hour) / s.cfg.Interval)
	tel := make([]household.Telemetry, 0, eventCount)

	destinationTime := s.currentTime.Add(24 * time.Hour)

	// Include this run's start; leave its end boundary for the next run
	// -> [interval)
	for s.currentTime.Before(destinationTime) {
		event := household.Telemetry{
			Timestamp:         s.currentTime,
			DeviceID:          s.cfg.DeviceID,
			BatterySOCPercent: s.cfg.StartingBatterySOCPercent,
		}
		s.currentTime = s.currentTime.Add(s.cfg.Interval)
		tel = append(tel, event)
	}

	return tel
}
