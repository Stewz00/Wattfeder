---
name: readability-challenger
description: >-
  Manually invoked read-only review of code, tests, configuration, and documentation from the perspective of a
  junior software engineer without energy-domain knowledge. Use only when the user explicitly writes
  `$readability-challenger`; identify implicit domain assumptions or missing explanations and write a structured
  report without modifying implementation files. Never auto-trigger for general review or readability requests.
---

# Readability challenger

## Purpose

Perform a read-only review from the perspective of a junior software engineer without energy-domain knowledge.

Do not modify implementation files.

Create only a review report.

Do not infer missing meaning. Report where a reader must guess.

## Scope

Review the requested files and enough surrounding code to understand them.

Include related:

* production code
* tests
* configuration
* documentation

## Review questions

For each non-trivial function, determine:

1. What does it model?
2. What do its inputs mean?
3. Which units and ranges apply?
4. Why was the formula or branch chosen?
5. What happens at boundaries?
6. Which behavior is simplified?
7. Which tests protect it?

Create a finding when the repository does not provide a clear answer.

## Required findings

Report cases where a reader must guess:

* units or ranges
* sign conventions
* percentage versus fraction
* local time versus UTC
* mathematical model choice
* normalization purpose
* constant meaning
* boundary or midnight behavior
* physical limit or clamp purpose
* measured versus simulated data
* current versus planned behavior
* which test protects an important decision

Also report:

* misleading or stale comments
* comments that only repeat syntax
* vague names
* unexplained abbreviations
* mixed responsibilities
* unclear test scenarios
* unsupported documentation claims

## Exclusions

Do not report:

* personal style preferences
* standard formatting
* obvious syntax
* trivial assignments
* function length without a specific comprehension problem
* missing abstractions without a clear readability benefit

## Severity

**Blocker**

Likely to cause an incorrect understanding.

Examples:

* reversed or hidden sign convention
* mixed units
* comment contradicting code
* documentation describing missing behavior

**Major**

Important reasoning requires external investigation.

Examples:

* unexplained formula
* hidden unit
* missing boundary behavior
* undocumented simplification
* important behavior without a focused test

**Minor**

Meaning is recoverable but unnecessarily difficult.

Examples:

* vague name
* weak test name
* distant explanation

## Report path

Write to:

```text
reviews/readability/YYYY-MM-DD-<scope>.md
```

Do not overwrite existing reports. Add a numeric suffix when needed.

## Report format

```md
# Readability review: <scope>

## Summary

Files reviewed:
- `<path>`

Findings:
- Blocker: 0
- Major: 0
- Minor: 0

Overall assessment:

<short factual assessment>

## Findings

### R-001: <title>

Severity: Major  
Location: `path/file.go:42-57`

Reader problem:

<what cannot be understood>

Why it matters:

<risk or likely misunderstanding>

Evidence:

<relevant code or behavior>

What would clarify it:

<minimum information or structural change required>

## Unanswered questions

- <question not answered by the repository>

## Clear areas

- <important section that was already understandable>

## Review limits

- <anything not inspected or executed>
```

Each finding must describe one concrete comprehension problem.

Do not implement fixes.

Suggestions are secondary. Describe the missing information before proposing a solution.

## Completion response

Return:

* report path
* finding count by severity
* highest-risk areas
* unreviewed scope
