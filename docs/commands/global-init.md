# `sync-agents global init`

Create the user's global `.agents/` tree with the standard skeleton:
`rules/`, `skills/`, `workflows/`, and a `config` file that lists every
known sync target.

## Synopsis

```text
sync-agents global init [--dry-run] [--global-root PATH]
```

## What it creates

```
~/.agents/
├── rules/
├── skills/
├── workflows/
└── config
```

The `config` file looks like:

```text
# sync-agents global configuration
# Comma-separated list of sync targets (available: claude, codeium, cursor, copilot, codex, opencode, openclaw)
# Note: 'codeium' is the user-scope name for Windsurf; the project-scope dir is .windsurf/
# Note: global sync does not read this line; it serves every tool whose home directory exists.
# Override per-command with: sync-agents global sync --targets claude,cursor
# Naming a tool in --targets also consents to editing a file of yours that it reads.
targets = claude,codeium,cursor,copilot,codex
```

`global sync` does not read the `targets` line. It serves every tool
whose home directory exists, or the tools `--targets` names (see
[global sync § Consent](./global-sync.md#consent)).

The target list is fixed at five tools and **not** derived from any
project's local `.agents/config`. SPEC-002 AC-11 mandates this
independence — the global config should not change based on which
directory happens to be `cwd` when `global init` runs.

## Idempotency

Running `global init` repeatedly is safe:

- Existing subdirectories are left alone.
- Missing subdirectories are created.
- An existing `config` file is **never** overwritten, even if its
  contents differ from `GlobalConfigContent`. If you want to reset
  the config, delete it manually first.

This matches SPEC-002's "existing files preserved" requirement.

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--dry-run` | false | Print the plan and exit without writing. |
| `--global-root <path>` | `$HOME/.agents` | Override the global root. Mostly for tests and shared-machine setups; see `docs/architecture/global-root-resolution.md`. |

## Examples

```bash
# Standard one-time setup.
sync-agents global init

# Preview without writing.
sync-agents global init --dry-run

# Initialize an alternate global root (test rig or shared host).
SYNC_AGENTS_GLOBAL_ROOT=/srv/shared/.agents sync-agents global init
# or equivalently:
sync-agents global init --global-root /srv/shared/.agents
```

## When to run it

`global init` is the first step in setting up a user-scope `.agents/`
tree. After init:

1. `promote` artifacts from a project into `~/.agents/`.
2. (Future PR) `global sync` to fan them out into `~/.claude/`,
   `~/.codeium/`, `~/.cursor/`, `~/.github/copilot/`, `~/.codex/`.

If you already have a `~/.agents/` from manual setup (as documented
in the README's Inheritance section), running `global init` will
add any missing subdirectories without disturbing existing content.

## Exit codes

- `0` on success.
- non-zero on filesystem errors (permissions, etc.).

## See also

- SPEC-002 §Global agents directory (shipped; spec retired to git history)
- [Global root resolution](../architecture/global-root-resolution.md)
- [Scope and target directories](../architecture/scope-and-targets.md)
- `internal/agent/globalinit.go` — the implementation.
