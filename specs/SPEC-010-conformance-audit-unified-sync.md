---
id: SPEC-010
title: "Conformance audit + unified fold/drill sync — bring order to per-tool dirs"
status: Draft — Phase 1 (audit) in flight; Phases 2–3 open for discussion
owner: nmccready
created: 2026-07-28
updated: 2026-07-28
related: SPEC-002, SPEC-003, SPEC-007
---

# [SPEC-010] Feature: Conformance audit + unified fold/drill sync

## Overview

Per-tool directories (`~/.claude`, `~/.cursor`, `.claude/`, …) accrete
artifacts from many writers: `sync-agents`, the tools themselves
(plugin/skill installers), and humans. Today `sync-agents` can only see
the artifacts *it* expects to manage — it has no view of what else
occupies the trees it fans out into, and its collision behavior differs
between scopes (local `sync` whole-bucket skip-or-`RemoveAll`; global
`sync` per-artifact skip-or-backup).

This spec brings **conformance and tracking** to the per-tool dirs,
sourced from the canonical `.agents/` tree + `sources.yaml`/lock, under
three precedence rules:

1. **`.agents/` wins for its own artifacts.** If an artifact exists in
   the canonical tree, the per-tool slot for it should be a (possibly
   folded) symlink back into `.agents/`. A real file/dir shadowing it
   is a **conflict** — surfaced, never silently destroyed.
2. **Destination-only artifacts are hands-off.** Anything in a
   per-tool dir that `.agents/` doesn't claim is **foreign**: report
   it, never touch it.
3. **Adoption is explicit.** Relocating a foreign artifact into
   `.agents/` (for encapsulation + tracking) happens only on an
   explicit command, never as a side effect of `sync`.

## Ownership taxonomy

Every entry in a managed subdir of a per-tool dir is exactly one of:

| State | Meaning | Action |
|---|---|---|
| `synced` | symlink at the expected path, pointing at the canonical artifact | none |
| `folded` | expected path *resolves* to the canonical artifact via an ancestor symlink (e.g. `.claude/skills/foo -> ../../.agents/skills/foo`, so `foo/SKILL.md` resolves correctly) | none — conformant at a coarser granularity |
| `drifted` | symlink exists but points elsewhere | `sync` repairs |
| `missing` | nothing at the expected path | `sync` creates |
| `conflict` (`not-a-symlink`) | real file/dir shadows an artifact `.agents/` claims | warn + skip; resolve only via explicit `--force` (backup-rename, never delete) or adoption |
| `orphaned` | symlink pointing *into* the `.agents/` tree, but no current artifact claims it (artifact removed/renamed) | prune candidate (`clean` / `fix`) |
| `foreign` | anything else — not claimed by `.agents/`, not pointing into it | **hands off**; adoption candidate |

**Hard boundary:** the audit and every future phase operate ONLY on the
known artifact subdirs of each tool (for Claude: `skills/`, `commands/`,
`rules/`, `agents/`, `plans/`, `specs/`, `adrs/`; for Windsurf:
`windsurf/global_workflows/`; for Cursor: `rules/`; Copilot/Codex:
their concat file). The tool root itself is **application state**
(credentials, sessions, history, caches) and is never enumerated,
reported on, or mutated.

## Phase 1 — Conformance audit (in flight)

Extend `global status` with the **reverse sweep**. Today the command is
forward-only: it classifies every destination the `.agents/` tree
expects. Phase 1 adds enumeration of what actually occupies the managed
subdirs, classifying unexpected entries as `foreign` or `orphaned`, and
adds `folded` recognition to the forward pass (a dir-level symlink that
correctly resolves an artifact is conformant, not a conflict).

Requirements:

- Read-only. `global status` performs no writes, exactly as today.
- Forward pass: a destination whose expected path resolves (via
  `EvalSymlinks`) to the canonical artifact through an ancestor symlink
  reports `[folded]` instead of `[not-a-symlink]`.
- Reverse sweep: one entry per unexpected child of each managed subdir
  — `[foreign]` (hands-off) or `[orphaned]` (points into the `.agents/`
  tree but unclaimed). Sweep is one level deep; foreign dirs are
  reported as a single entry, not recursed into.
- Summary line with counts per state so a messy tree is sizeable at a
  glance.
- Existing output lines for the forward pass keep their format
  (regex-friendly `[state] tool/type/name -> path`).

### Acceptance criteria

- AC-1: a real dir at an expected artifact path still reports
  `not-a-symlink` (conflict).
- AC-2: a dir-level symlink into `.agents/` that resolves the expected
  artifact reports `folded`, and the sweep does not double-report it.
- AC-3: an entry in a managed subdir with no claiming artifact and no
  link into `.agents/` reports `foreign` and is never mutated.
- AC-4: a symlink into the `.agents/` tree whose artifact no longer
  exists reports `orphaned`.
- AC-5: entries outside managed subdirs (tool app state) never appear
  in output.

## Phase 2 — Adoption (open, discussion)

An explicit command to relocate a `foreign` artifact into `.agents/`
(move + re-sync replaces it with a symlink), reusing `fix`'s
move/merge/`--no-clobber` machinery and the `.replaced-by-sync-agents`
backup convention. Open questions:

- Surface: `adopt <tool> <bucket>/<name>…` as its own verb vs
  `fix --from <tool>`. Leaning: own verb — adoption is a policy action
  ("take ownership"), `fix` is repair/migration.
- Bulk mode (`--all-foreign`) with mandatory `--dry-run`-first UX for
  large cleanups.
- Conflict resolution: per-artifact `--ours`/`--theirs` instead of the
  run-wide `--force` blunderbuss.

## Phase 3 — Unified fold/drill materializer (open, discussion)

Local `sync` and `global sync` converge on one algorithm (GNU Stow's
tree-folding model):

- Destination absent → one symlink at the highest safe level
  (**fold**), maximally `<tool>/<bucket>`.
- Destination is a real dir → descend and link per-child (**drill**),
  recursing; foreign siblings coexist untouched.
- Destination is a real file conflicting with a claimed artifact →
  conflict per the taxonomy above.

Notes and hazards to resolve before implementation:

- **Never fold the tool root.** `~/.claude -> ~/.agents` would route
  the tool's own state writes (credentials, sessions) into the
  source-of-truth tree. Fold depth starts at the bucket level.
- **Fold capture semantics:** through a folded bucket link, a tool
  that later installs into e.g. `.claude/skills/` writes *into*
  `.agents/skills/` — silent auto-adoption. Drilled links keep future
  foreign writes foreign. Decide default (likely: drill once any
  foreign content exists; fold only pristine buckets) and document.
- Local `CmdSync` today ignores the semantic router entirely
  (whole-bucket links, flat target list, no Codex). Unification means
  deciding whether project scope adopts semantic routing (skills →
  `.claude/commands/` etc.) or the router grows a "mirror" mode for
  local scope. This is the largest open decision.
- Migration: existing whole-bucket symlinks are valid folds under the
  new model; drilling happens lazily only when coexistence is needed.

## Non-goals

- Auto-adoption of foreign artifacts (violates rule 3).
- Enumerating or mutating tool application state.
- Two-way sync (destination edits flowing back into `.agents/` other
  than via explicit adoption).
