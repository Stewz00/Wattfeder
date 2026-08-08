# Workspace instructions

@.codex/skills/coding-file-docs/SKILL.md

# Agent rules

## Objective

Wattfeder must be readable by software engineers without prior energy-domain knowledge.

Readers must be able to identify:

* what the code models
* which units and conventions it uses
* why non-trivial calculations exist
* which assumptions simplify reality
* which tests protect important behavior

Do not explain basic syntax.

## Priorities

1. Correctness
2. Explicit assumptions
3. Readability
4. Testability
5. Simplicity
6. Performance

Deviate only for a documented requirement.

## Required explanations

Explain code that contains:

* domain calculations
* mathematical models
* unit conversions
* sign conventions
* time conventions
* normalization or interpolation
* boundary or wraparound behavior
* clamps and physical limits
* simplified models
* non-obvious constants
* decisions with several reasonable alternatives

Comments must explain **why**, not repeat **what** the code does.

Bad:

```go
// Divide distance by width.
normalizedDistance := distance / widthHours
```

Good:

```go
// Express distance in peak widths so widthHours directly controls
// how quickly the Gaussian falls away from its peak.
normalizedDistance := distance / widthHours
```

Document domain and mathematical functions with:

* purpose
* input units and ranges
* output unit or range
* important boundary behavior
* relevant simplifications

## Naming and units

Use domain-specific names.

Prefer:

```go
capacityKWh
powerKW
durationHours
priceEURPerMWh
stateOfChargePercent
```

Avoid vague names such as `value`, `amount`, `factor`, or `data`.

Include units in names when the type does not provide them.

Use one term per concept across code, tests, configuration, and documentation.

## Constants

Extract unexplained domain values into named constants.

Explain values based on:

* physical limits
* domain conventions
* simulation assumptions
* business rules
* arbitrary demo choices

## Domain conventions

Make these explicit wherever relevant:

* power versus energy
* percentage versus fraction
* import versus export
* charging versus discharging
* local time versus UTC
* measured versus simulated values

Do not require the reader to infer a sign convention or unit.

## Structure

Functions should expose:

1. input
2. transformation
3. decision
4. output

Extract a function when its name clarifies a domain operation.

Do not create abstractions that only add indirection.

## Tests

Add focused tests for:

* domain calculations
* unit conversions
* mathematical models
* boundaries
* sign conventions
* wraparound behavior
* physical limits
* invalid inputs
* deterministic simulation behavior

Every non-trivial domain or mathematical function requires direct tests.

Use manually verifiable inputs.

Name tests by protected behavior:

```go
func TestDailyGaussianWrapsDistanceAcrossMidnight(t *testing.T)
```

Do not test trivial assignments or language behavior.

## Documentation

Use short technical sentences.

Separate:

* current behavior
* assumptions
* limitations
* planned behavior

Do not use marketing claims or unexplained abbreviations.

## Completion

Before finishing:

1. Run formatting.
2. Run relevant tests.
3. Check affected documentation.
4. Report assumptions, tests, commands, and remaining limitations.

Never report a command as successful unless it was executed.
