---
id: SPEC-004
title: "New asset buckets: agents, hooks, plans, specs — .claude-first routing on a centralized bucket registry"
status: Draft
owner: nmccready
created: 2026-07-02
updated: 2026-07-02
related: SPEC-002, SPEC-003, SPEC-005
---

# [SPEC-004] Feature: New asset buckets (agents, hooks, plans, specs)

## Overview

Extend the canonical `.agents/` tree beyond the current three buckets
(`rules`, `skills`, `workflows`) with four new ones:

| Bucket        | Contents                                   | Primary destination                     |
| ------------- | ------------------------------------------ | --------------------------------------- |
| `agents/`     | Subagent definitions (`<name>.md`)         | `.claude/agents/<name>.md`              |
| `hooks/`      | Harness hook fragments (`<name>.json`)     | merged into `.claude/settings.json`     |
| `plans/`      | Implementation plans / task breakdowns     | indexed reference docs (see Part D)     |
| `specs/`      | Durable design / requirements documents    | indexed reference docs (see Part D)     |

Claude Code is the first-class target for all four. Other tools skip
buckets they have no equivalent for, using the same graceful-skip
pattern `destination.go` already uses (e.g. Passive skill → skipped).

Prerequisite: a **bucket registry refactor** (Part A) so that adding a
bucket is one registry entry instead of edits to ~6 hardcoded
`[]string{"rules", "skills", "workflows"}` sites in `agent.go`.

## Motivation

1. **Subagents are un-syncable today.** Claude Code reads
   `.claude/agents/<name>.md` (project) and `~/.claude/agents/`
   (global). There is no `.agents/` bucket for them, so teams
   copy-paste subagent files per project — exactly the problem
   sync-agents exists to solve.
2. **Hooks are un-syncable today.** Claude Code hooks live inside
   `.claude/settings.json`. Nothing in sync-agents can carry a hook
   from `.agents/` into that file, and naive syncing would clobber
   user-authored settings.
3. **Plans and specs are first-class agent context.** Spec-driven
   workflows (including this repo's own `specs/SPEC-00X` process)
   produce durable documents that agents should discover. Today they
   have no bucket, no index entry, and no routing.

### plans vs specs — why both?

The distinction is **lifecycle, not plumbing**:

- **`specs/`** — durable design/requirements documents: _what_ to build
  and why. Long-lived, committed, the source of truth. Survive the
  effort that produced them.
- **`plans/`** — implementation plans and task breakdowns: _how/when_.
  Working documents scoped to an effort; typically archived or deleted
  when the effort lands.

Both route identically (Part D). Supporting both costs one registry
entry each after Part A, and conflating them forces teams to choose
between polluting durable specs with ephemeral task lists or losing
plan history.

## Goals

- Part A: single bucket registry consumed by `sync`, `status`, `clean`,
  `fix`, `init`, `add`, `index`, and the semantic router. Pure
  refactor; zero behavior change.
- Part B: `agents` bucket synced to `.claude/agents/` (local symlink,
  global copy via `promote`/`global sync`).
- Part C: `hooks` bucket merged into `.claude/settings.json` with
  strict ownership tracking — sync-agents only ever adds/removes
  entries it wrote; user-authored settings are never touched.
- Part D: `plans` + `specs` buckets as indexed reference docs.
- All changes backwards compatible (see Backwards Compatibility).

## Non-Goals

- New tool targets (gemini, opencode) — separate effort.
- Executing or validating hook commands — SPEC-005 territory.
- Remote install of the new bucket types — SPEC-003 gains them for
  free once buckets are registry-driven.
- Translating Claude hook fragments to other harnesses' hook systems.

## Part A — Bucket registry refactor (Phase 0, standalone PR)

Today the bucket list is hardcoded in at least: `CmdSync`
(`agent.go:363`), `CmdStatus` (`:436`), `CmdClean` (`:474`), `CmdFix`
(`:668-673`), `CmdInit` (`:245-247`), `CmdAdd` (`:295-305`), plus the
`ArtifactType` enum in `promote.go:16-22` and routing in
`destination.go`.

Design:

```go
// internal/agent/bucket.go
type Bucket struct {
    Name      string       // directory name under .agents/
    Artifact  ArtifactType // singular artifact type ("rule", "agent", …)
    Aliases   []string     // CmdAdd aliases ("rule", "rules", …)
    Layout    BucketLayout // FlatFiles | SkillDirs | JSONFragments
    InInit    bool         // created by `sync-agents init`
    Reference bool         // Part D reference-doc semantics
}

var Buckets = []Bucket{ /* rules, skills, workflows — then Part B–D append here */ }
```

All call sites iterate `Buckets` (or filter it) instead of literal
slices. `CmdAdd`'s type switch and `CmdFix`'s validation derive from
`Aliases`. Behavior with only the classic three registered is
byte-for-byte identical — verified by the existing bats + go suites
passing unmodified.

## Part B — `agents` bucket (subagents)

- Source: `.agents/agents/<name>.md`. Frontmatter is Claude's subagent
  schema (`name`, `description`, `tools`, `model`) — sync-agents treats
  it as opaque except `name` for collision checks.
- Local sync: symlink the bucket dir, same as existing buckets:
  `.claude/agents -> ../.agents/agents`. If a real (non-symlink)
  `.claude/agents/` already exists, warn and skip (never merge or
  clobber) — same conflict rule `CreateSymlink` applies today.
- Global: `promote agent <name>` copies to `~/.agents/agents/`;
  `global sync` routes to `~/.claude/agents/<name>.md`.
- Semantics: always Invocable-like (Claude auto-discovers agents);
  never included in the managed CLAUDE.md import block.
- Other tools: skipped (no subagent equivalent in cursor / windsurf /
  copilot / codex today).

## Part C — `hooks` bucket

- Source: `.agents/hooks/<name>.json`. Each file is one hook fragment
  in Claude's settings schema:

```json
{
  "event": "PreToolUse",
  "matcher": "Bash",
  "hooks": [{ "type": "command", "command": "./scripts/check.sh" }]
}
```

- Destination: `.claude/settings.json` → `hooks.<event>[]`. JSON has no
  comments, so marker-block ownership (the CLAUDE.md approach) doesn't
  work. Instead, ownership is tracked in a sidecar state file
  `.agents/.sync/claude-hooks-state.json` recording exactly which
  entries sync-agents wrote (fragment name → event + content hash).
  Regeneration removes precisely the recorded entries and re-adds
  current fragments; user-authored hook entries and all other settings
  keys pass through untouched. Writes are atomic (tmp + rename) and
  key-order stable to keep diffs reviewable.
- `clean` removes only recorded entries; if `hooks` becomes empty it is
  dropped; if settings.json becomes `{}` it is deleted only when
  sync-agents created it (recorded in the state file).
- Idempotency: `sync` twice → identical settings.json (tested).
- Naming collision: the existing `sync-agents hook` command (git
  pre-commit installer) is renamed `sync-agents git-hook`; `hook`
  remains as a hidden deprecated alias printing a pointer to the new
  name. Docs updated.
- Other tools: skipped in v1.

## Part D — `plans` + `specs` buckets (reference docs)

- Source: `.agents/plans/<name>.md`, `.agents/specs/<name>.md`. Flat
  files; subdirectories allowed (indexed recursively) since plan/spec
  sets are often grouped per effort.
- Local sync: symlink bucket dirs into `.claude/` (`.claude/plans`,
  `.claude/specs`) so they're @-mentionable and discoverable, plus an
  **AGENTS.md index section** listing each doc with its frontmatter
  `description` (same generator that indexes rules today).
- NOT included in the managed CLAUDE.md import block by default —
  reference docs are pulled on demand, not preloaded into context.
  Frontmatter `import: true` opts a doc in.
- Semantics: new `Reference` semantic alongside Invocable/Passive in
  `semantic.go`; `promote`/`global sync` route them to
  `~/.claude/{plans,specs}/` for the global scope.
- Other tools: indexed in AGENTS.md (which codex/copilot consume via
  concat); no per-tool dirs in v1.

## Backwards Compatibility

- New buckets activate **only when their directory exists**. A v0.3.x
  `.agents/` tree syncs byte-for-byte identically.
- `sync-agents init` keeps creating only the classic three buckets;
  new ones are created on demand by `sync-agents add
  agent|hook|plan|spec <name>`.
- `.agents/config` schema unchanged; `targets=` still the only key.
- AGENTS.md index gains sections only for present buckets.
- `hook` → `git-hook` rename ships with a working deprecated alias; no
  script breaks.
- Index/lock sidecar lives under `.agents/.sync/` (new, gitignored by
  the existing `updateGitignore` mechanism).

## Test Plan

- Part A: full existing go + bats suites pass with zero test edits.
- Part B/D: golden-tree tests per bucket (sync, re-sync idempotent,
  clean restores pristine state, status output).
- Part C: table tests for settings.json merge — empty file, missing
  file, user-authored hooks present, ours interleaved with theirs,
  fragment removed upstream, tampered state file (fail loud, no
  writes), `clean` on each of the above.
- Collision tests: pre-existing real `.claude/agents/` dir → warn+skip.

## Rollout

1. PR 0 (Part A) — registry refactor, no behavior change.
2. PR 1 (Part B) — `agents` bucket.
3. PR 2 (Part D) — `plans` + `specs` (simpler than hooks; ships early).
4. PR 3 (Part C) — `hooks` + `git-hook` rename.
5. Docs: README bucket table, `docs/architecture/buckets.md`.
