package household

import "time"

// DeviceHealthStatus reports the operational health of one device's telemetry contact.
type DeviceHealthStatus string

const (
	// HealthOnline means a strictly newer valid event has been accepted within the stale threshold.
	HealthOnline DeviceHealthStatus = "online"
	// HealthStale means the latest accepted event is older than the stale threshold.
	HealthStale DeviceHealthStatus = "stale"
	// HealthOffline means contact has timed out or the source reported unavailability.
	HealthOffline DeviceHealthStatus = "offline"
	// HealthInvalid means the most recent observation was rejected and has not yet been resolved.
	HealthInvalid DeviceHealthStatus = "invalid"
)

// DeviceHealth is the durable health status of one device's telemetry contact.
type DeviceHealth struct {
	Status         DeviceHealthStatus
	Reason         string
	TransitionTime time.Time
	LastContactAt  time.Time
}
