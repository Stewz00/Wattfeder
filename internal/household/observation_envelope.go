package household

import (
	"errors"
	"strings"
	"time"
)

// ObservationEnvelope is one delivery from a telemetry source for one interval.
// Telemetry is nil when the source explicitly reported unavailability.
type ObservationEnvelope struct {
	SourceDeviceID string
	ReceivedAt     time.Time
	Telemetry      *RawTelemetry
	Available      bool
}

// Validate reports whether the envelope is well-formed and internally consistent.
// It does not validate the enclosed telemetry measurements themselves.
func (e ObservationEnvelope) Validate() error {
	if strings.TrimSpace(e.SourceDeviceID) == "" {
		return errors.New("source device ID must not be empty")
	}

	if e.ReceivedAt.IsZero() {
		return errors.New("receive time must not be zero")
	}
	if e.ReceivedAt.Location() != time.UTC {
		return errors.New("receive time must use UTC")
	}

	if !e.Available && e.Telemetry != nil {
		return errors.New("an unavailable source must not carry telemetry")
	}

	return nil
}
