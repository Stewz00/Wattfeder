# Workspace instructions

@.codex/skills/coding-file-docs/SKILL.md

The long-form standard behind this file is
[`docs/engineering/READABILITY.md`](docs/engineering/READABILITY.md). It defines
the target reader, the cases every kind of operation must test, and the review
standard a change is judged against. Read it when this file does not settle a
question.

## Go Comments and Domain Documentation

Write code that explains its mechanics through names and structure. Comments explain domain meaning, assumptions, constraints, or non-obvious decisions.

### Comment when

* units, sign conventions, time zones, interval boundaries, or reference frames are not obvious;
* a formula encodes domain knowledge;
* the implementation is an approximation or simplification;
* a constraint, invariant, or edge case explains an otherwise surprising operation;
* a future maintainer might incorrectly “simplify” the code;
* an exported type or function needs a short usage contract.

### Do not comment when

* the comment only repeats the code;
* a clearer name or extracted function would explain the operation;
* the comment describes syntax or control flow;
* the information is already stated nearby.

Prefer one precise comment over several line-by-line comments.

### Go API documentation

Document exported types, functions, methods, interfaces, and constants.

Keep comments to one sentence when possible. Add details only for behavior callers must know.

```go
// ApplyDispatch advances the battery state by one interval.
// Positive power discharges the battery; the result may be constrained.
func ApplyDispatch(...) DispatchResult
```

Do not write empty descriptions:

```go
// Battery represents a battery.
type Battery struct{}
```

### Domain examples

Explain sign conventions:

```go
// Positive power exports energy to the grid; negative power imports it.
GridPowerKW float64
```

Explain approximations and their limits:

```go
// daylightFactor approximates PV output with a half-sine between sunrise
// and sunset. It is deterministic, not a physical irradiance model.
func daylightFactor(...) float64
```

Explain non-obvious formulas:

```go
// Convert SOC to energy because dispatch changes stored energy in kWh,
// not SOC percentage directly.
storedEnergyKWh := socPercent / maximumSOCPercent * capacityKWh
```

Explain protective operations:

```go
// Clamp measurement noise before dispatch to preserve the physical SOC range.
socPercent = clamp(socPercent, minimumSOCPercent, maximumSOCPercent)
```

Do not explain obvious operations:

```go
// Calculate the available energy.
availableEnergyKWh := storedEnergyKWh - reserveEnergyKWh
```

### Decision order

Before adding a comment:

1. Can clearer naming remove the need? Rename.
2. Can a small function express the concept? Extract it.
3. Does the reader still need domain context or rationale? Comment it.
4. Does the explanation exceed roughly four lines? Move it to package docs, domain docs, or an ADR.

Comments must remain correct when the implementation changes. Remove stale or speculative comments.
