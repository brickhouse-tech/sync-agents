---
id: SPEC-002
title: Global-scope sync, relocate, and promote for agent artifacts
status: Draft
owner: nmccready
created: 2026-05-13
updated: 2026-05-13
related: SPEC-001
---

# [SPEC-002] Feature: Global-scope sync / relocate / promote

## Overview

Extend `sync-agents` so any artifact — skill, rule, workflow, slash command,
subagent definition, hooks/settings fragment — can be **synced**, **relocated**,
or **promoted** between project (local) scope and user (global) scope, using
each tool's native filesystem convention.

The model mirrors what already works at `$HOME` on a properly-configured
machine: `~/.agents/` is the canonical source of truth, and the per-tool
global directories (`~/.claude/`, `~/.codeium/`, `~/.cursor/`) carry symlinks
back into it. This spec generalizes that pattern into first-class CLI
operations and adds the inverse direction (project → global) so a contributor
who writes a useful skill inside one repo can elevate it to apply everywhere.

## Motivation

Today `sync-agents` is project-scoped: `.agents/` is canonical for **one
repo**, and `sync` fans out to that repo's `.claude/`, `.windsurf/`, etc. The
existing **inheritance** mechanism lets a project read parents' `AGENTS.md`,
but it does not move artifacts between scopes — every level is hand-curated.

Real workflows demand bidirectional flow:

1. **Promotion** — A skill written inside `repo-A/.agents/skills/foo/` proves
   broadly useful. Today the user must manually copy the directory to
   `~/.agents/skills/foo/`, then update `~/AGENTS.md`. Error-prone, easy to
   forget. We want one command.
2. **Sync** — A user maintains a skill at the global level and wants a
   specific project to track it (`~/.agents/skills/foo → repo/.agents/skills/foo`)
   without re-cloning or copy-paste drift.
3. **Relocation** — An artifact installed globally turns out to belong to a
   single project (or vice versa). Move it, update references, leave a
   forwarding symlink if requested.
4. **Tool-native global directories already exist on disk** — Claude has
   `~/.claude/`, Codeium has `~/.codeium/` (note: `.windsurf/` is *local* to a
   project; `.codeium/` is the user-scoped equivalent), Cursor has
   `~/.cursor/`. We must honor these conventions, not invent new ones.

## Goals

- Single command per operation: `sync-agents promote <artifact>`,
  `sync-agents relocate <artifact> --to <scope>`, and an extended
  `sync-agents sync --scope=global` (or equivalent).
- **Auto-detect artifact type** from path — a `SKILL.md` under a
  `skills/<name>/` directory is a skill; a file under `.github/workflows/` is
  a GitHub Actions workflow; a file under `.claude/commands/` is a Claude
  slash command; a file under `.claude/agents/` is a subagent; a JSON
  fragment in `.claude/settings.json` is a hook/setting.
- **Canonical-source convention is configurable but defaults to the
  `.agents/` model.** After promote/sync, the canonical copy lives at one
  scope and the other scope holds a symlink (default), unless the user passes
  `--copy` for a hard copy.
- Operations are **idempotent** and **dry-runnable**. Re-running `promote`
  on something already promoted is a no-op (or a re-link/repair if drifted).
- **References get updated.** Promoting a skill out of `.agents/skills/foo/`
  must remove its line from the local project's `AGENTS.md` index (or replace
  it with an inheritance pointer) and add it under the global `AGENTS.md`.
- **No data loss.** Anything that would overwrite or delete an existing file
  requires `--force` or interactive confirmation; otherwise the op fails
  cleanly.

## Non-Goals

- **Network sync** — no git-pull / dotfiles-repo orchestration. Out of scope.
  Users who want their `~/.agents/` versioned can do that with their own
  dotfiles tooling.
- **Cross-machine sync** — same as above.
- **Conflict resolution UX beyond fail-fast** — if local and global have
  diverged copies of the same artifact, we report the diff and require
  `--force` or `--prefer=local|global`. No three-way merge.
- **VS Code workspace settings** — `.vscode/settings.json` is out of scope.
- **MCP server installs** — covered separately; this spec only handles agent
  artifact files.

## Definitions

| Term | Meaning |
|---|---|
| **Scope** | `local` (project root, the cwd's `.agents/` and per-tool dirs) or `global` (`$HOME` or a configured prefix; the user's `.agents/` and per-tool dirs). |
| **Tool** | One of: `claude`, `codeium` (alias `windsurf` when local), `cursor`, `copilot`. Drives target-directory naming. |
| **Canonical location** | The single copy that is *not* a symlink. By default this is the `.agents/` tree at whichever scope the artifact "lives" at. |
| **Mirror location** | A symlink (or hard copy, with `--copy`) of a canonical artifact inside a tool-specific dir. |
| **Artifact** | A rule, skill, workflow, command, subagent, or settings/hook fragment. Detected by path + filename. |
| **Promote** | Make a local artifact also exist at global scope. |
| **Demote** | Make a global artifact also exist at local scope (inverse of promote). |
| **Relocate** | Move an artifact's canonical location to a different scope; previous location either disappears or becomes a symlink. |

## Filesystem conventions (authoritative)

These are the **only** paths the tool reads/writes. They reflect each tool's
native convention as of 2026-05.

### Canonical (source of truth) trees

| Scope | Path | Notes |
|---|---|---|
| Local | `<project>/.agents/` | Existing canonical tree. |
| Global | `$HOME/.agents/` | Default global canonical. Override via `--global-root <path>` or `SYNC_AGENTS_GLOBAL_ROOT`. |

### Per-tool target directories

| Tool | Local target | Global target |
|---|---|---|
| Claude Code | `<project>/.claude/` | `$HOME/.claude/` |
| Codeium / Windsurf | `<project>/.windsurf/` | `$HOME/.codeium/` |
| Cursor | `<project>/.cursor/` | `$HOME/.cursor/` |
| GitHub Copilot | `<project>/.github/copilot/` | best-effort: `$HOME/.config/github-copilot/` (see Open Questions) |

Notes:

- **`.windsurf` ↔ `.codeium` is intentional asymmetry.** Windsurf reads
  `.windsurf/` at the repo root and `.codeium/` at the user home. The tool
  abstracts this so the user never has to think about it: passing
  `--tool=windsurf` or `--tool=codeium` does the right thing for the scope
  in question.
- The existing `AllTargets = ["claude","windsurf","cursor","copilot"]` set
  (see `internal/agent/agent.go:19`) MUST be extended/refactored so that
  `windsurf` and `codeium` are aliases for the same logical "tool" with
  scope-dependent directory naming.

### Artifact-class subdirectories (under canonical tree or target)

| Class | Subdir | Filename pattern |
|---|---|---|
| Rule | `rules/` | `*.md` |
| Skill | `skills/<name>/` | dir containing `SKILL.md` |
| Workflow (sync-agents) | `workflows/` | `*.md` |
| Workflow (GitHub Actions) | `.github/workflows/` | `*.yml` / `*.yaml` |
| Command (Claude slash) | `commands/` | `*.md` |
| Subagent (Claude) | `agents/` | `*.md` |
| Hook / settings fragment | `settings.json` | merged JSON fragment |

## Requirements

### Requirement: Auto-detect artifact type

The system SHALL determine an artifact's class and tool affinity from its
path. The user MAY override with `--type` and `--tool` flags.

Detection rules (first match wins):

1. Path ends in `SKILL.md` OR is a directory containing a `SKILL.md` →
   **skill**, tool-agnostic.
2. Path is under `**/rules/` and ends in `.md` → **rule**, tool-agnostic.
3. Path is under `**/.github/workflows/` and ends in `.yml`/`.yaml` →
   **github-workflow**, tool: copilot/github.
4. Path is under `**/workflows/` (not `.github/`) and ends in `.md` →
   **sync-agents-workflow**, tool-agnostic.
5. Path is under `**/.claude/commands/` and ends in `.md` → **command**,
   tool: claude.
6. Path is under `**/.claude/agents/` and ends in `.md` → **subagent**,
   tool: claude.
7. Path is `**/settings.json` under a tool dir → **settings**, tool from
   path.

If no rule matches, the command SHALL exit non-zero with a message
listing the detected attempts and a suggestion to pass `--type`/`--tool`.

#### Scenario: Skill auto-detected from directory
- **GIVEN** `.agents/skills/foo/SKILL.md` exists
- **WHEN** `sync-agents promote .agents/skills/foo` runs
- **THEN** the artifact is classified as a skill named `foo`
- **AND** the entire `foo/` directory is the unit of promotion

#### Scenario: Rule auto-detected from path
- **GIVEN** `.agents/rules/bash.md` exists
- **WHEN** `sync-agents promote .agents/rules/bash.md` runs
- **THEN** the artifact is classified as rule `bash`

#### Scenario: Ambiguous file rejected
- **GIVEN** `notes/random.md` exists outside any conventional dir
- **WHEN** `sync-agents promote notes/random.md` runs
- **THEN** the command exits non-zero
- **AND** stderr names the unrecognised path and suggests `--type`

### Requirement: Promote local → global

`sync-agents promote <path>` SHALL copy or move the artifact from local
canonical (`<project>/.agents/...`) into global canonical (`$HOME/.agents/...`),
then refresh symlinks under `$HOME/.claude/`, `$HOME/.codeium/`, etc.,
according to the global `.agents/config` target list.

Behavior matrix (controlled by flags):

| Flag combo | Local copy after | Global copy after |
|---|---|---|
| `promote` (default) | symlink → global | canonical |
| `promote --copy` | canonical (untouched) | canonical (hard copy) |
| `promote --move` | absent | canonical (moved) |
| `promote --link-back` | symlink → global | canonical (= default) |

The default (`promote` with no flags) is `--link-back`: the local project
keeps a symlink so anything reading `.agents/skills/foo/` still works, but
edits flow through the global canonical.

#### Scenario: Default promote leaves a back-symlink
- **GIVEN** `.agents/skills/foo/SKILL.md` exists with content X
- **AND** `$HOME/.agents/skills/foo/` does not exist
- **WHEN** `sync-agents promote .agents/skills/foo` runs
- **THEN** `$HOME/.agents/skills/foo/SKILL.md` exists with content X
- **AND** `.agents/skills/foo` is a symlink pointing to
  `$HOME/.agents/skills/foo` (relative path preferred when feasible)
- **AND** `$HOME/.claude/skills/foo` is a symlink into
  `$HOME/.agents/skills/foo`
- **AND** `$HOME/AGENTS.md` lists `foo` under `## Skills`
- **AND** the project's `AGENTS.md` no longer lists `foo` under its own
  `## Skills` (it is now provided by inheritance)

#### Scenario: Promote with --copy keeps both canonical
- **GIVEN** `.agents/rules/bash.md` exists
- **WHEN** `sync-agents promote .agents/rules/bash.md --copy` runs
- **THEN** both `.agents/rules/bash.md` and `$HOME/.agents/rules/bash.md`
  exist as regular files
- **AND** a warning is printed that the two copies will drift unless one is
  later relocated

#### Scenario: Promote when global already exists fails without --force
- **GIVEN** `.agents/skills/foo/SKILL.md` exists locally
- **AND** `$HOME/.agents/skills/foo/SKILL.md` already exists with different
  content
- **WHEN** `sync-agents promote .agents/skills/foo` runs without `--force`
- **THEN** the command exits non-zero
- **AND** stderr shows a unified diff of the two SKILL.md files
- **AND** suggests `--force` or `--prefer=local|global`

### Requirement: Demote global → local

`sync-agents demote <path>` SHALL be the symmetric inverse of `promote`.
Same flag set (`--copy`, `--move`, `--link-back`). Default is to move the
canonical copy into the project's `.agents/` and leave a symlink at the
global path (so other projects that inherited it still resolve).

#### Scenario: Demote with --move removes global entirely
- **GIVEN** `$HOME/.agents/skills/foo/` exists and is referenced by
  `$HOME/AGENTS.md`
- **AND** the current project has no `.agents/skills/foo/`
- **WHEN** `sync-agents demote $HOME/.agents/skills/foo --move` runs in
  the project root
- **THEN** the directory is gone from `$HOME/.agents/skills/`
- **AND** `.agents/skills/foo/` exists in the project as canonical
- **AND** `$HOME/AGENTS.md` no longer references it
- **AND** the project's `AGENTS.md` references it

### Requirement: Relocate (scope-shift)

`sync-agents relocate <path> --to <scope>` SHALL be the umbrella for both
directions, with `<scope>` ∈ `{local, global}`. It is equivalent to
`promote` when `--to=global` and `demote` when `--to=local`. Same flags.

This requirement exists so external scripts have a single, scope-explicit
command form to drive.

### Requirement: Sync at global scope

`sync-agents sync --scope=global` SHALL run the existing sync algorithm
(create per-tool symlinks from canonical `.agents/`) but rooted at
`$HOME/.agents/` and writing into `$HOME/.claude/`, `$HOME/.codeium/`,
`$HOME/.cursor/` according to the **global** `.agents/config` file.

`--scope=local` is the existing behavior (default). `--scope=both` runs
both passes.

Implementation note: this is mostly a re-parameterization of the existing
`Sync` function in `internal/agent/agent.go`. The hard work is the
local↔global asymmetry in tool-target naming (`.windsurf` vs `.codeium`).

#### Scenario: Global sync uses .codeium not .windsurf
- **GIVEN** `$HOME/.agents/skills/foo/` exists
- **AND** `$HOME/.agents/config` lists `codeium` (or `windsurf`) as a target
- **WHEN** `sync-agents sync --scope=global` runs
- **THEN** `$HOME/.codeium/skills/foo` is a symlink to
  `$HOME/.agents/skills/foo`
- **AND** **no** `$HOME/.windsurf/` directory is created

#### Scenario: Local sync uses .windsurf not .codeium
- **GIVEN** `<project>/.agents/skills/foo/` exists
- **AND** `<project>/.agents/config` lists `codeium` (or `windsurf`)
- **WHEN** `sync-agents sync --scope=local` runs
- **THEN** `<project>/.windsurf/skills/foo` is a symlink
- **AND** **no** `<project>/.codeium/` directory is created

### Requirement: Reference updates on promote/demote/relocate

The system SHALL keep `AGENTS.md` index files in sync with the canonical
location of each artifact. Specifically:

- On promote: remove the entry from the local `AGENTS.md` (under its
  class section) and add it to the global `AGENTS.md`. If the local
  `AGENTS.md` has an `## Inherits` block pointing at global, the entry is
  inferred from inheritance — no local re-add needed.
- On demote: inverse — add locally, remove globally.
- On relocate within the same scope (rare; reserved for future): update
  both sides if both sides referenced it.
- `AGENTS.md` regeneration uses the existing `index` machinery; the
  promote/demote commands MUST call it (or its library equivalent) before
  returning success.

#### Scenario: Local AGENTS.md is regenerated after promote
- **GIVEN** `.agents/rules/bash.md` exists and is listed in local
  `AGENTS.md` under `## Rules`
- **WHEN** `sync-agents promote .agents/rules/bash.md` runs
- **THEN** `bash` is removed from local `AGENTS.md` `## Rules`
- **AND** `bash` appears in `$HOME/AGENTS.md` `## Rules`
- **AND** local `AGENTS.md` carries (or already carried) an `## Inherits`
  entry pointing at `$HOME/AGENTS.md`

### Requirement: Dry-run and status

All three new commands (`promote`, `demote`, `relocate`) and the extended
`sync --scope=global` SHALL honor the existing global `--dry-run` flag.
Output MUST enumerate every filesystem operation that would occur
(create-symlink, remove-file, write-file, regen-index), one per line,
prefixed with `[dry-run]`.

`sync-agents status` SHALL gain a `--scope=global|local|both` option that
reports symlink integrity for the requested scope(s).

#### Scenario: Dry-run promote prints plan and changes nothing
- **GIVEN** `.agents/skills/foo/SKILL.md` exists
- **WHEN** `sync-agents promote .agents/skills/foo --dry-run` runs
- **THEN** stdout lists each step (mkdir, copy, symlink-replace, index-
  regen) prefixed with `[dry-run]`
- **AND** no files under `$HOME/.agents/` or the project are created,
  modified, or removed

### Requirement: Idempotency and repair

Re-running `promote`/`demote`/`relocate`/`sync --scope=global` on an
already-applied state SHALL be a no-op (return 0, no filesystem writes
beyond fixing broken symlinks if drift is detected).

If a previously-promoted artifact's back-symlink is broken (e.g. user
manually deleted the global canonical), re-running `promote` SHALL repair
it by re-creating the canonical from the back-symlink's intended target,
provided the local path still resolves to a file. Otherwise it exits
non-zero with a remediation hint.

### Requirement: Safety — no destructive default

No promote / demote / relocate operation SHALL `rm -rf` a directory it did
not create. Specifically:

- Overwriting an existing non-symlink target requires `--force`.
- Overwriting an existing symlink whose target differs from what we are
  about to create requires `--force` (the symlink itself is cheap to
  re-create, but the *intent* mismatch is a signal).
- `--move` is allowed to delete the source path *only after* the
  destination is verified to exist and match the source's checksum.

#### Scenario: Force required when global non-symlink would be overwritten
- **GIVEN** `$HOME/.agents/rules/bash.md` exists as a regular file (not a
  symlink) with content Y
- **AND** local `.agents/rules/bash.md` has content X
- **WHEN** `sync-agents promote .agents/rules/bash.md` runs without
  `--force`
- **THEN** exit code is non-zero, no files change, diff is shown

### Requirement: Configurable global root

`$SYNC_AGENTS_GLOBAL_ROOT` env var OR `--global-root <path>` flag SHALL
override the default `$HOME` prefix for *both* canonical and per-tool
targets. Resolution order: flag > env var > `$HOME`.

This requirement exists so corporate / shared-machine users can park their
"global" tree somewhere other than `$HOME`, and so the test suite can use
a temp dir.

## Technical Design

### Command surface

New cobra subcommands (alongside the existing `init`, `sync`, etc.):

```
sync-agents promote   <path>           [--copy|--move|--link-back] [--force] [--dry-run] [--type T] [--tool T]
sync-agents demote    <path>           [same flags]
sync-agents relocate  <path> --to S    [same flags, S ∈ {local,global}]
sync-agents sync                       [--scope=local|global|both] (extends existing)
sync-agents status                     [--scope=local|global|both]  (extends existing)
```

Common new flag: `--global-root <path>`, applies to every command that
touches global scope.

### Module layout

Add `internal/agent/scope.go` with:

```go
type Scope int
const (
    ScopeLocal Scope = iota
    ScopeGlobal
)

type Artifact struct {
    Class    ArtifactClass  // rule, skill, workflow, command, subagent, settings
    Tool     string          // "" if tool-agnostic, else "claude"|"codeium"|...
    Name     string          // "bash" for a rule, "foo" for a skill
    SrcPath  string          // canonical path at source scope
    SrcScope Scope
}

func DetectArtifact(absPath string, projectRoot, globalRoot string) (*Artifact, error)
func Promote(a *Artifact, opts PromoteOpts) error
func Demote (a *Artifact, opts PromoteOpts) error
```

Refactor `agent.go`'s `targetDir(target, root string)` into a method that
takes `(tool string, scope Scope, root string)` so the local-windsurf /
global-codeium mapping lives in one place.

### Tool aliasing

Introduce a `Tool` type with stable IDs and per-scope dir names:

```go
type Tool struct {
    ID         string                  // "claude", "codeium", "cursor", "copilot"
    DirByScope map[Scope]string        // {local: ".windsurf", global: ".codeium"} for codeium
}
```

The existing `AllTargets` list becomes a slice of `Tool` values. CLI users
can pass any alias (e.g. `--targets windsurf` and `--targets codeium` both
resolve to the codeium `Tool`).

### Index regen

Both project and global `AGENTS.md` are regenerated via the existing
`index` machinery after every promote/demote/relocate. Add a `RegenIndex(root)`
helper that the new commands can call without going through cobra.

### Inheritance interaction

After `promote`, the project's `AGENTS.md` should declare inheritance from
the global `AGENTS.md` if it doesn't already. Reuse the existing `inherit`
command's library function. If the link is added automatically, log it.

## Test Plan

- **Unit**
  - [ ] `DetectArtifact` covers each detection rule + the ambiguous-reject
        case
  - [ ] `Tool.DirByScope` mapping for every tool, both scopes
  - [ ] Symlink helper preserves relative paths when source/dest share a
        common prefix
  - [ ] `RegenIndex` produces stable output (sorted, idempotent)
- **Integration** (against a tempdir-rooted `--global-root`)
  - [ ] Promote skill: directory copied, back-symlink created, global
        `.claude/skills/foo` symlink created, global `AGENTS.md` updated
  - [ ] Promote rule: file copied, back-symlink, both AGENTS.md updated
  - [ ] Demote symmetry: round-trip promote→demote leaves filesystem
        identical to start (checksum'd directory tree)
  - [ ] `sync --scope=global` builds `~/.codeium/` not `~/.windsurf/`
  - [ ] `sync --scope=local` builds `.windsurf/` not `.codeium/`
  - [ ] `--dry-run` prints plan and leaves filesystem untouched
  - [ ] `--force` overwrite path; diff-on-failure path; conflict-rejection
        path
  - [ ] Idempotent re-runs: same plan executes twice with second run a
        no-op
- **Cross-platform**
  - [ ] macOS, Linux: same symlink behavior
  - [ ] Windows: junctions or skip-with-fallback-to-copy; document
- **End-to-end demo** (mirrors `examples/fix/run-demo.sh`):
  - [ ] `examples/promote/run-demo.sh` exercises promote→sync→demote
        against `$TMPDIR/sync-agents-demo-home`

## Open Questions (to resolve before approval)

These are the design choices I took a default position on. Flag any you
want flipped and I'll revise:

1. **`promote` default = `--link-back`** (local becomes a symlink to
   global). Alternative: default `--copy`, leaving two canonical copies
   and warning about drift. Rationale for current default: matches the
   existing `~/.claude/rules -> ../.agents/rules` pattern already on the
   user's machine; one canonical, no drift.
2. **GitHub Copilot global path** — I picked
   `$HOME/.config/github-copilot/` as a best-effort, but Copilot's global
   convention is fuzzy (mostly VS Code settings). Options: (a) keep this,
   (b) skip copilot at global scope and only support it locally, (c) make
   it configurable per-user via `.agents/config`. Currently leaning (b),
   wrote (a) so the surface is complete.
3. **Windows symlinks** — Windows requires admin or developer mode for
   symlinks; fall back to junctions or hard copies? Currently spec'd as
   "junctions or skip-with-fallback-to-copy; document." Need a firm call.
4. **Settings.json fragments** — the spec lists `settings` as an artifact
   class but a JSON merge is not a symlink. Proposed: treat
   promote-of-settings as a JSON-patch operation (merge global settings.json
   with the listed keys from local) rather than file-level symlink. This
   adds non-trivial complexity; recommend deferring to a follow-up spec
   (SPEC-003-settings-fragments). Drop from this spec?
5. **`relocate` vs `promote`/`demote`** — keep all three commands, or
   make `relocate --to <scope>` the only public surface and have
   `promote`/`demote` as aliases? Currently spec'd as all three first-class
   for ergonomics.

## Acceptance Criteria

- AC-1: Given a local skill directory, when `sync-agents promote` runs,
  then the skill's canonical copy is at `$HOME/.agents/skills/<name>/`,
  the project path is a symlink to it, all global per-tool dirs have
  fresh symlinks, both `AGENTS.md` files are updated, and re-running the
  command is a no-op.
- AC-2: Given a global rule, when `sync-agents demote --move` runs, then
  the rule's canonical copy moves to the project's `.agents/rules/`, the
  global path is gone, global `AGENTS.md` no longer lists it, project
  `AGENTS.md` does, and re-running is a no-op.
- AC-3: Given `sync-agents sync --scope=global`, then global symlinks
  exist for every artifact under `$HOME/.agents/`, with `codeium` mapped
  to `$HOME/.codeium/` (never `$HOME/.windsurf/`).
- AC-4: Given any conflict (non-symlink overwrite, divergent content, or
  broken target), then the command exits non-zero without filesystem
  writes unless `--force` is passed.
- AC-5: Given `--dry-run` on any new command, then stdout enumerates all
  planned operations and the filesystem is unchanged after the run.
- AC-6: Given `$SYNC_AGENTS_GLOBAL_ROOT` set to a temp dir, then every
  global operation reads/writes under that temp dir and never touches
  `$HOME`. (Required for the test suite.)

## Implementation order (suggested)

1. Refactor `Tool` typing and `targetDir` scope-awareness — touches
   `internal/agent/agent.go:19,78,85`, no behavior change yet.
2. Add `Scope` + `DetectArtifact` + tests.
3. Implement `Promote` library function + `promote` CLI.
4. Implement `Demote` + CLI.
5. Implement `relocate` thin wrapper.
6. Extend `sync` / `status` with `--scope`.
7. Index regen integration.
8. Cross-platform/Windows handling.
9. End-to-end demo script.

Each step lands as its own PR with the matching test slice.
