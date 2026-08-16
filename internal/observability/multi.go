package observability

import (
	"context"

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
		for _, end := range ends {
			end(record, err)
		}
	}
}
