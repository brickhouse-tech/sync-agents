# Scope and target directories

How `sync-agents` reasons about *where things live*: the two scopes
(project and user) and the per-tool target directories that fan out
from each scope's canonical `.agents/` tree.

## The two scopes

`sync-agents` operates on artifacts (rules, skills, workflows) at one
of two scopes:

| Scope | Canonical tree | Per-tool fan-out |
|---|---|---|
| **Local** (project) | `<project>/.agents/` | `<project>/.claude/`, `<project>/.windsurf/`, `<project>/.cursor/`, `<project>/.github/copilot/`, `<project>/.codex/` |
| **Global** (user) | `$HOME/.agents/` (overridable, see [global-root-resolution.md](./global-root-resolution.md)) | `$HOME/.claude/`, `$HOME/.codeium/`, `$HOME/.cursor/`, `$HOME/.github/copilot/`, `$HOME/.codex/` |

The canonical tree is the **single source of truth**. The per-tool
directories are derived from it: `sync` creates symlinks that point
back at the canonical tree.

## The asymmetry: `.windsurf` vs `.codeium`

Most tools use the same directory name at both scopes — Claude is
`.claude/` whether it's `<project>/.claude/` or `~/.claude/`. Windsurf
is the exception:

- **Project scope:** `.windsurf/` (this is Windsurf-the-IDE's
  per-project config directory)
- **User scope:** `~/.codeium/` (Windsurf is built by Codeium; the
  user-level config lives under the company name)

`sync-agents` hides this asymmetry from the caller. A `Tool` value
named `codeium` carries a per-scope directory map that resolves to
`.windsurf` for local and `.codeium` for global. Callers ask "what's
the directory for tool X at scope Y" and get back the right path
without having to remember the rule.

See `internal/agent/tool.go` for the `Tool` type and the registry of
known tools.

## Per-tool directories

| Tool | Local dir | Global dir |
|---|---|---|
| `claude` | `.claude/` | `~/.claude/` |
| `codeium` | `.windsurf/` | `~/.codeium/` |
| `cursor` | `.cursor/` | `~/.cursor/` |
| `copilot` | `.github/copilot/` | `~/.github/copilot/` |
| `codex` | `.codex/` | `~/.codex/` |

`copilot` is the other shape-changer: it lives nested under `.github/`
rather than as a top-level dot-dir.

## How callers compose scope + tool

```go
// Resolve the per-tool directory for tool "codeium" at global scope:
dir := tool.DirForScope(agent.ScopeGlobal, app.ResolveGlobalRoot())
// dir == "/Users/tars/.codeium"

// Same tool, local scope:
dir = tool.DirForScope(agent.ScopeLocal, app.ProjectRoot)
// dir == "/Users/tars/my-project/.windsurf"
```

`ResolveGlobalRoot()` and the per-tool dir method are the two pieces
that make scope/target composition uniform. Callers should not build
these paths by hand — that's how the windsurf/codeium asymmetry leaks.

## Why this matters

Without a typed scope + tool model:

- Every command that touches a target directory has to remember the
  asymmetry rules.
- New tools (Codex added in SPEC-002) require finding every site that
  hardcodes a name like `"claude"`.
- Tests that want to redirect the global tree (to a `t.TempDir()` so
  they don't write to the real `$HOME`) have to monkey-patch
  filesystem helpers.

With it, all three concerns flow through `Tool.DirForScope` and
`ResolveGlobalRoot()`, and adding a new tool means adding one entry to
the registry.

## See also

- [Global root resolution](./global-root-resolution.md) — the
  precedence chain that determines the global canonical path.
- [SPEC-002](../../specs/SPEC-002-promote-global-sync.md) — the spec
  that introduced global scope, the `Tool.DirByScope` model, and the
  semantic-aware routing layer that builds on top of it.
- `internal/agent/scope.go`, `internal/agent/tool.go`,
  `internal/agent/globalroot.go` — the code.
