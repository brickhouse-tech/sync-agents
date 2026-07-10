# Global root resolution

How `sync-agents` decides where the user-scope canonical `.agents/`
tree lives, and how that decision flows through to per-tool global
directories like `~/.claude/`, `~/.codeium/`, and so on.

## What "global root" means

The **global root** is the absolute path to the canonical `.agents/`
tree at user scope. On a default setup it is `$HOME/.agents/`. The
parent directory of the global root is the **global root parent** —
the place where per-tool global dirs (`.claude/`, `.codeium/`, etc.)
get derived. By default that parent is `$HOME`.

These two concepts split because tests and shared-machine setups need
to override the parent without inventing a fake `$HOME` to scope all
the per-tool dirs under.

## Precedence

`(*App).ResolveGlobalRoot()` returns an absolute path resolved in the
following order. The first match wins; later entries are not consulted.

1. **`App.GlobalRoot` field** (set programmatically). Used by tests to
   point at a `t.TempDir()` and by library callers that want to
   bypass the environment. If non-empty, returned as-is after
   `filepath.Abs` normalization.
2. **`$SYNC_AGENTS_GLOBAL_ROOT` env var.** Used by CI and by
   shared-machine users who don't want the global tree under `$HOME`.
   If set and non-empty, returned after absolute-path normalization.
3. **`$HOME/.agents`.** The default. `$HOME` is determined by
   `os.UserHomeDir()`; on platforms where that fails, the function
   falls back to `/.agents` and logs a warning (which is a near-fatal
   misconfiguration — the call site should usually fail loudly).

The `--global-root <path>` CLI flag, when supplied, populates the
`App.GlobalRoot` field before commands run. That means the flag wins
over the env var, which wins over the default — exactly the order the
spec calls for.

## Per-tool global directory derivation

Once the global root is resolved, per-tool global directories are
derived by taking the **parent** of the global root and appending the
tool's per-scope dir name (see [scope-and-targets.md](./scope-and-targets.md)):

```
globalRoot       = ResolveGlobalRoot()         // /Users/tars/.agents
globalRootParent = filepath.Dir(globalRoot)    // /Users/tars
claudeGlobalDir  = filepath.Join(globalRootParent, ".claude")
codeiumGlobalDir = filepath.Join(globalRootParent, ".codeium")
copilotGlobalDir = filepath.Join(globalRootParent, ".github", "copilot")
```

Concrete examples:

| Scenario | Global root | Per-tool dir |
|---|---|---|
| Default | `/Users/tars/.agents` | `~/.claude` resolves to `/Users/tars/.claude` |
| `$SYNC_AGENTS_GLOBAL_ROOT=/tmp/g` | `/tmp/g/.agents` | `~/.claude` resolves to `/tmp/g/.claude` |
| `--global-root=/tmp/h/.agents` | `/tmp/h/.agents` | `~/.claude` resolves to `/tmp/h/.claude` |
| Test sets `App.GlobalRoot = "/tmp/t/.agents"` | `/tmp/t/.agents` | `~/.claude` resolves to `/tmp/t/.claude` |

In every case `$HOME` itself is untouched when an override is in play.
This is the property that lets tests run without polluting the real
home directory.

## Why three layers (field, env, flag)

Each of the three precedence levels exists for a distinct caller:

- **Field** — Go-level callers (tests, library users, future
  programmatic embedders). No subprocess, no env munging.
- **Env var** — Shell-level callers (CI, dotfiles setups,
  shared-machine users). Inherited naturally by child processes;
  doesn't require touching `$HOME`.
- **Flag** — Interactive CLI use. Discoverable via `--help`,
  documented per-command, and overrides any ambient configuration in
  the surrounding shell.

The flag-over-env-over-field order matches user expectations elsewhere
in the Unix ecosystem (e.g., `git`'s flag > env > config-file
precedence).

## Absolute path normalization

Both the field and the env var paths are passed through `filepath.Abs`
before return. That means callers can use relative paths
(`--global-root=./tmp-root`) and the resolver returns the absolute
form, which downstream code (`os.Symlink`, `os.WriteFile`) can use
unconditionally.

If `filepath.Abs` fails (extremely rare; means `os.Getwd` is broken),
the resolver returns the original string. The next filesystem call
will fail with a clearer error than the resolver itself could give.

## See also

- [Scope and target directories](./scope-and-targets.md) — how the
  resolved global root composes with per-tool dir names.
- SPEC-002 §Configurable global root (shipped; spec retired to git history) —
  the spec that requires this resolver.
- `internal/agent/globalroot.go` — the implementation.
