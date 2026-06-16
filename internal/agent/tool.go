package agent

import (
	"path/filepath"
	"sort"
)

// Tool describes a single AI coding agent target that sync-agents
// supports.
//
// Each Tool carries a stable ID (used in config files, CLI flags, and
// log messages) and a map from Scope to the per-scope directory name
// that the tool reads. Most tools use the same name at both scopes,
// but Windsurf/Codeium intentionally differs: ".windsurf" at project
// scope, ".codeium" at user scope. The DirByScope map captures that
// asymmetry once so callers never need to special-case it.
//
// Aliases let users refer to a Tool by multiple names from the
// command line and from .agents/config. For instance, "windsurf" and
// "codeium" both resolve to the same Tool — which one the user types
// is a matter of which mental model they're working from on a given
// day. Resolution is case-sensitive (lowercase canonical).
//
// See docs/architecture/scope-and-targets.md for the conceptual model
// and SPEC-002 §Filesystem conventions for the canonical per-tool
// directory map this implements.
type Tool struct {
	// ID is the canonical, stable identifier used in config files,
	// CLI flags, log messages, and the `Tools` registry. Lowercase,
	// no punctuation. Examples: "claude", "codeium", "cursor",
	// "copilot", "codex".
	ID string

	// Aliases are alternate strings that resolve to this Tool when
	// passed to ResolveTool. ID itself need not appear in Aliases;
	// ResolveTool checks both.
	//
	// The primary use is the windsurf/codeium pair: the canonical ID
	// is "codeium" (matching the user-scope dir), but "windsurf" is a
	// universally-recognized alias for the same Tool.
	Aliases []string

	// DirByScope maps a Scope to the path *segment* (not the absolute
	// path) where this tool's data lives at that scope. Examples:
	//
	//   Claude:    {ScopeLocal: ".claude",         ScopeGlobal: ".claude"}
	//   Codeium:   {ScopeLocal: ".windsurf",       ScopeGlobal: ".codeium"}
	//   Copilot:   {ScopeLocal: ".github/copilot", ScopeGlobal: ".github/copilot"}
	//
	// The segment is joined to a scope-appropriate parent (project
	// root for local, the global root parent for global) to form the
	// absolute path. Use the DirForScope method rather than reading
	// this map directly.
	DirByScope map[Scope]string

	// LocalOnly is true for tools that have no user-scope presence.
	// This is currently empty (every tool we support has a global
	// concept), but the field exists so a tool like a hypothetical
	// per-repo-only target can opt out without changing every loop
	// that fans across Tools.
	//
	// Callers iterating Tools for a global operation should skip any
	// Tool where LocalOnly is true.
	LocalOnly bool
}

// DirForScope returns the absolute directory path for this tool at the
// given scope, using the provided parent root.
//
// For ScopeLocal, callers pass App.ProjectRoot — the project's working
// directory. For ScopeGlobal, callers pass the *parent* of the global
// root (typically $HOME, or whatever ResolveGlobalRoot's parent
// resolves to). Mixing those up will produce a path inside .agents/
// itself, which is almost certainly a bug — see ResolveGlobalRoot for
// the parent-derivation pattern.
//
// If this Tool has no entry for the given scope (which today shouldn't
// happen for any registered tool), DirForScope returns the empty
// string. Callers that need scope-completeness can pre-check with
// HasScope.
func (t Tool) DirForScope(scope Scope, parentRoot string) string {
	seg, ok := t.DirByScope[scope]
	if !ok || seg == "" {
		return ""
	}
	return filepath.Join(parentRoot, seg)
}

// HasScope reports whether this Tool has a directory mapping for the
// given scope. Used by global-only loops to skip LocalOnly tools and
// by validation in tests.
func (t Tool) HasScope(scope Scope) bool {
	if t.LocalOnly && scope == ScopeGlobal {
		return false
	}
	seg, ok := t.DirByScope[scope]
	return ok && seg != ""
}

// Matches reports whether the given name resolves to this Tool. The
// match is case-sensitive against ID and Aliases — keep CLI inputs
// lowercased before calling.
func (t Tool) Matches(name string) bool {
	if t.ID == name {
		return true
	}
	for _, a := range t.Aliases {
		if a == name {
			return true
		}
	}
	return false
}

// Tools is the registry of every Tool sync-agents knows about. The
// order here is the canonical ordering used for status output and the
// default --targets list.
//
// Adding a new tool requires:
//
//  1. An entry here with ID, Aliases, and DirByScope filled in.
//  2. Verification that no existing call site hardcodes a tool name
//     (grep for the string).
//  3. A doc table update in docs/architecture/scope-and-targets.md
//     and the spec table in SPEC-002 §Filesystem conventions.
//
// The order of Tools should not be relied on for semantic behavior —
// only for stable user-facing output ordering.
var Tools = []Tool{
	{
		ID: "claude",
		DirByScope: map[Scope]string{
			ScopeLocal:  ".claude",
			ScopeGlobal: ".claude",
		},
	},
	{
		// Canonical ID is "codeium" (matching the user-scope dir).
		// "windsurf" is the alias because that's what the IDE is
		// branded as and what users will type more often.
		ID:      "codeium",
		Aliases: []string{"windsurf"},
		DirByScope: map[Scope]string{
			ScopeLocal:  ".windsurf",
			ScopeGlobal: ".codeium",
		},
	},
	{
		ID: "cursor",
		DirByScope: map[Scope]string{
			ScopeLocal:  ".cursor",
			ScopeGlobal: ".cursor",
		},
	},
	{
		// Copilot lives nested under .github/ at both scopes. The
		// DirByScope value here is the path *segment* that gets
		// joined; filepath.Join handles the "/" correctly on every
		// platform.
		ID: "copilot",
		DirByScope: map[Scope]string{
			ScopeLocal:  filepath.Join(".github", "copilot"),
			ScopeGlobal: filepath.Join(".github", "copilot"),
		},
	},
	{
		// Codex was added in SPEC-002 as a global target. The local
		// dir is ".codex/" mirroring the user-scope convention.
		ID: "codex",
		DirByScope: map[Scope]string{
			ScopeLocal:  ".codex",
			ScopeGlobal: ".codex",
		},
	},
}

// ResolveTool returns the Tool whose ID or alias matches the given
// name, plus a bool indicating success. The bool is false when no
// registered tool matches; callers should surface that as a
// user-facing error since the name almost certainly came from a CLI
// flag or config file.
//
// ResolveTool is case-sensitive against lowercased canonical names.
// CLI parsing should lowercase user input before calling; this
// function does not, to avoid silently accepting "CLAUDE" or
// "Windsurf" and complicating the surface contract.
func ResolveTool(name string) (Tool, bool) {
	for _, t := range Tools {
		if t.Matches(name) {
			return t, true
		}
	}
	return Tool{}, false
}

// ToolIDs returns the canonical IDs of every registered tool, in the
// registry's declared order. Used to build defaults and help text.
// The returned slice is a fresh copy; callers may mutate it freely.
func ToolIDs() []string {
	ids := make([]string, len(Tools))
	for i, t := range Tools {
		ids[i] = t.ID
	}
	return ids
}

// ToolIDsForScope returns the canonical IDs of every tool that has a
// directory mapping for the given scope, in registry order. Used by
// global-scope loops to skip LocalOnly tools without re-implementing
// the filter at every call site.
func ToolIDsForScope(scope Scope) []string {
	var ids []string
	for _, t := range Tools {
		if t.HasScope(scope) {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

// SortToolIDs returns a sorted copy of the given IDs. Useful for
// rendering status output in a deterministic order regardless of
// where the IDs came from.
func SortToolIDs(ids []string) []string {
	out := make([]string, len(ids))
	copy(out, ids)
	sort.Strings(out)
	return out
}
