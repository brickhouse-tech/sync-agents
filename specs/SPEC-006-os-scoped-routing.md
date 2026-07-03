---
id: SPEC-006
title: "OS-scoped artifact routing — per-platform rules, skills, and workflows"
status: Draft
owner: nmccready
created: 2026-07-02
updated: 2026-07-02
related: SPEC-004, SPEC-005
---

# [SPEC-006] Feature: OS-scoped artifact routing

## Overview

Allow any `.agents/` bucket to carry optional OS-scoped subdirectories
(`macos/`, `linux/`, `unix/`, `windows/`) whose contents route **only**
when the host matches.

```
.agents/
├── rules/
│   ├── security.md          → always
│   ├── macos/
│   │   └── brew.md          → Darwin only
│   ├── linux/
│   │   └── apt.md           → Linux only
│   └── unix/
│       └── posix.md         → Darwin + Linux (not Windows)
├── skills/
│   ├── macos/
│   │   └── screen-recording/  → Darwin only
│   └── ...
```

The OS gate is applied at **sync time** via `runtime.GOOS`: artifacts
in `macos/` only symlink on Darwin, `linux/` only on Linux, `unix/` on
both, `windows/` only on Windows. The root-level files are always
synced (no gate), preserving backward compatibility.

This is a **routing** feature — it gates which artifacts reach which
machines. It is orthogonal to SPEC-005's runtime sandboxing (which
controls what a skill can *do* once loaded).

## Motivation

- Homelab/server skill sets differ from desktop Mac workflows.
  `brew` rules are noise on a Linux box; `apt` rules are noise on a
  Mac. Today you either maintain separate repos or pollute every
  machine with irrelevant context.
- Cross-platform teams commit one `.agents/` tree. Without OS gating,
  every dev gets every rule regardless of their OS.
- The `unix/` subdir covers the common case (POSIX conventions that
  apply to both macOS and Linux but not Windows) without duplicating
  files.

## Goals

- `macos/`, `linux/`, `unix/`, `windows/` subdirectories under any
  bucket are recognized by the artifact discovery loop.
- Non-matching OS subdirs are silently skipped at sync time (no
  symlink, no concat contribution, no index entry).
- The managed CLAUDE.md @-import block only includes rules from
  matching OS subdirs.
- AGENTS.md index lists OS-scoped entries with a visual indicator
  (`[macos]`, `[linux]`, `[unix]`) so readers know which artifacts
  are gated.
- All existing tests pass unmodified (no OS subdirs = no behavior
  change).
- `sync-agents init` does NOT create OS subdirs — they activate when
  the user creates them (same opt-in model as plans/specs/agents).

## Non-Goals

- Windows per-seat policies, Group Policy, or registry — Windows
  support in sync-agents is speculative; `windows/` subdirs are
  included for spec completeness but not a v1 deliverable.
- Runtime sandboxing (SPEC-005) — orthogonal.
- OS-specific `.agents/config` overrides — the config file is
  platform-agnostic.

## Design

### Discovery

`DiscoverArtifacts` (or a new wrapper) walks each bucket's directory
tree. When it enters an OS-scoped subdirectory, it checks:

```go
var osScopes = map[string]func(string) bool{
    "macos":   func(goos string) bool { return goos == "darwin" },
    "linux":   func(goos string) bool { return goos == "linux" },
    "unix":    func(goos string) bool { return goos == "darwin" || goos == "linux" },
    "windows": func(goos string) bool { return goos == "windows" },
}
```

If the scope matches, artifacts within are discovered normally. If it
doesn't match, the entire subdirectory is skipped (no `WalkDir` into
it). This is cheaper than discovering then filtering.

Scoped names retain the subdirectory prefix as context: a rule at
`rules/macos/brew.md` is named `macos/brew` internally. The destination
path resolves to `rules/macos/brew.md` → symlinked as
`.claude/rules/macos/brew.md` (the OS subdir is preserved so multiple
platforms' rules don't collide in the same flat `rules/` target dir).

### Routing

The destination path for a scoped artifact mirrors the source tree:

```
.agents/rules/macos/brew.md → .claude/rules/macos/brew.md
.agents/skills/macos/screen-recording/SKILL.md → .claude/skills/macos/screen-recording/SKILL.md
```

This means tool directories gain the same OS subdirectory structure as
`.agents/`. Claude loads them from nested `rules/macos/` the same way
it loads from `rules/` — no per-tool changes needed.

### Index

AGENTS.md renders OS-scoped entries with a badge:

```markdown
## Rules

- [security](.agents/rules/security.md) — Locks down auth and secrets
- [brew](.agents/rules/macos/brew.md) `[macos]` — Homebrew install audit
- [apt](.agents/rules/linux/apt.md) `[linux]` — APT package audit
- [posix](.agents/rules/unix/posix.md) `[unix]` — POSIX file perms baseline
```

The badge is informational — the index is a static file checked into
the repo. A Linux developer reading AGENTS.md knows `[macos]` rules
won't apply to them.

### Managed CLAUDE.md block

The @-import block only lists rules from matching OS subdirs. On macOS:

```markdown
@/Users/me/.claude/rules/security.md
@/Users/me/.claude/rules/macos/brew.md
@/Users/me/.claude/rules/unix/posix.md
```

`linux/apt.md` is excluded because `runtime.GOOS != "linux"`.
`RegenerateClaudeImports` already receives a filtered list from the
discovery loop; OS filtering happens at the source.

### Tool support

OS subdirs route to **all tools**, same as the parent bucket — no
per-tool restrictions beyond what the bucket already has. A rule in
`rules/macos/` routes through the same destination logic as
`rules/`, including Windsurf concat, Cursor symlink, etc.

The only difference is the extra `macos/` path segment in the
destination.

### Config override

`.agents/config` gains an optional `os = <goos>` key for testing and
cross-compile CI. When set, discovery uses the configured OS instead
of `runtime.GOOS`. Default: absent → use `runtime.GOOS`.

## Backwards Compatibility

- Zero change for `.agents/` trees with no OS subdirectories.
- OS subdirs are never created by `init` — opt-in by creating the
  directory.
- AGENTS.md index gains the `[os]` badge inline; existing entries are
  unchanged.
- Concat files (Windsurf, Copilot, Codex) append scoped content with
  an OS header comment so readers of `global_rules.md` know which
  platform a block targets.

## Test Plan

- Table-driven test: each OS subdir name → expected match on
  darwin/linux/windows. `unix` matches darwin + linux but not
  freebsd (out of scope for v1).
- Discovery test: seed `rules/`, `rules/macos/`, `rules/linux/`,
  `rules/unix/` → run discovery on each GOOS → assert correct set.
- Sync golden-tree test per platform: seed the same tree → sync on
  each OS → assert correct subset of symlinks.
- Idempotency: sync twice on the same OS → no changes.
- Index test: AGENTS.md includes `[macos]` badge on scoped entries,
  no badge on unscoped; all platforms see all entries (index is
  static).
- Concat test: Windsurf `global_rules.md` includes `<!-- OS: macos -->`
  headers on scoped sections.
- Cross-compile test: set `os = linux` → discovery returns
  `linux/` + `unix/` artifacts but not `macos/`.

## Rollout

Single PR — the feature is self-contained:
1. Extend `DiscoverArtifacts` with OS scope awareness.
2. Extend `destination.go` path construction to preserve the OS subdir.
3. Extend `generateAgentsMD` to badge scoped entries.
4. Extend `RegenerateClaudeImports` (already filtered by the caller;
   minimal change).
5. Add `os` key to `.agents/config` parser.
6. Tests (table + golden-tree + discovery + index + concat).

No dependency on SPEC-004 Part C (hooks) or SPEC-005.