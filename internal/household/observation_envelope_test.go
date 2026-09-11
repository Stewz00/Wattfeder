package household

import (
	"testing"
	"time"
)

func TestObservationEnvelopeValidateAcceptsAvailableWithTelemetry(t *testing.T) {
	envelope := validObservationEnvelope()

	if err := envelope.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestObservationEnvelopeValidateAcceptsUnavailableWithoutTelemetry(t *testing.T) {
	envelope := validObservationEnvelope()
	envelope.Available = false
	envelope.Telemetry = nil

	if err := envelope.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestObservationEnvelopeValidateRejects(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*ObservationEnvelope)
	}{
		{name: "empty source device ID", modify: func(e *ObservationEnvelope) { e.SourceDeviceID = "" }},
		{name: "zero received time", modify: func(e *ObservationEnvelope) { e.ReceivedAt = time.Time{} }},
		{name: "non-UTC received time", modify: func(e *ObservationEnvelope) {
			e.ReceivedAt = e.ReceivedAt.In(time.FixedZone("CEST", 2*60*60))
		}},
		{name: "unavailable source with telemetry", modify: func(e *ObservationEnvelope) {
			e.Available = false
		}},
		{name: "available source without telemetry", modify: func(e *ObservationEnvelope) {
			e.Telemetry = nil
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validObservationEnvelope()
			tt.modify(&envelope)

			if err := envelope.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
		})
	}
}

func TestReceivedAtOrNow(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	receivedAt := now.Add(-time.Hour)

	tests := []struct {
		name     string
		envelope *ObservationEnvelope
		want     time.Time
	}{
		{
			name:     "nil envelope falls back to now",
			envelope: nil,
			want:     now,
		},
		{
			name:     "zero ReceivedAt falls back to now",
			envelope: &ObservationEnvelope{ReceivedAt: time.Time{}},
			want:     now,
		},
		{
			name:     "non-UTC ReceivedAt falls back to now",
			envelope: &ObservationEnvelope{ReceivedAt: receivedAt.In(time.FixedZone("CEST", 2*60*60))},
			want:     now,
		},
		{
			name:     "well-formed UTC ReceivedAt is used",
			envelope: &ObservationEnvelope{ReceivedAt: receivedAt},
			want:     receivedAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReceivedAtOrNow(tt.envelope, now)
			if !got.Equal(tt.want) {
				t.Fatalf("ReceivedAtOrNow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func validObservationEnvelope() ObservationEnvelope {
	raw := validRawTelemetry()
	return ObservationEnvelope{
		SourceDeviceID: raw.DeviceID,
		ReceivedAt:     raw.EventTime.Add(time.Second),
		Telemetry:      &raw,
		Available:      true,
	}
}
