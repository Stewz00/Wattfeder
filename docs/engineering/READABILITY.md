# Readability standard

## Target reader

Assume the reader understands basic programming and arithmetic.

Do not assume knowledge of:

* battery models
* electricity markets
* power-system conventions
* Wattfeder architecture
* mathematical model choices
* repository-specific units or terminology

The reader does not need to understand every implementation detail immediately. They must be able to find the purpose, assumptions, units, and boundaries without guessing.

## When explanation is required

Add an explanation when any answer is yes:

1. Does the code encode a domain assumption?
2. Could another engineer reasonably choose a different formula?
3. Are the units or valid range not obvious?
4. Is boundary behavior important?
5. Is the behavior deliberately simplified?
6. Would the reader otherwise need external domain knowledge?

## What to explain

### Domain meaning

```go
// Positive battery power means charging.
// Grid export uses the opposite sign convention.
```

### Formula choice

```go
// A sine curve creates a smooth daylight profile that is zero
// at sunrise and sunset and reaches its maximum at noon.
```

### Unit conversion

```go
// Power in kW multiplied by time in hours produces energy in kWh.
intervalHours := interval.Minutes() / minutesPerHour
```

### Boundaries

```go
// Clamp charging so stored energy cannot exceed battery capacity.
```

### Simplifications

```go
// Efficiency is constant. Temperature, battery age, and charge rate
// are not represented.
```

## What not to explain

Do not restate syntax.

Bad:

```go
// Check whether power is positive.
if powerKW > 0 {
```

Do not use comments to repair vague names.

Bad:

```go
// x is battery capacity.
x := config.Value
```

Rename the value instead.

Do not call simplified output realistic or accurate without evidence.

## Mathematical code

Names and comments should expose:

* input domain
* normalization
* formula purpose
* output range
* boundary handling

A reader should be able to answer:

* What does the formula model?
* Why was this shape selected?
* What units and ranges are used?
* What happens at boundaries?
* What real behavior is omitted?

## Energy-domain code

Always distinguish:

* power from energy
* state of charge from stored energy
* import from export
* generation from consumption
* physical limits from control targets
* measured values from simulated values

Conversions must expose all required values:

```go
// Convert relative state of charge to stored energy because power
// changes energy directly, not percentage.
storedEnergyKWh :=
	stateOfChargePercent / maximumStateOfChargePercent * capacityKWh
```

## Tests

Tests should protect decisions that could be changed incorrectly.

Required cases depend on the operation:

| Operation       | Cases                                |
| --------------- | ------------------------------------ |
| Conversion      | Simple manually verified input       |
| Curve           | Peak, boundaries, intermediate value |
| Wraparound      | Both sides of midnight               |
| Limit           | Below, at, and above the limit       |
| Sign convention | Positive, zero, and negative         |
| Validation      | Invalid inputs                       |
| Simulation      | Repeated deterministic result        |

A readable test shows:

* scenario
* expected behavior
* reason the behavior matters

Use tolerances only for floating-point results.

## Review standard

Code is sufficiently readable when a junior software engineer without energy experience can:

1. State what the code models.
2. Follow the main transformations.
3. Identify units and sign conventions.
4. Identify important assumptions.
5. Explain boundary behavior.
6. Find tests for important decisions.
7. Identify major limitations.
