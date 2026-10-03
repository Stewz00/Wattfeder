package demo

import (
	"strings"
	"testing"
)

func TestCompareExpectedSkipsEmptyExpected(t *testing.T) {
	if err := compareExpected("decision", []string{"charge", "idle"}, nil); err != nil {
		t.Errorf("compareExpected() error = %v, want nil", err)
	}
}

func TestCompareExpectedRejectsShorterProduced(t *testing.T) {
	err := compareExpected("decision", []string{"charge"}, []string{"charge", "idle"})
	if err == nil {
		t.Fatal("compareExpected() error = nil, want a count mismatch error")
	}
	wantErr := "decision count = 1, expected 2"
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("compareExpected() error = %q, want it to contain %q", err.Error(), wantErr)
	}
}

func TestCompareExpectedRejectsLongerProduced(t *testing.T) {
	err := compareExpected("decision", []string{"charge", "idle", "discharge"}, []string{"charge", "idle"})
	if err == nil {
		t.Fatal("compareExpected() error = nil, want a count mismatch error")
	}
	wantErr := "decision count = 3, expected 2"
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("compareExpected() error = %q, want it to contain %q", err.Error(), wantErr)
	}
}

func TestCompareExpectedReportsDifferingElement(t *testing.T) {
	err := compareExpected("decision", []string{"charge", "idle"}, []string{"charge", "discharge"})
	if err == nil {
		t.Fatal("compareExpected() error = nil, want a value mismatch error")
	}
	wantErr := `decision 1 = "idle", expected "discharge"`
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("compareExpected() error = %q, want it to contain %q", err.Error(), wantErr)
	}
}
