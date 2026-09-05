package household

import (
	"fmt"
	"time"
)

// ClassifyInput is everything the domain needs to classify one interval's observation.
// Envelope is nil when no observation arrived for the interval at all (a missing heartbeat).
// PriorState and PriorHealth must be the prior values for the same device the envelope
// reports as its source (Envelope.SourceDeviceID), when Envelope is non-nil.
type ClassifyInput struct {
	Envelope    *ObservationEnvelope
	PriorState  State
	PriorHealth DeviceHealth
	Policy      HealthPolicy
	Interval    time.Duration
	Now         time.Time
}

// ClassifyResult is the outcome of classifying one interval's observation.
// Telemetry is non-nil when the event must be stored as history.
// State is non-nil only when the latest device state must be replaced.
type ClassifyResult struct {
	Disposition     Disposition
	Reason          string
	Telemetry       *Telemetry
	State           *State
	SuppressCommand bool
	Health          DeviceHealth
}

// Classify applies the shared disposition and health-transition rules: it decides how one
// interval's observation must be recorded, whether it replaces the latest device state,
// whether its command may be applied, and the resulting device health.
func Classify(in ClassifyInput) ClassifyResult {
	if in.Envelope == nil {
		return classifyMissing(in)
	}
	if err := in.Envelope.Validate(); err != nil {
		return rejected(in, err.Error())
	}

	if !in.Envelope.Available {
		return classifyUnavailable(in)
	}
	return classifyTelemetry(in)
}

func classifyTelemetry(in ClassifyInput) ClassifyResult {
	envelope := in.Envelope
	event, err := envelope.Telemetry.Validate()

	switch {
	case err != nil:
		return rejected(in, err.Error())
	case event.DeviceID != envelope.SourceDeviceID:
		return rejected(in, fmt.Sprintf(
			"telemetry device ID %q does not match source device ID %q", event.DeviceID, envelope.SourceDeviceID,
		))
	case event.EventTime.After(envelope.ReceivedAt):
		return rejected(in, "event time is after the receive time")
	}

	if !in.PriorState.UpdatedAt.IsZero() && !event.EventTime.After(in.PriorState.UpdatedAt) {
		return historyOnly(in, event)
	}

	return accepted(in, event)
}

func accepted(in ClassifyInput, event Telemetry) ClassifyResult {
	newState := in.PriorState
	if err := newState.ApplyTelemetry(event); err != nil {
		// event was already validated, matches PriorState's device (see ClassifyInput), and is
		// confirmed strictly newer, so this cannot fail
		panic(fmt.Sprintf("apply already-validated telemetry: %v", err))
	}

	receivedAt := in.Envelope.ReceivedAt
	delayed := receivedAt.Sub(event.EventTime) > in.Interval

	status, reason := HealthOnline, ""
	if delayed {
		status, reason = HealthStale, "event arrived after its telemetry interval had elapsed"
	}

	return ClassifyResult{
		Disposition:     DispositionAccepted,
		Telemetry:       &event,
		State:           &newState,
		SuppressCommand: delayed,
		Health:          transitionHealth(in.PriorHealth, status, reason, receivedAt, receivedAt),
	}
}

func historyOnly(in ClassifyInput, event Telemetry) ClassifyResult {
	receivedAt := in.Envelope.ReceivedAt
	health := evaluateHealth(in.PriorHealth, in.PriorState, in.Policy, receivedAt, receivedAt)

	return ClassifyResult{
		Disposition:     DispositionHistoryOnly,
		Reason:          "event time is not strictly newer than the latest state",
		Telemetry:       &event,
		SuppressCommand: true,
		Health:          health,
	}
}

func rejected(in ClassifyInput, reason string) ClassifyResult {
	receivedAt := ReceivedAtOrNow(in.Envelope, in.Now)

	return ClassifyResult{
		Disposition:     DispositionRejected,
		Reason:          reason,
		SuppressCommand: true,
		Health:          transitionHealth(in.PriorHealth, HealthInvalid, reason, receivedAt, receivedAt),
	}
}

func classifyMissing(in ClassifyInput) ClassifyResult {
	reason := "no observation received for this interval"
	health := evaluateHealth(in.PriorHealth, in.PriorState, in.Policy, in.Now, in.PriorHealth.LastContactAt)

	return ClassifyResult{
		Disposition:     DispositionMissing,
		Reason:          reason,
		SuppressCommand: true,
		Health:          health,
	}
}

func classifyUnavailable(in ClassifyInput) ClassifyResult {
	reason := "source reported unavailability"

	return ClassifyResult{
		Disposition:     DispositionUnavailable,
		Reason:          reason,
		SuppressCommand: true,
		Health: transitionHealth(
			in.PriorHealth, HealthOffline, reason, in.Envelope.ReceivedAt, in.PriorHealth.LastContactAt,
		),
	}
}

// evaluateHealth applies the shared health precedence: offline contact timeout, then
// unresolved invalid data, then stale latest-event age, otherwise online.
func evaluateHealth(prior DeviceHealth, latestState State, policy HealthPolicy, now, lastContactAt time.Time) DeviceHealth {
	var status DeviceHealthStatus
	var reason string

	switch {
	case !lastContactAt.IsZero() && now.Sub(lastContactAt) >= policy.OfflineAfter:
		status, reason = HealthOffline, "no contact within the offline threshold"
	case prior.Status == HealthInvalid:
		status, reason = HealthInvalid, prior.Reason
	case !latestState.UpdatedAt.IsZero() && now.Sub(latestState.UpdatedAt) >= policy.StaleAfter:
		status, reason = HealthStale, "latest accepted event age exceeds the stale threshold"
	default:
		status, reason = HealthOnline, ""
	}

	return transitionHealth(prior, status, reason, now, lastContactAt)
}

// transitionHealth carries the transition time forward unless the status actually changed.
func transitionHealth(prior DeviceHealth, status DeviceHealthStatus, reason string, at, lastContactAt time.Time) DeviceHealth {
	transitionTime := prior.TransitionTime
	if status != prior.Status || transitionTime.IsZero() {
		transitionTime = at
	}

	return DeviceHealth{
		Status:         status,
		Reason:         reason,
		TransitionTime: transitionTime,
		LastContactAt:  lastContactAt,
	}
}
