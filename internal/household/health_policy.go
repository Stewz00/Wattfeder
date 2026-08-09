package household

import (
	"errors"
	"time"
)

// HealthPolicy configures the durations after which a device's telemetry contact
// is considered stale, then offline.
type HealthPolicy struct {
	StaleAfter   time.Duration
	OfflineAfter time.Duration
}

// NewHealthPolicy returns a HealthPolicy for the given telemetry interval.
// A zero staleAfter or offlineAfter defaults to two or three telemetry intervals.
// Explicit durations must satisfy 0 < staleAfter < offlineAfter.
func NewHealthPolicy(interval, staleAfter, offlineAfter time.Duration) (HealthPolicy, error) {
	if interval <= 0 {
		return HealthPolicy{}, errors.New("telemetry interval must be positive")
	}

	if staleAfter == 0 {
		staleAfter = 2 * interval
	}
	if offlineAfter == 0 {
		offlineAfter = 3 * interval
	}

	if staleAfter <= 0 {
		return HealthPolicy{}, errors.New("stale threshold must be positive")
	}
	if offlineAfter <= staleAfter {
		return HealthPolicy{}, errors.New("offline threshold must be greater than the stale threshold")
	}

	return HealthPolicy{StaleAfter: staleAfter, OfflineAfter: offlineAfter}, nil
}
