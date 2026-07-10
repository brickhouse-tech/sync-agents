# Command reference

Every `sync-agents` command and global option, with links to the deep-dive doc where one exists.

## Commands

| Command | Description |
|---|---|
| `init` | Initialize the `.agents/` directory structure with `rules/`, `skills/`, `workflows/`, `STATE.md`, and generate `AGENTS.md` |
| `sync` | Create symlinks from `.agents/` into all target directories, and symlink `AGENTS.md` to `CLAUDE.md` |
| `watch` | Watch `.agents/` for changes and auto-regenerate `AGENTS.md` |
| `import <url>` | Import a rule/skill/workflow from a URL |
| `pull [--dry-run\|--offline\|--force\|--only NAME\|--global]` | Fetch every `sources.yaml` entry, verify integrity, install into the matching buckets ([sources](../sources.md)) |
| `update [NAME]` | Re-resolve refs and re-pull entries whose upstream moved; SHA-pinned entries are skipped |
| `source add <entry>` | Append an entry to `sources.yaml` and pull it |
| `source add --link[=<path>] [<entry>]` | Declare a **linked (editable)** source — symlink a live local checkout instead of a fetched snapshot ([linked sources](../linked-sources.md)) |
| `source remove <name> [--keep]` | Remove the manifest entry and delete the artifact (`--keep` converts it to manual) |
| `source list [--json]` | Show each entry's local state: `ok` / `outdated` / `modified` / `missing` / `linked` |
| `source bundle` | Rebuild `sources.yaml` from installed artifacts' origin metadata |
| `source detach <name>` | Un-manage an artifact: flip its origin to manual and drop the manifest entry (for a **linked** source, freeze the live copy into a vendored snapshot) |
| `quarantine` | List remotely-fetched artifacts awaiting review, with their scan findings ([quarantine](../quarantine.md)) |
| `approve <name>\|--all [--force]` | Promote a quarantined artifact into `.agents/` (`--force` accepts critical findings, recorded in the lock) |
| `reject <name>\|--all` | Delete a quarantined artifact without installing it |
| `git-hook` | Install a pre-commit git hook for auto-sync (`hook` remains as a deprecated alias) |
| `inherit <label> <path>` | Add an inheritance link to AGENTS.md ([inheritance](../inheritance.md)) |
| `inherit --list` | List current inheritance links |
| `inherit --remove <label>` | Remove an inheritance link by label |
| `status` | Show the current sync status of all targets and symlinks |
| `add <type> <name>` | Add a new artifact from a template (type is `rule`, `skill`, `workflow`, `agent`, `plan`, `spec`, `hook`, or `adr`) |
| `index [--no-fix]` | Regenerate `AGENTS.md` by scanning `.agents/`. Backfills fixable skill frontmatter first (`--no-fix` skips the backfill) ([index](./index.md)) |
| `adr <accept\|deny\|propose> <name>` | Move an ADR between status directories, update its `status:` frontmatter, and reindex ([ADRs](../adrs.md)) |
| `lint [skills] [--fix]` | Validate SKILL.md frontmatter against [Claude's skill authoring rules](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices); `--fix` amends fixable findings in place ([lint](./lint.md)) |
| `clean` | Remove all synced symlinks and empty target directories (does not remove `.agents/`) |
| `fix [type]` | Migrate legacy dirs into `.agents/`, convert flat skill files to directory layout, and repair broken symlinks. Type: any bucket dir, or `all` (default) ([fix](./fix.md)) |
| `promote <type> <name>` | Copy an artifact from the project's `.agents/` to the user-level global store (`~/.agents/`) ([promote](./promote.md)) |
| `global init` | Initialize the global `~/.agents/` store ([global init](./global-init.md)) |
| `global sync` | Fan the global store out to each tool's user-level config dir with semantic-aware routing ([global sync](./global-sync.md)) |
| `global status` | Show per-artifact sync state across global tool dirs ([global status](./global-status.md)) |
| `global clean` | Remove global symlinks/concat files owned by sync-agents ([global clean](./global-clean.md)) |

## Options

| Option | Description |
|---|---|
| `-h`, `--help` | Show help message |
| `-v`, `--version` | Show version |
| `-d`, `--dir <path>` | Set project root directory (default: current directory) |
| `--targets <list>` | Comma-separated list of sync targets (default: `claude,windsurf,cursor,copilot`) |
| `--dry-run` | Show what would be done without making changes |
| `--force` | Overwrite existing files and symlinks |
| `--no-clobber` | (fix only) Skip items that already exist in `.agents/` instead of merging |
| `--fix` | (lint only) Amend fixable frontmatter findings in place |
| `--no-fix` | (index only) Skip the skill frontmatter backfill |
| `--trust` | (pull/update only) Bypass the quarantine gate; the scan still runs and prints findings |

## Common usage

```bash
# Initialize .agents/ structure in the current project
sync-agents init

# Add a new rule / skill / workflow
sync-agents add rule no-eval
sync-agents add skill debugging
sync-agents add workflow deploy

# Add a Claude subagent / a plan / a spec (creates the bucket on demand)
sync-agents add agent reviewer
sync-agents add plan q3-roadmap
sync-agents add spec sso-login

# Validate + upgrade skill frontmatter
sync-agents lint
sync-agents lint --fix

# Sync to all targets
sync-agents sync

# Sync to a specific target only
sync-agents sync --targets claude

# Preview sync without making changes
sync-agents sync --dry-run

# Force overwrite existing symlinks
sync-agents sync --force

# Check sync status
sync-agents status

# Regenerate the AGENTS.md index
sync-agents index

# Remove all synced symlinks
sync-agents clean

# Fix legacy layouts and broken symlinks
sync-agents fix

# Work in a different directory
sync-agents sync --dir /path/to/project
```

## Importing single artifacts

The [`examples/`](../../examples/) directory contains ready-to-use
rules, skills, and workflows. Import them directly:

```bash
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/rules/no-secrets.md
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/skills/code-review.md
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/workflows/pr-checklist.md
```

See [examples/README.md](../../examples/README.md) for the full list.
For reproducible, SHA-pinned installs prefer the
[source manifest](../sources.md) over one-shot `import`.

## See also

- [Topology & configuration](../topology.md)
- [Sources, lockfile & provenance](../sources.md)
- [Docs index](../README.md)
