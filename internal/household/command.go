package household

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
