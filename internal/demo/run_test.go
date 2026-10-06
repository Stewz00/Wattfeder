package demo

import (
	"testing"
)

func TestCompareExpected(t *testing.T) {
	tests := []struct {
		name     string
		produced []string
		expected []string
		wantErr  string
	}{
		{
			name:     "equal lengths with a mismatch",
			produced: []string{"charge"},
			expected: []string{"discharge"},
			wantErr:  `label 0 = "charge", expected "discharge"`,
		},
		{
			name:     "produced longer than expected",
			produced: []string{"charge", "idle"},
			expected: []string{"charge"},
			wantErr:  "label count = 2, expected 1",
		},
		{
			name:     "produced shorter than expected",
			produced: []string{"charge"},
			expected: []string{"charge", "idle"},
			wantErr:  "label count = 1, expected 2",
		},
		{
			name:     "empty expected skips the check",
			produced: []string{"charge", "idle"},
			expected: nil,
			wantErr:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := compareExpected("label", test.produced, test.expected)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("compareExpected() error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("compareExpected() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
