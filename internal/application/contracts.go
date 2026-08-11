package application

import (
	"context"
	"errors"

	"github.com/Stewz00/wattfeder/internal/household"
)

// ErrSourceExhausted is returned by TelemetrySource.Next when the source has no more
// observations to give. It ends a Run cleanly rather than as a failure.
var ErrSourceExhausted = errors.New("telemetry source exhausted")

// TelemetrySource produces one household observation per interval.
type TelemetrySource interface {
	// Next returns the next observation, or ErrSourceExhausted when the source has no more to
	// give. A nil envelope with a nil error means the interval produced no telemetry at all,
	// which is how a missing heartbeat arrives.
	Next(ctx context.Context) (*household.ObservationEnvelope, error)
}

// CommandSink applies one battery command to whatever holds the battery.
// A nil command means the interval produced no command and the battery idles.
type CommandSink interface {
	Apply(ctx context.Context, command *household.Command) error
}
