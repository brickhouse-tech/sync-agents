---
trigger: always_on
---

# comments and documentation

This project favors **thorough inline documentation** and a populated
out-of-tree `docs/` folder over terseness. The tool's purpose is to ship
rules to AI agents; the codebase has to be legible to those same agents
when they read it later. Consistency is the point.

This rule overrides any general "default to no comments" guidance from
agent-level system prompts when working inside this repository.

## Standards

### Go code

- **Every exported symbol** (type, function, method, variable, constant)
  carries a doc comment per Go convention. The comment starts with the
  symbol's name and describes what it is and what it's for — not what
  the next line does.
- **Every package has a `doc.go`** with a package-level comment
  explaining the package's purpose, the major types it exports, and how
  callers should compose them. Place the `// Package <name> ...` comment
  in `doc.go`, not scattered across other files.
- **Non-obvious decisions** (precedence orders, edge-case routing,
  retry policies, idempotency guarantees, "we do X here because Y") get
  an explanatory paragraph as a comment in the relevant function or
  next to the relevant constant. If the decision was made in a SPEC,
  reference the SPEC ID and section. Example:

  ```go
  // ResolveGlobalRoot returns the canonical path of the user's global
  // .agents tree. Precedence per SPEC-002 §Configurable global root:
  //
  //   1. App.GlobalRoot field (set programmatically, used by tests)
  //   2. $SYNC_AGENTS_GLOBAL_ROOT env var
  //   3. $HOME/.agents
  //
  // The returned path is always absolute. Callers that need the parent
  // directory (to compute per-tool dirs like ~/.claude/) should use
  // filepath.Dir on this result.
  func (a *App) ResolveGlobalRoot() string { ... }
  ```

- **Internal (unexported) helpers** may go uncommented when their
  behavior is obvious from a well-chosen signature. When in doubt,
  write a one-liner. The bar to omit a comment is "a contributor
  reading this file cold understands it in <5 seconds without scrolling."

### docs/ folder

- `docs/README.md` is the index. Every doc file in the tree is listed
  there with a one-line description.
- `docs/architecture/` contains conceptual pieces — how the local↔global
  model works, why a decision was made, what the constraint is.
- `docs/commands/` contains one file per CLI command when the README's
  command table gets too thin (more than ~one paragraph of behavior to
  explain).
- `docs/internal/` contains pointers into the code for contributors
  ("where is X implemented?", "the call path for `sync`").
- **Every doc file** starts with a one-sentence purpose line under the
  title, and ends with a `## See also` section linking related docs and
  the relevant SPEC IDs.

### Spec references

- When code implements a SPEC requirement, reference it by ID in the
  comment near the implementation. This gives future readers (humans
  and agents) a stable pointer back to the decision context.
- When a doc explains a behavior that was specified, link to the
  SPEC file at the bottom (`specs/SPEC-XXX-name.md`).

### Markdown rules and skills

- Rules under `.agents/rules/` keep their existing form (frontmatter +
  prose). When a rule encodes a non-obvious convention, include a
  "Why" paragraph so future readers understand the constraint rather
  than just the rule.

## What NOT to comment

These remain off-limits even under this rule's "more is better" stance:

- **Don't restate what the code obviously does.** `// increments i` on
  `i++` is noise.
- **Don't include planning notes** (`// TODO: refactor later`,
  `// FIXME(nick): clean this up`) in code comments. Open a GitHub
  issue or add to a SPEC's Open Questions section.
- **Don't reference the current task or PR** (`// added for issue #42`,
  `// see PR #17`) — those belong in commit messages and rot fast.
- **Don't write multi-paragraph docstrings on trivial helpers.** A
  three-line wrapper does not need a three-paragraph comment.

## Why

The project's deliverable is *agent instructions*. Two consequences:

1. Contributors who arrive after model rollovers (Opus 4.7 → 5.0,
   Sonnet 4.6 → 5.0, etc.) need the codebase to be self-explanatory.
   Models change; the docs persist.
2. The tool's primary users are AI agents that read code during their
   work. A well-documented codebase teaches those agents the project's
   conventions automatically.

The cost is ~20-30% more bytes per file. The benefit is durable
intelligibility across model generations and contributor turnover.
