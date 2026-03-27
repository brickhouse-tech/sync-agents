---
trigger: always_on
---

# Code Review

Systematic approach to reviewing pull requests.

## Process

1. **Understand context** — Read the PR description, linked issues, and related docs before looking at code
2. **Check scope** — Does the PR do one thing? Flag scope creep early
3. **Read tests first** — Tests reveal intent; if there are no tests, that's the first comment
4. **Review logic** — Walk through the code path as if you're the runtime
5. **Check edge cases** — Empty inputs, nulls, large datasets, concurrent access, error paths
6. **Verify naming** — Variables, functions, and files should be self-documenting
7. **Security scan** — Look for injection, auth bypass, data exposure, unsafe deserialization
8. **Performance** — N+1 queries, unbounded loops, missing pagination, large allocations

## Comment Style

- **Blocking:** Prefix with `[blocking]` — must fix before merge
- **Suggestion:** Prefix with `[nit]` or `[suggestion]` — take it or leave it
- **Question:** Prefix with `[question]` — clarify intent, not a change request
- Be specific: link to the line, show the fix, explain why

## Approve When

- All blocking comments are resolved
- Tests pass and cover the change
- No security concerns
- Code is readable by someone who didn't write it
