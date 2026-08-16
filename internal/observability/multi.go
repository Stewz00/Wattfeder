package observability

import (
	"context"
	"slices"

	"github.com/Stewz00/wattfeder/internal/application"
)

// MultiObserver fans one interval scope out to several observers — logging, metrics, tracing —
// so Agent.Observer only ever needs to hold one value.
type MultiObserver struct {
	observers []application.Observer
}

// NewMultiObserver combines observers into one.
func NewMultiObserver(observers ...application.Observer) *MultiObserver {
	return &MultiObserver{observers: observers}
}

func (m *MultiObserver) BeginInterval(ctx context.Context) (context.Context, application.EndInterval) {
	ends := make([]application.EndInterval, len(m.observers))
	for i, observer := range m.observers {
		var end application.EndInterval
		ctx, end = observer.BeginInterval(ctx)
		ends[i] = end
	}
	return ctx, func(record application.Record, err error) {
		// The scopes nest: each observer's BeginInterval received the context the one before it
		// returned, so they unwind in the reverse order. That keeps an observer's scope open for
		// as long as every scope opened inside it.
		for _, end := range slices.Backward(ends) {
			end(record, err)
		}
	}
}
