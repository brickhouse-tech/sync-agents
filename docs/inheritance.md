# Inheritance

Convention-based hierarchical rule sharing: projects link upward to team, org, and global `AGENTS.md` files instead of duplicating rules.

## How it works

Add an `## Inherits` section to your project's `AGENTS.md` that links
to parent-level agent configs:

```markdown
## Inherits
- [global](../../AGENTS.md)
- [team](../AGENTS.md)
```

AI agents (Claude, Codex, etc.) follow markdown links natively — when
they read your project's `AGENTS.md`, they'll traverse the inheritance
chain and apply rules from all levels.

## Hierarchy example

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

**Inheritance is upward-only.** A project declares what it inherits
from. Parent directories don't need to know about their children — when
an agent works at the org level, it already has access to org-level
rules.

## Managing inheritance

```bash
# Add an inheritance link
sync-agents inherit global ../../AGENTS.md
sync-agents inherit team ../AGENTS.md

# List current inheritance links
sync-agents inherit --list

# Remove an inheritance link
sync-agents inherit --remove global
```

The `## Inherits` section is preserved across `sync-agents index`
regenerations.

## Full example

Set up a three-level hierarchy: global rules → org standards → project
config.

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

When an AI agent reads this file, it follows the `## Inherits` links
and applies rules from all three levels — project-specific API
conventions, org-wide Go standards, and global security rules.

## Verifying inheritance

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

## See also

- [`sync-agents index`](./commands/index.md)
- [Global root resolution](./architecture/global-root-resolution.md)
- [`sync-agents promote`](./commands/promote.md) — the other way to
  share rules across projects (via the user-level `~/.agents/`)
