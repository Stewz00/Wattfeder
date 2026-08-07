package household

import (
	"errors"
	"strings"
)

type Decision string

const (
	DecisionCharge    Decision = "charge"
	DecisionDischarge Decision = "discharge"
	DecisionIdle      Decision = "idle"
)

type Command struct {
	Decision Decision
	PowerKW  float64
	Reason   string
}

// Validate reports whether the command can safely control a battery interval.
func (c Command) Validate() error {
	if strings.TrimSpace(c.Reason) == "" {
		return errors.New("command reason must not be empty")
	}

	if !isFinite(c.PowerKW) || c.PowerKW < 0 {
		return errors.New("command power must be finite and non-negative")
	}

	switch c.Decision {
	case DecisionCharge, DecisionDischarge:
		if c.PowerKW == 0 {
			return errors.New("charge and discharge command power must be greater than 0")
		}
	case DecisionIdle:
		if c.PowerKW != 0 {
			return errors.New("idle command power must be 0")
		}
	default:
		return errors.New("command decision must be charge, discharge, or idle")
	}

	return nil
}
