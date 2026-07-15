---
trigger: always_on
---

# specs lifecycle and precedence

`docs/` and the code are **current truth**. `specs/` contains **active
proposals only** — never treat a spec (active, retired, or dug out of
git history) as outranking docs or code; where they conflict, docs/code
win.

- The permanent index of every spec ever written is
  [`specs/README.md`](../../specs/README.md) (the ledger). Consult it —
  not git archaeology — to learn whether a SPEC ID shipped and where its
  durable content now lives in `docs/`.
- When a spec ships **fully**: in the same PR, promote its durable
  decisions/contracts into `docs/`, DELETE the spec file, and update its
  ledger row (status, retire commit, doc destinations). Git history is
  the archive; do not create `specs/completed/` or similar graveyards.
- When a spec ships **partially**: trim shipped parts to a short
  summary, keep only open work in the body, update frontmatter
  `status:` and the ledger row.
- Reference specs by ID (`SPEC-00N`), which stays resolvable via the
  ledger even after the file is deleted.
