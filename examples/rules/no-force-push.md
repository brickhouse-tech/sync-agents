---
trigger: always_on
---

# No Force Push

Protect shared branches from history rewrites.

## Rules

- Never `git push --force` to `main`, `master`, `develop`, or any release branch
- Use `--force-with-lease` on feature branches only, and only when you own the branch
- If a rebase is needed on a shared branch, coordinate with the team first
- Prefer merge commits or squash merges over rebasing shared history
- If you accidentally force-pushed a shared branch, notify the team immediately and restore from reflog
