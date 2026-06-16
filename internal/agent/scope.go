package agent

// Scope identifies which of the two filesystem realms an artifact or
// operation belongs to.
//
// sync-agents manages a canonical .agents/ tree at one of two scopes:
//
//   - ScopeLocal: the project's tree, rooted at <project>/.agents/.
//     Used for per-repo rules, skills, and workflows.
//   - ScopeGlobal: the user's tree, rooted at $HOME/.agents/ (overridable
//     via SPEC-002's resolver — see ResolveGlobalRoot in globalroot.go).
//     Used for rules, skills, and workflows that apply across every
//     project on the machine.
//
// The Scope value is the *only* thing that should distinguish "local"
// from "global" in code that operates on either. Functions that take a
// Scope can be unit-tested by toggling the value rather than
// monkey-patching $HOME or chdir-ing.
//
// See docs/architecture/scope-and-targets.md for the conceptual model
// and SPEC-002 §Configurable global root for the originating
// requirement.
type Scope int

const (
	// ScopeLocal denotes the current project's .agents/ tree and the
	// per-tool target directories that live alongside it (.claude/,
	// .windsurf/, .cursor/, .github/copilot/, .codex/).
	ScopeLocal Scope = iota

	// ScopeGlobal denotes the user's .agents/ tree and the per-tool
	// global directories derived from it (~/.claude/, ~/.codeium/,
	// ~/.cursor/, ~/.github/copilot/, ~/.codex/).
	//
	// Note that the per-tool *name* may differ between scopes: Windsurf
	// uses .windsurf/ locally but ~/.codeium/ globally. The Tool type
	// (see tool.go) encapsulates that asymmetry.
	ScopeGlobal
)

// String returns the lowercase scope name suitable for log messages
// and the --scope= flag. It is the inverse of ParseScope.
func (s Scope) String() string {
	switch s {
	case ScopeLocal:
		return "local"
	case ScopeGlobal:
		return "global"
	default:
		return "unknown"
	}
}

// ParseScope converts a string from the --scope= flag into a Scope
// value. It accepts "local" and "global" (case-insensitive). Any other
// input returns (ScopeLocal, false); callers should treat the false
// return as an invalid-flag error rather than silently defaulting.
//
// Why no error: the only caller is CLI flag parsing, which already has
// access to the original string for use in its error message. Returning
// a bool keeps the call sites tidy.
func ParseScope(s string) (Scope, bool) {
	switch s {
	case "local", "Local", "LOCAL":
		return ScopeLocal, true
	case "global", "Global", "GLOBAL":
		return ScopeGlobal, true
	default:
		return ScopeLocal, false
	}
}
