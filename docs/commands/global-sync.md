# `sync-agents global sync`

Fan the user's global `~/.agents/` tree out to per-tool global
directories — `~/.claude/`, `~/.codeium/`, `~/.cursor/`,
`~/.github/copilot/`, `~/.codex/` — using semantic-aware routing.

## Synopsis

```text
sync-agents global sync [--targets t1,t2] [--dry-run] [--force] [--global-root PATH]
```

## What it does

For every artifact under `~/.agents/{rules,skills,workflows}/` and
every registered tool, the sync:

1. **Resolves the artifact's semantic** (`invocable` or `passive`)
   via YAML frontmatter or, when absent, the bucket default. See
   [Semantic routing](../architecture/semantic-routing.md).
2. **Resolves the per-tool destination** for that (semantic, tool)
   pair using the routing table in SPEC-002.
3. **Performs the appropriate filesystem operation**:
   - Per-artifact **symlink** (Claude, Cursor, Windsurf invocables)
   - Append to a **concat batch** (Windsurf passive, Copilot, Codex)
   - Append to a **region batch** (OpenClaw passive): one marked
     region inside a file another program owns
   - **Skip with warning** (multi-file invocable skills targeting
     Windsurf workflows; passive skill dirs targeting Claude)
4. **Regenerates concat targets** at the end — one atomic
   tmp+rename write per concat file. If the new content equals the
   existing content (byte-identical), the file is left alone
   preserving its mtime. Region hosts get the same treatment, and
   only the bytes between their markers ever change.

## Per-tool routing reference

| Tool | Invocable destination | Passive destination |
|---|---|---|
| `claude` | `~/.claude/skills/<name>/SKILL.md` (skill) or `~/.claude/commands/<name>.md` (rule/workflow) | `~/.claude/rules/<name>.md` |
| `codeium` | `~/.codeium/windsurf/global_workflows/<name>.md` (single-file only) | concat → `~/.codeium/windsurf/memories/global_rules.md` |
| `cursor` | `~/.cursor/rules/<name>.md` | `~/.cursor/rules/<name>.md` (same — Cursor doesn't distinguish) |
| `copilot` | concat → `~/.github/copilot/instructions.md` | same concat |
| `codex` | concat → `~/.codex/instructions.md` | same concat |
| `openclaw` | skipped (no command surface; skills load natively) | region `openclaw-rules` → `<workspace>/AGENTS.md` |

Skipped cases:

- **Multi-file invocable skill → Codeium**: Windsurf workflows are
  single `.md` files, so a skill dir with `SKILL.md` plus supporting
  files can't be a workflow without flattening. Skipped for codeium
  only with a warning; the skill still syncs to other tools.
- **Passive skill → Claude**: a directory marked `invocable: false`
  has no clean Claude destination (Claude's passive surface is a
  single `rules/*.md` file). Skipped with a warning. Decide whether
  to split the skill into rules or accept the gap.

## OpenClaw

OpenClaw reads a fixed set of files from its workspace and follows
neither `@`-imports nor links, so passive rule **text** is inlined
into `<workspace>/AGENTS.md` between
`<!-- sync-agents:openclaw-rules:start -->` and `...:end -->`.
Everything outside the markers (the project index, a `## Tools`
section OpenClaw's doctor added, your own notes) is left untouched.

- **Workspace.** Resolved the way OpenClaw resolves it: config
  `agents.defaults.workspace`, then `$OPENCLAW_WORKSPACE_DIR`, then
  `<state dir>/workspace`. The state dir is `$OPENCLAW_STATE_DIR`, or
  `~/.openclaw` (`~/.openclaw-<profile>` for `$OPENCLAW_PROFILE`), under
  `$OPENCLAW_HOME` when set. The config is `$OPENCLAW_CONFIG_PATH` or
  `<state dir>/openclaw.json`. A config that fails to parse (including
  JSON5 comments) skips the target with a warning rather than guessing.
- **Consent.** A plain `global sync` leaves OpenClaw alone until you
  opt in once with `--targets openclaw`, which adds the markers. From
  then on the default sync keeps the region current. Delete the region
  (or run `global clean`) to opt out.
- **Never creates the file.** If `<workspace>/AGENTS.md` does not
  exist, the target is skipped: run OpenClaw once so it seeds its own
  template.
- **Size cap.** OpenClaw cuts the middle out of any bootstrap file
  longer than `agents.defaults.bootstrapMaxChars` (default 20000), so
  sync fits the region to the cap. If the file would pass 90% of it,
  sync replaces the largest rules, one at a time, with their
  description (or first paragraph) and the absolute path of the full
  text. If the file still passes the cap with every rule summarized,
  sync exits non-zero and leaves AGENTS.md untouched. `global status`
  uses the same fit, so a fitted region reads `synced`.
- **Skipped.** Skills (OpenClaw loads `~/.agents/skills` itself),
  invocable rules and workflows, agents, and reference docs.

`global clean` strips only the region and keeps the file. `global
status` prints one `[region synced|stale|missing]` row for it.

## Idempotency

Re-running `global sync` on an unchanged tree:

- **Symlinks** that already point at the canonical artifact are
  left alone.
- **Symlinks** that point at a different (or broken) target are
  silently repaired and logged as `[repair]`.
- **Concat files** whose newly-computed content byte-equals the
  existing content are not rewritten — `mtime` is preserved so
  external watchers don't fire spuriously.

If everything is current, the command exits 0 with summary lines
like `… already current (N entries)`.

## Conflicts

If a **non-symlink** file or directory exists at a per-tool symlink
destination, the sync refuses to overwrite it without `--force`. The
warning names the path so you can investigate.

With `--force`, the conflicting file is moved to
`<path>.replaced-by-sync-agents` (so it's recoverable) before the
fresh symlink is placed.

Concat files don't go through the conflict check — they're
sync-agents-owned by design. Any user edits to a generated concat
will be overwritten on the next sync; that's why the banner says
"do not edit by hand."

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--targets <list>` | all | Comma-separated tool IDs to sync. Aliases honored: `windsurf` resolves to `codeium`. Unknown names produce a warning and are skipped. The default includes `openclaw` only once its region markers exist (see [OpenClaw](#openclaw)). |
| `--dry-run` | false | Print every planned operation prefixed with `[dry-run]`. No filesystem writes. |
| `--force` | false | Replace non-symlink files in the way (saved as `*.replaced-by-sync-agents`). |
| `--global-root <path>` | `$HOME/.agents` | Override the global root. See [Global root resolution](../architecture/global-root-resolution.md). |

`--dry-run` and `--force` are persistent flags inherited from `rootCmd`.

## Examples

```bash
# Standard sync of everything.
sync-agents global sync

# Only sync Claude and Cursor.
sync-agents global sync --targets claude,cursor

# Windsurf alias works.
sync-agents global sync --targets windsurf

# Preview without writing.
sync-agents global sync --dry-run

# Use a temp root (test rig).
sync-agents global sync --global-root /tmp/.agents

# Compose with promote in one shot.
sync-agents promote rule security --sync
```

## Promote-and-sync composition

`sync-agents promote ... --sync` runs `global sync` immediately after
a successful promote. Use `--sync-targets <list>` on the `promote`
command to limit which tools are synced in that follow-up:

```bash
# Promote a rule and immediately fan it out to just Claude.
sync-agents promote rule security --sync --sync-targets claude
```

When `--sync` is omitted, `--sync-targets` is ignored.

## Exit codes

- `0` on success or a clean dry-run. Skipped artifacts produce
  warnings but do not change the exit code.
- non-zero if `~/.agents/` doesn't exist (run `global init`) or if
  a filesystem error halts the whole sync (rare).

Per-tool failures (concat regen error, symlink creation error)
produce warnings but do not abort the run — partial progress is
preferable to all-or-nothing for fan-out operations.

## What `global sync` does NOT do

- It does not pull artifacts from upstream repos — that's
  SPEC-003 (shipped; spec retired to git history).
- It does not regenerate per-project sync targets (the local
  `.claude/`, `.windsurf/`, etc.). Use the existing `sync-agents
  sync` command for project scope.
- It does not modify or delete content inside `~/.agents/`. The
  canonical tree is read-only from sync's perspective.

## See also

- SPEC-002 §Requirement: Global sync (shipped; spec retired to git history)
- SPEC-002 §Requirement: Semantic-aware routing (shipped; spec retired to git history)
- [Semantic routing](../architecture/semantic-routing.md)
- [Scope and target directories](../architecture/scope-and-targets.md)
- [`sync-agents promote`](./promote.md)
- [`sync-agents global init`](./global-init.md)
- `internal/agent/globalsync.go`, `internal/agent/destination.go`,
  `internal/agent/concat.go`, `internal/agent/semantic.go` —
  the implementation.
