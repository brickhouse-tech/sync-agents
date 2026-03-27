---
trigger: always_on
---

# Debugging

Structured approach to diagnosing and resolving issues.

## Process

1. **Reproduce** — Can you trigger the bug reliably? Define exact steps, inputs, and environment
2. **Isolate** — Narrow the scope. Binary search: comment out half the code, which half breaks?
3. **Read the error** — Stack traces, logs, error codes. The answer is often in the message
4. **Check recent changes** — `git log --oneline -20` and `git diff` against last known working state
5. **Form a hypothesis** — State what you think is wrong *before* changing code
6. **Test the hypothesis** — One change at a time. If it didn't fix it, revert before trying the next thing
7. **Verify the fix** — Reproduce the original steps. Confirm the bug is gone. Check for regressions
8. **Document** — Add a test that would have caught it. Update comments if the code was misleading

## Anti-Patterns

- Shotgun debugging: changing multiple things at once
- Print-statement overload without a hypothesis
- Fixing symptoms instead of root causes
- "It works on my machine" without checking environment differences
- Ignoring warnings and deprecation notices

## Tools

- **Logs:** Check application logs, system logs, browser console
- **Debugger:** Step through with breakpoints rather than guessing
- **Git bisect:** `git bisect start` → `git bisect bad` → `git bisect good <sha>` to find the breaking commit
- **Minimal reproduction:** Strip the problem to the smallest possible case
