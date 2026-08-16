package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/Stewz00/wattfeder/internal/application"
)

type spyObserver struct {
	begun int
	ended []error
}

func (s *spyObserver) BeginInterval(ctx context.Context) (context.Context, application.EndInterval) {
	s.begun++
	return ctx, func(_ application.Record, err error) {
		s.ended = append(s.ended, err)
	}
}

func TestMultiObserverFansOutToEveryObserver(t *testing.T) {
	first, second := &spyObserver{}, &spyObserver{}
	multi := NewMultiObserver(first, second)

	wantErr := errors.New("boom")
	_, end := multi.BeginInterval(context.Background())
	end(application.Record{}, wantErr)

	for i, spy := range []*spyObserver{first, second} {
		if spy.begun != 1 {
			t.Errorf("observer %d begun = %d, want 1", i, spy.begun)
		}
		if len(spy.ended) != 1 || !errors.Is(spy.ended[0], wantErr) {
			t.Errorf("observer %d ended = %v, want one entry wrapping %v", i, spy.ended, wantErr)
		}
	}
}
