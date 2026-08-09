package household

import (
	"testing"
	"time"
)

func TestNewHealthPolicyDefaultsToTwoAndThreeIntervals(t *testing.T) {
	policy, err := NewHealthPolicy(10*time.Minute, 0, 0)
	if err != nil {
		t.Fatalf("NewHealthPolicy() error = %v", err)
	}

	if policy.StaleAfter != 20*time.Minute {
		t.Errorf("StaleAfter = %v, want %v", policy.StaleAfter, 20*time.Minute)
	}
	if policy.OfflineAfter != 30*time.Minute {
		t.Errorf("OfflineAfter = %v, want %v", policy.OfflineAfter, 30*time.Minute)
	}
}

func TestNewHealthPolicyAcceptsExplicitThresholds(t *testing.T) {
	policy, err := NewHealthPolicy(10*time.Minute, 5*time.Minute, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewHealthPolicy() error = %v", err)
	}

	if policy.StaleAfter != 5*time.Minute {
		t.Errorf("StaleAfter = %v, want %v", policy.StaleAfter, 5*time.Minute)
	}
	if policy.OfflineAfter != 15*time.Minute {
		t.Errorf("OfflineAfter = %v, want %v", policy.OfflineAfter, 15*time.Minute)
	}
}

func TestNewHealthPolicyRejects(t *testing.T) {
	tests := []struct {
		name         string
		interval     time.Duration
		staleAfter   time.Duration
		offlineAfter time.Duration
	}{
		{name: "non-positive interval", interval: 0, staleAfter: 0, offlineAfter: 0},
		{name: "negative explicit stale threshold", interval: 10 * time.Minute, staleAfter: -time.Minute, offlineAfter: 15 * time.Minute},
		{name: "offline threshold equal to stale threshold", interval: 10 * time.Minute, staleAfter: 5 * time.Minute, offlineAfter: 5 * time.Minute},
		{name: "offline threshold less than stale threshold", interval: 10 * time.Minute, staleAfter: 10 * time.Minute, offlineAfter: 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewHealthPolicy(tt.interval, tt.staleAfter, tt.offlineAfter); err == nil {
				t.Fatal("NewHealthPolicy() error = nil, want error")
			}
		})
	}
}
