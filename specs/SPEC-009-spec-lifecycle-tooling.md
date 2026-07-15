---
id: SPEC-009
title: "Spec lifecycle tooling: lint spec-bucket checks + `spec retire` verb"
status: Draft
owner: nmccready
created: 2026-07-14
updated: 2026-07-14
related: SPEC-004, SPEC-008
---

# [SPEC-009] Feature: Spec lifecycle tooling (`lint` spec checks + `sync-agents spec retire`)

## Problem

Real-world repos (ours and sync-agents customers') accumulate completed
specs and plans that linger in the working tree, where agents grep or
glob them and treat stale intent as current. The convention shipped in
PR #78 (ledger + precedence rule + delete-on-ship, see
[`specs/README.md`](README.md)) defines *what* should happen; nothing
automates *finding* the crud or *executing* the retirement. Doing it by
hand doesn't scale past one repo.

## Design principles

- **`fix` stays judgment-free.** Its contract is structural repair with
  exactly one canonical answer per operation. Spec retirement involves
  a doneness judgment and content deletion — it must not live there.
- **Split detect from execute**, mirroring the shipped ADR verbs
  (`adr accept|deny|propose`): `lint` *reports*, a `spec` verb family
  *acts on declared state*. Judgment lives only in a human/agent
  flipping frontmatter `status:`; every command stays mechanical.
- Git history is the archive. No `specs/completed/` graveyards.

## Part A — `lint` spec-bucket checks

New per-bucket check group (extends SPEC-004 Part E's engine) for
`specs/` (and `plans/` where present):

| Code | Rule | `--fix` action |
| ---- | ---- | -------------- |
| E201 | spec file missing frontmatter `id`/`status` | inject `id` from filename; `status: Draft` stub |
| E202 | `id` ≠ filename SPEC number | report only |
| E203 | frontmatter `status` says shipped/implemented but file still in active tree | report only → retire candidate |
| E204 | SPEC-ID reference resolves to neither a live spec file nor a ledger row | report only (dangling ref) |
| E205 | ledger row says retired but spec file still exists (or vice-versa) | report only |
| W201 | spec `updated:` older than N days (default 90) with status Draft/Active | report only (stale candidate) |
| W202 | `related:` entries pointing at retired specs without ledger annotation | report only |

Exit-code semantics identical to skill lint: unfixed E-level findings
fail CI; W-level never affects exit code.

## Part B — `sync-agents spec` verb family

```bash
sync-agents spec list                 # table: id, title, status, file/ledger state
sync-agents spec retire <id>          # retire one spec (see below)
sync-agents spec retire --marked      # retire everything with shipped/implemented status
sync-agents spec retire --marked --dry-run   # the customer "crud sweep": list candidates + why
```

`spec retire <id>` mechanics (all-or-nothing, idempotent):

1. Verify frontmatter `status` is a shipped/terminal value (refuse
   otherwise — no judgment in the tool).
2. Append/update the spec's **ledger row** in `specs/README.md`:
   status, retire commit (filled post-commit or `HEAD` placeholder),
   durable-doc destinations (from a `--docs <paths>` flag or a
   `promoted_to:` frontmatter list; empty allowed with a warning).
3. **Delete the spec body** (fully shipped) or, with `--partial`, keep
   the file and print a reminder to trim shipped parts (trimming prose
   is judgment — left to the author/agent).
4. Regenerate the index (`sync-agents index`) so `AGENTS.md` reflects
   the tree.
5. Create/refresh `specs/README.md` from a template if absent
   (bootstraps the convention in customer repos that never had it).

## Part C — customer onboarding sweep

`spec retire --marked --dry-run` + `lint` gives existing repos a
one-command audit: "7 specs look shipped but linger (E203), 3 dangling
SPEC refs (E204), ledger missing (bootstrap)". The README gains a
"cleaning up an existing repo" recipe: run lint → confirm/flip
statuses → `spec retire --marked` → review the single resulting PR.

## Optional sugar (defer)

`sync-agents tidy` = `lint --fix` + `spec retire --marked` + `index`.
Build the verbs first; a broom that hides judgment recreates the
problem this spec exists to avoid. Ship only if the two-step flow
proves annoying in practice.

## Non-goals

- Auto-detecting doneness from merged PRs/issues (judgment; maybe a
  future `--suggest` that cross-references closed PRs, report-only).
- Managing `docs/` promotion content (prose extraction is authorship,
  not tooling).
- ADR bucket changes (already has its lifecycle verbs).

## Acceptance

- [ ] `lint` emits E201–W202 on a fixture repo with lingering specs
- [ ] `spec retire <id>` on a fixture: ledger row written, file
      deleted, index regenerated, second run is a no-op
- [ ] `spec retire --marked --dry-run` lists candidates with reasons,
      changes nothing
- [ ] Ledger bootstrap works in a repo with specs but no ledger
- [ ] Docs: `docs/commands/spec.md`, lint doc table extended, README
      recipe for existing-repo cleanup
