---
trigger: always_on
---

# Commit Conventions

All commits must follow [Conventional Commits](https://www.conventionalcommits.org/).

## Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

## Types

| Type | When to use |
|---|---|
| `feat` | New feature or capability |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `style` | Formatting, whitespace (no logic change) |
| `refactor` | Code restructuring (no feature or fix) |
| `perf` | Performance improvement |
| `test` | Adding or updating tests |
| `build` | Build system or dependencies |
| `ci` | CI/CD configuration |
| `chore` | Maintenance tasks |

## Rules

- Subject line: imperative mood, lowercase, no period, max 72 characters
- Body: wrap at 72 characters, explain *what* and *why* (not *how*)
- Breaking changes: add `BREAKING CHANGE:` in footer or `!` after type
- Reference issues: `Fixes #123` or `Closes #456` in footer
- One logical change per commit — don't mix features with refactors
