package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestApp returns an App with captured output, rooted at a
// temp dir with .agents/ already created.
func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agents"), 0o755)
	var buf bytes.Buffer
	app := &App{
		ProjectRoot:   dir,
		GlobalRoot:    filepath.Join(dir, ".agents"),
		ActiveTargets: []string{"claude"},
		Stdout:        &buf,
		Stderr:        &buf,
	}
	return app, dir
}

func TestCmdInit(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf}

	if err := app.CmdInit(); err != nil {
		t.Fatal(err)
	}

	// Check .agents structure created
	for _, name := range []string{".agents", ".agents/rules", ".agents/skills", ".agents/workflows"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		} else if !fi.IsDir() {
			t.Errorf("expected %s to be a dir", name)
		}
	}

	// AGENTS.md created
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Error("AGENTS.md not created")
	}
}

func TestCmdInit_Idempotent(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf}

	app.CmdInit()
	// Second init should not error.
	if err := app.CmdInit(); err != nil {
		t.Errorf("second init errored: %v", err)
	}
}

func TestCmdAdd_Rule(t *testing.T) {
	app, dir := newTestApp(t)

	if err := app.CmdAdd("rule", "security"); err != nil {
		t.Fatal(err)
	}

	p := filepath.Join(dir, ".agents", "rules", "security.md")
	if _, err := os.Stat(p); err != nil {
		t.Fatal("rule file not created:", err)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "security") {
		t.Errorf("template not filled: %s", data)
	}
}

func TestCmdAdd_UnknownType(t *testing.T) {
	app, _ := newTestApp(t)
	err := app.CmdAdd("nonsense", "test")
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestCmdClean_RemovesBucketSymlinks(t *testing.T) {
	app, dir := newTestApp(t)
	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	// Create dir-level symlink for a bucket.
	rulesSymlink := filepath.Join(dir, ".claude", "rules")
	os.Symlink("../.agents/rules", rulesSymlink)

	if err := app.CmdClean(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(rulesSymlink); err == nil {
		t.Error("bucket symlink not cleaned up")
	}
}

func TestCmdStatus(t *testing.T) {
	app, dir := newTestApp(t)
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	var buf bytes.Buffer
	app.Stdout = &buf
	if err := app.CmdStatus(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "sync-agents") {
		t.Error("status output missing")
	}
}

func TestCmdIndex(t *testing.T) {
	app, dir := newTestApp(t)

	// Seed a rule.
	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "security.md"), []byte("body"), 0o644)

	if err := app.CmdIndex(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	content := string(data)
	if !strings.Contains(content, "security") {
		t.Errorf("rule not indexed in AGENTS.md:\n%s", content)
	}
	if !strings.HasPrefix(content, "---\ntrigger: always_on\n---\n") {
		t.Errorf("missing trigger header:\n%s", content[:80])
	}
}

func TestMigrateLegacyState(t *testing.T) {
	app, dir := newTestApp(t)

	// Write legacy STATE.md
	os.WriteFile(filepath.Join(dir, ".agents", "STATE.md"), []byte("old state"), 0o644)

	agentsDir := filepath.Join(dir, ".agents")
	app.migrateLegacyState(agentsDir)

	if _, err := os.Stat(filepath.Join(dir, ".agents", "STATE.md")); err == nil {
		t.Error("legacy STATE.md not moved")
	}
}

func TestMigrateLegacyState_NoFile(t *testing.T) {
	app, dir := newTestApp(t)
	agentsDir := filepath.Join(dir, ".agents")
	app.migrateLegacyState(agentsDir) // should not panic
}

func TestApplySymlinkDestination_Conflict(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf}

	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "test.md"), []byte("content"), 0o644)

	// Pre-seed a regular file where the symlink should go.
	os.WriteFile(filepath.Join(dir, ".claude", "rules", "test.md"), []byte("blocker"), 0o644)

	art := Artifact{
		Type:       ArtifactRule,
		Name:       "test",
		SourcePath: filepath.Join(dir, ".agents", "rules", "test.md"),
	}
	dest := Destination{
		Strategy: StrategySymlink,
		Path:     filepath.Join(dir, ".claude", "rules", "test.md"),
	}

	err := app.applySymlinkDestination("claude", art, dest)
	if err == nil {
		t.Error("expected conflict error without --force")
	}
}

func TestApplySymlinkDestination_Force(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	app := &App{ProjectRoot: dir, Stdout: &buf, Stderr: &buf, Force: true}

	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "test.md"), []byte("content"), 0o644)
	os.WriteFile(filepath.Join(dir, ".claude", "rules", "test.md"), []byte("blocker"), 0o644)

	art := Artifact{
		Type:       ArtifactRule,
		Name:       "test",
		SourcePath: filepath.Join(dir, ".agents", "rules", "test.md"),
	}
	dest := Destination{
		Strategy: StrategySymlink,
		Path:     filepath.Join(dir, ".claude", "rules", "test.md"),
	}

	if err := app.applySymlinkDestination("claude", art, dest); err != nil {
		t.Fatal(err)
	}

	// Symlink should exist now.
	fi, _ := os.Lstat(dest.Path)
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink after --force")
	}
}


func TestScrubClaudeManagedBlock_OnlyBlock(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	app := &App{Stdout: &buf, Stderr: &buf}

	claudeMD := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(claudeMD, []byte(ManagedImportBlockStart+"\n"+managedImportBanner+"\n@/a.md\n"+ManagedImportBlockEnd+"\n"), 0o644)

	removed, pruned, err := app.scrubClaudeManagedBlock(claudeMD, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Error("expected 1 removed")
	}
	if !pruned {
		t.Error("expected directory pruned (file was entire block)")
	}
	if _, err := os.Stat(claudeMD); err == nil {
		t.Error("CLAUDE.md should be removed when it's all managed block")
	}
}

func TestScrubClaudeManagedBlock_UserContentSurvives(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	app := &App{Stdout: &buf, Stderr: &buf}

	claudeMD := filepath.Join(dir, "CLAUDE.md")
	content := "# My config\n\nCustom prose.\n\n" +
		ManagedImportBlockStart + "\n" +
		managedImportBanner + "\n" +
		"@/a.md\n" +
		ManagedImportBlockEnd + "\n\n" +
		"More user content.\n"
	os.WriteFile(claudeMD, []byte(content), 0o644)

	_, _, err := app.scrubClaudeManagedBlock(claudeMD, false)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(claudeMD)
	remaining := string(data)
	if !strings.Contains(remaining, "# My config") {
		t.Errorf("user content before block lost:\n%s", remaining)
	}
	if !strings.Contains(remaining, "More user content") {
		t.Errorf("user content after block lost:\n%s", remaining)
	}
	if strings.Contains(remaining, "sync-agents:claude-imports") {
		t.Errorf("managed block not scrubbed:\n%s", remaining)
	}
}

func TestScrubClaudeManagedBlock_NoBlock(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	app := &App{Stdout: &buf, Stderr: &buf}

	claudeMD := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(claudeMD, []byte("# Just user content\n"), 0o644)

	removed, _, err := app.scrubClaudeManagedBlock(claudeMD, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Error("expected 0 removed when no managed block")
	}
	// File preserved.
	if _, err := os.Stat(claudeMD); err != nil {
		t.Error("user file removed")
	}
}

func TestScrubClaudeManagedBlock_Missing(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	app := &App{Stdout: &buf, Stderr: &buf}

	claudeMD := filepath.Join(dir, "CLAUDE.md") // doesn't exist
	removed, _, err := app.scrubClaudeManagedBlock(claudeMD, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Error("expected 0 removed for missing file")
	}
}

func TestCmdSync_HappyPath(t *testing.T) {
	app, dir := newTestApp(t)
	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "security.md"), []byte("# rule\n"), 0o644)

	if err := app.CmdSync(); err != nil {
		t.Fatal(err)
	}

	// Check .claude/rules was created as a symlink.
	rulesLink := filepath.Join(dir, ".claude", "rules")
	fi, err := os.Lstat(rulesLink)
	if err != nil {
		t.Fatal(".claude/rules not created:", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error(".claude/rules should be a symlink")
	}
}

func TestCmdSync_AgentsMDToClaudeMD(t *testing.T) {
	app, dir := newTestApp(t)
	os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agents", "rules", "test.md"), []byte("# rule\n"), 0o644)

	// Pre-create AGENTS.md so CLAUDE.md symlink is created.
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("test"), 0o644)

	if err := app.CmdSync(); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Lstat(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal("CLAUDE.md not created:", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("CLAUDE.md should be a symlink")
	}
}

func TestCmdInheritList(t *testing.T) {
	app, _ := newTestApp(t)
	// Just verify it doesn't panic.
	if err := app.CmdInheritList(); err != nil {
		t.Fatal(err)
	}
}


func TestCopyFile_Basic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.md")
	dst := filepath.Join(dir, "dst.md")
	os.WriteFile(src, []byte("hello"), 0o644)

	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "hello" {
		t.Errorf("copyFile: got %q, want 'hello'", data)
	}
}

func TestCopyFile_DstExists(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.md")
	dst := filepath.Join(dir, "dst.md")
	os.WriteFile(src, []byte("new"), 0o644)
	os.WriteFile(dst, []byte("old"), 0o644)

	err := copyFile(src, dst, 0o644)
	if err == nil {
		t.Error("expected error when dst exists")
	}
}

func TestCopyDir(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	dstDir := filepath.Join(dir, "dst")
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "b.md"), []byte("b"), 0o644)

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.md", "b.md"} {
		data, _ := os.ReadFile(filepath.Join(dstDir, name))
		if len(data) == 0 {
			t.Errorf("copyDir: %s missing", name)
		}
	}
}

func TestCopyDir_DstExists(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	dstDir := filepath.Join(dir, "dst")
	os.MkdirAll(srcDir, 0o755)
	os.MkdirAll(dstDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "x.md"), []byte("x"), 0o644)

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dstDir, "x.md"))
	if string(data) != "x" {
		t.Errorf("copyDir: got %q, want 'x'", data)
	}
}
