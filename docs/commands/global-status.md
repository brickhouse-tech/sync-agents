# `sync-agents global status`

Inspect the user's global per-tool directories and report the state
of every destination that `global sync` would manage. Read-only.

## Synopsis

```text
sync-agents global status [--targets t1,t2] [--global-root PATH]
```

## Output format

One header line, one row per per-artifact destination, then one row per
delivery channel:

```text
[info] global status (1 artifacts, 7 tool(s)):
[synced] claude/rule/security -> /home/u/.claude/rules/security.md
[unmounted] codeium -> /home/u/.codeium/windsurf/memories/global_rules.md  (tool not installed (its home directory is missing); run with --targets codeium to create it)
[synced] copilot -> /home/u/.copilot/instructions/sync-agents.instructions.md  (.copilot/instructions/sync-agents.instructions.md -> .agents/index/copilot.md)
[unmounted] codex -> /home/u/.codex/AGENTS.md  (the file exists and is yours; run once with --targets codex to let sync-agents edit it)
[gap] cursor  (Cursor keeps user rules in app settings, not in files; global rules reach Cursor through each project's .cursor/rules/sync-agents.mdc when index = local)
[info] audit: 1 gap, 2 synced, 2 unmounted
```

The bracketed state at the start of each line is parseable — useful
for shell pipelines that want to grep for `[drifted]` or `[stale]`.

## States

### Symlink destinations

| State | Meaning |
|---|---|
| `[synced]` | A symlink exists and points at the correct canonical artifact under `~/.agents/`. No action needed. |
| `[drifted]` | A symlink exists but its target is wrong (the canonical artifact moved, or a previous sync used a different path). Re-run `global sync` to repair. |
| `[not-a-symlink]` | A regular file or directory occupies the destination — a conflict: the canonical tree claims this artifact but something real shadows it. `global sync` will skip it unless `--force` is set. |
| `[missing]` | Nothing at the destination. `global sync` will create the symlink. |
| `[skipped]` | The (artifact, tool) pair is intentionally not routable — e.g. a multi-file invocable skill targeting Windsurf workflows. The Detail field explains. |
| `[folded]` | The exact per-artifact link is absent, but the path *resolves* to the canonical artifact through an ancestor symlink (e.g. a dir-level `.claude/skills/<name>` link). Conformant at a coarser granularity — no action needed. (SPEC-010) |

### Channel rows

Each tool with a user-level delivery channel gets one row: Windsurf
(`codeium`), Copilot, Codex, opencode, and OpenClaw. Cursor gets one
`[gap]` row, because it has no user-rules file. The row compares the
rendered bytes with what is on disk, so `[synced]` means `global sync`
would write nothing.

| State | Meaning |
|---|---|
| `[synced]` | The index file, region, or entry matches the render and the mount reaches it. |
| `[stale]` | The sources changed since the last `global sync`. |
| `[unmounted]` | The mount is absent. The detail says why: the tool is not installed, the file is yours and needs one `--targets` run, or the tool creates the host itself (OpenClaw). |
| `[conflict]` | A real file holds the link path. |
| `[shadowed]` | The mount is in place, but the tool reads another file first (a non-empty `$CODEX_HOME/AGENTS.override.md`). |
| `[manual]` | `opencode.json` is JSONC or unparseable; the detail holds the entry to add by hand. |
| `[gap]` | The tool has no file to deliver to (Cursor). |
| `[error]` | Rendering failed; the detail holds the error. |

See [Delivery channels](../architecture/delivery-channels.md).

### Audit sweep (SPEC-010)

After the per-destination rows, `global status` enumerates the managed
artifact subdirs of each tool (for Claude: `skills/`, `commands/`,
`rules/`, `agents/`, `plans/`, `specs/`, `adrs/`; Windsurf:
`global_workflows/`; Cursor: `rules/`) and reports entries the
canonical tree does *not* claim. The tool root itself is application
state (credentials, sessions, caches) and is never enumerated.

| State | Meaning |
|---|---|
| `[foreign]` | An entry no `~/.agents/` artifact claims and that doesn't point into the canonical tree. Hands-off: reported, never touched. Adoption into `~/.agents/` is a future explicit command (SPEC-010 Phase 2). |
| `[orphaned]` | A symlink pointing into the `~/.agents/` tree that no current artifact claims — usually left behind by an artifact removal or rename. Prune candidate. |

The report ends with a one-line summary counting every state, e.g.
`audit: 4 synced, 1 folded, 2 foreign, 1 orphaned`.

### Special states

| State | Meaning |
|---|---|
| `[frontmatter-error]` | The artifact's frontmatter couldn't be parsed; the source path is shown and `global sync` will skip the artifact entirely. |

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--targets <list>` | all | Comma-separated tool IDs (or aliases) to report on. |
| `--global-root <path>` | `$HOME/.agents` | Override the global root. |

## Examples

```bash
# Full report.
sync-agents global status

# Only Claude and Cursor.
sync-agents global status --targets claude,cursor

# Grep for drift in a CI script.
sync-agents global status | grep '^\[drifted\]' && echo "needs sync"

# Use a temp root (test rig).
sync-agents global status --global-root /tmp/.agents
```

## Exit codes

- `0` on a successful inspection regardless of the states reported.
  A drifted or stale destination is information, not an error.
- non-zero if the global root doesn't exist (run `global init`).

## What it does NOT do

- It does not write to the filesystem. Use `global sync` to act on
  any drifted/missing/stale states it reports.
- It does not validate the *content* of synced symlink targets. The
  symlink itself pointing at the right path is enough — checking the
  body would duplicate `global sync`'s integrity logic.
- It does not check artifacts that are skipped by routing (e.g. a
  passive skill against Claude). Those show up as `[skipped]`.

## See also

- SPEC-002 §Requirement: Global status (shipped; spec retired to git history)
- [`sync-agents global sync`](./global-sync.md)
- [`sync-agents global clean`](./global-clean.md)
- [Delivery channels](../architecture/delivery-channels.md)
- SPEC-013 (delivery channels)
- `internal/agent/globalstatus.go`, `internal/agent/deliver_status.go`
  — the implementation.
