---
name: update-roadmap
description: >-
  Manually invoked roadmap-sync workflow. Use only when the user explicitly
  writes `$update-roadmap`; inspect current staged, unstaged, and relevant
  untracked work, then update `docs/roadmap.md` and the reader guides with
  verified execution state only when the diff is roadmap-relevant. Leave
  documentation unchanged for unrelated work, then always hand off to the local
  `$cmmt-msg` workflow. Invoking `$update-roadmap` authorizes that commit
  handoff.
---

# Update Roadmap

1. Confirm the repository and `docs/roadmap.md` exist.
2. Inspect `git status --short`, `git diff`, `git diff --cached`, and relevant
   untracked files.
3. Apply a relevance gate before editing. Work is relevant only when it changes
   a milestone's status, its completion evidence, current product behavior or a
   documented gap, or the current focus. Skills, agent instructions, the commit
   workflow, and the documentation itself are not evidence by themselves.
4. If no changed work passes the gate, leave the documentation byte-for-byte
   unchanged and continue to the commit handoff. For mixed diffs, consider only
   the relevant subset.
5. Verify relevant behavior with proportionate checks; do not mark unverified or
   aspirational work complete.
6. Update `docs/roadmap.md`: the progress table, the affected milestone's status
   and completion evidence, and the current focus. A milestone becomes complete
   only when every exit criterion is verified.
7. Reconcile `README.md` and the reader guides (`docs/SETUP.md`,
   `docs/DEMO.md`, `docs/ARCHITECTURE.md`, and `docs/MODEL.md`) with verified
   current behavior and update any that are no longer current. Persistence
   changes also belong in `docs/engineering/PERSISTENCE-DESIGN.md`.
8. Do not create a decision record here. The record set is capped and its rules
   live in `docs/engineering/adr/README.md`; a record is written only after the
   behavior it describes exists.
9. Review the documentation diff for accuracy and report what changed.
10. Read and execute `../cmmt-msg/SKILL.md` after the roadmap update or bypass.

Never change implementation to match the documentation. Invoking
`$update-roadmap` explicitly authorizes local commits through its chained
workflow, but never a push or history rewrite.
