# OS-scoped routing

Per-platform rules, skills, and workflows: OS-named subdirectories under any bucket route only on matching hosts.

## Layout

Any `.agents/` bucket may carry optional OS-scoped subdirectories
(`macos/`, `linux/`, `unix/`, `windows/`) whose contents route **only**
when the host matches:

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
```

The OS gate is applied at **sync time** via `runtime.GOOS`. Root-level
files are always synced (no gate), so a tree without OS subdirs behaves
exactly as before. `init` never creates OS subdirs — they activate when
you create them, the same opt-in model as the optional buckets.

## Why

- Homelab/server skill sets differ from desktop Mac workflows: `brew`
  rules are noise on a Linux box, `apt` rules are noise on a Mac.
- Cross-platform teams commit one `.agents/` tree; without OS gating,
  every dev gets every rule regardless of their OS.
- `unix/` covers the common case (POSIX conventions that apply to
  macOS and Linux but not Windows) without duplicating files.

## Routing details

Destination paths mirror the source tree
(`.agents/rules/macos/brew.md` → `.claude/rules/macos/brew.md`), so
multiple platforms' artifacts never collide in a flat target dir. The
managed CLAUDE.md `@`-import block includes only rules from matching OS
subdirs. `.agents/config` accepts an `os = <goos>` override for testing
and cross-compile CI.

### How scoped artifacts are shown

- **AGENTS.md index** lists every OS-scoped artifact with a badge, after
  the unscoped entries in each section:

  ```markdown
  - [security](.agents/rules/security.md) — Locks down auth and secrets
  - [brew](.agents/rules/macos/brew.md) `[macos]` — Homebrew install audit
  - [apt](.agents/rules/linux/apt.md) `[linux]` — APT package audit
  ```

  The index is a static file checked into the repo, so it lists entries
  for **every** platform regardless of the host OS; the badge tells a
  reader which ones won't apply to them. Only sync is OS-gated.
- **Concat targets** (Windsurf `global_rules.md`, Copilot, Codex) put an
  `<!-- OS: macos -->` comment before each OS-scoped section. It is
  invisible when the tool renders Markdown.

This is a **routing** feature — it gates which artifacts reach which
machines. It is orthogonal to the quarantine/sandboxing work, which
controls what a skill can *do* once loaded.

## See also

- [Topology & configuration](./topology.md)
- SPEC-006 (retired; fully shipped) — see the [spec ledger](../specs/README.md)
