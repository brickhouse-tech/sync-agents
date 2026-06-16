# sync-agents documentation

This folder is the prose companion to the code in `internal/` and the
specs under `specs/`. Code carries doc comments at every exported
symbol (see `.agents/rules/comments.md`); the files here explain
**concepts**, **architecture**, and **the why behind decisions** in a
form that's easier to read end-to-end than scattered code comments.

## Index

### Architecture

- [Scope and target directories](./architecture/scope-and-targets.md) —
  How project scope (local) and user scope (global) compose with the
  per-tool target directories (`.claude/`, `.windsurf/`, `.cursor/`,
  `.github/copilot/`, `.codex/`). Explains the Windsurf-vs-Codeium
  asymmetry: `.windsurf/` is the local-only Windsurf dir, while
  `~/.codeium/` is the user-level Codeium dir.
- [Global root resolution](./architecture/global-root-resolution.md) —
  The precedence chain that determines where the global `.agents/`
  tree lives: programmatic field → env var → `$HOME/.agents`. Also
  describes how per-tool global directories (like `~/.claude/`) are
  derived from the resolver.
- [Semantic routing](./architecture/semantic-routing.md) — Why an
  artifact's bucket (`rules/`, `skills/`, `workflows/`) is not the
  same thing as its behavioral category (`invocable` / `passive`),
  and how `sync-agents` resolves the latter via YAML frontmatter or
  per-bucket defaults to route correctly into each tool's matching
  destination.

### Commands

- [`sync-agents promote`](./commands/promote.md) — Copy a project
  artifact (rule, skill, or workflow) into the user-scope
  `~/.agents/` tree. Supports both canonical
  (`promote <type> <name>`) and path-form (`promote <path>`)
  invocation.
- [`sync-agents global init`](./commands/global-init.md) — Create
  the user's global `.agents/` skeleton (`rules/`, `skills/`,
  `workflows/`, `config`). Idempotent.
- [`sync-agents global sync`](./commands/global-sync.md) — Fan the
  global `~/.agents/` tree out to per-tool global directories
  (`~/.claude/`, `~/.codeium/`, …) using semantic-aware routing.
  Composable with `promote --sync`.
- [`sync-agents global status`](./commands/global-status.md) —
  Read-only report of every per-tool destination's state: synced,
  drifted, missing, not-a-symlink, or per-concat ok/stale/missing/
  foreign. Output is grep-friendly.
- [`sync-agents global clean`](./commands/global-clean.md) — Remove
  sync-agents-owned symlinks and concat files from per-tool global
  dirs. Two-check safety contract: symlinks must point into
  `~/.agents/`, files must carry the banner.

### Specs

The authoritative source for *what we're building and why* lives in
[`specs/`](../specs/) at the repo root. This folder may reference SPEC
IDs but is never the source of truth for them.

- [SPEC-001](../specs/SPEC-001-go-install-first-class.md) — first-class
  `go install` and goreleaser-based release.
- [SPEC-002](../specs/SPEC-002-promote-global-sync.md) — promote and
  global sync with semantic-aware routing across Claude, Windsurf,
  Cursor, Copilot, Codex.
- [SPEC-003](../specs/SPEC-003-source-manifest-pull.md) — declarative
  source manifest for pulling rules, skills, and workflows from
  upstream repositories.

## Conventions

- Every doc file starts with a one-sentence purpose line under its
  title.
- Every doc file ends with a `## See also` section linking related
  docs and the relevant SPEC IDs.
- When a doc explains a behavior that was specified, link to the SPEC
  at the bottom rather than restating it.
- Prefer prose over bullet salad for conceptual material. Reserve
  bullets for enumerations and checklists.

See `.agents/rules/comments.md` for the full project rule on
documentation.

## See also

- [The comments and documentation rule](../.agents/rules/comments.md)
- [All specs](../specs/)
