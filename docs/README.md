# sync-agents documentation

This folder is the prose companion to the code in `internal/` and the
specs under `specs/`. Code carries doc comments at every exported
symbol (see `.agents/rules/comments.md`); the files here explain
**concepts**, **architecture**, and **the why behind decisions** in a
form that's easier to read end-to-end than scattered code comments.

## Index

### Getting started

- [Installation](./install.md) — every install channel (npm, Homebrew,
  `go install`, GitHub Releases) with the trade-offs of each.
- [Topology & configuration](./topology.md) — the `.agents/` tree
  layout, optional buckets (`agents/`, `plans/`, `specs/`, `hooks/`,
  `adrs/`), `STATE.md`, and the `.agents/config` file.
- [Command reference](./commands/README.md) — every command and flag
  in one table, plus common usage.

### Features

- [Source manifest, lockfile & provenance](./sources.md) — declare
  upstream rules/skills/trees in `sources.yaml`; `pull` installs them
  SHA-pinned, content-hashed against `sources.lock`, with per-artifact
  origin metadata.
- [Quarantine (remote content review)](./quarantine.md) — remotely
  fetched artifacts are statically scanned and held in
  `.agents/.quarantine/` until approved; escape hatches are loud and
  audited.
- [Linked (editable) sources](./linked-sources.md) — `npm link` for
  agent context: symlink a source at a live local checkout; edits flow
  both ways; recorded declaratively in the manifest.
- [Inheritance](./inheritance.md) — hierarchical rule sharing via
  `## Inherits` links in `AGENTS.md` (project → team → org → global).
- [ADRs](./adrs.md) — Architecture Decision Records with status
  encoded by subdirectory; denied records stay on disk so they aren't
  re-proposed.
- [OS-scoped routing](./os-scoped-routing.md) — `macos/`, `linux/`,
  `unix/`, `windows/` subdirectories under any bucket sync only on
  matching hosts.

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

### Command deep-dives

- [`sync-agents index`](./commands/index.md) — AGENTS.md regeneration,
  section-by-section, including the skill frontmatter backfill.
- [`sync-agents lint`](./commands/lint.md) — SKILL.md compliance
  checks against Claude's authoring rules, with the full finding
  table.
- [`sync-agents fix`](./commands/fix.md) — legacy-layout migration,
  flat-skill conversion, and symlink repair.
- [`sync-agents promote`](./commands/promote.md) — copy a project
  artifact into the user-scope `~/.agents/` tree.
- [`sync-agents global init`](./commands/global-init.md) — create the
  user's global `.agents/` skeleton. Idempotent.
- [`sync-agents global sync`](./commands/global-sync.md) — fan the
  global `~/.agents/` tree out to per-tool global directories with
  semantic-aware routing.
- [`sync-agents global status`](./commands/global-status.md) —
  read-only, grep-friendly report of every per-tool destination's
  state.
- [`sync-agents global clean`](./commands/global-clean.md) — remove
  sync-agents-owned symlinks and concat files from per-tool global
  dirs.

### Specs

The authoritative source for *what we're building and why* lives in
[`specs/`](../specs/) at the repo root. This folder may reference SPEC
IDs but is never the source of truth for them.

Only specs with **open work** are kept in the tree; fully-shipped specs
are promoted into `docs/` and retired to git history. The
[**spec ledger**](../specs/README.md) is the permanent index — every
SPEC ID ever used has a row there (status, retire commit, where its
durable content lives), so IDs stay resolvable after the file is gone.

- [SPEC-005](../specs/SPEC-005-sandboxing-quarantine.md) — supply-chain
  safety: Parts A+B (fetch hardening, quarantine + scan) shipped;
  Part C (sandboxed skill exec) open.
- [SPEC-006](../specs/SPEC-006-os-scoped-routing.md) — OS-scoped
  routing: core shipped; AGENTS.md OS badge and concat OS headers
  open.

## Conventions

- Every doc file starts with a one-sentence purpose line under its
  title.
- Every doc file ends with a `## See also` section linking related
  docs and the relevant SPEC IDs.
- When a doc explains a behavior that was specified, link to the SPEC
  at the bottom rather than restating it. If the spec has been retired
  to git history, reference the SPEC ID as plain text.
- Prefer prose over bullet salad for conceptual material. Reserve
  bullets for enumerations and checklists.

See `.agents/rules/comments.md` for the full project rule on
documentation.

## See also

- [The comments and documentation rule](../.agents/rules/comments.md)
- [All specs](../specs/)
