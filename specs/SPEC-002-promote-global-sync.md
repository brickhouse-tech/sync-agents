---
id: SPEC-002
title: "Promote to Global: sync skills, rules, and workflows to provider global dirs"
status: Draft
owner: nmccready
created: 2026-05-13
updated: 2026-05-13
---

# [SPEC-002] Feature: Promote / Global Sync for Skills, Rules, and Workflows

## Overview

Add a `promote` command and a `global` subcommand group so that any item in a
project's `.agents/` directory can be elevated to a user-level global store
(`~/.agents/`), and then synced from there to the standard global config
directories of every supported AI coding agent.

This spec also establishes the **Go test suite** (`go test ./...`) as a
first-class requirement alongside the existing bats integration tests.

## Motivation

Today `sync-agents sync` links `.agents/` into per-project provider dirs
(`.claude/`, `.windsurf/`, `.cursor/`, `.github/copilot/`). There is no path
to make a rule, skill, or workflow available *globally* — across every project
on the machine — without manually copying files.

Provider conventions for global config differ from local:

| Provider      | Local dir             | Global dir (user-level)        |
|---------------|-----------------------|-------------------------------|
| Claude        | `.claude/`            | `~/.claude/`                  |
| Windsurf      | `.windsurf/`          | (local only; Codeium is global)|
| Codeium       | `.windsurf/`          | `~/.codeium/`                 |
| Cursor        | `.cursor/`            | `~/.cursor/`                  |
| Copilot       | `.github/copilot/`    | `~/.github/copilot/`          |
| Codex         | `.codex/`             | `~/.codex/`                   |

The `.windsurf/` directory is **local-only**; Codeium's global directory
(`~/.codeium/`) is the correct global target for Windsurf/Codeium users.

## Goals

- `sync-agents promote <type> <name>` copies a local `.agents/` item into the
  global store (`~/.agents/`) and optionally triggers a global sync.
- `sync-agents global init` creates `~/.agents/` with the standard skeleton.
- `sync-agents global sync` links `~/.agents/` subdirectories into all
  supported global provider dirs.
- `sync-agents global status` reports the state of global symlinks.
- `sync-agents global clean` removes global symlinks created by sync-agents.
- Go unit tests (`go test ./...`) cover all promote and global-sync logic;
  bats tests cover CLI surface and integration.

## Non-Goals

- Merging or diffing global vs local versions (tracked separately).
- Network/remote global stores (e.g., git-hosted shared `.agents/`).
- Automatic conflict resolution — local always wins on `promote`, global
  always wins on `global pull` (future spec).
- Homebrew or system-level install paths.

---

## Requirements

### Requirement: Global agents directory

The system SHALL treat `~/.agents/` as the global source of truth, mirroring
the same directory structure as a project-level `.agents/`.

#### Scenario: Init creates global skeleton

- **GIVEN** `~/.agents/` does not exist
- **WHEN** `sync-agents global init` runs
- **THEN** `~/.agents/rules/`, `~/.agents/skills/`, `~/.agents/workflows/`
  are created
- **AND** `~/.agents/config` is written with the full global target list
  (`claude,codeium,cursor,copilot,codex`)
- **AND** `sync-agents global status` shows all targets as `[not synced]`

#### Scenario: Init is idempotent

- **GIVEN** `~/.agents/` already exists with content
- **WHEN** `sync-agents global init` runs again
- **THEN** existing files are preserved
- **AND** missing subdirs are created
- **AND** the command exits 0

---

### Requirement: Promote command

The system SHALL provide a `promote` command that copies a named item from the
project's `.agents/` directory into `~/.agents/`, making it globally available.

Supported types: `rule`, `skill`, `workflow`.

#### Scenario: Promote a rule

- **GIVEN** `.agents/rules/my-rule.md` exists in the current project
- **WHEN** `sync-agents promote rule my-rule` runs
- **THEN** `~/.agents/rules/my-rule.md` is created (copy, not symlink)
- **AND** `[info] Promoted rule my-rule to ~/.agents/rules/my-rule.md` is
  printed

#### Scenario: Promote a skill (directory layout)

- **GIVEN** `.agents/skills/my-skill/SKILL.md` exists
- **WHEN** `sync-agents promote skill my-skill` runs
- **THEN** `~/.agents/skills/my-skill/SKILL.md` is created (full dir copy)
- **AND** any supporting files within `my-skill/` are also copied

#### Scenario: Promote a workflow

- **GIVEN** `.agents/workflows/my-workflow.md` exists
- **WHEN** `sync-agents promote workflow my-workflow` runs
- **THEN** `~/.agents/workflows/my-workflow.md` is created

#### Scenario: Conflict requires --force

- **GIVEN** `~/.agents/rules/my-rule.md` already exists
- **WHEN** `sync-agents promote rule my-rule` runs **without** `--force`
- **THEN** the command prints a warning and exits non-zero
- **AND** the existing file is unchanged

- **WHEN** `sync-agents promote rule my-rule --force` runs
- **THEN** the existing file is overwritten with the project version

#### Scenario: --dry-run shows intent without writing

- **GIVEN** `.agents/skills/cool-skill/SKILL.md` exists
- **WHEN** `sync-agents promote skill cool-skill --dry-run` runs
- **THEN** the command prints what would be copied
- **AND** no files are written to `~/.agents/`

#### Scenario: Promote with auto global-sync

- **GIVEN** a valid `~/.agents/` and global provider dirs are set up
- **WHEN** `sync-agents promote rule my-rule --sync` runs
- **THEN** the rule is promoted AND `sync-agents global sync` is invoked
  automatically

#### Scenario: Missing source is an error

- **GIVEN** `.agents/rules/nonexistent.md` does not exist
- **WHEN** `sync-agents promote rule nonexistent` runs
- **THEN** `[error] rule "nonexistent" not found in .agents/rules/` is printed
- **AND** the command exits non-zero

---

### Requirement: Global sync

The system SHALL provide `sync-agents global sync` to link `~/.agents/`
subdirectories into each supported global provider directory.

Global provider directory map:

| Target token | Global directory                          | Subdirs linked             |
|--------------|-------------------------------------------|----------------------------|
| `claude`     | `~/.claude/`                              | `rules/`, `skills/`, `workflows/` |
| `codeium`    | `~/.codeium/`                             | `rules/`, `skills/`, `workflows/` |
| `cursor`     | `~/.cursor/`                              | `rules/`, `skills/`, `workflows/` |
| `copilot`    | `~/.github/copilot/`                      | `rules/`, `skills/`, `workflows/` |
| `codex`      | `~/.codex/`                               | `rules/`, `skills/`, `workflows/` |

Note: `.windsurf/` is **local-only**. The global Windsurf target is `codeium`
(`~/.codeium/`).

#### Scenario: Global sync links all targets

- **GIVEN** `~/.agents/` exists with `rules/`, `skills/`, `workflows/`
- **WHEN** `sync-agents global sync` runs
- **THEN** for each configured target, e.g. `claude`, the path
  `~/.claude/rules` is a symlink pointing to `~/.agents/rules`
- **AND** the same applies for `skills/` and `workflows/` in each target dir
- **AND** only subdirs that exist in `~/.agents/` are linked (no empty dir creation)

#### Scenario: --targets filters global sync

- **GIVEN** `~/.agents/` is initialized
- **WHEN** `sync-agents global sync --targets claude,codeium` runs
- **THEN** only `~/.claude/` and `~/.codeium/` are touched
- **AND** cursor, copilot, and codex dirs are not modified

#### Scenario: --dry-run for global sync

- **GIVEN** `~/.agents/` is initialized
- **WHEN** `sync-agents global sync --dry-run` runs
- **THEN** each proposed symlink is printed (`would link: …`)
- **AND** no filesystem changes are made

#### Scenario: Existing non-symlink conflicts

- **GIVEN** `~/.claude/rules` exists as a real directory (not a symlink)
- **WHEN** `sync-agents global sync` runs without `--force`
- **THEN** a warning is printed and that entry is skipped
- **WHEN** `sync-agents global sync --force` runs
- **THEN** the existing directory is removed and replaced with the symlink

---

### Requirement: Global status

The system SHALL provide `sync-agents global status` to report the state of all
global provider directories and their symlinks.

#### Scenario: Status shows synced and missing targets

- **GIVEN** `~/.claude/rules` → `~/.agents/rules` (symlink exists)
- **AND** `~/.cursor/rules` does not exist
- **WHEN** `sync-agents global status` runs
- **THEN** output includes `[synced]  ~/.claude/rules -> ~/.agents/rules`
- **AND** output includes `[missing] ~/.cursor/rules`

---

### Requirement: Global clean

The system SHALL provide `sync-agents global clean` to remove symlinks created
by `global sync`.

#### Scenario: Clean removes symlinks

- **GIVEN** `~/.claude/rules` is a symlink to `~/.agents/rules`
- **WHEN** `sync-agents global clean` runs
- **THEN** `~/.claude/rules` is removed
- **AND** `~/.agents/rules` (the source) is untouched
- **AND** empty parent dirs created by sync-agents are removed

---

### Requirement: Go test suite

The system SHALL have a Go unit test suite covering all business logic in
`internal/`. Tests MUST run with `go test ./...` from the repo root. The
existing `npm test` command MUST run both bats AND Go tests concurrently.

The Makefile `test-go` target SHALL run `go test ./...` (not bats).

#### Scenario: go test passes on clean tree

- **GIVEN** a fresh clone of the repo
- **WHEN** `go test ./...` runs
- **THEN** all tests pass and the command exits 0

#### Scenario: promote logic is unit tested

- **GIVEN** a temporary directory acting as a fake project root and `~/.agents/`
- **WHEN** `App.CmdPromote("rule", "my-rule")` is called in a test
- **THEN** the rule file is copied to the fake global dir
- **AND** no filesystem side-effects outside the temp dirs occur

#### Scenario: global sync logic is unit tested

- **GIVEN** a populated fake `~/.agents/`
- **WHEN** `App.CmdGlobalSync()` is called
- **THEN** the expected symlinks are created in the fake global provider dirs
- **AND** `App.CmdGlobalStatus()` reports all as `[synced]`

#### Scenario: npm test runs go tests

- **GIVEN** a clean repo with Go and Node installed
- **WHEN** `npm test` runs
- **THEN** `go test ./...` and `bats test/sync-agents.bats` both execute
- **AND** the overall exit code is 0 on a green tree

---

## Acceptance Criteria

- **AC-1**: `sync-agents promote rule foo` copies `.agents/rules/foo.md` to
  `~/.agents/rules/foo.md` and exits 0. Without `--force`, an existing
  destination causes a warning + non-zero exit.
- **AC-2**: `sync-agents promote skill bar` deep-copies
  `.agents/skills/bar/` (including all files) to `~/.agents/skills/bar/`.
- **AC-3**: `sync-agents global sync` creates symlinks from `~/.agents/`
  subdirs into `~/.claude/`, `~/.codeium/`, `~/.cursor/`, `~/.github/copilot/`,
  `~/.codex/` for each subdir that exists.
- **AC-4**: `sync-agents global status` prints `[synced]` / `[missing]` /
  `[not a symlink]` for each target×subdir combination.
- **AC-5**: `sync-agents global clean` removes only symlinks that point into
  `~/.agents/`; it does not remove real directories.
- **AC-10**: `sync-agents global sync` regenerates `~/.agents/AGENTS.md` after
  linking, so Claude reads a current global index at `~/.claude/`.
- **AC-11**: `sync-agents global init` always writes `targets = claude,codeium,cursor,copilot,codex`
  regardless of any project-level `.agents/config`.
- **AC-6**: `go test ./...` passes from repo root. Test coverage ≥ 70% on
  `internal/agent` package.
- **AC-7**: `make test-go` runs `go test ./...`, not bats.
- **AC-8**: `npm test` runs bats AND `go test ./...` concurrently; both must
  pass for overall success.
- **AC-9**: `.windsurf/` is never created by `global sync`; `codeium` is the
  only global Windsurf target.

---

## Technical Design

### New `App` methods

```go
// internal/agent/agent.go (additions)

// CmdPromote promotes a single item from .agents/ to ~/.agents/.
func (a *App) CmdPromote(typ, name string, autoSync bool) error

// CmdGlobalInit initializes ~/.agents/ with the standard skeleton.
func (a *App) CmdGlobalInit() error

// CmdGlobalSync links ~/.agents/ subdirs into global provider dirs.
func (a *App) CmdGlobalSync() error

// CmdGlobalStatus reports the state of global provider symlinks.
func (a *App) CmdGlobalStatus() error

// CmdGlobalClean removes global symlinks created by sync-agents.
func (a *App) CmdGlobalClean() error

// GlobalRoot returns the resolved global agents directory (~/.agents/).
func GlobalRoot() string
```

### Global target map

```go
// internal/agent/global.go (new file)

var GlobalTargets = []string{"claude", "codeium", "cursor", "copilot", "codex"}

func ResolveGlobalTargetDir(target string) string {
    home, _ := os.UserHomeDir()
    switch target {
    case "claude":
        return filepath.Join(home, ".claude")
    case "codeium":
        return filepath.Join(home, ".codeium")
    case "cursor":
        return filepath.Join(home, ".cursor")
    case "copilot":
        return filepath.Join(home, ".github", "copilot")
    case "codex":
        return filepath.Join(home, ".codex")
    }
    return ""
}
```

Note: `windsurf` is **not** in `GlobalTargets`. It maps to local `.windsurf/`
only. The global Windsurf target is `codeium`.

### Promote — copy semantics

`promote` uses `os.CopyFS` / recursive copy, **not** symlinks, because:
1. The global store should not have project-relative symlinks.
2. The promoted copy is the global canonical version; the project copy is the
   source of the promotion, not an ongoing reference.

Skill promotion copies the entire directory tree:
```
.agents/skills/foo/ → ~/.agents/skills/foo/   (deep copy)
```

### Global sync — symlink semantics

`global sync` creates **absolute symlinks** from global provider dirs into
`~/.agents/`:

```
~/.claude/rules    → ~/.agents/rules    (absolute symlink)
~/.claude/skills   → ~/.agents/skills
~/.claude/workflows→ ~/.agents/workflows
~/.codeium/rules   → ~/.agents/rules
...
```

Unlike project-level sync (which uses relative symlinks), global sync uses
absolute paths to ensure symlinks remain valid regardless of the working
directory.

### CLI surface

New top-level commands added to `main.go` / cobra command tree:

```
sync-agents promote <type> <name> [--force] [--dry-run] [--sync]
sync-agents global init
sync-agents global sync [--targets <t1,t2>] [--force] [--dry-run]
sync-agents global status [--targets <t1,t2>]
sync-agents global clean [--targets <t1,t2>] [--dry-run]
```

`global` is a cobra command group; `init`, `sync`, `status`, `clean` are
subcommands.

### Go test infrastructure

New file: `internal/agent/agent_test.go` (and `global_test.go`).

Test helpers use `t.TempDir()` to provide isolated fake project roots and
global roots. `App.GlobalRoot` must be overridable for tests:

```go
// App gains a field for testability:
type App struct {
    // ... existing fields ...
    GlobalRoot string  // defaults to os.UserHomeDir()+"/.agents" when empty
}
```

Updated `Makefile`:

```make
test-go:
	go test ./...

test: build
	SYNC_AGENTS_BIN=bin/sync-agents npx bats test/sync-agents.bats
```

`package.json` `test:go` script stays as `make test`, which now runs bats;
add `test:unit` as `go test ./...` and update `test` script:

```json
"test": "npx concurrently --names \"sh,go\" -c \"cyan,magenta\" \"npm run test:sh\" \"npm run test:unit\"",
"test:sh": "SYNC_AGENTS_BIN=bin/sync-agents npx bats test/sync-agents.bats",
"test:unit": "go test ./..."
```

---

## Test Plan

### Go unit tests (`go test ./...`)

- [ ] `TestPromoteRule` — copies rule from fake project root to fake global root.
- [ ] `TestPromoteSkill` — deep-copies skill dir to fake global root.
- [ ] `TestPromoteWorkflow` — copies workflow file.
- [ ] `TestPromoteConflictNoForce` — returns error when destination exists.
- [ ] `TestPromoteConflictForce` — overwrites when `--force`.
- [ ] `TestPromoteDryRun` — no files written, expected output printed.
- [ ] `TestPromoteMissingSource` — non-zero exit + error message.
- [ ] `TestGlobalInit` — creates skeleton dirs; idempotent on second call.
- [ ] `TestGlobalSync` — symlinks created for all configured targets; `~/.agents/AGENTS.md` is regenerated.
- [ ] `TestGlobalSyncTargetFilter` — only specified targets touched.
- [ ] `TestGlobalSyncConflictNoForce` — existing real dir skipped with warning.
- [ ] `TestGlobalSyncConflictForce` — existing real dir replaced.
- [ ] `TestGlobalSyncDryRun` — no filesystem changes.
- [ ] `TestGlobalStatus` — correct `[synced]` / `[missing]` output.
- [ ] `TestGlobalClean` — removes only sync-agents symlinks.
- [ ] `TestResolveGlobalTargetDir` — correct path for each token; `windsurf`
  returns `""` (not a global target).
- [ ] `TestGlobalRootOverride` — `App.GlobalRoot` field is used instead of
  `~/.agents/` when set.
- [ ] `TestVersionSmoke` — `version.Version` is not empty string (regression
  guard from SPEC-001).

### bats integration tests (`test/sync-agents.bats`)

- [ ] `promote rule` happy path via CLI.
- [ ] `promote skill` happy path via CLI.
- [ ] `global init` creates dirs.
- [ ] `global sync` creates symlinks; re-running is idempotent.
- [ ] `global status` output format.
- [ ] `global clean` removes symlinks.
- [ ] `promote --dry-run` prints intent, no writes.
- [ ] `global sync --targets claude` touches only Claude dir.

### Manual smoke tests

- [ ] On a fresh macOS ARM machine: `sync-agents global init && sync-agents promote rule foo && sync-agents global sync` — verify `~/.claude/rules/foo.md` resolves correctly through the symlink chain.
- [ ] `sync-agents global clean` removes all global symlinks; verify `~/.agents/` is intact.
- [ ] Verify `~/.windsurf/` is **never** created by any global command.

---

## Rollout

1. One PR: `internal/agent/global.go` (new), `internal/agent/agent_test.go`,
   `internal/agent/global_test.go`, CLI wiring in `main.go`,
   Makefile `test-go` fix, `package.json` script update, bats additions.
2. README: add `promote` and `global` command table entries.
3. CHANGELOG: `feat: promote command and global sync for skills/rules/workflows`.
4. Merge → reusable workflow cuts `vX.Y.Z` tag → goreleaser + npm publish.

---

## Open Questions

- **Q1**: Should `promote` also write back a "source" header in the promoted
  file so the global copy knows its origin? (Useful for future `global pull`.)
  Recommend defer — adds schema coupling.
- **Q2** ✅ **Resolved**: `global sync` SHALL regenerate `~/.agents/AGENTS.md`
  after each sync — mirrors the project-level `AGENTS.md` pattern and gives
  Claude a global index at `~/.claude/`.
- **Q3** ✅ **Resolved**: `global init` SHALL always default to all five global
  targets (`claude,codeium,cursor,copilot,codex`). Global config is independent
  of any project's `.agents/config`.
- **Q4**: For Copilot and Codex, which only officially support a single flat
  `instructions.md`, should `global sync` also generate a merged
  `instructions.md` from all rules? Defer to a separate spec.

---

## References

- Claude global config: `~/.claude/` (Claude CLI docs)
- Codeium/Windsurf global: `~/.codeium/` (Windsurf docs — local is `.windsurf/`)
- Cursor global rules: `~/.cursor/rules/` (Cursor docs)
- GitHub Copilot instructions: `~/.github/copilot/instructions.md`
- OpenAI Codex: `~/.codex/`
- SPEC-001: `go install` first-class + goreleaser
