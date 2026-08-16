package observability

import (
	"context"
	"errors"
	"strings"
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

func TestMultiObserverClosesScopesInReverseOrder(t *testing.T) {
	var order []string
	outer := &orderedObserver{name: "outer", order: &order}
	inner := &orderedObserver{name: "inner", order: &order}

	_, end := NewMultiObserver(outer, inner).BeginInterval(context.Background())
	end(application.Record{}, nil)

	want := "begin outer, begin inner, end inner, end outer"
	if got := strings.Join(order, ", "); got != want {
		t.Errorf("scope order = %q, want %q", got, want)
	}
}

// orderedObserver appends its name to a shared list, so a test can assert that scopes close in
// the reverse of the order they opened in.
type orderedObserver struct {
	name  string
	order *[]string
}

func (o *orderedObserver) BeginInterval(ctx context.Context) (context.Context, application.EndInterval) {
	*o.order = append(*o.order, "begin "+o.name)
	return ctx, func(application.Record, error) {
		*o.order = append(*o.order, "end "+o.name)
	}
}
