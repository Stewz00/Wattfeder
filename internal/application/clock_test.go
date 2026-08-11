package application

import (
	"context"
	"testing"
	"time"
)

func TestRealClockNowReturnsWallClockTime(t *testing.T) {
	clock := NewRealClock()

	before := time.Now()
	got := clock.Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Errorf("Now() = %v, want between %v and %v", got, before, after)
	}
}

func TestRealClockTickFiresAfterDuration(t *testing.T) {
	clock := NewRealClock()
	const wait = 5 * time.Millisecond

	start := time.Now()
	select {
	case fired := <-clock.Tick(t.Context(), wait):
		if fired.Before(start.Add(wait)) {
			t.Errorf("tick fired at %v, want at or after %v", fired, start.Add(wait))
		}
	case <-time.After(time.Second):
		t.Fatal("Tick() never fired")
	}
}

func TestRealClockTickEndsEarlyOnCancellation(t *testing.T) {
	clock := NewRealClock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	select {
	case fired := <-clock.Tick(ctx, time.Hour):
		t.Errorf("tick fired with %v, want no value after cancellation", fired)
	case <-time.After(50 * time.Millisecond):
		// No tick within the bound: cancellation ended the wait without firing, as expected.
	}
}

func TestInstantClockNeverWaits(t *testing.T) {
	start := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	clock := NewInstantClock(start)

	select {
	case fired := <-clock.Tick(t.Context(), time.Hour):
		if !fired.Equal(start.Add(time.Hour)) {
			t.Errorf("first tick = %v, want %v", fired, start.Add(time.Hour))
		}
	default:
		t.Fatal("Tick() blocked instead of returning an already-ready channel")
	}
}

func TestInstantClockNowAdvancesByEveryTickedDuration(t *testing.T) {
	start := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	clock := NewInstantClock(start)

	if got := clock.Now(); !got.Equal(start) {
		t.Fatalf("Now() before any tick = %v, want %v (the run's opening interval sees start)", got, start)
	}

	<-clock.Tick(t.Context(), time.Hour)
	want := start.Add(time.Hour)
	if got := clock.Now(); !got.Equal(want) {
		t.Errorf("Now() after first tick = %v, want %v", got, want)
	}

	<-clock.Tick(t.Context(), time.Hour)
	want = want.Add(time.Hour)
	if got := clock.Now(); !got.Equal(want) {
		t.Errorf("Now() after second tick = %v, want %v", got, want)
	}

	<-clock.Tick(t.Context(), 30*time.Minute)
	want = want.Add(30 * time.Minute)
	if got := clock.Now(); !got.Equal(want) {
		t.Errorf("Now() after third tick = %v, want %v (a tick advances by its own duration)", got, want)
	}
}
