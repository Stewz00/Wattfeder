package application

import "context"

// Observer lets an adapter attach itself to interval processing without the runtime importing
// that adapter. A nil Observer disables observation entirely.
type Observer interface {
	// BeginInterval opens one interval's observation scope. The returned context carries what
	// the observer attaches to the interval's work — a trace span, for instance — and the
	// returned function closes the scope.
	BeginInterval(ctx context.Context) (context.Context, EndInterval)
}

// EndInterval closes one interval with the record it produced and the error that ended it.
// record is the zero Record when the interval failed before producing one.
type EndInterval func(record Record, err error)

// noopObserver is used whenever Agent.Observer is nil, so Run never has to branch on it.
type noopObserver struct{}

func (noopObserver) BeginInterval(ctx context.Context) (context.Context, EndInterval) {
	return ctx, func(Record, error) {}
}
