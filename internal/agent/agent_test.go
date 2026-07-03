package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSameInode(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	os.WriteFile(a, []byte("x"), 0o644)
	os.WriteFile(b, []byte("x"), 0o644)

	if !sameInode(a, a) {
		t.Error("same file should have same inode")
	}
	if sameInode(a, b) {
		t.Error("different files should have different inodes")
	}
	c := filepath.Join(dir, "nonexistent")
	if sameInode(a, c) {
		t.Error("nonexistent file should not match")
	}
}

func TestContainsExactLine(t *testing.T) {
	content := "line one\nline two\nline three\n\n"
	if !containsExactLine(content, "line two") {
		t.Error("should find exact match")
	}
	if containsExactLine(content, "line") {
		t.Error("should not find partial match")
	}
	if containsExactLine(content, "line four") {
		t.Error("should not find missing line")
	}
	if containsExactLine("", "anything") {
		t.Error("empty content should not match")
	}
}

func TestReadConfigTargets_Valid(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agents"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "config"),
		[]byte("# comment\ntargets = claude,cursor,copilot\nmore=stuff\n"), 0o644)

	got := ReadConfigTargets(dir)
	if len(got) != 3 {
		t.Fatalf("expected 3 targets, got %d: %v", len(got), got)
	}
	want := []string{"claude", "cursor", "copilot"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("target[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestReadConfigTargets_NoConfig(t *testing.T) {
	dir := t.TempDir()
	got := ReadConfigTargets(dir)
	// Falls back to all targets when config is missing
	if len(got) == 0 {
		t.Errorf("expected all targets as fallback, got empty")
	}
}

func TestCopyTargets(t *testing.T) {
	src := []string{"a", "b", "c"}
	dst := copyTargets(src)
	if len(dst) != len(src) {
		t.Fatalf("expected %d, got %d", len(src), len(dst))
	}
	for i, v := range src {
		if dst[i] != v {
			t.Errorf("dst[%d] = %q, want %q", i, dst[i], v)
		}
	}
}

func TestAddDefaultGitignoreEntries(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf}
	app.addDefaultGitignoreEntries()

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, ".DS_Store") {
		t.Errorf("missing .DS_Store:\n%s", content)
	}
	if !strings.Contains(content, "sync-agents") {
		t.Errorf("missing sync-agents marker:\n%s", content)
	}
}

func TestUpdateGitignore_AddsEntries(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf, ActiveTargets: []string{"claude"}}
	app.updateGitignore()

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# sync-agents") {
		t.Errorf("missing sync-agents marker:\n%s", content)
	}
	if !strings.Contains(content, ".claude/") {
		t.Errorf("missing .claude/ entry:\n%s", content)
	}
}

func TestAddDefaultGitignoreEntries_Idempotent(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf}
	app.addDefaultGitignoreEntries()
	first, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	app.addDefaultGitignoreEntries()
	second, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(first) != string(second) {
		t.Errorf("gitignore changed on second run:\n1: %s\n2: %s", first, second)
	}
}

func TestPrintTree(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, "rules", "security.md"), []byte("body"), 0o644)

	var buf strings.Builder
	app := &App{Stdout: &buf}
	app.PrintTree(dir, "")

	out := buf.String()
	if !strings.Contains(out, "rules") {
		t.Errorf("missing 'rules' in output:\n%s", out)
	}
	if !strings.Contains(out, "security.md") {
		t.Errorf("missing 'security.md' in output:\n%s", out)
	}
}

func TestCreateSymlink_New(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "test.md"), []byte("content"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)

	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf}
	source := ".agents/rules/test.md"
	target := filepath.Join(dir, ".claude", "rules", "test.md")

	app.CreateSymlink(source, target, false)

	fi, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink")
	}
	link, _ := os.Readlink(target)
	if link != source {
		t.Errorf("symlink target = %q, want %q", link, source)
	}
}

func TestCreateSymlink_DryRun(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "test.md"), []byte("content"), 0o644)

	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf}
	source := ".agents/rules/test.md"
	target := filepath.Join(dir, ".claude", "rules", "test.md")

	app.CreateSymlink(source, target, true)

	if _, err := os.Lstat(target); err == nil {
		t.Error("dry-run should not create symlink")
	}
}

func TestFindProjectRoot(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", ".agents", "rules"), 0o755)

	// From inside project
	got := FindProjectRoot(filepath.Join(dir, "sub", "src"))
	if got != filepath.Join(dir, "sub") {
		t.Errorf("FindProjectRoot = %q, want %q", got, filepath.Join(dir, "sub"))
	}
}

func TestFindProjectRoot_NotFound(t *testing.T) {
	dir := t.TempDir()
	got := FindProjectRoot(dir)
	// Falls back to cwd when no .agents/ or .git/ is found
	if got == "" {
		t.Error("expected non-empty fallback")
	}
}
func TestAddDefaultGitignoreEntries_ExistingGitignore(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf}

	// Pre-create a .gitignore with content.
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o644)

	app.addDefaultGitignoreEntries()

	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	content := string(data)
	if !strings.Contains(content, "node_modules") {
		t.Error("existing content lost")
	}
	if !strings.Contains(content, "sync-agents") {
		t.Errorf("sync-agents section not added to existing gitignore:\n%s", content)
	}
}

func TestCreateSymlink_ReplaceFile(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf, Force: true}

	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "test.md"), []byte("content"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)

	target := filepath.Join(dir, ".claude", "rules", "test.md")
	// Place a regular file where the symlink should go.
	os.WriteFile(target, []byte("blocker"), 0o644)

	app.CreateSymlink(".agents/rules/test.md", target, false)

	fi, _ := os.Lstat(target)
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink after force replace")
	}
}
