package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestArtifactDescription covers description extraction edge cases.
func TestArtifactDescription(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "simple description",
			content: `---
description: Enforces security policies across the repository
---
# Rule
Body.`,
			want: "Enforces security policies across the repository",
		},
		{
			name:    "no frontmatter",
			content: `# Just a markdown file\nNo frontmatter here.`,
			want:    "",
		},
		{
			name: "TODO stub suppressed",
			content: `---
description: TODO — describe this later
---
Body.`,
			want: "",
		},
		{
			name: "multi-line scalar skipped",
			content: `---
description: |
  This is a
  multi-line description
---
Body.`,
			want: "",
		},
		{
			name: "long description truncated",
			content: fmt.Sprintf(`---
description: %s
---
Body.`, strings.Repeat("word ", 50)),
			want: func() string {
				s := strings.TrimSpace(strings.Repeat("word ", 50))
				if len(s) > 140 {
					// artifactDescription joins fields (strings.Fields then Join),
					// then truncates at maxIndexDescription=140 via truncateAtWord.
					// 50 repeats of "word " = 250 chars. Truncate to 140 chars
					// at word boundary, append ellipsis.
					s = truncateAtWord(s, 140) + "…"
				}
				return s
			}(),
		},
		{
			name:    "missing file",
			content: "",
			want:    "",
		},
		{
			name: "empty description",
			content: `---
description:
---
Body.`,
			want: "",
		},
		{
			name: "unterminated frontmatter",
			content: `---
description: something bad
Body without closing.`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			if tt.content != "" {
				f, err := os.CreateTemp(t.TempDir(), "test-*.md")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.WriteString(tt.content); err != nil {
					t.Fatal(err)
				}
				f.Close()
				path = f.Name()
			} else {
				path = filepath.Join(t.TempDir(), "nonexistent.md")
			}
			got := artifactDescription(path)
			if got != tt.want {
				t.Errorf("artifactDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIndexEntry covers the rendering of AGENTS.md index lines.
func TestIndexEntry(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name     string
		md       string
		linkPath string
	}{
		{
			name:     "with description",
			md:       "---\ndescription: Does something useful\n---\nBody.",
			linkPath: ".agents/rules/useful.md",
		},
		{
			name:     "without description",
			md:       "# No frontmatter",
			linkPath: ".agents/rules/boring.md",
		},
		{
			name:     "stub description suppressed",
			md:       "---\ndescription: TODO fill me in\n---\nBody.",
			linkPath: ".agents/rules/wip.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp(dir, "test-*.md")
			if err != nil {
				t.Fatal(err)
			}
			f.WriteString(tt.md)
			f.Close()

			got := indexEntry(tt.name, tt.linkPath, f.Name())
			if !strings.Contains(got, tt.linkPath) {
				t.Errorf("indexEntry missing link %q in:\n%s", tt.linkPath, got)
			}
			if !strings.Contains(got, tt.name) {
				t.Errorf("indexEntry missing name %q in:\n%s", tt.name, got)
			}
			if strings.Contains(got, "TODO") {
				t.Errorf("TODO should be suppressed in:\n%s", got)
			}
		})
	}
}
