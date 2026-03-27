---
trigger: always_on
---

# PR Checklist

Steps to complete before opening a pull request.

## Before Opening

- [ ] Code compiles/builds without errors
- [ ] All existing tests pass (`npm test`, `go test ./...`, etc.)
- [ ] New tests added for new functionality
- [ ] Linter passes with no new warnings
- [ ] No secrets, credentials, or PII in the diff
- [ ] Commit messages follow project conventions
- [ ] Branch is up to date with the target branch (rebase or merge)
- [ ] Self-reviewed the diff — read every line as if reviewing someone else's code

## PR Description

- [ ] Clear title summarizing the change
- [ ] Link to related issue(s)
- [ ] Explain *what* changed and *why*
- [ ] List any breaking changes
- [ ] Include screenshots/recordings for UI changes
- [ ] Note deployment steps if applicable

## After Opening

- [ ] CI passes (all checks green)
- [ ] Request review from appropriate team members
- [ ] Respond to review comments within 24 hours
- [ ] Squash or clean up commits if requested
- [ ] Delete the branch after merge
