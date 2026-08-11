package simulator

import (
	"context"

	"github.com/Stewz00/wattfeder/internal/household"
)

// Next returns the next observation for the current simulated interval. The simulator never
// exhausts on its own; it cycles through days indefinitely, matching a source that keeps
// producing telemetry until the caller stops asking. A nil envelope with a nil error means the
// interval produced no telemetry at all, which is how a missing heartbeat arrives. Next lets
// Simulator satisfy application.TelemetrySource without importing that package, since Go
// interfaces are structural.
func (s *Simulator) Next(context.Context) (*household.ObservationEnvelope, error) {
	envelope, _, err := s.NextObservation()
	return envelope, err
}

// Apply applies one battery command to the simulated household and advances the simulated
// clock by one interval. A nil command means the interval produced no command and the battery
// idles. Apply lets Simulator satisfy application.CommandSink without importing that package.
func (s *Simulator) Apply(_ context.Context, command *household.Command) error {
	return s.Complete(command)
}
