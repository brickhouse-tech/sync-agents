# Examples

Ready-to-use rules, skills, and workflows for `sync-agents`.

## Quick Import

```bash
# Import directly from GitHub
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/rules/no-secrets.md
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/skills/code-review.md
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/workflows/pr-checklist.md
```

## Contents

### Rules
| File | Description |
|---|---|
| [no-secrets](rules/no-secrets.md) | Prevent committing API keys, tokens, and credentials |
| [commit-conventions](rules/commit-conventions.md) | Enforce Conventional Commits format |
| [no-force-push](rules/no-force-push.md) | Protect shared branches from history rewrites |

### Skills
| File | Description |
|---|---|
| [code-review](skills/code-review.md) | Systematic PR review process |
| [debugging](skills/debugging.md) | Structured approach to diagnosing issues |

### Workflows
| File | Description |
|---|---|
| [pr-checklist](workflows/pr-checklist.md) | Pre-flight checklist before opening a PR |

## Contributing

Add your own examples via PR! Follow the templates in `src/md/` for consistent formatting.
