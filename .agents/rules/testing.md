---
trigger: always_on
---

# Testing Standards

## Core Principle
Tests are not optional. Code without tests is incomplete code. Run all tests after every change. Identify missing tests and write them.

## What to Test

### Happy Path
- Core functionality works as designed
- Normal inputs produce expected outputs
- Primary user flows complete successfully

### Edge Cases (Critical)
- Empty inputs, null values, undefined
- Boundary conditions (min/max, zero, negative, one-off)
- Very large inputs, very small inputs
- Special characters, unicode, whitespace
- Concurrent access, simultaneous operations
- Timeouts, retries, network failures
- Invalid state transitions
- Partial failures, degraded modes

### Race Conditions (High Priority)
- Shared mutable state access
- Concurrent read/write operations
- Async operations completing out of order
- Signal handling, interrupts
- Database transactions, locks
- Distributed system coordination

### Error Paths
- Invalid inputs rejected gracefully
- Error messages are clear and actionable
- Resources cleaned up on failure
- Partial failures don't corrupt state
- Recovery paths work correctly

## Test Design
- **One assertion per test** when possible — tests should verify one behavior
- **Descriptive names** — test names describe the scenario and expected outcome
- **Independent tests** — no test depends on another test's state or execution order
- **Fast feedback** — unit tests run in seconds, integration tests in minutes
- **Deterministic** — no flaky tests; if it depends on timing, mock it

## Coverage Targets
- **100% of new code** gets tests before merging
- **Missing tests** are bugs — track them in tasks/todo.md
- **Regression tests** for every bug fix — the fix is incomplete without a test that would have caught it

## Execution
- Run the full test suite after every change
- Fix failures immediately — don't merge with broken tests
- If a test is flaky, fix or disable it before merging
- Measure coverage, but don't obsess over numbers — coverage measures what's tested, not quality

## When to Write Tests
- **Before** fixing a bug: write a failing test that demonstrates the bug
- **During** feature development: write tests as you implement, not after
- **After** discovering missing coverage: add tests immediately

## Anti-patterns to Avoid
- Testing implementation details instead of behavior
- Mocking everything (you're not testing anything)
- Testing only the happy path
- Skipping "unimportant" edge cases
- Writing tests that pass without actually verifying the behavior
- Ignoring flaky tests instead of fixing them
