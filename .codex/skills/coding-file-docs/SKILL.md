---
name: coding-file-docs
description: >-
  Format and review source-code comments for maximum readability. Use when adding or editing internal comments,
  Go exported API documentation, or other code-file documentation in this repository.
---

# Coding File Docs

Apply these conventions whenever creating or revising source-code comments.

## Internal comments

- Keep every line at or below 120 characters, including the comment marker
- Do not end internal comment lines with a period
- Prefer one short thought per comment line; leave a thought on one line when it fits
- When a thought exceeds the limit, wrap at a natural phrase boundary without reshaping other lines
- Start a new comment line after a complete thought instead of tightly wrapping a prose paragraph
- Keep content after a colon on the same line when it fits
- Add spaces around a dash used as a separator: `input - output`
- Preserve hyphens within compound words such as `non-finite` and `half-open`

Use fragments when they make an implementation detail easier to scan. Prioritize a natural reading rhythm
over formal sentence grammar.

```go
// These fixed parameters describe simple synthetic daily profiles
// Peak widths are Gaussian standard deviations in hours; scales are multipliers of each profile's
// baseline
```

## Exported API comments

Treat documentation for exported symbols as public API text:

- Write complete, conventional sentences with terminal punctuation
- Follow the language's documentation convention; in Go, begin with the exported identifier's name
- Keep IDE hovers and generated documentation understandable without surrounding source context
- Apply the 120-character maximum by wrapping at natural boundaries

When one comment both documents an exported API and records an internal detail, keep the exported-symbol
documentation conventional and move the implementation detail to a separate internal comment.

## Review

Before finishing a code-comment change, check that each comment uses the correct category, each line is at
most 120 characters, and wrapped lines preserve thought boundaries.
