// Package persistence defines the records and atomic boundary used by durable storage adapters.
package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

// TelemetryRecord is one validated source event stored as history, along with the
// disposition that caused it to be stored.
type TelemetryRecord struct {
	Event             household.Telemetry
	ReceivedAt        time.Time
	Disposition       household.Disposition
	DispositionReason string
}

// CommandRecord is the command created from one telemetry event and its UTC creation time.
// EventID links the command to its source event; one event can produce at most one command.
type CommandRecord struct {
	EventID   household.EventID
	Command   household.Command
	CreatedAt time.Time
}

// ObservationResult is everything that becomes durable when one interval's observation is
// processed. Telemetry is nil when the observation was not stored (rejected, missing, or
// unavailable). LatestState is non-nil only when the latest device state must be replaced.
// Command is non-nil only when a command must be applied. Health is always required.
type ObservationResult struct {
	DeviceID    string
	Telemetry   *TelemetryRecord
	LatestState *household.State
	Command     *CommandRecord
	Health      household.DeviceHealth
}

// Validate reports whether an observation result is complete, UTC-normalized, and internally consistent.
func (r ObservationResult) Validate() error {
	if strings.TrimSpace(r.DeviceID) == "" {
		return errors.New("device ID must not be empty")
	}

	if r.Telemetry != nil {
		if err := r.Telemetry.Event.Validate(); err != nil {
			return fmt.Errorf("invalid telemetry record: %w", err)
		}
		if err := validateUTCTime("telemetry event time", r.Telemetry.Event.EventTime); err != nil {
			return err
		}
		if err := validateUTCTime("telemetry received time", r.Telemetry.ReceivedAt); err != nil {
			return err
		}
		if r.Telemetry.Event.DeviceID != r.DeviceID {
			return errors.New("telemetry device ID must match the observation device ID")
		}
		switch r.Telemetry.Disposition {
		case household.DispositionAccepted, household.DispositionHistoryOnly:
		default:
			return fmt.Errorf("telemetry disposition must be accepted or history_only, got %q", r.Telemetry.Disposition)
		}
	}

	if r.LatestState != nil {
		if r.Telemetry == nil || r.Telemetry.Disposition != household.DispositionAccepted {
			return errors.New("latest state requires an accepted telemetry record")
		}
		var expectedState household.State
		if err := expectedState.ApplyTelemetry(r.Telemetry.Event); err != nil {
			return fmt.Errorf("derive latest state: %w", err)
		}
		if *r.LatestState != expectedState {
			return errors.New("latest state must match the telemetry event")
		}
	}

	if r.Command != nil {
		if r.LatestState == nil {
			return errors.New("command requires a latest state update")
		}
		if r.Command.EventID != r.Telemetry.Event.EventID {
			return errors.New("command event ID must match the telemetry event ID")
		}
		if err := r.Command.Command.Validate(); err != nil {
			return fmt.Errorf("invalid command record: %w", err)
		}
		if err := validateUTCTime("command creation time", r.Command.CreatedAt); err != nil {
			return err
		}
	}

	if err := validateHealth(r.Health); err != nil {
		return err
	}

	return nil
}

func validateHealth(health household.DeviceHealth) error {
	switch health.Status {
	case household.HealthOnline, household.HealthStale, household.HealthOffline, household.HealthInvalid:
	default:
		return fmt.Errorf("health status must be a known value, got %q", health.Status)
	}
	if err := validateUTCTime("health transition time", health.TransitionTime); err != nil {
		return err
	}
	// LastContactAt may legitimately be zero for a device that has never made contact.
	if health.LastContactAt.Location() != time.UTC {
		return errors.New("health last contact time must use UTC")
	}

	return nil
}

func validateUTCTime(name string, timestamp time.Time) error {
	if timestamp.IsZero() {
		return fmt.Errorf("%s must not be zero", name)
	}
	if timestamp.Location() != time.UTC {
		return fmt.Errorf("%s must use UTC", name)
	}

	return nil
}

// CommitStatus reports whether an atomic observation result was newly stored or was already present.
type CommitStatus uint8

const (
	// CommitStored means the observation's durable records were committed.
	CommitStored CommitStatus = iota + 1
	// CommitDuplicate means the event ID was already processed and no records were changed.
	CommitDuplicate
)

// DeviceSnapshot is the complete restorable state for one device: its latest valid
// measurements and its durable health.
type DeviceSnapshot struct {
	State  household.State
	Health household.DeviceHealth
}

// Repository owns schema migrations and durable household processing records.
type Repository interface {
	// Migrate applies pending schema migrations in order and is safe to call on an up-to-date database.
	Migrate(ctx context.Context) error

	// Snapshot returns the most recently committed device snapshot: latest state and
	// durable health.
	Snapshot(ctx context.Context, deviceID string) (snapshot DeviceSnapshot, found bool, err error)

	// CommitProcessing stores one interval's observation result in one transaction.
	// A duplicate event ID returns CommitDuplicate without changing any record.
	// An error means none of the supplied records became durable.
	CommitProcessing(ctx context.Context, result ObservationResult) (CommitStatus, error)
}
