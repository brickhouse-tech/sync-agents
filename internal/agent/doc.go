// Package agent is the core library that powers the sync-agents CLI.
// It encapsulates the .agents/ directory model and the per-tool target
// directories (.claude/, .windsurf/, .cursor/, .github/copilot/,
// .codex/) into a set of plain functions and methods on the App
// struct.
//
// # Conceptual model
//
// sync-agents reasons about three things:
//
//  1. A canonical .agents/ tree (rules, skills, workflows) that serves
//     as the single source of truth.
//  2. Per-tool target directories that hold symlinks into the
//     canonical tree.
//  3. A scope distinction between the project-level tree and a
//     user-level (global) tree.
//
// The Scope type (scope.go) and the Tool type (tool.go) capture (3)
// and (2) respectively. ResolveGlobalRoot (globalroot.go) implements
// the precedence chain that decides where the global tree lives.
//
// # Composing the pieces
//
// Most operations follow the same shape:
//
//	// 1. Decide the scope from CLI flags or the operation's intent.
//	scope := agent.ScopeLocal
//
//	// 2. For each tool of interest, ask the App for the absolute dir.
//	for _, t := range agent.Tools {
//	    dir := app.ResolveToolDir(t, scope)
//	    if dir == "" { continue } // tool has no mapping at this scope
//	    // ... use dir
//	}
//
// Avoid building target paths by hand. The windsurf/codeium asymmetry
// (and any future asymmetry) is invisible to callers that go through
// ResolveToolDir.
//
// # Spec references
//
//   - SPEC-001 — first-class go install and goreleaser-based release.
//   - SPEC-002 — promote and global sync with semantic-aware routing.
//     The Scope/Tool/GlobalRoot machinery in this package is the
//     foundation that SPEC-002's promote and global sync commands
//     build on.
//   - SPEC-003 — declarative source manifest for pulling artifacts
//     from upstream repositories.
//
// # File layout
//
//   - agent.go      — the App struct and the existing project-level
//     commands (init, sync, index, fix, ...). Pre-dates the scope
//     model; will be progressively refactored to use it.
//   - scope.go      — the Scope type and ParseScope.
//   - tool.go       — the Tool type, the Tools registry, ResolveTool,
//     and helpers.
//   - globalroot.go — ResolveGlobalRoot, ResolveGlobalRootParent,
//     ResolveToolDir, and the $SYNC_AGENTS_GLOBAL_ROOT env var name.
//
// For prose-level documentation see docs/architecture/ in the repo
// root. For the .agents/ contract that drives the existing commands
// see the project README.
package agent
