---
trigger: always_on
---

# Code Integrity

## Purpose
Code should be clean, intentional, and ready for review. Remove anything that doesn't belong. Find problems before they reach production.

## What to Look For

### Dead Code
- Functions/methods that are never called
- Unreachable code paths (after returns/throws)
- Unused imports, variables, parameters
- Commented-out code blocks
- Deprecated code still present
- Feature flags for removed features

### Comment Hygiene
- TODO/FIXME/HACK comments → either fix now or create tracked task
- Outdated comments that contradict the code
- Debugging comments (`console.log("here")`, `// temp fix`)
- Embarrassing comments (`// this is stupid`, `// wtf`)
- Commented-out code with no explanation of why it's kept
- Comments explaining what the code does instead of why

### Code Smells
- Copy-pasted code (extract to shared function)
- Magic numbers/strings (use named constants)
- Overly complex logic (simplify, extract functions)
- Deep nesting (early returns, guard clauses)
- Inconsistent naming conventions
- Mixed abstraction levels in one function

### Inconsistencies
- Naming mismatches (function named `getUser` returns `account` object)
- Comments that describe different behavior than the code implements
- Type mismatches between interface and implementation
- Return types that don't match what callers expect
- Error handling that doesn't match the severity of failures

### Unresolved Items
- TODO/FIXME without corresponding task tracking
- Placeholder implementations (`// implement this later`)
- Stub functions that return hardcoded values
- Incomplete error handling (empty catch blocks)
- Partial implementations marked as "complete"

### Security & Performance
- Hardcoded credentials, API keys, secrets
- SQL injection, XSS vulnerabilities
- Unvalidated user inputs
- N+1 queries, missing indexes
- Unbounded loops, memory leaks
- Missing rate limits, pagination

## Process

### Before Committing
1. Read every line of your changes
2. Remove anything that doesn't belong
3. Update out-of-date comments
4. Resolve or track all TODOs
5. Check for accidental debug output
6. Verify naming consistency

### During Code Review
- Flag any of the above items
- Distinguish between "must fix" and "nice to have"
- Explain why it's a problem, not just that it is
- Suggest a fix, don't just criticize

### Automated Checks
- Use linters to catch dead code, unused imports
- Configure static analysis for security issues
- Run type checkers before committing
- Measure code complexity, flag hotspots

## Mindset
- **Code is communication** — write it for the next reader
- **Remove, don't accumulate** — dead code is liability, not optionality
- **Intentionality** — every line should have a purpose
- **Future self** — will you understand this in 6 months?
