# `sync-agents promote`

Copy a project-level artifact (rule, skill, or workflow) from the
project's `.agents/` tree into the user's global `~/.agents/` tree.

## Synopsis

```text
sync-agents promote <type> <name>     [--force] [--dry-run] [--global-root PATH]
sync-agents promote <path>            [--force] [--dry-run] [--global-root PATH]
```

`<type>` is one of `rule`, `skill`, `workflow` (singular or plural; case
insensitive). `<name>` is the artifact's identifier as it appears under
`.agents/<bucket>/`.

## Invocation forms

The canonical form is two-argument: `sync-agents promote skill cool`.
The single-argument *path form* is sugar — give it a path under
`.agents/{rules,skills,workflows}/` and it derives the type and name
for you:

| Path you pass | Resolved type | Resolved name |
|---|---|---|
| `.agents/skills/cool` | skill | `cool` |
| `.agents/skills/cool/SKILL.md` | skill | `cool` |
| `.agents/rules/security.md` | rule | `security` |
| `.agents/workflows/release.md` | workflow | `release` |

Paths outside the three known buckets are rejected with a hint to use
the canonical form.

## What gets copied

| Artifact | Source under `.agents/` | Destination under `~/.agents/` |
|---|---|---|
| Rule | `rules/<name>.md` | `rules/<name>.md` |
| Skill | `skills/<name>/` (directory) | `skills/<name>/` (deep copy) |
| Workflow | `workflows/<name>.md` | `workflows/<name>.md` |

Skills are directories, so the entire tree is copied — `SKILL.md` plus
any supporting files. Symlinks inside the source are skipped (see
`internal/agent/promote.go` for the rationale).

## Copy, not symlink

`promote` writes a **copy** at the destination, not a symlink back to
the project tree. This is the rev-2 design choice in SPEC-002 —
project-relative symlinks should not exist in the global tree, and the
global copy is the canonical version of the promoted artifact going
forward.

If you'd rather keep editing locally and have global mirror, the
inverse direction (`demote`) is tracked under Future Work in
SPEC-002. Today, after promote, you edit the global file directly and
let `global sync` (future PR) fan it out to per-tool dirs.

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--force` | false | Overwrite the destination if it already exists. Without this, an existing destination causes a non-zero exit and no filesystem changes. |
| `--dry-run` | false | Print the planned copy operation and exit without writing. |
| `--global-root <path>` | `$HOME/.agents` | Override the global root. Mostly for tests and shared-machine setups; see `docs/architecture/global-root-resolution.md`. |

`--force` and `--dry-run` are persistent flags inherited from `rootCmd`.

## Conflicts

If the destination exists and `--force` is not set, promote exits
non-zero with a message naming the destination. The local and global
copies are left untouched. There is intentionally no "content matches,
skip" branch — the absence of a destination is the only path that
silently succeeds. This matches SPEC-002's "fail fast on conflict"
posture.

With `--force` the destination is removed (recursively, for skill
directories) before the new copy is written.

## Auto-creating parents

You can `promote` into a global tree that doesn't yet have the
`rules/`, `skills/`, or `workflows/` subdirectory — `promote` will
create the missing parents. You do not need to run `global init`
first, though doing so is recommended because it also writes the
canonical `config` file.

## Examples

```bash
# Promote a rule the obvious way.
sync-agents promote rule security

# Same, via the path-form sugar.
sync-agents promote .agents/rules/security.md

# Deep-copy a multi-file skill.
sync-agents promote skill code-review

# Preview what would happen without writing.
sync-agents promote --dry-run workflow release

# Force-overwrite a stale global copy.
sync-agents promote --force rule security

# Use a temp global root (e.g. in a test rig).
sync-agents promote --global-root /tmp/test/.agents rule security
```

## Exit codes

- `0` on success or a clean dry-run.
- non-zero on missing source, conflict without `--force`, unknown
  type, or unrecognised path.

## What `promote` does NOT do

- It does not regenerate `~/.agents/AGENTS.md`. The global index is
  refreshed by `global sync` (future PR). AC-10 in SPEC-002 attaches
  that responsibility to sync, not promote.
- It does not run `global sync` automatically. The `--sync` flag is
  reserved for a future PR.
- It does not modify the local `.agents/` tree. The local copy stays
  intact even after `--force`.

## See also

- SPEC-002 §Promote command (shipped; spec retired to git history)
- [Global root resolution](../architecture/global-root-resolution.md)
- [Scope and target directories](../architecture/scope-and-targets.md)
- `internal/agent/promote.go` — the implementation.
