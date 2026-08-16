package observability

import (
	"context"
	"sync"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
)

// Readiness is an application.Observer that tracks whether the runtime can currently process
// and persist telemetry. It is fed by the interval scope rather than an out-of-band probe: the
// SQLite adapter allows exactly one open connection (repository.go's SetMaxOpenConns(1)), so a
// readiness handler that queried storage on its own would contend with the runtime's single
// writer and could report red simply because a commit was in flight.
type Readiness struct {
	interval time.Duration

	mu            sync.Mutex
	started       bool
	lastCompleted time.Time
	lastErr       error
}

// NewReadiness tracks readiness against the given processing interval: telemetry is unready
// once no interval has completed within 3x it.
func NewReadiness(interval time.Duration) *Readiness {
	return &Readiness{interval: interval}
}

// BeginInterval records that one interval completed, and with what error, every time the
// returned EndInterval runs. Every error path ends the run itself, so distinguishing exactly
// which step failed brings the readiness probe no closer to true than knowing an interval ended
// in error at all.
//
// Unlike Logger and Metrics, this records the empty scope an exhausted source closes too. By
// then the run is over and the ops server is shutting down, so no probe can still read the
// difference.
func (r *Readiness) BeginInterval(ctx context.Context) (context.Context, application.EndInterval) {
	return ctx, func(_ application.Record, err error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.started = true
		r.lastCompleted = time.Now()
		r.lastErr = err
	}
}

// Check reports which check is failing, if any, as of now. An empty failing name means every
// check passed.
func (r *Readiness) Check(now time.Time) (failing string, ready bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.started {
		return "telemetry", false
	}
	if now.Sub(r.lastCompleted) > 3*r.interval {
		return "telemetry", false
	}
	if r.lastErr != nil {
		return "storage", false
	}
	return "", true
}
