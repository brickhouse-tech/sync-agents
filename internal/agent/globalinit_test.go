package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newGlobalInitTestApp returns an App pointed at a fresh t.TempDir
// for both ProjectRoot and GlobalRoot, with output captured in buffers
// so tests can assert on stdout/stderr.
//
// The GlobalRoot is set to <tmpdir>/.agents, mirroring the production
// convention. Per ResolveGlobalRoot's contract, the parent of that
// path becomes the global root parent used to derive per-tool dirs —
// also <tmpdir>. So every per-tool dir derived from this App lives
// under <tmpdir>, isolating the test from $HOME.
func newGlobalInitTestApp(t *testing.T) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return &App{
		ProjectRoot: filepath.Join(root, "project"),
		GlobalRoot:  filepath.Join(root, ".agents"),
		Stdout:      stdout,
		Stderr:      stderr,
	}, stdout, stderr
}

// TestCmdGlobalInit_CreatesSkeleton covers the happy path: an empty
// global root gets rules/, skills/, workflows/, and a config file
// with the canonical five-target list.
func TestCmdGlobalInit_CreatesSkeleton(t *testing.T) {
	a, _, _ := newGlobalInitTestApp(t)

	if err := a.CmdGlobalInit(); err != nil {
		t.Fatalf("CmdGlobalInit returned %v", err)
	}

	root := a.ResolveGlobalRoot()
	for _, sub := range []string{"rules", "skills", "workflows"} {
		p := filepath.Join(root, sub)
		fi, err := os.Stat(p)
		if err != nil {
			t.Errorf("expected dir %s to exist: %v", p, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%s is not a directory", p)
		}
	}

	configBytes, err := os.ReadFile(filepath.Join(root, "config"))
	if err != nil {
		t.Fatalf("expected config file to be written: %v", err)
	}
	content := string(configBytes)
	if !strings.Contains(content, "targets = claude,codeium,cursor,copilot,codex") {
		t.Errorf("config does not contain the canonical target line; got:\n%s", content)
	}
}

// TestCmdGlobalInit_Idempotent runs init twice and verifies the
// second run is a no-op (no error, no file rewrites). The
// content-preservation assertion exercises SPEC-002's "existing files
// preserved" requirement.
func TestCmdGlobalInit_Idempotent(t *testing.T) {
	a, _, _ := newGlobalInitTestApp(t)

	if err := a.CmdGlobalInit(); err != nil {
		t.Fatalf("first CmdGlobalInit returned %v", err)
	}

	// Mutate the config file so we can detect if the second init
	// overwrites it.
	configPath := filepath.Join(a.ResolveGlobalRoot(), "config")
	marker := "# user-edited\ntargets = claude\n"
	if err := os.WriteFile(configPath, []byte(marker), 0o644); err != nil {
		t.Fatalf("setup: failed to write marker config: %v", err)
	}

	if err := a.CmdGlobalInit(); err != nil {
		t.Fatalf("second CmdGlobalInit returned %v", err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config disappeared on second init: %v", err)
	}
	if string(got) != marker {
		t.Errorf("config was rewritten on second init.\ngot:\n%s\nwant (unchanged):\n%s", got, marker)
	}
}

// TestCmdGlobalInit_MissingSubdirRecreated covers a partial-state
// recovery case: the user deleted one of the subdirs after a previous
// init. Re-running should recreate just the missing piece.
func TestCmdGlobalInit_MissingSubdirRecreated(t *testing.T) {
	a, _, _ := newGlobalInitTestApp(t)
	if err := a.CmdGlobalInit(); err != nil {
		t.Fatalf("first init: %v", err)
	}

	root := a.ResolveGlobalRoot()
	skills := filepath.Join(root, "skills")
	if err := os.RemoveAll(skills); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := a.CmdGlobalInit(); err != nil {
		t.Fatalf("re-init: %v", err)
	}
	if fi, err := os.Stat(skills); err != nil || !fi.IsDir() {
		t.Errorf("expected skills/ to be recreated; stat err=%v", err)
	}
}

// TestCmdGlobalInit_DryRun ensures the dry-run flag prevents any
// filesystem writes while still printing intent. We check that
// neither the subdirs nor the config file exist after the dry run.
func TestCmdGlobalInit_DryRun(t *testing.T) {
	a, stdout, _ := newGlobalInitTestApp(t)
	a.DryRun = true

	if err := a.CmdGlobalInit(); err != nil {
		t.Fatalf("CmdGlobalInit returned %v", err)
	}

	if _, err := os.Stat(a.ResolveGlobalRoot()); err == nil {
		t.Errorf("dry-run created the global root; should have left fs untouched")
	}

	if !strings.Contains(stdout.String(), "[dry-run]") {
		t.Errorf("expected '[dry-run]' marker in stdout; got:\n%s", stdout.String())
	}
}
