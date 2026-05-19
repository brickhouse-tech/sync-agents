package agent

import (
	"os"
	"path/filepath"
)

// EnvGlobalRoot is the name of the environment variable that overrides
// the default global root location. Documented at
// docs/architecture/global-root-resolution.md.
//
// The variable holds an absolute or relative path to the canonical
// .agents/ directory at user scope (not the parent directory). For
// example, EnvGlobalRoot=/tmp/g/.agents — not /tmp/g.
const EnvGlobalRoot = "SYNC_AGENTS_GLOBAL_ROOT"

// DefaultGlobalRootSegment is the path segment appended to $HOME to
// produce the default global root. It matches the segment used at
// project scope (".agents") so that the canonical-tree concept is
// uniform across both scopes.
const DefaultGlobalRootSegment = ".agents"

// ResolveGlobalRoot returns the absolute path of the user's global
// .agents/ tree.
//
// Precedence per SPEC-002 §Configurable global root:
//
//  1. App.GlobalRoot field — set programmatically; used by tests and
//     library callers that want to bypass the environment.
//  2. $SYNC_AGENTS_GLOBAL_ROOT env var — used by CI and shared
//     machines.
//  3. $HOME/.agents — the default.
//
// The returned path is always absolute. If filepath.Abs fails (rare;
// indicates a broken os.Getwd), the resolver returns the unnormalized
// input rather than an error — the next filesystem call will surface
// a clearer error than the resolver could.
//
// Callers that need the *parent* of the global root (to derive per-
// tool global directories like ~/.claude/) should use
// ResolveGlobalRootParent or call filepath.Dir on this result. Doing
// it inline at every call site is the pattern this method exists to
// eliminate.
//
// See docs/architecture/global-root-resolution.md for the conceptual
// model and concrete examples.
func (a *App) ResolveGlobalRoot() string {
	if a.GlobalRoot != "" {
		return absOrSelf(a.GlobalRoot)
	}
	if env := os.Getenv(EnvGlobalRoot); env != "" {
		return absOrSelf(env)
	}
	return absOrSelf(filepath.Join(homeOrFallback(), DefaultGlobalRootSegment))
}

// ResolveGlobalRootParent returns the absolute path of the directory
// containing the global root — i.e., the location where per-tool
// global directories like .claude/, .codeium/, .github/copilot/ are
// derived.
//
// On a default setup, this is $HOME. With $SYNC_AGENTS_GLOBAL_ROOT or
// the --global-root flag set, this is the parent of whatever the
// override pointed at. Tests typically set both the global root and
// derive per-tool paths under the same temp parent so they never
// touch the real $HOME.
//
// See docs/architecture/global-root-resolution.md for the derivation
// rule and worked examples.
func (a *App) ResolveGlobalRootParent() string {
	return filepath.Dir(a.ResolveGlobalRoot())
}

// ResolveToolDir returns the absolute directory path for the given
// tool at the given scope, using ProjectRoot for local and the
// resolved global root parent for global.
//
// This is the single recommended entry point for "where does tool X
// live at scope Y?" — it composes ResolveGlobalRoot (for the global
// path) with Tool.DirForScope (for the per-tool segment) so callers
// don't need to remember the parent-derivation rule. Code that builds
// these paths by hand is a regression risk for the windsurf/codeium
// asymmetry; prefer this method.
//
// If the tool has no mapping for the requested scope (e.g., a future
// LocalOnly tool asked about ScopeGlobal), the returned path is the
// empty string. Callers should check Tool.HasScope when iterating.
func (a *App) ResolveToolDir(tool Tool, scope Scope) string {
	if !tool.HasScope(scope) {
		return ""
	}
	var parent string
	switch scope {
	case ScopeLocal:
		parent = a.ProjectRoot
	case ScopeGlobal:
		parent = a.ResolveGlobalRootParent()
	default:
		return ""
	}
	return tool.DirForScope(scope, parent)
}

// absOrSelf returns filepath.Abs(p) or p itself if Abs fails. See
// ResolveGlobalRoot's doc comment for why we don't propagate the
// error.
func absOrSelf(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// homeOrFallback returns the user's home directory or "/" if
// os.UserHomeDir fails. Returning "/" makes the resulting default
// ResolveGlobalRoot value ("/.agents") obviously wrong rather than
// silently falling back to a cwd-relative path — the next filesystem
// op will fail with a permission or not-found error that the user can
// diagnose.
//
// In practice os.UserHomeDir only fails on extremely unusual systems
// (missing $HOME and no /etc/passwd entry, or Windows with no USERPROFILE).
// Real users should never hit this path; if they do, EnvGlobalRoot is
// the documented escape hatch.
func homeOrFallback() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return string(filepath.Separator)
	}
	return home
}
