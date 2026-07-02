package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// newLocalIndexTestApp returns an App pointed at a fresh t.TempDir
// as the project root, with a minimal .agents/rules/ directory and a
// passive rule ready to index. Stdout/Stderr are captured buffers.
func newLocalIndexTestApp(t *testing.T) (*App, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	agentsRules := filepath.Join(root, ".agents", "rules")
	if err := os.MkdirAll(agentsRules, 0o755); err != nil {
		t.Fatalf("setup rules dir: %v", err)
	}
	stdout := &bytes.Buffer{}
	a := &App{
		ProjectRoot:   root,
		GlobalRoot:    filepath.Join(root, ".agents"),
		ActiveTargets: []string{"claude"},
		Stdout:        stdout,
		Stderr:        &bytes.Buffer{},
	}
	return a, root, stdout
}

// TestCmdIndex_WritesClaudeImportsBlock verifies that local
// `sync-agents index` produces an AGENTS.md with a managed
// @-import block listing each passive rule using project-relative
// paths — the bridge that makes Claude actually load the rule
// content (see issue #46).
func TestCmdIndex_WritesClaudeImportsBlock(t *testing.T) {
	a, root, _ := newLocalIndexTestApp(t)

	// Seed two passive rules (no frontmatter — bucket default for
	// rules is Passive).
	for _, name := range []string{"security", "no-secrets"} {
		if err := os.WriteFile(filepath.Join(root, ".agents", "rules", name+".md"), []byte("body\n"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	if err := a.CmdIndex(); err != nil {
		t.Fatalf("CmdIndex: %v", err)
	}

	agentsMD := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(agentsMD)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	s := string(data)

	if !HasManagedImportBlock(s) {
		t.Fatalf("managed block not written in AGENTS.md:\n%s", s)
	}

	imports := ExtractManagedImports(s)
	want := map[string]bool{
		".claude/rules/no-secrets.md": true,
		".claude/rules/security.md":   true,
	}
	if len(imports) != len(want) {
		t.Errorf("got %d imports %v, want %v", len(imports), imports, want)
	}
	for _, imp := range imports {
		if !want[imp] {
			t.Errorf("unexpected import %q", imp)
		}
		// Must be relative — no absolute paths in checked-in
		// AGENTS.md so it ports across developers.
		if filepath.IsAbs(imp) {
			t.Errorf("import path should be relative: %q", imp)
		}
	}
}

// TestCmdIndex_EmptyAgentsTreeOmitsBlock verifies that a fresh
// project with no rules/skills/workflows doesn't write an empty @-
// import block (there's nothing to import).
func TestCmdIndex_EmptyAgentsTreeOmitsBlock(t *testing.T) {
	a, root, _ := newLocalIndexTestApp(t)

	if err := a.CmdIndex(); err != nil {
		t.Fatalf("CmdIndex: %v", err)
	}

	agentsMD := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(agentsMD)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	s := string(data)

	if HasManagedImportBlock(s) {
		t.Errorf("empty @-import block written when no passive rules exist:\n%s", s)
	}
}

// TestCmdIndex_StripsStaleBlock verifies that re-indexing after the
// user deletes all rules removes the managed block entirely rather
// than leaving a dead @-import list behind.
func TestCmdIndex_StripsStaleBlock(t *testing.T) {
	a, root, _ := newLocalIndexTestApp(t)

	// Seed + index once.
	if err := os.WriteFile(filepath.Join(root, ".agents", "rules", "security.md"), []byte("body\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := a.CmdIndex(); err != nil {
		t.Fatalf("first CmdIndex: %v", err)
	}

	// Now delete the rule and re-index.
	if err := os.Remove(filepath.Join(root, ".agents", "rules", "security.md")); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if err := a.CmdIndex(); err != nil {
		t.Fatalf("second CmdIndex: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	s := string(data)
	if HasManagedImportBlock(s) {
		t.Errorf("stale managed block not stripped after rule removal:\n%s", s)
	}
}
