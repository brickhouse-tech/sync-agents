---
id: SPEC-011
title: "`add agent` import + selective load + multi-tool subagent routing"
status: 🟡 Parts A–C shipped; Part D (selective load) open
owner: nmccready
created: 2026-09-09
updated: 2026-09-09  # rev 3: Parts A–C implemented; rev 2's mis-filing claim retracted
related: SPEC-002, SPEC-003, SPEC-004, SPEC-010
---

# [SPEC-011] Feature: `add agent` import + selective load + multi-tool subagent routing

## Overview

`sync-agents add agent <name>` already exists (SPEC-004 Part B): it
scaffolds `.agents/agents/<name>.md` from `templates.Agent` and the
bucket symlinks into `.claude/agents/` only. Three gaps remain:

1. **You cannot bring an agent you already have.** `add agent` only
   emits a blank template. Real subagent definitions already live
   elsewhere — a personas directory, another repo, a registered
   source — and the only way in today is `cp` by hand.
2. **The bucket is hard-pinned to Claude.** `Bucket.LocalTools` for
   `agents` is `["claude"]`. Cursor shipped native subagents at
   `.cursor/agents/` with the same markdown + YAML-frontmatter shape;
   opencode has `.opencode/agents/`. Neither receives anything.
3. **It is all-or-nothing.** Sync folds the whole bucket
   (`.claude/agents -> ../.agents/agents`), so every agent in the
   tree lands in every subagent-capable tool. There is no way to say
   "this one is Claude-only."

This spec closes all three, in that order of independence: Part A and
Part B ship standalone; Part C depends on both; Part D depends on
SPEC-010's drilled-sync work.

## Motivation

The canonical tree is supposed to be the single place an agent
definition lives. Today it is the single place a *newly scaffolded*
agent definition lives, for a single tool. Anyone who already curates
subagents (a personas directory symlinked into `~/.claude/agents`, a
shared team repo of reviewers) has no on-ramp, and anyone using more
than one harness maintains N copies.

## Goals

- Import an existing agent definition into `.agents/agents/` by copy
  or by symlink, from a local path or a registered source.
- Route the `agents` bucket to every tool with a native subagent
  surface: Claude, Cursor, opencode.
- Register opencode as a first-class `Tool`.
- Let an individual agent restrict which tools receive it.
- Keep tools without a subagent surface (Windsurf, Copilot, Codex)
  served through the `## Agents` section of `AGENTS.md`, unchanged.

## Non-Goals

- Translating frontmatter between harness dialects. Claude's `tools:`
  / `model:` and Cursor's `readonly:` / `is_background:` are not
  reconciled; unknown keys are preserved verbatim per SPEC-004 Part E
  and each tool ignores what it does not understand.
- A registry or marketplace of agents. `--from` takes a path or an
  already-registered source (SPEC-003); it does not resolve names
  against any index.
- Changing `add`'s behavior for other buckets beyond the generic
  `--from`/`--link` flags described in Part C.
- Adopting agents that already exist in a per-tool dir. That is
  SPEC-010's adoption command, not this one.

---

## Part A — Multi-tool subagent routing

### Requirement: agents bucket reaches every subagent-capable tool

The `agents` bucket SHALL sync to Claude, Cursor, and opencode at both
local and global scope; it SHALL NOT sync to tools with no subagent
surface.

Confirmed destination conventions:

| Tool | Local | Global | Native subagent dir |
|---|---|---|---|
| claude | `.claude/agents/` | `~/.claude/agents/` | yes |
| cursor | `.cursor/agents/` | `~/.cursor/agents/` | yes (Cursor 2.4) |
| opencode | `.opencode/agents/` | `~/.config/opencode/agents/` | yes |
| codeium/windsurf | — | — | no — index only |
| copilot | — | — | no — index only |
| codex | — | — | no — index only |

Implementation: `Buckets` entry for `agents` becomes
`LocalTools: []string{"claude", "cursor", "opencode"}`.

#### Scenario: cursor receives the bucket
- **GIVEN** a project with `.agents/agents/reviewer.md` and `cursor` in
  `.agents/config` targets
- **WHEN** `sync-agents sync` runs
- **THEN** `.cursor/agents` is a symlink to `../.agents/agents` and
  `.cursor/agents/reviewer.md` resolves

#### Scenario: copilot is untouched
- **GIVEN** the same project with `copilot` in targets
- **WHEN** `sync-agents sync` runs
- **THEN** no `agents` symlink is created under `.github/copilot/`, and
  `reviewer` appears in the `## Agents` section of `AGENTS.md`

#### Scenario: global sync honors the same restriction
- **GIVEN** `~/.agents/agents/reviewer.md`
- **WHEN** `sync-agents global sync` runs
- **THEN** `~/.claude/agents`, `~/.cursor/agents`, and
  `~/.config/opencode/agents` resolve to it, and no other tool's tree
  gains an `agents` entry

### Requirement: global scope routes agents as subagents

**Correction to rev 2.** This spec previously claimed global sync
mis-filed agents at every target — that agents fell through to the
per-tool semantic tables and landed in `~/.claude/commands/`,
`~/.cursor/rules/`, and the Copilot/Codex concat files. **That was
wrong.** `TargetDestination` has guarded `ArtifactAgent` since
SPEC-004 Part B, ahead of the per-tool switch: Claude got
`~/.claude/agents/<name>.md` and every other tool got a
`StrategySkip`. The rev 2 claim came from reading
`claudeDestination`/`cursorDestination` in isolation and missing the
early return above them. No mis-filing bug exists and no Part A0 fix
was needed.

What Part A actually changes is narrow: the existing agent branch
widens from "Claude only" to "every tool the agents bucket allows",
and takes the destination directory from `Tool.DirForScope` instead of
a hardcoded `.claude`.

Ownership of the who-has-a-subagent-surface decision lives in **one**
place — the agents bucket's `Tools` field — consulted by both local
sync and `TargetDestination`. A test asserts the two agree for every
registered tool, because a disagreement means an agent that appears at
one scope and vanishes at the other.

#### Scenario: global agent lands in the subagent dir
- **GIVEN** `~/.agents/agents/reviewer.md`
- **WHEN** `sync-agents global sync` runs
- **THEN** `~/.claude/agents/reviewer.md`,
  `~/.cursor/agents/reviewer.md`, and
  `~/.config/opencode/agents/reviewer.md` are symlinks to it

#### Scenario: concat tools skip with a reason
- **GIVEN** the same artifact and `copilot` in targets
- **WHEN** `sync-agents global sync` runs
- **THEN** it reports a skip naming the missing subagent surface, and
  the agent body is never inlined into `instructions.md`

### Requirement: foreign wiring is never destroyed

**This hazard was real and is fixed.** `applySymlinkDestination`
treated *any* symlink whose target differed as drifted and repaired it
with a bare `os.Remove` + relink — no backup, no `--force` gate. The
backup-rename path covered only **non**-symlinks. A user whose
`~/.claude/agents/<name>.md` was a hand-made symlink into a personas
directory lost that wiring silently the first time a same-named agent
existed in `~/.agents/agents/`.

Verified against the pre-fix binary: the hand-made link was replaced
outright, with no backup left behind.

The fix applies the SPEC-010 ownership taxonomy at the one place that
conflated the two states:

- Symlink resolving **into** the canonical tree → *drifted*, ours,
  repaired silently as before.
- Symlink resolving **outside** it → *foreign*, treated exactly like a
  real file: refused without `--force`, backup-renamed with `--force`.

Two supporting changes: the backup name is timestamped, so a second
conflict at the same path cannot clobber the first backup; and
conflicts are restated in an end-of-run summary, because a warning
three hundred lines up is a warning nobody reads.

#### Scenario: hand-made symlink survives a sync
- **GIVEN** `~/.claude/agents/tars.md` symlinked to a personas
  directory, and `~/.agents/agents/tars.md` present
- **WHEN** `sync-agents global sync` runs without `--force`
- **THEN** the link and its target are untouched, the conflict is
  warned inline, and the end-of-run summary lists the path

#### Scenario: --force displaces without deleting
- **GIVEN** the same setup
- **WHEN** `sync-agents global sync --force` runs
- **THEN** the canonical symlink is placed and the original is renamed
  to a timestamped `.replaced-by-sync-agents-<ts>` sibling

---

## Part B — opencode as a registered Tool

### Requirement: opencode is a first-class target

The `Tools` registry SHALL include opencode, resolvable by ID from
`.agents/config` and `--targets`.

```
{
    ID: "opencode",
    DirByScope: map[Scope]string{
        ScopeLocal:  ".opencode",
        ScopeGlobal: filepath.Join(".config", "opencode"),
    },
},
```

The global segment nests like Copilot's `.github/copilot`, so
`DirForScope` needs no change.

opencode reads `AGENTS.md` natively, so the concat/index path needs no
opencode-specific branch — but the rules/skills/workflows buckets now
fan into `.opencode/` too. That is a **behavior change for existing
users** who have an `.opencode/` directory they hand-manage.

**Decision required (see Open Questions):** opt-in vs. default-on.

#### Scenario: opencode resolves from config
- **GIVEN** `.agents/config` with `targets = claude,opencode`
- **WHEN** `sync-agents sync` runs
- **THEN** `.opencode/rules`, `.opencode/skills`, `.opencode/workflows`,
  and `.opencode/agents` all symlink into `.agents/`

#### Scenario: unknown tool still errors
- **GIVEN** `targets = claude,opencodex`
- **WHEN** any sync command runs
- **THEN** it fails with an unknown-target error naming the valid IDs

> **Verify during implementation:** opencode's subagent directory is
> documented as `agents/` in current docs, but older releases used
> `agent/` (singular). If both are live, `Bucket` needs a per-tool
> directory-name override (`DirByTool map[string]string`) rather than
> assuming `Bucket.Dir` is the destination name everywhere. Check the
> installed opencode version before choosing.

---

## Part C — `add agent --from` (import)

### Requirement: `add` can import an existing definition

`add` SHALL accept `--from <ref>` to populate the new artifact from an
existing file instead of the bucket template, and `--link` to symlink
rather than copy. Both flags are bucket-generic (they work for `rule`,
`skill`, etc.), but `agent` is the driving case.

```
sync-agents add agent reviewer --from ~/.claude/agents/reviewer.md
sync-agents add agent tars     --from ~/personas/tars.md --link
sync-agents add agent critic   --from team-agents:agents/critic.md
```

`<ref>` resolution order:

1. Contains `:` and the prefix matches a registered source entry
   (SPEC-003) → resolve inside that source's checkout/cache.
2. Otherwise → a filesystem path (`~` expanded, relative to CWD).

Behavior:

- **Copy (default).** Read the source file, normalize frontmatter,
  write to the canonical path. The source is untouched.
- **`--link`.** Create a relative symlink at the canonical path
  pointing at the source. Refuse if the target is inside `.agents/`
  already, or if the link would dangle.
- **Frontmatter normalization.** Set `name:` to `<name>` (rewriting a
  mismatch, with a warning). Require a non-empty `description:`;
  error if absent, since every consumer's delegation logic keys on it.
  Every other key is preserved verbatim, including tool-specific ones.
  On `--link`, normalization is skipped — the file is not ours to
  rewrite — and a mismatched `name:` is a hard error instead.
- **`--force`** governs overwriting an existing canonical path, as it
  does today.
- Directory-per-artifact buckets (`skill`) accept a `--from` pointing
  at either the directory or its `SKILL.md`.

#### Scenario: copy import rewrites the name
- **GIVEN** `/tmp/rev.md` with frontmatter `name: old-name` and a
  description
- **WHEN** `sync-agents add agent reviewer --from /tmp/rev.md` runs
- **THEN** `.agents/agents/reviewer.md` exists with `name: reviewer`,
  the body and all other frontmatter keys unchanged, a warning naming
  the rewrite is printed, and `/tmp/rev.md` is unmodified

#### Scenario: link import preserves the original
- **GIVEN** `~/personas/tars.md` with `name: tars` and a description
- **WHEN** `sync-agents add agent tars --from ~/personas/tars.md --link`
  runs
- **THEN** `.agents/agents/tars.md` is a symlink resolving to that file
  and no content is copied

#### Scenario: missing description is rejected
- **GIVEN** a source file whose frontmatter has no `description:`
- **WHEN** `add agent … --from` runs
- **THEN** it exits non-zero, names the missing key and the source
  path, and creates nothing

#### Scenario: link with a mismatched name is rejected
- **GIVEN** `/tmp/rev.md` with `name: old-name`
- **WHEN** `add agent reviewer --from /tmp/rev.md --link` runs
- **THEN** it exits non-zero explaining that `--link` cannot rewrite
  the source, and suggests dropping `--link`

#### Scenario: source-relative import
- **GIVEN** a registered source `team-agents` with
  `agents/critic.md` in its checkout
- **WHEN** `add agent critic --from team-agents:agents/critic.md` runs
- **THEN** the file is copied in and normalized, using the same
  quarantine gate a source pull uses

#### Scenario: template path is unchanged
- **GIVEN** no `--from`
- **WHEN** `sync-agents add agent reviewer` runs
- **THEN** the behavior is byte-identical to today's template scaffold

---

## Part D — Selective load (per-agent tool restriction)

**Depends on SPEC-010 Phase 2/3 (drilled sync).** Do not start Part D
before that lands.

### Requirement: an agent can restrict which tools receive it

An agent's frontmatter MAY carry `sync-tools:` — a list of tool IDs.
When present, only those tools receive the artifact. Absent means
every subagent-capable target, i.e. today's behavior.

```yaml
---
name: trader
description: Trading only — Robinhood and Alpaca.
sync-tools: [claude]
---
```

### Why not just `tools:`

The key is namespaced rather than reusing `tools:` because agent files
are **synced as symlinks** — every harness reads the identical bytes,
so there is no rewrite point at which a sync-only key could be
stripped. `tools:` is already claimed, with two incompatible shapes:

| Consumer | Meaning of `tools:` | Shape |
|---|---|---|
| Claude | tool-permission allowlist | inline CSV — `Read, Grep, Bash(git diff *)` |
| opencode | per-tool permission map | mapping — `{ task: allow }` |
| sync-agents (proposed) | which harnesses receive the file | list of tool IDs |

Writing `tools: [claude, cursor]` would therefore not be inert — Claude
would read it as an allowlist naming two tools that do not exist and
hand the subagent an empty toolset. That is silent lobotomy, not a
parse error, which makes it worse than a crash. Four of the nine
personas in the reference setup already carry a real Claude `tools:`
line, so the collision is observed, not hypothetical.

Three non-destructive alternatives, in preference order:

1. **`sync-tools:`** (proposed). Unknown key; every harness ignores
   it, `frontmatter.go` already preserves it verbatim. One-line parse.
2. **`x-sync-agents:` nested block.** `x-` marks a vendor extension by
   convention, so it can never be claimed by a harness later. Costs a
   nested-mapping parse — `ParseFrontmatterInvocable` is deliberately
   flat-only (`semantic.go:113`), so this needs a real YAML dependency
   or a hand-rolled one-level descent.
3. **Out-of-band in `.agents/config`.** Keeps agent files pristine and
   portable to a non-sync-agents consumer. Costs a second place to
   look, and routing is lost when a file is renamed or moved.

Option 1 is recommended: it is the only one that is both a one-line
parse and free of a rename-fragility footgun. Option 3 remains the
right answer if agent files must stay byte-identical to upstream —
revisit if `--link` imports turn out to be the common case.

### Fold/drill is a local-scope problem only

`discoverArtifactsForOS` already walks buckets **per artifact**, so
global sync is drilled today and needs no fold/drill switch — the
`sync-tools` filter is a single predicate in the destination loop.
Only `CmdSync` (local) folds a whole bucket into one directory
symlink. Part D's dependency on SPEC-010 is therefore narrower than
first assessed: **global-scope selective load can ship with Parts A–C**;
only the local fold→drill transition needs the audit's ownership
taxonomy.

**Fold/drill interaction.** A whole-bucket symlink cannot express
per-artifact selection. Therefore:

- If **no** agent in the bucket declares `sync-tools`, sync folds as
  today (one directory symlink) — no regression, no churn.
- If **any** agent declares it, the bucket **drills** for every target:
  the destination becomes a real directory of per-artifact symlinks,
  and each artifact is linked only into the tools it allows.
- Switching between fold and drill must be idempotent in both
  directions, and must not delete a destination that SPEC-010's audit
  classifies as `foreign`.

#### Scenario: restriction drills the bucket
- **GIVEN** `.agents/agents/trader.md` with `sync-tools: [claude]` and
  `.agents/agents/reviewer.md` with none, targets `claude,cursor`
- **WHEN** `sync-agents sync` runs
- **THEN** `.claude/agents/` is a real directory containing symlinks to
  both, and `.cursor/agents/` contains a symlink to `reviewer.md` only

#### Scenario: no restriction keeps the fold
- **GIVEN** no agent declares `sync-tools`
- **WHEN** `sync-agents sync` runs
- **THEN** `.claude/agents` is a single directory symlink, exactly as
  before this spec

#### Scenario: removing the last restriction re-folds
- **GIVEN** a drilled `.claude/agents/` and `sync-tools` removed from
  every agent
- **WHEN** `sync-agents sync` runs
- **THEN** the per-artifact links are replaced by the single directory
  symlink, and no unmanaged file in the destination is removed

#### Scenario: unknown tool ID in sync-tools
- **GIVEN** `sync-tools: [claud]`
- **WHEN** `sync-agents lint` or `sync` runs
- **THEN** it reports the unknown ID and the file it came from

---

## Technical Design

| Change | File |
|---|---|
| `agents` bucket `LocalTools` → claude, cursor, opencode | `internal/agent/bucket.go` |
| opencode `Tool` entry | `internal/agent/tool.go` |
| Optional `Bucket.DirByTool` override (only if opencode uses `agent/`) | `internal/agent/bucket.go`, `destination.go` |
| `--from` / `--link` flags on `add` | `main.go` |
| `CmdAdd` import branch + frontmatter normalization | `internal/agent/agent.go`, `frontmatter.go` |
| Source-ref resolution for `--from` | `internal/agent/source/manifest.go`, `sourcecmd.go` |
| agent branch in per-tool global routing (claude/cursor → `agents/`; codeium/copilot/codex → skip) | `internal/agent/destination.go` |
| drifted out-of-tree symlink gets backup-rename, not bare remove | `internal/agent/globalsync.go` |
| `sync-tools` parsing + drill decision | `internal/agent/semantic.go`, `agent.go` (`CmdSync`), `globalsync.go` |
| `sync-tools` unknown-ID check | `internal/agent/lint.go` |

Docs to update in the same PR:
`docs/topology.md` (agents bucket is no longer Claude-only),
`docs/architecture/scope-and-targets.md` (opencode row),
`docs/commands/` (new `add.md` — none exists today),
`README.md` target list, `.agents/config` default comment.

## Test Plan

- [ ] `bucket_test.go` — `SyncsToLocalTarget` for agents across all six tools
- [ ] `tool_test.go` — opencode resolves by ID; `DirForScope` yields `.config/opencode` at global
- [ ] `agent_test.go` — `add agent --from` copy: normalization, preserved keys, untouched source
- [ ] `agent_test.go` — `add agent --from --link`: symlink shape, name-mismatch rejection
- [ ] `agent_test.go` — missing `description:` rejected, nothing created
- [ ] `sourcecmd_test.go` — `--from <source>:<path>` resolution + quarantine gate
- [ ] `integration_test.go` — local sync fans agents to claude/cursor/opencode only
- [ ] `globalsync_test.go` — same restriction at global scope
- [ ] `index_test.go` — `## Agents` section still lists every agent regardless of `sync-tools`
- [ ] Part D: fold→drill→fold idempotence; foreign entries never deleted
- [ ] `lint_test.go` — unknown `sync-tools` ID reported with file path

## Open Questions

1. ~~**opencode default-on or opt-in?**~~ **Resolved: opt-in.**
   opencode is in the `Tools` registry but not in `AllTargets`, so no
   existing user's `.opencode/` is touched until they ask for it in
   `.agents/config` or `--targets`. A test pins this.
2. ~~**`agents/` vs `agent/` for opencode**~~ **Resolved: `agents/`
   (plural)** at both scopes, per current opencode docs. It matches
   `Bucket.Dir`, so no per-tool directory override was needed.
3. ~~**Should `--from` also accept a URL?**~~ **Resolved: no**, and the
   `<source>:<path>` form is out too. Registered sources already stage
   artifacts into the bucket tree via `source add` + `pull`, and that
   route applies the SPEC-005 quarantine gate. A second import path
   would duplicate the first while bypassing its safety check, so the
   ref form is *recognized* and redirected rather than left to fail as
   a missing file. **This descopes a scenario rev 1 specified** —
   reinstate it only if the redirect proves to be the wrong call in
   practice.
4. **Part D key name** — `sync-tools` proposed; see *Why not just
   `tools:`* for why the bare name is unsafe and for the two
   alternatives. `targets:` (matching `.agents/config` vocabulary) is
   also collision-free today but reads ambiguously next to a harness's
   own routing keys. **Still open — Part D is not implemented.**
5. **Should `add --from` normalize `description:` for skills too?**
   Today the requirement is agents-only. Skills have their own
   backfill (`CmdBackfillSkills`) and lint checks, so duplicating the
   rule here would give two places to change it.
6. **opencode's non-agent surfaces are unrouted at global scope.**
   `opencodeDestination` skips everything but agents, deliberately:
   opencode reads `AGENTS.md` natively for rules, and its user-scope
   command surface has not been verified against an installed build
   (opencode is not installed on the development machine). Local sync
   is unaffected — it symlinks bucket directories generically. A
   contributor with opencode installed should confirm the layout and
   replace the skip.

## Rollout

- ~~**Part A0 (bug fix)**~~ — withdrawn; the mis-filing bug it was
  written against does not exist (see the correction above). The
  drifted-symlink fix it also carried shipped inside Part A, where it
  belongs — Part A is what widens the blast radius of that code path
  from one tool to three.
- **Parts A + B + C** — shipped together. Minor bump. Additive: the
  only behavior change to an existing path is that global sync now
  refuses to overwrite a foreign symlink without `--force`, which is a
  deliberate safety regression in permissiveness.
- **Part D** — not started. Global-scope selective load is
  unblocked (global sync is already drilled per-artifact); the local
  fold→drill transition still needs SPEC-010's ownership taxonomy.

## Implementation status

| Part | Status | Where |
|---|---|---|
| A — multi-tool subagent routing | ✅ shipped | `bucket.go` (`Tools`, `subagentTools`), `destination.go` (agent branch) |
| A — foreign-symlink protection | ✅ shipped | `globalsync.go` (`pointsIntoGlobalTree`, timestamped backup, conflict summary) |
| B — opencode as a Tool | ✅ shipped | `tool.go`; non-agent global routing deliberately skipped |
| C — `add --from` / `--link` | ✅ shipped | `addimport.go`, `agent.go` (`CmdAdd`), `main.go` |
| C — `<source>:<path>` refs | ❌ descoped | redirected to `source add` + `pull` (Open Question 3) |
| D — `sync-tools:` selective load | ⬜ not started | blocked on SPEC-010 for local scope only |
