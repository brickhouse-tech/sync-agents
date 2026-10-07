package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStatusTestApp returns an App pointed at a fresh temp root and a
// captured stdout. The global root is seeded as an empty dir so
// CmdGlobalStatus doesn't error on missing root.
func newStatusTestApp(t *testing.T) (*App, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	globalRoot := filepath.Join(root, ".agents")
	if err := os.MkdirAll(globalRoot, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	stdout := &bytes.Buffer{}
	return &App{
		ProjectRoot: filepath.Join(root, "project"),
		GlobalRoot:  globalRoot,
		Stdout:      stdout,
		Stderr:      &bytes.Buffer{},
	}, root, stdout
}

// TestClassifySymlinkDestination_AllStates exercises every state
// branch of the symlink classifier. Keeps the test pure (no App)
// since classifySymlinkDestination is the pure logic that everything
// else depends on.
func TestClassifySymlinkDestination_AllStates(t *testing.T) {
	tmp := t.TempDir()

	// StateMissing: nothing at the path.
	state, _ := classifySymlinkDestination(filepath.Join(tmp, "missing.md"), "/anything")
	if state != StateMissing {
		t.Errorf("missing: got %q, want StateMissing", state)
	}

	// StateSynced: symlink to the want target.
	target := filepath.Join(tmp, "real.md")
	if err := os.WriteFile(target, []byte("body"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	synced := filepath.Join(tmp, "synced.md")
	if err := os.Symlink(target, synced); err != nil {
		t.Fatalf("setup: %v", err)
	}
	state, _ = classifySymlinkDestination(synced, target)
	if state != StateSynced {
		t.Errorf("synced: got %q, want StateSynced", state)
	}

	// StateDrifted: symlink exists but points elsewhere.
	other := filepath.Join(tmp, "other.md")
	if err := os.WriteFile(other, []byte("other"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	drifted := filepath.Join(tmp, "drifted.md")
	if err := os.Symlink(other, drifted); err != nil {
		t.Fatalf("setup: %v", err)
	}
	state, detail := classifySymlinkDestination(drifted, target)
	if state != StateDrifted {
		t.Errorf("drifted: got %q, want StateDrifted", state)
	}
	if !strings.Contains(detail, "points at") {
		t.Errorf("drifted detail should mention current target; got %q", detail)
	}

	// StateNotSymlink: regular file at the path.
	notLink := filepath.Join(tmp, "notlink.md")
	if err := os.WriteFile(notLink, []byte("regular"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	state, _ = classifySymlinkDestination(notLink, target)
	if state != StateNotSymlink {
		t.Errorf("not-a-symlink: got %q, want StateNotSymlink", state)
	}
}

// TestCmdGlobalStatus_Empty exercises the no-artifacts path: the
// command runs cleanly and produces an "0 artifacts" header line
// with no per-destination rows.
func TestCmdGlobalStatus_Empty(t *testing.T) {
	a, _, stdout := newStatusTestApp(t)

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("CmdGlobalStatus: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "0 artifacts") {
		t.Errorf("expected '0 artifacts' header in output:\n%s", out)
	}
}

// TestCmdGlobalStatus_AfterSync exercises the happy path: seed a
// rule, run sync, then status should report [synced] for every
// per-tool symlink destination and one synced row for the Windsurf
// channel (its home exists).
func TestCmdGlobalStatus_AfterSync(t *testing.T) {
	a, root, stdout := newStatusTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "security", "body\n")
	mkdirs(t, filepath.Join(root, ".codeium"))

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Reset stdout buffer so we only inspect the status output.
	stdout.Reset()

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[synced]") {
		t.Errorf("expected at least one [synced] line:\n%s", out)
	}
	host := filepath.Join(root, ".codeium", "windsurf", "memories", "global_rules.md")
	if !strings.Contains(out, "[synced] codeium -> "+host) {
		t.Errorf("expected a synced codeium channel row:\n%s", out)
	}
}

// TestCmdGlobalStatus_DriftedDetected creates a drifted symlink
// manually (without going through sync) and verifies status spots it.
func TestCmdGlobalStatus_DriftedDetected(t *testing.T) {
	a, root, stdout := newStatusTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "x", "body\n")

	// Create a drifted symlink at the Claude destination pointing
	// at /elsewhere.
	driftTarget := filepath.Join(root, "drift-target.md")
	if err := os.WriteFile(driftTarget, []byte("drift"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	claudeRule := filepath.Join(root, ".claude", "rules", "x.md")
	if err := os.MkdirAll(filepath.Dir(claudeRule), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(driftTarget, claudeRule); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[drifted]") {
		t.Errorf("expected a [drifted] line:\n%s", out)
	}
}

// TestCmdGlobalStatus_MissingDestinations covers the pre-sync state:
// after seeding artifacts but never running sync, every destination
// should be reported as [missing].
func TestCmdGlobalStatus_MissingDestinations(t *testing.T) {
	a, _, stdout := newStatusTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "x", "body\n")

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[missing]") {
		t.Errorf("expected at least one [missing] line before sync:\n%s", out)
	}
}

// TestCmdGlobalStatus_NoGlobalRootErrors verifies the actionable
// error when the global root is absent.
func TestCmdGlobalStatus_NoGlobalRootErrors(t *testing.T) {
	tmp := t.TempDir()
	a := &App{
		ProjectRoot: tmp,
		GlobalRoot:  filepath.Join(tmp, "does-not-exist"),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
	}
	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err == nil {
		t.Fatal("expected error for missing global root; got nil")
	}
	stderr := a.Stderr.(*bytes.Buffer)
	if !strings.Contains(stderr.String(), "global init") {
		t.Errorf("error should suggest `global init`; got %q", stderr.String())
	}
}

// TestCmdGlobalStatus_TargetsFilter limits the report to one tool.
// All emitted lines should mention that tool ID exclusively.
func TestCmdGlobalStatus_TargetsFilter(t *testing.T) {
	a, _, stdout := newStatusTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "x", "body\n")

	if err := a.CmdGlobalStatus(GlobalStatusOpts{Targets: []string{"claude"}}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "claude/") {
		t.Errorf("expected claude/ lines in filtered output:\n%s", out)
	}
	for _, other := range []string{"codeium/", "cursor/", "copilot/", "codex/"} {
		if strings.Contains(out, other) {
			t.Errorf("filtered output contains %s; targets filter not respected:\n%s", other, out)
		}
	}
}

// TestCmdGlobalStatus_OSScopedChannelReadsSynced pins that status
// renders a channel with the same bytes sync writes. An OS-scoped rule
// gets an `<!-- OS: x -->` header; a status render that omitted it
// would report a freshly synced channel as stale forever.
func TestCmdGlobalStatus_OSScopedChannelReadsSynced(t *testing.T) {
	app, root, _ := newStatusTestApp(t)
	writeArtifact(t, app.GlobalRoot, "config", "os = linux\n")
	writeArtifact(t, app.GlobalRoot, filepath.Join("rules", "linux", "apt.md"), "Use apt.\n")
	mkdirs(t, filepath.Join(root, ".copilot"))

	if err := app.CmdGlobalSync(GlobalSyncOpts{Targets: []string{"copilot"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	copilot, _ := ResolveTool("copilot")
	rows, err := app.channelRows(ChannelRun{Scope: ScopeGlobal, Tools: []Tool{copilot}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].State != string(ChannelSynced) {
		t.Fatalf("rows = %+v; want one synced copilot row", rows)
	}
	if idx := readFile(t, filepath.Join(app.GlobalRoot, "index", "copilot.md")); !strings.Contains(idx, "<!-- OS: linux -->\n## linux/apt") {
		t.Errorf("index lacks the OS header:\n%s", idx)
	}
}
