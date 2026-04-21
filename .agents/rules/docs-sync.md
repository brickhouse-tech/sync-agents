---
trigger: on .agents changes
---

# Documentation Sync Rule

Trigger: After adding, modifying, or deleting files in `.agents/`, or updating sync-agents configuration and implementation

Purpose:
- Keep documentation and examples synchronized with sync-agents configuration and implementation details
- Ensure users have current, accurate guidance on using the tool and its features
- Maintain consistency across README.md, examples, and .agents/ directory structure

## Areas to Keep in Sync

### README.md
- Update installation instructions if CLI commands or setup process changes
- Update topology section if .agents/ directory structure changes
- Add/update usage examples if new commands or workflows are added
- Update feature descriptions if core functionality changes
- Keep troubleshooting section current with common issues

### examples/ Directory
- Update example rules, skills, and workflows if standards or formats change
- Refresh examples in `examples/rules/`, `examples/skills/`, `examples/workflows/` when new patterns emerge
- Keep example fixture files aligned with current implementation
- Update example README.md with current best practices and use cases

### .agents/ Directory Structure
- Ensure AGENTS.md is up-to-date by running `sync-agents index` when rules/skills/workflows are added or removed
- Keep STATE.md current with any configuration state changes
- Document any new file types or structure changes

### src/ Documentation
- Keep `src/md/` templates (RULE_TEMPLATE.md, SKILL_TEMPLATE.md, etc.) aligned with actual implementation
- Update template frontmatter if new trigger types or fields are introduced
- Ensure template examples reflect current best practices

## When to Apply

Apply this rule when:
- Adding a new rule, skill, or workflow to `.agents/`
- Modifying the .agents/ directory structure
- Updating sync-agents CLI commands or behavior
- Changing rule/skill/workflow file format or frontmatter
- Implementing new features or breaking changes

## Checklist

Before committing changes to `.agents/` or core implementation:
- [ ] Run `sync-agents index` to regenerate AGENTS.md if structure changed
- [ ] Review README.md for outdated commands or topology descriptions
- [ ] Check examples/ directory for alignment with new patterns
- [ ] Verify template files in src/md/ match current format standards
- [ ] Update CHANGELOG.md with significant changes
- [ ] Ensure docstrings and comments in src/sh/sync-agents.sh are current
- [ ] Test examples work with current implementation

## Guidelines

- Documentation should be a single source of truth; avoid duplicating information
- Use clear, actionable language in examples
- Include trigger types and context for each rule/skill/workflow
- Keep template files as the authoritative format reference
- Link to relevant sections rather than duplicating content
