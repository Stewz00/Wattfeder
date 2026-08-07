---
name: update-roadmap
description: >-
  Manually invoked roadmap-sync workflow. Use only when the user explicitly
  writes `$update-roadmap`; inspect current staged, unstaged, and relevant
  untracked work, then update `docs/llm-checklist-roadmap.md` with verified
  execution state only when the diff is roadmap-relevant. Leave tracking
  unchanged for unrelated work, then always hand off to the local `$cmmt-msg`
  workflow. Invoking `$update-roadmap` authorizes that commit handoff.
---

# Update Roadmap

1. Confirm the repository and `docs/llm-checklist-roadmap.md` exist.
2. Inspect `git status --short`, `git diff`, `git diff --cached`, and relevant
   untracked files.
3. Apply a relevance gate before editing. Work is relevant only when it changes
   evidence for a checklist item, current product behavior or gap, preserved
   product decision, or next product task. Skills, agent instructions, commit
   workflow, and the checklist itself are not evidence by themselves.
4. If no changed work passes the gate, leave the checklist byte-for-byte
   unchanged and continue to the commit handoff. For mixed diffs, consider only
   the relevant subset.
5. Verify relevant behavior with proportionate checks; do not mark unverified or
   aspirational work complete.
6. Update only completed items, current behavior and gaps, preserved decisions,
   and the next task. Keep `docs/roadmap.md` authoritative.
7. Review the roadmap diff for accuracy and report what changed.
8. Read and execute `../cmmt-msg/SKILL.md` after the roadmap update or bypass.

Never change implementation to match the checklist. Invoking `$update-roadmap`
explicitly authorizes local commits through its chained workflow, but never a
push or history rewrite.
