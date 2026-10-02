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
multiple platforms' artifacts never collide in a flat target dir.
`.agents/config` accepts an `os = <goos>` override for testing
and cross-compile CI.

### How scoped artifacts are shown

- **Delivery channel files** (`.agents/index/*`, and the regions in
  Windsurf's `global_rules.md`, `~/.codex/AGENTS.md`, and the OpenClaw
  workspace) put an `<!-- OS: macos -->` comment before each OS-scoped
  section. It is invisible when the tool renders Markdown.
- Under `index = local` (the default) those files hold only the rules
  for this machine's OS. Under `index = commit` they hold **every**
  platform's rules, so the committed bytes are the same on every
  contributor's machine; the OS comment tells the reading agent which
  sections apply. See
  [Delivery channels](./architecture/delivery-channels.md#index-policy).

This is a **routing** feature — it gates which artifacts reach which
machines. It is orthogonal to the quarantine/sandboxing work, which
controls what a skill can *do* once loaded.

## See also

- [Topology & configuration](./topology.md)
- [Delivery channels](./architecture/delivery-channels.md)
- SPEC-006 (retired; fully shipped) — see the [spec ledger](../specs/README.md)
