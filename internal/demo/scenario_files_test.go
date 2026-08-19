package demo

import (
	"context"
	"io"
	"testing"
)

// TestShippedScenariosLoadAndRun exercises the two scenario files documented in README.md as
// the project's headline demos (`make demo` and `make demo-faults`), which are otherwise only
// ever run manually.
func TestShippedScenariosLoadAndRun(t *testing.T) {
	paths := []string{
		"../../scenarios/demo.json",
		"../../scenarios/unreliable-telemetry-day.json",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			scenario, err := LoadScenario(path)
			if err != nil {
				t.Fatalf("LoadScenario(%q) error = %v", path, err)
			}

			if err := Run(context.Background(), scenario, io.Discard); err != nil {
				t.Fatalf("Run(%q) error = %v", path, err)
			}
		})
	}
}
