package application

import (
	"context"
	"time"
)

// Clock paces the runtime loop. Production uses the real clock; tests inject a fake one so a
// run of a thousand intervals still finishes instantly.
type Clock interface {
	Now() time.Time
	Tick(ctx context.Context, d time.Duration) <-chan time.Time
}

// NewRealClock returns a Clock backed by the wall clock. Tick waits the full duration before
// firing, ending early with no value if ctx is cancelled first.
func NewRealClock() Clock {
	return realClock{}
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) Tick(ctx context.Context, d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	go func() {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case fired := <-timer.C:
			ch <- fired
		case <-ctx.Done():
		}
	}()
	return ch
}

// NewInstantClock returns a Clock that never waits: Tick always fires immediately. Now advances
// by each ticked duration instead of following the wall clock, so a fast-paced run's notion of
// "now" tracks a simulated schedule starting at start rather than real time.
func NewInstantClock(start time.Time) Clock {
	return &instantClock{now: start}
}

type instantClock struct {
	now time.Time
}

func (c *instantClock) Now() time.Time {
	return c.now
}

func (c *instantClock) Tick(_ context.Context, d time.Duration) <-chan time.Time {
	c.now = c.now.Add(d)

	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}
