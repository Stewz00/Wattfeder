// Package persistence defines the records and atomic boundary used by durable storage adapters.
package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

// TelemetryRecord is one validated source event and the UTC time at which Wattfeder received it.
type TelemetryRecord struct {
	Event      household.Telemetry
	ReceivedAt time.Time
}

// CommandRecord is the command created from one telemetry event and its UTC creation time.
// EventID links the command to its source event; one event can produce at most one command.
type CommandRecord struct {
	EventID   household.EventID
	Command   household.Command
	CreatedAt time.Time
}

// ProcessingResult contains every record that becomes durable when one event is processed.
type ProcessingResult struct {
	Telemetry   TelemetryRecord
	LatestState household.State
	Command     CommandRecord
}

// Validate reports whether a processing result is complete, UTC-normalized, and internally consistent.
func (r ProcessingResult) Validate() error {
	if err := r.Telemetry.Event.Validate(); err != nil {
		return fmt.Errorf("invalid telemetry record: %w", err)
	}
	if err := validateUTCTime("telemetry event time", r.Telemetry.Event.EventTime); err != nil {
		return err
	}
	if err := validateUTCTime("telemetry received time", r.Telemetry.ReceivedAt); err != nil {
		return err
	}

	var expectedState household.State
	if err := expectedState.ApplyTelemetry(r.Telemetry.Event); err != nil {
		return fmt.Errorf("derive latest state: %w", err)
	}
	if r.LatestState != expectedState {
		return errors.New("latest state must match the telemetry event")
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

// CommitStatus reports whether an atomic processing result was newly stored or was already present.
type CommitStatus uint8

const (
	// CommitStored means the telemetry, command, and latest state were committed together.
	CommitStored CommitStatus = iota + 1
	// CommitDuplicate means the event ID was already processed and no records were changed.
	CommitDuplicate
)

// Repository owns schema migrations and durable household processing records.
type Repository interface {
	// Migrate applies pending schema migrations in order and is safe to call on an up-to-date database.
	Migrate(ctx context.Context) error

	// LatestState returns the most recently committed state for a device.
	LatestState(ctx context.Context, deviceID string) (state household.State, found bool, err error)

	// CommitProcessing stores telemetry, its resulting command, and latest state in one transaction.
	// A duplicate event ID returns CommitDuplicate without changing any record.
	// An error means none of the supplied records became durable.
	CommitProcessing(ctx context.Context, result ProcessingResult) (CommitStatus, error)
}
