# `sync-agents global sync`

Fan the user's global `~/.agents/` tree out to each installed tool's
user-level files, using semantic-aware routing for per-artifact links
and delivery channels for passive rule content.

## Synopsis

```text
sync-agents global sync [--targets t1,t2] [--dry-run] [--force] [--global-root PATH]
```

## What it does

1. **Strips the 1.x `claude-imports` block** from `~/.claude/CLAUDE.md`
   and removes legacy placements whose proof holds (see
   [migration-v2.md](../migration-v2.md#legacy-files-and-the-proofs-used)).
2. **Routes each artifact** under `~/.agents/` per tool. It resolves
   the artifact's semantic (`invocable` or `passive`) from frontmatter
   or the bucket default (see
   [Semantic routing](../architecture/semantic-routing.md)), then:
   - places a per-artifact **symlink** (Claude rules, commands, skills,
     agents, plans, specs, ADRs; Windsurf workflows; Cursor and
     opencode agents);
   - hands **passive** content to the tool's delivery channel;
   - or **skips** with a reason (for example, Codex, Cursor, opencode,
     and OpenClaw load `~/.agents/skills` natively).
3. **Delivers passive content through channels.** For each tool with a
   user-level channel it renders `~/.agents/index/<tool>.md` and mounts
   it. See [Delivery channels](../architecture/delivery-channels.md).
   A file whose rendered bytes are unchanged is not rewritten, so its
   mtime is kept.

A tool is served only when its home directory exists (it is installed)
or `--targets` names it. Editing a file you own needs one `--targets`
run; see [Consent](#consent).

## Per-tool routing reference

| Tool | Invocable | Passive (rule content) |
|---|---|---|
| `claude` | `~/.claude/skills/<name>/SKILL.md` (skill) or `~/.claude/commands/<name>.md` (rule/workflow) | `~/.claude/rules/<name>.md` (one link per rule) |
| `codeium` | `~/.codeium/windsurf/global_workflows/<name>.md` (single-file only) | region `codeium-rules` in `~/.codeium/windsurf/memories/global_rules.md` |
| `cursor` | skipped (skills load natively; no user-scope command surface) | none: Cursor has no user-rules file. User rules reach Cursor through each project's `.cursor/rules/sync-agents.mdc` |
| `copilot` | skipped | link `~/.copilot/instructions/sync-agents.instructions.md` -> `~/.agents/index/copilot.md` |
| `codex` | skipped (skills load natively) | region `codex-rules` in `$CODEX_HOME/AGENTS.md` (default `~/.codex`) |
| `opencode` | skipped (skills load natively) | entry `"<home>/.agents/index/opencode.md"` in `~/.config/opencode/opencode.json` `instructions` |
| `openclaw` | skipped (skills load natively) | region `openclaw-rules` in `<workspace>/AGENTS.md` |

Rules over a tool's size limit become one-line pointers; sync names
them and the setting that raises the limit. See
[Budgets](../architecture/delivery-channels.md#budgets).

Skipped cases with a warning:

- **Multi-file invocable skill → Codeium**: Windsurf workflows are
  single `.md` files, so a skill dir with `SKILL.md` plus supporting
  files can't be a workflow without flattening. The skill still syncs
  to other tools.
- **Passive skill → Claude**: a directory marked `invocable: false`
  has no clean Claude destination (Claude's passive surface is a
  single `rules/*.md` file). Decide whether to split the skill into
  rules or accept the gap.

## Consent

- **Creating** a file that does not exist needs no consent:
  `~/.codex/AGENTS.md` when `~/.codex` exists, Windsurf's
  `global_rules.md` when `~/.codeium` exists, a new `opencode.json`.
- **Editing** a file that exists with your content needs one run that
  names the tool, for example `sync-agents global sync --targets codex`.
  After that, the region markers or the config entry carry consent and
  a plain `global sync` keeps them current.
- An undeliverable channel is silent unless `--targets` names the tool.
  `global status` shows it as `unmounted` with the reason.

## OpenClaw

OpenClaw reads a fixed set of files from its workspace and follows
neither `@`-imports nor links, so passive rule **text** is inlined
into `<workspace>/AGENTS.md` between
`<!-- sync-agents:openclaw-rules:start -->` and `...:end -->`.
Everything outside the markers (a `## Tools` section OpenClaw's doctor
added, your own notes) is left untouched.

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
  longer than `agents.defaults.bootstrapMaxChars` (default 20000). Sync
  fits the region so the whole file stays under the cap, and turns
  rules that do not fit into pointers.
- **Skipped.** Skills (OpenClaw loads `~/.agents/skills` itself),
  invocable rules and workflows, agents, and reference docs.

When the workspace is also a sync-agents project, its `AGENTS.md` is
that project's file too. The project migration and the region coexist:
neither writes outside its own bytes.

`global clean` strips only the region and keeps the file. `global
status` prints one channel row for it.

## Idempotency

Re-running `global sync` on an unchanged tree:

- **Symlinks** that already point at the canonical artifact are
  left alone.
- **Symlinks** that point at a different (or broken) target are
  silently repaired and logged as `[repair]`.
- **Index files, regions, and config entries** whose new content
  byte-equals the existing content are not rewritten, so `mtime` is
  preserved and external watchers do not fire.

If everything is current, the command exits 0 with summary lines
like `… already current (N entries)`.

## Conflicts

If a **non-symlink** file or directory exists at a per-tool symlink
destination, the sync refuses to overwrite it without `--force`. The
warning names the path so you can investigate.

With `--force`, the conflicting file is moved to
`<path>.replaced-by-sync-agents` (so it's recoverable) before the
fresh symlink is placed.

A real file at a channel link path (for example
`~/.copilot/instructions/sync-agents.instructions.md`) is a conflict:
it is left alone, and `global sync` exits non-zero. Files under
`~/.agents/index/` are generated; edit the sources in `~/.agents/`
instead.

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--targets <list>` | every installed tool | Comma-separated tool IDs to sync. Aliases honored: `windsurf` resolves to `codeium`. Unknown names produce a warning and are skipped. Naming a tool consents to editing a file of yours it reads, and creates its home when missing (see [Consent](#consent)). |
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
- non-zero if `~/.agents/` doesn't exist (run `global init`), if a
  channel link path is held by a real file, or if a filesystem error
  halts the whole sync (rare).

Other per-tool failures (a render error, a symlink creation error)
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
- [Delivery channels](../architecture/delivery-channels.md)
- [Migrating to 2.0](../migration-v2.md)
- [Scope and target directories](../architecture/scope-and-targets.md)
- [`sync-agents promote`](./promote.md)
- [`sync-agents global init`](./global-init.md)
- SPEC-013 (delivery channels), SPEC-012 (OpenClaw target)
- `internal/agent/globalsync.go`, `internal/agent/destination.go`,
  `internal/agent/deliver.go`, `internal/agent/channel.go`,
  `internal/agent/semantic.go` — the implementation.
