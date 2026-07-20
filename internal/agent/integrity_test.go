package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brickhouse-tech/sync-agents/internal/agent/source"
)

// newIntegrityApp builds an App rooted at a fresh temp project with a
// .agents/ tree, returning the app, the agents dir, and its output
// buffers.
func newIntegrityApp(t *testing.T) (*App, string, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".agents")
	mustMkdir(t, agentsDir)
	var out, errb bytes.Buffer
	app := &App{ProjectRoot: root, Stdout: &out, Stderr: &errb}
	return app, agentsDir, &out, &errb
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeRule drops a flat rule at rules/<name>.md.
func writeRule(t *testing.T, agentsDir, name, body string) {
	writeFile(t, filepath.Join(agentsDir, "rules", name+".md"), body)
}

// writeSkill drops a dir skill at skills/<name>/SKILL.md (+ extra files).
func writeSkillDir(t *testing.T, agentsDir, name, skillBody string, extra map[string]string) {
	dir := filepath.Join(agentsDir, "skills", name)
	writeFile(t, filepath.Join(dir, "SKILL.md"), skillBody)
	for rel, content := range extra {
		writeFile(t, filepath.Join(dir, rel), content)
	}
}

func mustLock(t *testing.T, app *App) {
	t.Helper()
	if err := app.CmdLock(SourceCmdOpts{}); err != nil {
		t.Fatalf("lock: %v", err)
	}
}

func TestLock_Deterministic(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "security", "# security\n")
	writeSkillDir(t, agentsDir, "foo", "---\nname: foo\n---\nbody\n", map[string]string{"scripts/run.sh": "echo hi\n"})

	mustLock(t, app)
	lock1, _ := os.ReadFile(AgentsLockPath(agentsDir))
	sum1, _ := os.ReadFile(AgentsSumPath(agentsDir))

	mustLock(t, app)
	lock2, _ := os.ReadFile(AgentsLockPath(agentsDir))
	sum2, _ := os.ReadFile(AgentsSumPath(agentsDir))

	if !bytes.Equal(lock1, lock2) {
		t.Errorf("agents.lock not byte-identical across runs:\n---1---\n%s\n---2---\n%s", lock1, lock2)
	}
	if !bytes.Equal(sum1, sum2) {
		t.Errorf("agents.sum not byte-identical across runs")
	}
}

func TestLock_Ordering(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	// Deliberately out-of-order names.
	writeRule(t, agentsDir, "zebra", "z\n")
	writeRule(t, agentsDir, "alpha", "a\n")
	writeRule(t, agentsDir, "mango", "m\n")
	mustLock(t, app)

	st, err := computeIntegrityState(agentsDir)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, la := range st.artifacts {
		paths = append(paths, la.Path)
	}
	want := []string{"rules/alpha.md", "rules/mango.md", "rules/zebra.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("artifact order = %v, want %v", paths, want)
	}

	sum, _ := os.ReadFile(AgentsSumPath(agentsDir))
	lines := strings.Split(strings.TrimSpace(string(sum)), "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i-1] > lines[i] {
			t.Errorf("agents.sum not sorted: %q > %q", lines[i-1], lines[i])
		}
	}
}

func TestLock_OriginResolution(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)

	// local rule (no origin metadata)
	writeRule(t, agentsDir, "state", "local\n")

	// source: skill (manifest _origin.json)
	writeSkillDir(t, agentsDir, "vendored", "---\nname: vendored\n---\n", nil)
	if err := source.WriteOriginFor(filepath.Join(agentsDir, "skills", "vendored"), true, source.Origin{
		Owner: "my-org", Repo: "agent-norms", Path: "skills/vendored", Ref: "main",
		SHA: "abc123", Source: source.SourceManifest,
	}); err != nil {
		t.Fatal(err)
	}

	// imported: skill (manual _origin.json)
	writeSkillDir(t, agentsDir, "oneoff", "---\nname: oneoff\n---\n", nil)
	if err := source.WriteOriginFor(filepath.Join(agentsDir, "skills", "oneoff"), true, source.Origin{
		Owner: "foo", Repo: "bar", Path: "skills/oneoff/SKILL.md", Ref: "main",
		SHA: "def456", Source: source.SourceManual,
	}); err != nil {
		t.Fatal(err)
	}

	mustLock(t, app)
	st, err := computeIntegrityState(agentsDir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, la := range st.artifacts {
		got[la.Name] = la.Origin
	}
	if got["state"] != "local" {
		t.Errorf("state origin = %q, want local", got["state"])
	}
	if got["vendored"] != "source:skill:my-org/agent-norms@main/skills/vendored" {
		t.Errorf("vendored origin = %q", got["vendored"])
	}
	if !strings.HasPrefix(got["oneoff"], "imported:https://github.com/foo/bar/blob/main/") {
		t.Errorf("oneoff origin = %q", got["oneoff"])
	}
}

func TestLock_Exclusions(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "keep", "kept\n")
	// quarantine holding area — never locked
	writeFile(t, filepath.Join(agentsDir, ".quarantine", "rules", "evil.md"), "evil\n")
	// origin file alongside a flat rule — excluded from sums
	writeFile(t, filepath.Join(agentsDir, "rules", "keep.origin.json"), `{"schema":1}`)
	// unshared STATE snapshot — excluded
	writeFile(t, filepath.Join(agentsDir, "rules", "STATE_scratch.md"), "scratch\n")
	// shared STATE snapshot — included
	writeFile(t, filepath.Join(agentsDir, "rules", "STATE_task.md"), "---\nshared: true\n---\ntask\n")

	mustLock(t, app)
	sum, _ := os.ReadFile(AgentsSumPath(agentsDir))
	s := string(sum)
	for _, bad := range []string{".quarantine", "keep.origin.json", "STATE_scratch.md", AgentsSumFileName} {
		if strings.Contains(s, bad) {
			t.Errorf("agents.sum unexpectedly contains %q:\n%s", bad, s)
		}
	}
	if !strings.Contains(s, "rules/STATE_task.md") {
		t.Errorf("shared STATE snapshot missing from sum:\n%s", s)
	}
}

func TestLock_OSSubdirs(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	// Present regardless of host OS.
	writeFile(t, filepath.Join(agentsDir, "rules", "macos", "brew.md"), "brew\n")
	writeFile(t, filepath.Join(agentsDir, "rules", "windows", "choco.md"), "choco\n")
	mustLock(t, app)

	st, err := computeIntegrityState(agentsDir)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, la := range st.artifacts {
		names[la.Name] = true
	}
	if !names["macos/brew"] || !names["windows/choco"] {
		t.Errorf("OS-scoped artifacts missing: %v", names)
	}
}

func TestSum_IncludesLockAndSources(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "r\n")
	writeFile(t, filepath.Join(agentsDir, source.ManifestFileName), "version: 1\nsources: []\n")
	writeFile(t, filepath.Join(agentsDir, source.LockFileName), "version: 1\nentries: []\n")
	mustLock(t, app)

	sum := loadSumMap(t, agentsDir)
	for _, want := range []string{AgentsLockFileName, source.ManifestFileName, source.LockFileName} {
		if _, ok := sum[want]; !ok {
			t.Errorf("agents.sum missing line for %q", want)
		}
	}
	if _, ok := sum[AgentsSumFileName]; ok {
		t.Errorf("agents.sum must not contain itself")
	}
}

func TestVerify_Clean(t *testing.T) {
	app, agentsDir, out, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "r\n")
	writeSkillDir(t, agentsDir, "s", "---\nname: s\n---\n", map[string]string{"a.txt": "a\n"})
	mustLock(t, app)
	out.Reset()

	if err := app.CmdVerify(SourceCmdOpts{}, false, ""); err != nil {
		t.Fatalf("verify should be clean, got %v", err)
	}
	if !strings.Contains(out.String(), "clean") {
		t.Errorf("expected clean message, got: %s", out.String())
	}
}

func TestVerify_LocalEdit_Error(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "original\n")
	mustLock(t, app)

	writeRule(t, agentsDir, "r", "TAMPERED\n")
	err := app.CmdVerify(SourceCmdOpts{}, false, "")
	assertExit(t, err, 1)
}

func TestVerify_UnlockedFile_Error(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "r\n")
	mustLock(t, app)

	writeRule(t, agentsDir, "injected", "surprise\n")
	err := app.CmdVerify(SourceCmdOpts{}, false, "")
	assertExit(t, err, 1)
}

func TestVerify_MissingArtifact_Error(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "r\n")
	writeRule(t, agentsDir, "gone", "here\n")
	mustLock(t, app)

	if err := os.Remove(filepath.Join(agentsDir, "rules", "gone.md")); err != nil {
		t.Fatal(err)
	}
	err := app.CmdVerify(SourceCmdOpts{}, false, "")
	assertExit(t, err, 1)
}

func TestVerify_TamperedLock_Error(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "r\n")
	mustLock(t, app)

	// Hand-edit agents.lock so its bytes no longer match its sum line.
	data, _ := os.ReadFile(AgentsLockPath(agentsDir))
	writeFile(t, AgentsLockPath(agentsDir), string(data)+"# sneaky\n")
	err := app.CmdVerify(SourceCmdOpts{}, false, "")
	assertExit(t, err, 1)
}

func TestVerify_NoLock_Noop(t *testing.T) {
	app, agentsDir, out, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "r\n")
	// no lock written
	if err := app.CmdVerify(SourceCmdOpts{}, false, ""); err != nil {
		t.Fatalf("no-lock verify should be a friendly no-op, got %v", err)
	}
	if !strings.Contains(out.String(), "not locked") {
		t.Errorf("expected 'not locked' notice, got %s", out.String())
	}
	_ = agentsDir
}

func TestVerify_JSONSchema(t *testing.T) {
	app, agentsDir, out, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "orig\n")
	mustLock(t, app)
	writeRule(t, agentsDir, "r", "changed\n")
	out.Reset()

	err := app.CmdVerify(SourceCmdOpts{JSON: true}, false, "")
	assertExit(t, err, 1)

	var res verifyResult
	if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil {
		t.Fatalf("json output not parseable: %v\n%s", jerr, out.String())
	}
	if res.Status != "drift" {
		t.Errorf("status = %q, want drift", res.Status)
	}
	if len(res.Findings) == 0 || res.Findings[0].Path != "rules/r.md" {
		t.Errorf("findings = %+v", res.Findings)
	}
	if res.Counts[sevError] < 1 {
		t.Errorf("expected an ERROR count, got %v", res.Counts)
	}
}

func TestExplain(t *testing.T) {
	app, agentsDir, out, _ := newIntegrityApp(t)
	writeSkillDir(t, agentsDir, "s", "---\nname: s\n---\n", map[string]string{"run.sh": "one\n"})
	mustLock(t, app)
	// modify one file
	writeFile(t, filepath.Join(agentsDir, "skills", "s", "run.sh"), "two\n")
	out.Reset()

	_ = app.CmdVerify(SourceCmdOpts{}, false, "skills/s")
	s := out.String()
	if !strings.Contains(s, "origin:") || !strings.Contains(s, "DRIFT") || !strings.Contains(s, "run.sh") {
		t.Errorf("explain output missing expected fields:\n%s", s)
	}
}

func TestVerify_StrictDoesNotBreakClean(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "r\n")
	mustLock(t, app)
	if err := app.CmdVerify(SourceCmdOpts{}, true, ""); err != nil {
		t.Fatalf("strict verify on clean tree should pass, got %v", err)
	}
}

// linkSkill creates an external skill checkout and symlinks it into
// skills/<name> with a relative target, returning the checkout dir.
func linkSkill(t *testing.T, root, agentsDir, name string) string {
	t.Helper()
	ext := filepath.Join(root, "ext", name)
	writeFile(t, filepath.Join(ext, "SKILL.md"), "---\nname: "+name+"\n---\nlinked\n")
	link := filepath.Join(agentsDir, "skills", name)
	mustMkdir(t, filepath.Dir(link))
	// relative target from skills/ up to root/ext/<name>
	if err := os.Symlink(filepath.Join("..", "..", "ext", name), link); err != nil {
		t.Fatal(err)
	}
	return ext
}

func TestLock_LinkedOrigin(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	linkSkill(t, app.ProjectRoot, agentsDir, "widget")
	mustLock(t, app)

	st, err := computeIntegrityState(agentsDir)
	if err != nil {
		t.Fatal(err)
	}
	var origin string
	for _, la := range st.artifacts {
		if la.Name == "widget" {
			origin = la.Origin
		}
	}
	if !strings.HasPrefix(origin, "linked:file:") {
		t.Errorf("widget origin = %q, want linked:file:…", origin)
	}
	// The symlink itself must contribute a link: line to the sum.
	sum := loadSumMap(t, agentsDir)
	if v, ok := sum["skills/widget"]; !ok || !strings.HasPrefix(v, "link:") {
		t.Errorf("expected link: sum line for skills/widget, got %q", v)
	}
}

func TestVerify_LinkedDrift_InfoThenStrict(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	ext := linkSkill(t, app.ProjectRoot, agentsDir, "widget")
	mustLock(t, app)

	// A linked checkout drifts by design.
	writeFile(t, filepath.Join(ext, "SKILL.md"), "---\nname: widget\n---\nEDITED\n")

	if err := app.CmdVerify(SourceCmdOpts{}, false, ""); err != nil {
		t.Fatalf("linked drift should be INFO (exit 0), got %v", err)
	}
	err := app.CmdVerify(SourceCmdOpts{}, true, "")
	assertExit(t, err, 1)
}

func TestVerify_RepointedLink_Error(t *testing.T) {
	app, agentsDir, _, _ := newIntegrityApp(t)
	linkSkill(t, app.ProjectRoot, agentsDir, "widget")
	// A second checkout with identical content.
	other := filepath.Join(app.ProjectRoot, "ext", "widget2")
	writeFile(t, filepath.Join(other, "SKILL.md"), "---\nname: widget\n---\nlinked\n")
	mustLock(t, app)

	// Re-point the symlink at the twin (same contents, different target).
	link := filepath.Join(agentsDir, "skills", "widget")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "ext", "widget2"), link); err != nil {
		t.Fatal(err)
	}
	err := app.CmdVerify(SourceCmdOpts{}, false, "")
	assertExit(t, err, 1)
}

func TestLock_DryRunAndJSON(t *testing.T) {
	app, agentsDir, out, _ := newIntegrityApp(t)
	writeRule(t, agentsDir, "r", "r\n")
	mustLock(t, app)

	// dry-run after adding an artifact: reports a delta, writes nothing.
	writeRule(t, agentsDir, "added", "new\n")
	sumBefore, _ := os.ReadFile(AgentsSumPath(agentsDir))
	out.Reset()
	app.DryRun = true
	if err := app.CmdLock(SourceCmdOpts{}); err != nil {
		t.Fatal(err)
	}
	app.DryRun = false
	if !strings.Contains(out.String(), "rules/added.md") {
		t.Errorf("dry-run should mention the added artifact, got: %s", out.String())
	}
	sumAfter, _ := os.ReadFile(AgentsSumPath(agentsDir))
	if !bytes.Equal(sumBefore, sumAfter) {
		t.Errorf("dry-run must not modify agents.sum")
	}

	// JSON summary.
	out.Reset()
	if err := app.CmdLock(SourceCmdOpts{JSON: true}); err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if err := json.Unmarshal(out.Bytes(), &summary); err != nil {
		t.Fatalf("lock --json not parseable: %v\n%s", err, out.String())
	}
	if summary["artifacts"] == nil {
		t.Errorf("json summary missing artifacts count: %v", summary)
	}
}

// ---- helpers ----

func loadSumMap(t *testing.T, agentsDir string) map[string]string {
	t.Helper()
	m, err := loadAgentsSum(agentsDir)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func assertExit(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected exit-code %d error, got nil", code)
	}
	ee, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if ee.Code != code {
		t.Fatalf("exit code = %d, want %d", ee.Code, code)
	}
}
