# Scope and target directories

How `sync-agents` reasons about *where things live*: the two scopes
(project and user) and the per-tool target directories that fan out
from each scope's canonical `.agents/` tree.

## The two scopes

`sync-agents` operates on artifacts (rules, skills, workflows) at one
of two scopes:

| Scope | Canonical tree | Per-tool fan-out |
|---|---|---|
| **Local** (project) | `<project>/.agents/` | `<project>/.claude/`, `<project>/.devin/`, `<project>/.cursor/`, `<project>/.github/copilot/`, `<project>/.codex/`, `<project>/.opencode/`, `<project>/.cline/`, `<project>/.gemini/` |
| **Global** (user) | `$HOME/.agents/` (overridable, see [global-root-resolution.md](./global-root-resolution.md)) | `$HOME/.claude/`, `$HOME/.config/devin/`, `$HOME/.cursor/`, `$HOME/.copilot/`, `$HOME/.codex/`, `$HOME/.config/opencode/`, `$HOME/.cline/`, `$HOME/.gemini/config/`, `$HOME/.gemini/` |

The canonical tree is the **single source of truth**. The per-tool
directories are derived from it: `sync` creates symlinks that point
back at the canonical tree.

## The asymmetry: `.devin` vs `.config/devin`

Most tools use the same directory name at both scopes. Claude is
`.claude/` in `<project>/.claude/` and in `~/.claude/`. Devin is an
exception:

- **Project scope:** `.devin/`, the directory Devin Local and Devin CLI
  read in a repository.
- **User scope:** `~/.config/devin/`, the XDG home that Devin CLI
  documents and Devin Local shares.

`sync-agents` hides this asymmetry from the caller. The `devin` tool
carries a per-scope directory map that resolves to `.devin` for local
and `.config/devin` for global. Callers ask for the directory of tool X
at scope Y and get the right path.

See `internal/agent/tool.go` for the `Tool` type and the registry of
known tools.

## Per-tool directories

| Tool | Local dir | Global dir |
|---|---|---|
| `claude` | `.claude/` | `~/.claude/` |
| `devin` | `.devin/` | `~/.config/devin/` |
| `cursor` | `.cursor/` | `~/.cursor/` |
| `copilot` | `.github/copilot/` | `~/.copilot/` |
| `codex` | `.codex/` | `~/.codex/` |
| `opencode` | `.opencode/` | `~/.config/opencode/` |
| `cline` | `.cline/` | `$CLINE_DIR`, default `~/.cline/` |
| `openclaw` | none (global only) | resolved OpenClaw workspace, default `~/.openclaw/workspace/` |
| `antigravity` | none (reads `.agents/` natively) | `~/.gemini/config/` |
| `gemini` (opt-in) | `.gemini/` | `~/.gemini/` |
| `codeium` (retired) | `.windsurf/` | `~/.codeium/` |

## Retired tools and deprecated names

Windsurf became Devin Desktop on 2026-06-02 (SPEC-014). The target
names `windsurf` and `codeium` are deprecated. A target list from
`--targets` or a `targets =` line that names either one syncs `devin`
and prints one warning to stderr per run:

```text
warning: target "windsurf" is deprecated; Windsurf is now Devin. Use "devin". Removed in 5.0.0.
```

A list that names `devin` and a deprecated name syncs `devin` once.
The names live in one table, `deprecatedTargets` in
`internal/agent/tool.go`. The two target boundaries read it:
`resolveProjectTargets` for project commands and `resolveSyncTools`
for global commands.

The `codeium` tool stays in the registry as a retired tool. No target
name selects it, and the default global target list skips it. Its
entries still describe what earlier versions wrote: the
`.windsurf/<bucket>` folds (or the per-entry links drilled into a real
`.windsurf/<bucket>/`), the `codeium-rules` region in
`~/.codeium/windsurf/memories/global_rules.md`, and the workflow links
in `~/.codeium/windsurf/global_workflows/`. Sync leaves these in place
during 4.x because legacy Cascade still reads them. The exception is a
`.windsurf/skills` link into `.agents/skills`: Devin imports it and
reads `.agents/skills` as well, so sync removes it when it targets
`devin`. `status` and
`global status` list each one as `deprecated (removed in 5.0.0)`.
`clean` and `global clean` given `windsurf` or `codeium` clean `devin`
and list the Windsurf placements they left.

`opencode` also differs by scope, by convention rather than by
branding. It uses a plain `.opencode/`
dotdir inside a project, but follows XDG at user scope, so its global
home is `~/.config/opencode/` rather than `~/.opencode/`. Writing to
the latter would produce a directory opencode never reads.

`opencode` is registered but **not** enabled by default. Adding a tool
to the registry makes it available to opt into (via `.agents/config`
or `--targets`); it does not enroll existing users, whose `.opencode/`
directories may be hand-managed.

`cline` (SPEC-016) is registered but not enabled by default either.
`init` and `global init` leave it out of the `targets =` line they
write, and a global tree with no `targets =` line leaves it out too
(`Tool.OptIn`), so no sync creates `~/.cline/` until you name it. Its
user dir follows `$CLINE_DIR` the way Codex follows `$CODEX_HOME`.
Cline reads `AGENTS.md` and `.agents/skills` itself, so it receives only
rules and workflows: folded into `.cline/` in a project, and linked one
by one into `~/.cline/rules/` and `~/.cline/workflows/` for the user.
Cline reads those directories one level deep, so an OS-scoped rule is
linked flat as `<scope>-<name>.md` (see
[Delivery channels](./delivery-channels.md#per-tool-delivery)). In
`$HOME` the project's `.cline/` is the user dir, so local sync leaves it
to `global sync`.

`copilot` is the other shape-changer: at project scope it lives nested
under `.github/` rather than as a top-level dot-dir. At user scope its
home is `~/.copilot/`, whose `instructions/` directory Copilot reads.
2.0 moved it there from `~/.github/copilot/`, which Copilot never read.

`openclaw` is global only and its directory is not a fixed segment.
`Tool.ResolveGlobalDir` computes the workspace from OpenClaw's env vars
and config each run, and the command layer pins the result into
`DirByScope` as an absolute path before routing. A local `sync
--targets openclaw` skips it with a warning rather than creating
`.openclaw/` in the project. Its delivery channel is a region: it
never owns a file, it splices one marker region into the workspace
`AGENTS.md` OpenClaw created, and a plain `global sync` touches it only
after an explicit `--targets openclaw` has put the markers there. See
[global sync § OpenClaw](../commands/global-sync.md#openclaw).

`antigravity` has no project directory (SPEC-015). Antigravity App and
CLI read a project's `.agents/` tree natively, so a local `sync
--targets antigravity` reports `antigravity reads .agents/ natively`
and writes nothing. Its user-scope home is `~/.gemini/config/`, where
global sync links each passive rule into `rules/` (flat: Antigravity
scans only the top level, so `macos/brew` becomes `macos-brew.md`),
each skill into `skills/<name>/`, and each agent into `agents/`.
Invocable rules and workflows are skipped, since Antigravity workflows
are deprecated. `init` and `global init` write `antigravity` in the
default targets.

`gemini` is Gemini CLI, which Google now keeps for enterprise and
paid-API users. It is registered **opt-in** (`Tool.OptIn`): no default
target list names it, including the every-tool default of `global
sync`, because Antigravity CLI also reads `~/.gemini/GEMINI.md` and
would load the global rules twice. `global sync` warns when both are
targeted. Its home `~/.gemini/` contains Antigravity's
`~/.gemini/config/`, so `global clean --targets gemini` skips that
directory.

Antigravity and Devin drop a rule file without a valid `trigger:`
(`always_on`, `model_decision`, `glob`, `manual`), with no warning.
Those two tools carry `Tool.RequiresRuleTrigger`, and sync prints one
warning listing such rules wherever they read rules as files. See
[lint](../commands/lint.md) for E010 and its `--fix`.

The directory is the tool's home. What sync-agents writes inside it
for rule content (a link, a region, or a config entry) is the tool's
delivery channel; see [Delivery channels](./delivery-channels.md).

## How callers compose scope + tool

```go
// Resolve the per-tool directory for tool "devin" at global scope:
dir := tool.DirForScope(agent.ScopeGlobal, app.ResolveGlobalRootParent())
// dir == "/Users/tars/.config/devin"

// Same tool, local scope:
dir = tool.DirForScope(agent.ScopeLocal, app.ProjectRoot)
// dir == "/Users/tars/my-project/.devin"
```

`ResolveGlobalRootParent()` and the per-tool dir method are the two
pieces that make scope/target composition uniform. Do not build these
paths by hand, because that is how a per-scope asymmetry leaks.

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
- [Delivery channels](./delivery-channels.md) — what each tool
  receives inside these directories (SPEC-013).
- SPEC-002 (shipped; spec retired to git history) — the spec
  that introduced global scope, the `Tool.DirByScope` model, and the
  semantic-aware routing layer that builds on top of it.
- `internal/agent/scope.go`, `internal/agent/tool.go`,
  `internal/agent/globalroot.go` — the code.
