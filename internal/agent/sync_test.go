package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests in this file pin the "fold or drill" contract of project-mode
// `sync` and `fix`: a bucket path that is free (or a symlink) is folded
// into a single directory symlink, while a bucket path that is a real
// directory is drilled into, linking each .agents/ entry individually
// and never moving, renaming, or deleting the directory itself.

// backupSuffix is the rename suffix --overwrite uses when it moves a
// real file or directory out of the way of a symlink.
const backupSuffix = ".replaced-by-sync-agents"

// newSyncApp builds an App rooted at a fresh temp dir with .agents/
// seeded with one skill (skills/foo/SKILL.md) and one rule (rules/r.md),
// syncing only to the given targets.
func newSyncApp(t *testing.T, targets ...string) (*App, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "skills", "foo", "SKILL.md"), "agents foo")
	writeFile(t, filepath.Join(root, ".agents", "rules", "r.md"), "agents rule")
	var buf bytes.Buffer
	app := &App{
		ProjectRoot:   root,
		ActiveTargets: targets,
		Stdout:        &buf,
		Stderr:        &buf,
	}
	return app, root, &buf
}

// assertResolvesTo fails unless link is a symlink whose fully resolved
// path equals the resolved path of want. Both sides go through
// EvalSymlinks because macOS temp dirs live under the /var symlink.
func assertResolvesTo(t *testing.T, link, want string) {
	t.Helper()
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink (mode %v)", link, fi.Mode())
	}
	got, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("resolve %s: %v", link, err)
	}
	exp, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("resolve expected %s: %v", want, err)
	}
	if got != exp {
		t.Fatalf("%s resolves to %s, want %s", link, got, exp)
	}
}

// assertRealDir fails unless p is a directory and not a symlink.
func assertRealDir(t *testing.T, p string) {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatalf("lstat %s: %v", p, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		t.Fatalf("%s should be a real directory, got mode %v", p, fi.Mode())
	}
}

// assertContent fails unless the regular file at p holds want.
func assertContent(t *testing.T, p, want string) {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatalf("lstat %s: %v", p, err)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("%s should be a regular file, got mode %v", p, fi.Mode())
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	if string(b) != want {
		t.Fatalf("%s = %q, want %q", p, b, want)
	}
}

// assertAbsent fails if anything (including a dangling symlink) is at p.
func assertAbsent(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(p); err == nil {
		t.Fatalf("%s should not exist", p)
	}
}

// seedWaveSkillsDir creates .wave/skills as a real directory holding a
// tool-native skill that .agents/ does not claim.
func seedWaveSkillsDir(t *testing.T, root string) string {
	t.Helper()
	native := filepath.Join(root, ".wave", "skills", "wave-native", "SKILL.md")
	writeFile(t, native, "native")
	return native
}

// seedShadowingSkill creates .wave/skills/foo/SKILL.md as a real file
// that collides with .agents/skills/foo.
func seedShadowingSkill(t *testing.T, root string) string {
	t.Helper()
	mine := filepath.Join(root, ".wave", "skills", "foo", "SKILL.md")
	writeFile(t, mine, "mine")
	return mine
}

func TestCmdSync_RealDirIsDrilled(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	native := seedWaveSkillsDir(t, root)

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	assertRealDir(t, filepath.Join(root, ".wave", "skills"))
	assertResolvesTo(t, filepath.Join(root, ".wave", "skills", "foo"), filepath.Join(root, ".agents", "skills", "foo"))
	assertContent(t, native, "native")
	link, err := os.Readlink(filepath.Join(root, ".wave", "rules"))
	if err != nil || link != "../.agents/rules" {
		t.Fatalf(".wave/rules should fold to ../.agents/rules, got %q (%v)", link, err)
	}
}

func TestCmdSync_RealDirNeverDeleted_ConflictExitsNonZero(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	mine := seedShadowingSkill(t, root)

	err := app.CmdSync()

	if err == nil {
		t.Fatalf("CmdSync should fail on a conflict\n%s", buf)
	}
	assertContent(t, mine, "mine")
	out := buf.String()
	for _, want := range []string{"conflict", ".wave/skills/foo", "nothing was deleted"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	assertResolvesTo(t, filepath.Join(root, ".wave", "rules"), filepath.Join(root, ".agents", "rules"))
}

func TestCmdSync_OverwriteMovesConflictAside(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	seedShadowingSkill(t, root)
	app.Overwrite = true

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	assertResolvesTo(t, filepath.Join(root, ".wave", "skills", "foo"), filepath.Join(root, ".agents", "skills", "foo"))
	assertContent(t, filepath.Join(root, ".wave", "skills", "foo"+backupSuffix, "SKILL.md"), "mine")
	if !strings.Contains(buf.String(), "moved existing") {
		t.Errorf("output missing %q:\n%s", "moved existing", buf)
	}
}

func TestCmdSync_OverwriteBackupNameDoesNotClobberExistingBackup(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	seedShadowingSkill(t, root)
	oldBackup := filepath.Join(root, ".wave", "skills", "foo"+backupSuffix, "SKILL.md")
	writeFile(t, oldBackup, "old backup")
	app.Overwrite = true

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	assertContent(t, oldBackup, "old backup")
	matches, _ := filepath.Glob(filepath.Join(root, ".wave", "skills", "foo"+backupSuffix+".*"))
	if len(matches) != 1 {
		t.Fatalf("want exactly one timestamped backup, got %v", matches)
	}
	assertContent(t, filepath.Join(matches[0], "SKILL.md"), "mine")
}

func TestCmdSync_ForceIsDeprecatedAliasForOverwrite(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	seedShadowingSkill(t, root)
	native := seedWaveSkillsDir(t, root)
	app.Force = true

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	if !strings.Contains(buf.String(), "deprecated") {
		t.Errorf("output missing deprecation warning:\n%s", buf)
	}
	assertResolvesTo(t, filepath.Join(root, ".wave", "skills", "foo"), filepath.Join(root, ".agents", "skills", "foo"))
	assertContent(t, filepath.Join(root, ".wave", "skills", "foo"+backupSuffix, "SKILL.md"), "mine")
	assertContent(t, native, "native")
}

func TestCmdSync_ForceNeverDeletesRealDir(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	native := seedWaveSkillsDir(t, root)
	app.Force = true

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	assertRealDir(t, filepath.Join(root, ".wave", "skills"))
	assertContent(t, native, "native")
}

func TestCmdSync_ExistingFoldIsNoop(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	link := filepath.Join(root, ".wave", "skills")
	mustMkdir(t, filepath.Dir(link))
	if err := os.Symlink("../.agents/skills", link); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Lstat(link)

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	after, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("correct fold symlink was recreated; want untouched")
	}
	if got, _ := os.Readlink(link); got != "../.agents/skills" {
		t.Errorf("fold symlink = %q, want ../.agents/skills", got)
	}
}

func TestCmdSync_DriftedFoldRepairedWithoutFlag(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	link := filepath.Join(root, ".wave", "skills")
	mustMkdir(t, filepath.Dir(link))
	if err := os.Symlink("../elsewhere", link); err != nil {
		t.Fatal(err)
	}

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	if got, _ := os.Readlink(link); got != "../.agents/skills" {
		t.Errorf("drifted symlink = %q, want ../.agents/skills", got)
	}
}

func TestCmdSync_RealFileAtBucketPathIsConflict(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	blocker := filepath.Join(root, ".wave", "rules")
	writeFile(t, blocker, "blocker")

	err := app.CmdSync()

	if err == nil {
		t.Fatalf("CmdSync should fail when a file sits at the bucket path\n%s", buf)
	}
	assertContent(t, blocker, "blocker")
	if !strings.Contains(buf.String(), "conflict") {
		t.Errorf("output missing %q:\n%s", "conflict", buf)
	}
}

func TestCmdSync_OverwriteMovesRealFileAtBucketPathAside(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	blocker := filepath.Join(root, ".wave", "rules")
	writeFile(t, blocker, "blocker")
	app.Overwrite = true

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	assertContent(t, blocker+backupSuffix, "blocker")
	assertResolvesTo(t, blocker, filepath.Join(root, ".agents", "rules"))
}

func TestCmdSync_DryRunDrillWritesNothing(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	seedWaveSkillsDir(t, root)
	app.DryRun = true

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	assertAbsent(t, filepath.Join(root, ".wave", "skills", "foo"))
	assertAbsent(t, filepath.Join(root, ".wave", "rules"))
}

func TestCmdSync_DryRunDoesNotMoveConflict(t *testing.T) {
	app, root, _ := newSyncApp(t, "wave")
	mine := seedShadowingSkill(t, root)
	app.DryRun = true
	app.Overwrite = true

	_ = app.CmdSync()

	assertContent(t, mine, "mine")
	assertAbsent(t, filepath.Join(root, ".wave", "skills", "foo"+backupSuffix))
}

func TestCmdSync_CopilotDrillRelativePathResolves(t *testing.T) {
	app, root, buf := newSyncApp(t, "copilot")
	native := filepath.Join(root, ".github", "copilot", "skills", "copilot-native", "SKILL.md")
	writeFile(t, native, "native")

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	assertResolvesTo(t, filepath.Join(root, ".github", "copilot", "skills", "foo"), filepath.Join(root, ".agents", "skills", "foo"))
	assertResolvesTo(t, filepath.Join(root, ".github", "copilot", "rules"), filepath.Join(root, ".agents", "rules"))
	assertContent(t, native, "native")
}

func TestCmdSync_DrilledRuleFileIsLinkedAsFile(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	native := filepath.Join(root, ".wave", "rules", "wave-native.md")
	writeFile(t, native, "native rule")

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	assertRealDir(t, filepath.Join(root, ".wave", "rules"))
	assertResolvesTo(t, filepath.Join(root, ".wave", "rules", "r.md"), filepath.Join(root, ".agents", "rules", "r.md"))
	assertContent(t, native, "native rule")
}

func TestCmdSync_ConflictInOneTargetStillSyncsOthers(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave", "windsurf")
	seedShadowingSkill(t, root)

	if err := app.CmdSync(); err == nil {
		t.Fatalf("CmdSync should fail on a conflict\n%s", buf)
	}

	assertResolvesTo(t, filepath.Join(root, ".windsurf", "skills"), filepath.Join(root, ".agents", "skills"))
}

func TestCmdSync_NoConflictReportsComplete(t *testing.T) {
	app, _, buf := newSyncApp(t, "wave")

	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}

	if !strings.Contains(buf.String(), "Sync complete.") {
		t.Errorf("output missing %q:\n%s", "Sync complete.", buf)
	}
}

func TestCmdStatus_ReportsMergedBucket(t *testing.T) {
	app, root, buf := newSyncApp(t, "claude")
	writeFile(t, filepath.Join(root, ".claude", "skills", "claude-native", "SKILL.md"), "native")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("CmdSync: %v\n%s", err, buf)
	}
	buf.Reset()

	if err := app.CmdStatus(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "[merged] skills (1/1 linked") {
		t.Errorf("status missing merged line:\n%s", buf)
	}
}

func TestCmdStatus_UnlinkedRealDirStaysLocal(t *testing.T) {
	app, root, buf := newSyncApp(t, "claude")
	writeFile(t, filepath.Join(root, ".claude", "skills", "claude-native", "SKILL.md"), "native")

	if err := app.CmdStatus(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "[local] skills (not symlinked)") {
		t.Errorf("status missing local line:\n%s", buf)
	}
}

func TestCmdFix_RealDirIsDrilledNotReplaced(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	native := seedWaveSkillsDir(t, root)

	if err := app.CmdFix("all", false); err != nil {
		t.Fatalf("CmdFix: %v\n%s", err, buf)
	}

	assertRealDir(t, filepath.Join(root, ".wave", "skills"))
	assertContent(t, native, "native")
	assertResolvesTo(t, filepath.Join(root, ".wave", "skills", "foo"), filepath.Join(root, ".agents", "skills", "foo"))
	backups, _ := filepath.Glob(filepath.Join(root, ".wave", "skills", "*"+backupSuffix+"*"))
	if len(backups) != 0 {
		t.Errorf("fix without --overwrite must not rename anything, found %v", backups)
	}
}

func TestCmdFix_ForceNeverDeletesRealDir(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	native := seedWaveSkillsDir(t, root)
	app.Force = true

	if err := app.CmdFix("all", false); err != nil {
		t.Fatalf("CmdFix: %v\n%s", err, buf)
	}

	assertRealDir(t, filepath.Join(root, ".wave", "skills"))
	assertContent(t, native, "native")
	if !strings.Contains(buf.String(), "deprecated") {
		t.Errorf("output missing deprecation warning:\n%s", buf)
	}
}
