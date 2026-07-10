---
id: SPEC-006
title: "OS-scoped artifact routing — per-platform rules, skills, and workflows"
status: Core Shipped (v1.3.0); open — AGENTS.md OS badge + concat OS headers
owner: nmccready
created: 2026-07-02
updated: 2026-07-09 (rev 2: shipped core trimmed to a summary; doc now tracks only the two cosmetic gaps)
related: SPEC-004, SPEC-005
---

# [SPEC-006] Feature: OS-scoped artifact routing

## Overview

Allow any `.agents/` bucket to carry optional OS-scoped subdirectories
(`macos/`, `linux/`, `unix/`, `windows/`) whose contents route **only**
when the host matches. The OS gate is applied at sync time via
`runtime.GOOS`; root-level files are always synced, preserving backward
compatibility.

This is a **routing** feature — it gates which artifacts reach which
machines. It is orthogonal to SPEC-005's runtime sandboxing (which
controls what a skill can *do* once loaded).

## Shipped (core, v1.3.0 — PR #67, 3 of 4 rollout steps)

User-facing behavior is documented in
[docs/os-scoped-routing.md](../docs/os-scoped-routing.md); this list is
the requirements-level summary:

- **Discovery:** `macos/`, `linux/`, `unix/`, `windows/` subdirs under
  any bucket are recognized; non-matching OS subdirs are skipped
  entirely (no walk-in), matching subdirs discover normally. `unix`
  matches darwin + linux.
- **Routing:** destination paths mirror the source tree
  (`.agents/rules/macos/brew.md` → `.claude/rules/macos/brew.md`), so
  platforms never collide in a flat target dir.
- **Managed CLAUDE.md @-import block:** includes only rules from
  matching OS subdirs.
- **Config override:** `os = <goos>` in `.agents/config` overrides
  `runtime.GOOS` for testing and cross-compile CI.
- **Opt-in:** `init` never creates OS subdirs; zero behavior change
  for trees without them. Existing tests pass unmodified.

## Open — cosmetic surface

Two index/concat presentation items from the original design remain
unimplemented:

### 1. AGENTS.md OS badge

The index should render OS-scoped entries with a visual indicator so
readers know which artifacts are gated:

```markdown
## Rules

- [security](.agents/rules/security.md) — Locks down auth and secrets
- [brew](.agents/rules/macos/brew.md) `[macos]` — Homebrew install audit
- [apt](.agents/rules/linux/apt.md) `[linux]` — APT package audit
- [posix](.agents/rules/unix/posix.md) `[unix]` — POSIX file perms baseline
```

The badge is informational — the index is a static file checked into
the repo, so all platforms see all entries; a Linux developer reading
AGENTS.md knows `[macos]` rules won't apply to them.

### 2. Concat OS headers

Concat files (Windsurf `global_rules.md`, Copilot, Codex) should append
scoped content with an OS header comment so readers know which platform
a block targets:

```markdown
<!-- OS: macos -->
```

### Remaining test plan

- Index test: AGENTS.md includes `[macos]` badge on scoped entries, no
  badge on unscoped; all platforms see all entries (index is static).
- Concat test: Windsurf `global_rules.md` includes `<!-- OS: macos -->`
  headers on scoped sections.

## Non-Goals (unchanged)

- Windows per-seat policies, Group Policy, or registry — Windows
  support in sync-agents is speculative; `windows/` subdirs exist for
  spec completeness but are not a deliverable.
- Runtime sandboxing (SPEC-005) — orthogonal.
- OS-specific `.agents/config` overrides beyond the `os` key — the
  config file is platform-agnostic.
