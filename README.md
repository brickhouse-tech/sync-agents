# sync-agents

One set of agent rules to rule them all. `sync-agents` keeps your AI coding agent configurations in a single `.agents/` directory and syncs them to agent-specific directories (`.claude/`, `.windsurf/`) via symlinks. This ensures all agents follow the same rules, skills, and workflows without duplicating files.

AGENTS.md serves as an auto-generated index of everything in `.agents/` and is symlinked to CLAUDE.md for Claude compatibility.

## Installation

```bash
npm install @brickhouse-tech/sync-agents
```

or globally:

```bash
npm install -g @brickhouse-tech/sync-agents
```

## Topology

`.agents/` is the source of truth. It contains all rules, skills, workflows, and state for your agents:

```
.agents/
  ├── rules/
  │   ├── rule1.md
  │   ├── rule2.md
  │   └── ...
  ├── skills/
  │   ├── skill1.md
  │   ├── skill2.md
  │   └── ...
  ├── workflows/
  │   ├── workflow1.md
  │   ├── workflow2.md
  │   └── ...
  └── STATE.md
```

Running `sync-agents sync` creates symlinks from `.agents/` subdirectories into `.claude/` and `.windsurf/`. Any changes to `.agents/` are automatically reflected in the target directories because they are symlinks, not copies.

AGENTS.md is also symlinked to CLAUDE.md so that Claude reads the index natively.

## STATE.md

`.agents/STATE.md` tracks the current state of your project from the agent's perspective. It serves as a resumption point after failures or interruptions -- the agent can read STATE.md to determine where it left off and what tasks remain. Update it regularly to keep agents in sync with progress.

## Commands

| Command | Description |
|---|---|
| `init` | Initialize the `.agents/` directory structure with `rules/`, `skills/`, `workflows/`, `STATE.md`, and generate `AGENTS.md` |
| `sync` | Create symlinks from `.agents/` into `.claude/` and `.windsurf/`, and symlink `AGENTS.md` to `CLAUDE.md` |
| `status` | Show the current sync status of all targets and symlinks |
| `add <type> <name>` | Add a new rule, skill, or workflow from a template (type is `rule`, `skill`, or `workflow`) |
| `index` | Regenerate `AGENTS.md` by scanning the contents of `.agents/` |
| `clean` | Remove all synced symlinks and empty target directories (does not remove `.agents/`) |

## Options

| Option | Description |
|---|---|
| `-h`, `--help` | Show help message |
| `-v`, `--version` | Show version |
| `-d`, `--dir <path>` | Set project root directory (default: current directory) |
| `--targets <list>` | Comma-separated list of sync targets (default: `claude,windsurf`) |
| `--dry-run` | Show what would be done without making changes |
| `--force` | Overwrite existing files and symlinks |

## Usage

```bash
# Initialize .agents/ structure in the current project
sync-agents init

# Add a new rule
sync-agents add rule no-eval

# Add a new skill
sync-agents add skill debugging

# Add a new workflow
sync-agents add workflow deploy

# Sync to all targets (.claude/ and .windsurf/)
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

# Work in a different directory
sync-agents sync --dir /path/to/project
```
