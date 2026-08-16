package observability

import (
	"errors"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
)

func TestReadinessNotReadyBeforeFirstInterval(t *testing.T) {
	r := NewReadiness(time.Hour)
	failing, ready := r.Check(time.Now())
	if ready {
		t.Error("ready = true, want false before any interval completed")
	}
	if failing != "telemetry" {
		t.Errorf("failing = %q, want %q", failing, "telemetry")
	}
}

func TestReadinessReadyAfterAnIntervalCompletes(t *testing.T) {
	r := NewReadiness(time.Hour)
	_, end := r.BeginInterval(t.Context())
	end(application.Record{}, nil)

	failing, ready := r.Check(time.Now())
	if !ready || failing != "" {
		t.Errorf("Check() = (%q, %v), want (\"\", true)", failing, ready)
	}
}

func TestReadinessUnreadyWhenTelemetryStalls(t *testing.T) {
	r := NewReadiness(time.Minute)
	_, end := r.BeginInterval(t.Context())
	end(application.Record{}, nil)

	failing, ready := r.Check(time.Now().Add(4 * time.Minute))
	if ready {
		t.Error("ready = true, want false once no interval has completed within 3x the interval")
	}
	if failing != "telemetry" {
		t.Errorf("failing = %q, want %q", failing, "telemetry")
	}
}

func TestReadinessUnreadyWhenLastCommitFailed(t *testing.T) {
	r := NewReadiness(time.Hour)
	_, end := r.BeginInterval(t.Context())
	end(application.Record{}, errors.New("commit processing result: boom"))

	failing, ready := r.Check(time.Now())
	if ready {
		t.Error("ready = true, want false after a commit failure")
	}
	if failing != "storage" {
		t.Errorf("failing = %q, want %q", failing, "storage")
	}
}
