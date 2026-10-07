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
			name: "literal block scalar resolved onto one line",
			content: `---
description: |
  This is a
  multi-line description
---
Body.`,
			want: "This is a multi-line description",
		},
		{
			// #95: the folded form rendered blank in AGENTS.md.
			name: "folded block scalar resolved",
			content: `---
name: pr-review
description: >
  Perform adversarial pull request and code reviews
  that hunt for real bugs.
---
Body.`,
			want: "Perform adversarial pull request and code reviews that hunt for real bugs.",
		},
		{
			name: "folded strip-chomp scalar resolved",
			content: `---
description: >-
  Folded with strip chomping.
---
Body.`,
			want: "Folded with strip chomping.",
		},
		{
			name: "quoted multi-line scalar resolved",
			content: `---
description: "Quoted across
  two lines"
---
Body.`,
			want: "Quoted across two lines",
		},
		{
			// Invalid YAML elsewhere in the block must not blank a
			// readable folded description (line-based fallback).
			name: "folded scalar resolved when block is not valid YAML",
			content: `---
bad: [unclosed
description: >
  Still readable via the fallback.
---
Body.`,
			want: "Still readable via the fallback.",
		},
		{
			name: "empty folded scalar renders nothing",
			content: `---
description: >
name: x
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

func TestIsBucketActive(t *testing.T) {
	app := &App{ActiveTargets: []string{"claude", "cursor"}}
	if !app.isBucketActive("claude") {
		t.Error("claude should be active")
	}
	if !app.isBucketActive("cursor") {
		t.Error("cursor should be active")
	}
	if app.isBucketActive("copilot") {
		t.Error("copilot should NOT be active")
	}
	if app.isBucketActive("") {
		t.Error("empty string should NOT be active")
	}
}

func TestResolveTargetDir(t *testing.T) {
	root := "/project"
	tests := []struct {
		target string
		want   string
	}{
		{"claude", "/project/.claude"},
		{"cursor", "/project/.cursor"},
		{"windsurf", "/project/.windsurf"},
		{"copilot", "/project/.github/copilot"},
		{"codex", "/project/.codex"},
	}
	for _, tt := range tests {
		got := ResolveTargetDir(tt.target, root)
		if got != tt.want {
			t.Errorf("ResolveTargetDir(%q, %q) = %q, want %q", tt.target, root, got, tt.want)
		}
	}
}

func TestResolveAgentsRel(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{"claude", "../.agents"},
		{"windsurf", "../.agents"},
		{"cursor", "../.agents"},
		{"copilot", "../../.agents"},
	}
	for _, tt := range tests {
		got := ResolveAgentsRel(tt.target)
		if got != tt.want {
			t.Errorf("ResolveAgentsRel(%q) = %q, want %q", tt.target, got, tt.want)
		}
	}
}

func TestListMDFilesRecursive_Nested(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "effort-a"), 0o755)
	os.MkdirAll(filepath.Join(dir, "effort-b"), 0o755)
	os.WriteFile(filepath.Join(dir, "plan.md"), []byte("root"), 0o644)
	os.WriteFile(filepath.Join(dir, "effort-a", "rollout.md"), []byte("nested"), 0o644)
	os.WriteFile(filepath.Join(dir, "effort-b", "testing.md"), []byte("nested2"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden.md"), []byte("hidden"), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not md"), 0o644)

	names, warns := listMDFilesRecursive(dir)
	if len(warns) > 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(names) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(names), names)
	}
	want := map[string]bool{
		"plan":             true,
		"effort-a/rollout": true,
		"effort-b/testing": true,
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected file: %q", n)
		}
	}
}

func TestListMDFilesRecursive_MissingDirIsSilent(t *testing.T) {
	// Optional sections (plans/specs, adrs/{accepted,proposed,denied})
	// are commonly absent. A missing directory must index as empty
	// without emitting a warning.
	missing := filepath.Join(t.TempDir(), "adrs", "accepted")
	names, warns := listMDFilesRecursive(missing)
	if len(warns) != 0 {
		t.Errorf("a missing optional directory must not warn; got: %v", warns)
	}
	if len(names) != 0 {
		t.Errorf("a missing directory must index as empty; got: %v", names)
	}
}

func TestCreateSymlink_Repair(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf, Force: true}

	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "test.md"), []byte("content"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)

	target := filepath.Join(dir, ".claude", "rules", "test.md")
	os.Symlink("wrong-target", target)

	app.CreateSymlink(".agents/rules/test.md", target, false)

	link, _ := os.Readlink(target)
	if link != ".agents/rules/test.md" {
		t.Errorf("symlink not repaired: got %q", link)
	}
}

func TestBucketForArtifact_Hooks(t *testing.T) {
	if _, ok := BucketForArtifact(ArtifactHook); !ok {
		t.Error("ArtifactHook should be found")
	}
}

func TestCmdIndex_NoFix(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "test.md"), []byte("body"), 0o644)

	var buf bytes.Buffer
	app := &App{
		ProjectRoot:   dir,
		GlobalRoot:    filepath.Join(dir, ".agents"),
		ActiveTargets: []string{"claude"},
		Stdout:        &buf,
		Stderr:        &buf,
	}

	// Just verify CmdIndex doesn't fail — no-fix is handled at CLI layer.
	if err := app.CmdIndex(); err != nil {
		t.Fatal(err)
	}
}

// TestMergeHooks_WarnsOnScriptOnlyDir: a hooks dir with only scripts
// never merges anything — the sync must say so rather than silently
// doing nothing.
func TestMergeHooks_WarnsOnScriptOnlyDir(t *testing.T) {
	a, _, _ := newLocalIndexTestApp(t)
	hooksDir := filepath.Join(a.ProjectRoot, ".agents", "hooks")
	os.MkdirAll(hooksDir, 0o755)
	os.WriteFile(filepath.Join(hooksDir, "sync-permissions.sh"), []byte("#!/bin/sh\n"), 0o755)

	var out bytes.Buffer
	a.Stdout = &out
	n, err := a.MergeHooks(hooksDir,
		filepath.Join(a.ProjectRoot, ".claude", "settings.json"),
		filepath.Join(a.ProjectRoot, ".agents", ".sync", "claude-hooks-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("merged %d fragments from a script-only dir", n)
	}
	if !strings.Contains(out.String(), "NOT merged") {
		t.Fatalf("no warning for script-only hooks dir; output:\n%s", out.String())
	}
	// And no settings.json conjured out of nothing.
	if _, err := os.Stat(filepath.Join(a.ProjectRoot, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("script-only dir created a settings.json")
	}
}
