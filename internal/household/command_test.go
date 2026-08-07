package household

import (
	"math"
	"testing"
)

func TestCommandValidate(t *testing.T) {
	tests := []struct {
		name    string
		command Command
		wantErr bool
	}{
		{
			name:    "charge command",
			command: Command{Decision: DecisionCharge, PowerKW: 1.5, Reason: "PV surplus"},
		},
		{
			name:    "discharge command",
			command: Command{Decision: DecisionDischarge, PowerKW: 1.5, Reason: "High price"},
		},
		{
			name:    "idle command",
			command: Command{Decision: DecisionIdle, Reason: "Battery reserve"},
		},
		{
			name:    "missing reason",
			command: Command{Decision: DecisionIdle},
			wantErr: true,
		},
		{
			name:    "blank reason",
			command: Command{Decision: DecisionIdle, Reason: "  \t"},
			wantErr: true,
		},
		{
			name:    "unknown decision",
			command: Command{Decision: "unknown", Reason: "Unknown"},
			wantErr: true,
		},
		{
			name:    "negative power",
			command: Command{Decision: DecisionCharge, PowerKW: -1, Reason: "Invalid"},
			wantErr: true,
		},
		{
			name:    "NaN power",
			command: Command{Decision: DecisionCharge, PowerKW: math.NaN(), Reason: "Invalid"},
			wantErr: true,
		},
		{
			name:    "infinite power",
			command: Command{Decision: DecisionCharge, PowerKW: math.Inf(1), Reason: "Invalid"},
			wantErr: true,
		},
		{
			name:    "zero charge power",
			command: Command{Decision: DecisionCharge, Reason: "Invalid"},
			wantErr: true,
		},
		{
			name:    "non-zero idle power",
			command: Command{Decision: DecisionIdle, PowerKW: 1, Reason: "Invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.command.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
