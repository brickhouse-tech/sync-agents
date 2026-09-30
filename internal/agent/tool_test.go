package agent

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestTool_DirForScope_Codeium is the canonical test for the
// windsurf/codeium asymmetry. It is intentionally named for the tool
// rather than the scope so a failure points directly at the rule
// being violated.
//
// Local scope MUST resolve to .windsurf; global scope MUST resolve to
// .codeium. Reversing this would break every Windsurf user.
func TestTool_DirForScope_Codeium(t *testing.T) {
	codeium, ok := ResolveTool("codeium")
	if !ok {
		t.Fatal(`ResolveTool("codeium") returned ok=false; the codeium tool must always be registered`)
	}

	local := codeium.DirForScope(ScopeLocal, "/proj")
	if local != filepath.Join("/proj", ".windsurf") {
		t.Errorf("codeium local dir = %q, want %q", local, filepath.Join("/proj", ".windsurf"))
	}

	global := codeium.DirForScope(ScopeGlobal, "/home/u")
	if global != filepath.Join("/home/u", ".codeium") {
		t.Errorf("codeium global dir = %q, want %q", global, filepath.Join("/home/u", ".codeium"))
	}
}

// TestResolveTool_AliasResolution covers the windsurf alias: a user
// who types "windsurf" should get the same Tool as one who types
// "codeium".
func TestResolveTool_AliasResolution(t *testing.T) {
	w, okW := ResolveTool("windsurf")
	c, okC := ResolveTool("codeium")
	if !okW || !okC {
		t.Fatalf("expected both windsurf and codeium to resolve; got okW=%v okC=%v", okW, okC)
	}
	if w.ID != c.ID {
		t.Errorf("windsurf and codeium resolved to different IDs: %q vs %q", w.ID, c.ID)
	}
}

// TestResolveTool_UnknownReturnsFalse is the negative case — unknown
// names must return ok=false rather than silently falling back to a
// default.
func TestResolveTool_UnknownReturnsFalse(t *testing.T) {
	if _, ok := ResolveTool("not-a-real-tool"); ok {
		t.Error(`ResolveTool("not-a-real-tool") returned ok=true; want false`)
	}
}

// TestResolveTool_CaseSensitivity asserts the documented contract:
// ResolveTool is case-sensitive on lowercase canonical names. Anything
// upper-cased should fail to resolve so CLI parsing must lowercase
// before calling.
func TestResolveTool_CaseSensitivity(t *testing.T) {
	for _, in := range []string{"CLAUDE", "Codeium", "Windsurf"} {
		if _, ok := ResolveTool(in); ok {
			t.Errorf("ResolveTool(%q) returned ok=true; ResolveTool must be lowercase-only", in)
		}
	}
}

// TestTools_Registry_HasExpectedIDs is a regression guard: SPEC-002
// names 5 tools (claude, codeium, cursor, copilot, codex) and
// SPEC-011 Part B adds opencode, SPEC-012 openclaw. Anything that
// changes that set should require an update here and a corresponding
// spec change.
func TestTools_Registry_HasExpectedIDs(t *testing.T) {
	want := []string{"claude", "codeium", "cursor", "copilot", "codex", "opencode", "openclaw"}
	if got := ToolIDs(); !reflect.DeepEqual(got, want) {
		t.Errorf("ToolIDs() = %v, want %v", got, want)
	}
}

// TestTool_Opencode_XDGGlobalDir pins opencode's asymmetric scope
// mapping (SPEC-011 Part B): a plain dotdir at project scope, but an
// XDG path under ~/.config at user scope. Getting this backwards
// would write into a directory opencode never reads.
func TestTool_Opencode_XDGGlobalDir(t *testing.T) {
	tool, ok := ResolveTool("opencode")
	if !ok {
		t.Fatal("opencode must resolve from the registry")
	}
	if got, want := tool.DirForScope(ScopeLocal, "/proj"), filepath.Join("/proj", ".opencode"); got != want {
		t.Errorf("local dir = %q, want %q", got, want)
	}
	if got, want := tool.DirForScope(ScopeGlobal, "/home/u"), filepath.Join("/home/u", ".config", "opencode"); got != want {
		t.Errorf("global dir = %q, want %q", got, want)
	}
}

// TestTools_OpencodeNotInDefaultTargets guards SPEC-011 Open Question
// 1: registering opencode must not enroll existing users. A tool in
// the registry is available to opt into, not switched on — otherwise
// the next sync walks into a hand-managed .opencode/ tree uninvited.
func TestTools_OpencodeNotInDefaultTargets(t *testing.T) {
	for _, target := range AllTargets {
		if target == "opencode" {
			t.Fatal("opencode must not be in AllTargets; it is opt-in via config or --targets")
		}
	}
}

// globalOnlyTools have no project-scope surface (SPEC-012: OpenClaw's
// workspace is per user).
var globalOnlyTools = map[string]bool{"openclaw": true}

// TestTool_HasScope covers the LocalOnly opt-out and the standard
// case: every tool has a global scope unless LocalOnly, and a local
// scope unless listed in globalOnlyTools.
func TestTool_HasScope(t *testing.T) {
	for _, tool := range Tools {
		if !tool.HasScope(ScopeLocal) && !globalOnlyTools[tool.ID] {
			t.Errorf("tool %q is missing ScopeLocal mapping", tool.ID)
		}
		if !tool.HasScope(ScopeGlobal) {
			// Acceptable only if LocalOnly is set.
			if !tool.LocalOnly {
				t.Errorf("tool %q is missing ScopeGlobal mapping and is not LocalOnly", tool.ID)
			}
		}
	}
}

// TestTool_DirForScope_Copilot guards the .github/copilot nested path
// which is the second shape-changer in the registry. The path must
// use the OS separator (filepath.Join handles this).
func TestTool_DirForScope_Copilot(t *testing.T) {
	copilot, ok := ResolveTool("copilot")
	if !ok {
		t.Fatal("copilot tool not registered")
	}
	want := filepath.Join("/r", ".github", "copilot")
	if got := copilot.DirForScope(ScopeLocal, "/r"); got != want {
		t.Errorf("copilot local dir = %q, want %q", got, want)
	}
	if got := copilot.DirForScope(ScopeGlobal, "/r"); got != want {
		t.Errorf("copilot global dir = %q, want %q", got, want)
	}
}

// TestToolIDsForScope: every tool is global (none are LocalOnly);
// every tool but the global-only ones is local.
func TestToolIDsForScope(t *testing.T) {
	local := ToolIDsForScope(ScopeLocal)
	global := ToolIDsForScope(ScopeGlobal)
	var wantLocal []string
	for _, id := range ToolIDs() {
		if !globalOnlyTools[id] {
			wantLocal = append(wantLocal, id)
		}
	}
	if !reflect.DeepEqual(local, wantLocal) {
		t.Errorf("ToolIDsForScope(ScopeLocal) = %v, want %v", local, wantLocal)
	}
	if !reflect.DeepEqual(global, ToolIDs()) {
		t.Errorf("ToolIDsForScope(ScopeGlobal) = %v, want %v", global, ToolIDs())
	}
}

// TestSortToolIDs verifies the helper returns a sorted copy without
// mutating its input.
func TestSortToolIDs(t *testing.T) {
	in := []string{"cursor", "claude", "codeium"}
	cp := []string{"cursor", "claude", "codeium"}
	got := SortToolIDs(in)
	want := []string{"claude", "codeium", "cursor"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SortToolIDs = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(in, cp) {
		t.Errorf("SortToolIDs mutated input: %v, want %v", in, cp)
	}
}
