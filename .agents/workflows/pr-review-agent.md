# PR Review Agent Workflow

## Description
Automated code review agent that runs on every pull request. Performs comprehensive analysis and posts all feedback as PR comments. Operates in a secure GitHub Actions environment.

## Trigger
- Pull request opened, synchronized, or reopened
- Manual workflow dispatch

## Environment
- GitHub Actions with pull-requests:write permission
- Secure secrets access (if needed)
- Read-only repository access for analysis

## Process

### 1. Setup
- Checkout the PR branch
- Identify changed files (diff from base branch)
- Configure review scope (exclude generated files, test data, etc.)

### 2. Analysis
For each changed file:
- **Correctness**: Logic errors, off-by-one, null handling, type mismatches
- **Security**: SQL injection, XSS, hardcoded secrets, improper validation
- **Performance**: N+1 queries, missing indexes, unnecessary computation, memory leaks
- **Maintainability**: Code clarity, naming, comments, complexity
- **Testing**: Missing tests, inadequate coverage, flaky test patterns
- **Error handling**: Empty catch blocks, swallowed errors, unclear error messages

### 3. Contextual Review
- Check for backwards compatibility issues
- Verify error handling matches failure severity
- Look for race conditions in concurrent code
- Flag magic numbers, hardcoded values
- Identify copy-pasted code that should be refactored
- Check for TODO/FIXME without tracking

### 4. Output Format
All feedback posted as PR comments:

#### General Comments
- Summary of review findings
- Overall assessment (approve, request changes, comment)
- High-level architectural concerns

#### Line Comments
- Specific issues with line references
- Severity: Critical / Warning / Suggestion / Nit
- Explanation of why it's a problem
- Suggested fix (code snippet when possible)

#### Inline Suggestions
- GitHub suggestion blocks for simple fixes
- One-click apply for minor improvements

### 5. Severity Levels
- **Critical**: Must fix before merge (bugs, security issues, data loss)
- **Warning**: Should fix (performance issues, maintainability concerns)
- **Suggestion**: Nice to have (style improvements, alternative approaches)
- **Nit**: Optional (minor style preferences, cosmetic issues)

### 6. Exclusions
Skip review for:
- Generated files (package-lock.json, dist/, build/)
- Test data/fixtures
- Documentation-only changes (unless they contain code examples)
- Files matching patterns in `.reviewignore` (if present)

## Implementation

### GitHub Actions Workflow
```yaml
name: PR Code Review
on:
  pull_request:
    types: [opened, synchronize, reopened]
  workflow_dispatch:

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Get changed files
        id: changed
        run: |
          echo "files=$(git diff --name-only origin/${{ github.base_ref }}...HEAD | tr '\n' ' ')" >> $GITHUB_OUTPUT
      
      - name: Run code review
        uses: AI code review action or custom script
        with:
          files: ${{ steps.changed.outputs.files }}
          # Configuration for review scope
          
      - name: Post review comments
        # Use GitHub API to post comments
        # Format: line-level comments with severity tags
```

### Alternative: Custom Script
If building a custom review agent:
- Use GitHub REST API for PR comments
- Parse diff to identify line numbers
- Post comments with `position` field for inline feedback
- Use `commit_id` to lock comments to specific commit

## Configuration
Repositories can configure review behavior via `.github/review-config.yml`:
```yaml
exclude:
  - "package-lock.json"
  - "yarn.lock"
  - "**/*.min.js"
  - "dist/**"

severity_threshold: warning  # only post warnings and above

custom_rules:
  - name: "No console.log"
    pattern: "console\\.log\\("
    severity: warning
    message: "Remove console.log before merging"
```

## Integration with Testing
- After review, run test suite
- Review findings may indicate missing tests
- Suggest test additions as part of review
- Link review comments to test coverage reports

## Benefits
- Consistent review quality across all PRs
- Catches issues early, before human reviewers
- Reduces review burden on team members
- Provides instant feedback to PR authors
- Maintains code quality standards
- Scales with team size and PR volume

## Limitations
- Cannot replace human architectural review
- May generate false positives (tune exclusions)
- Requires GitHub Actions or CI/CD infrastructure
- Needs maintenance as codebase evolves
