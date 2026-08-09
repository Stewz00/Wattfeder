package household

// Disposition classifies how one observation was handled.
type Disposition string

const (
	// DispositionAccepted means the event became the latest state and history.
	DispositionAccepted Disposition = "accepted"
	// DispositionHistoryOnly means the event was stored but did not replace the latest state.
	DispositionHistoryOnly Disposition = "history_only"
	// DispositionDuplicate means the event ID was already processed.
	DispositionDuplicate Disposition = "duplicate"
	// DispositionRejected means the event was not stored due to invalid or inconsistent data.
	DispositionRejected Disposition = "rejected"
	// DispositionMissing means no observation arrived for the interval.
	DispositionMissing Disposition = "missing"
	// DispositionUnavailable means the source explicitly reported unavailability.
	DispositionUnavailable Disposition = "unavailable"
)
