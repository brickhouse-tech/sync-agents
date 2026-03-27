# sync-agents

One set of agent rules to rule them all. `sync-agents` keeps your AI coding agent configurations in a single `.agents/` directory and syncs them to agent-specific directories (`.claude/`, `.windsurf/`, `.cursor/`, `.github/copilot/`) via symlinks. This ensures all agents follow the same rules, skills, and workflows without duplicating files.

AGENTS.md serves as an auto-generated index of everything in `.agents/` and is symlinked to CLAUDE.md for Claude compatibility.

## Installation

### npm

```bash
npm install @brickhouse-tech/sync-agents
```

or globally:

```bash
npm install -g @brickhouse-tech/sync-agents
```

### Standalone (no npm required)

```bash
curl -fsSL https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/src/sh/sync-agents.sh -o /usr/local/bin/sync-agents
chmod +x /usr/local/bin/sync-agents
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

Running `sync-agents sync` creates symlinks from `.agents/` subdirectories into `.claude/`, `.windsurf/`, `.cursor/`, and `.github/copilot/`. Any changes to `.agents/` are automatically reflected in the target directories because they are symlinks, not copies.

AGENTS.md is also symlinked to CLAUDE.md so that Claude reads the index natively.

## STATE.md

`.agents/STATE.md` tracks the current state of your project from the agent's perspective. It serves as a resumption point after failures or interruptions -- the agent can read STATE.md to determine where it left off and what tasks remain. Update it regularly to keep agents in sync with progress.

## Commands

| Command | Description |
|---|---|
| `init` | Initialize the `.agents/` directory structure with `rules/`, `skills/`, `workflows/`, `STATE.md`, and generate `AGENTS.md` |
| `sync` | Create symlinks from `.agents/` into all target directories, and symlink `AGENTS.md` to `CLAUDE.md` |
| `watch` | Watch `.agents/` for changes and auto-regenerate `AGENTS.md` |
| `import <url>` | Import a rule/skill/workflow from a URL |
| `hook` | Install a pre-commit git hook for auto-sync |
| `inherit <label> <path>` | Add an inheritance link to AGENTS.md |
| `inherit --list` | List current inheritance links |
| `inherit --remove <label>` | Remove an inheritance link by label |
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
| `--targets <list>` | Comma-separated list of sync targets (default: `claude,windsurf,cursor,copilot`) |
| `--dry-run` | Show what would be done without making changes |
| `--force` | Overwrite existing files and symlinks |

## Inheritance

Projects can inherit agent rules from parent directories (org, team, global) using a convention-based approach. This enables hierarchical rule sharing without duplicating files.

### How It Works

Add an `## Inherits` section to your project's `AGENTS.md` that links to parent-level agent configs:

```markdown
## Inherits
- [global](../../AGENTS.md)
- [team](../AGENTS.md)
```

AI agents (Claude, Codex, etc.) follow markdown links natively — when they read your project's `AGENTS.md`, they'll traverse the inheritance chain and apply rules from all levels.

### Hierarchy Example

```
~/code/                     # Global: security norms, universal rules
  ├── .agents/
  ├── AGENTS.md
  └── org/                  # Org-level: coding standards, shared workflows
      ├── .agents/
      ├── AGENTS.md
      └── team/             # Team-level: language-specific rules
          ├── .agents/
          ├── AGENTS.md
          └── project/      # Project: project-specific rules + inherits
              ├── .agents/
              └── AGENTS.md  → ## Inherits links to team, org, global
```

**Inheritance is upward-only.** A project declares what it inherits from. Parent directories don't need to know about their children — when an agent works at the org level, it already has access to org-level rules.

### Managing Inheritance

```bash
# Add an inheritance link
sync-agents inherit global ../../AGENTS.md
sync-agents inherit team ../AGENTS.md

# List current inheritance links
sync-agents inherit --list

# Remove an inheritance link
sync-agents inherit --remove global
```

The `## Inherits` section is preserved across `sync-agents index` regenerations.

### Full Example

Set up a three-level hierarchy: global rules → org standards → project config.

```bash
# 1. Create global rules (e.g. ~/code/.agents/)
cd ~/code
sync-agents init
sync-agents add rule security
cat > .agents/rules/security.md << 'EOF'
---
trigger: always_on
---
# Security
- Never commit secrets or API keys
- Validate all external input
- Use parameterized queries for database access
EOF

# 2. Create org-level rules (e.g. ~/code/myorg/.agents/)
cd ~/code/myorg
sync-agents init
sync-agents add rule go-standards
cat > .agents/rules/go-standards.md << 'EOF'
---
trigger: always_on
---
# Go Standards
- Use `gofmt` and `golangci-lint` on all Go files
- Prefer table-driven tests
- Export only what consumers need
EOF

# 3. Create project with inheritance
cd ~/code/myorg/api-service
sync-agents init
sync-agents add rule api-conventions

# Link to parent levels
sync-agents inherit org ../AGENTS.md
sync-agents inherit global ../../AGENTS.md

# Sync to agent directories
sync-agents sync
```

The project's `AGENTS.md` now looks like:

```markdown
## Inherits
- [org](../AGENTS.md)
- [global](../../AGENTS.md)

## Rules
- [api-conventions](.agents/rules/api-conventions.md)

## Skills
_No skills defined yet._

## Workflows
_No workflows defined yet._
```

When an AI agent reads this file, it follows the `## Inherits` links and applies rules from all three levels — project-specific API conventions, org-wide Go standards, and global security rules.

### Verifying Inheritance

```bash
# Check what's inherited
sync-agents inherit --list
# Output:
# - [org](../AGENTS.md)
# - [global](../../AGENTS.md)

# Remove a link if no longer needed
sync-agents inherit --remove global

# Re-add with a different path
sync-agents inherit global ../../AGENTS.md
```

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
