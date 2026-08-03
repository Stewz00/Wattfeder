---
name: cmmt-msg
description: >-
  Manually invoked Git commit workflow. Use only when the user explicitly writes
  `$cmmt-msg`; never auto-trigger for ordinary coding, review, or commit
  requests. Inspect current repository changes, split them into coherent local
  commits when justified, use a conventional type-and-scope subject, never add
  AI attribution, and never push or rewrite history.
---
# Commit Changes

1. Confirm the current directory is a Git repository. Stop on merge conflicts or an empty working tree.
2. Inspect `git status --short`, `git diff`, `git diff --cached`, and relevant untracked files.
3. Group changes by one reviewable purpose. Keep implementation with its direct tests and documentation. Split only when groups are independently meaningful and the intermediate repository state remains coherent.
4. Keep unrelated subsystems or concerns in separate commits when they are independently committable.
5. Classify each group with exactly one type:
    - `feat`: add behavior or capability
    - `bugfix`: correct faulty behavior
    - `chores`: perform maintenance, tooling, dependency, or configuration work
    - `docs`: change documentation only; include direct feature or bugfix documentation in that implementation commit when appropriate
    - `refactor`: restructure code without intended behavior change
    - `tests`: change tests only; include direct tests in the corresponding feature or bugfix commit when appropriate
6. Format project changes as `<type>: <imperative subject>`; use `<type>(<scope>): <imperative subject>` only for a distinct supporting or non-project area.
    - Example: `feat: add meter-reading endpoint`
    - Example: `bugfix(parser): handle empty input`
    - Example: `chores(ci): align lint configuration`
    - Use an unscoped subject for changes to the core Wattfeder project.
    - Use a short scope for distinct areas such as skills, CI, or developer tooling; never use a scope-before-type form.
7. Treat the scope as metadata, not as the extraction boundary. Preserve clear directory ownership; rely primarily on paths and coherent commits.
8. Write a concise imperative subject. Do not add `Co-authored-by`, `Generated-by`, ChatGPT, OpenAI, or other AI attribution.
9. Stage only the files or hunks for the next group. Verify with `git diff --cached --stat` and `git diff --cached --check`, then commit locally.
10. Repeat until all intended changes are committed. Never push, amend, rebase, reset, or modify source files.
11. Report created commits using `git log --oneline -n <count>` and mention any changes intentionally left uncommitted.

Prefer one commit when splitting would create artificial, dependent, misleading, or non-buildable intermediate states.
