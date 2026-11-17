package agent

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// SPEC-015 Part A: the antigravity target.

// SPEC-015 A1
func TestAntigravityTool_Dirs(t *testing.T) {
	ag := asTool(t, "antigravity")
	if ag.HasScope(ScopeLocal) {
		t.Errorf("antigravity has a local dir %q; it reads .agents/ natively", ag.DirForScope(ScopeLocal, "/proj"))
	}
	if got, want := ag.DirForScope(ScopeGlobal, "/home/u"), filepath.Join("/home/u", ".gemini", "config"); got != want {
		t.Errorf("antigravity global dir = %q, want %q", got, want)
	}
}

// SPEC-015 A1
func TestSync_AntigravityReadsAgentsNatively(t *testing.T) {
	app, root, buf := newSyncApp(t, "antigravity")
	before := treeSnapshot(t, root)
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "antigravity reads .agents/ natively") {
		t.Errorf("missing A1 report:\n%s", out)
	}
	if strings.Contains(out, "global-only") {
		t.Errorf("antigravity reported as global-only:\n%s", out)
	}
	after := treeSnapshot(t, root)
	for p := range after {
		if _, ok := before[p]; !ok && !strings.HasPrefix(p, ".gitignore") {
			t.Errorf("sync --targets antigravity wrote %s", p)
		}
	}
}

// SPEC-015 A2: OS-scoped rules are linked flat, because Antigravity
// scans only the top level of its rules dir.
func TestGlobalSync_AntigravityRulesFlat(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	writeFile(t, filepath.Join(a.GlobalRoot, "config"), "os = darwin\n")
	seedRule(t, a.GlobalRoot, "security", "---\ntrigger: always_on\n---\nNever commit secrets.\n")
	writeArtifact(t, a.GlobalRoot, filepath.Join("rules", "macos", "brew.md"), "---\ntrigger: always_on\n---\nUse brew.\n")
	globalSync(t, a, "antigravity")

	rules := filepath.Join(root, ".gemini", "config", "rules")
	assertResolvesTo(t, filepath.Join(rules, "security.md"), filepath.Join(a.GlobalRoot, "rules", "security.md"))
	assertResolvesTo(t, filepath.Join(rules, "macos-brew.md"), filepath.Join(a.GlobalRoot, "rules", "macos", "brew.md"))
	if exists(filepath.Join(rules, "macos")) {
		t.Error("an OS-scoped rule was nested under rules/macos/, which Antigravity does not scan")
	}
}

// SPEC-015 A3
func TestGlobalSync_AntigravitySkillsLink(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedSkill(t, a.GlobalRoot, "deploy", "---\nname: deploy\ndescription: Deploys. Use when shipping.\n---\n", nil)
	globalSync(t, a, "antigravity")
	assertResolvesTo(t, filepath.Join(root, ".gemini", "config", "skills", "deploy", "SKILL.md"), filepath.Join(a.GlobalRoot, "skills", "deploy", "SKILL.md"))
}

// SPEC-015 A4
func TestGlobalSync_AntigravityAgentsLink(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	writeArtifact(t, a.GlobalRoot, filepath.Join("agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.\n")
	globalSync(t, a, "antigravity")
	assertResolvesTo(t, filepath.Join(root, ".gemini", "config", "agents", "reviewer.md"), filepath.Join(a.GlobalRoot, "agents", "reviewer.md"))
}

// SPEC-015 A2-A5
func TestTargetDestination_Antigravity(t *testing.T) {
	ag := asTool(t, "antigravity")
	home := filepath.Join("/home/u", ".gemini", "config")
	const deprecated = "Antigravity workflows are deprecated; make it a skill"
	cases := []struct {
		name     string
		typ      ArtifactType
		artName  string
		sem      Semantic
		strategy DestinationStrategy
		path     string
		reason   string
	}{
		{"passive rule", ArtifactRule, "security", Passive, StrategySymlink, filepath.Join(home, "rules", "security.md"), ""},
		{"os-scoped rule", ArtifactRule, "linux/apt", Passive, StrategySymlink, filepath.Join(home, "rules", "linux-apt.md"), ""},
		{"skill", ArtifactSkill, "deploy", Invocable, StrategySymlink, filepath.Join(home, "skills", "deploy", "SKILL.md"), ""},
		{"agent", ArtifactAgent, "reviewer", Invocable, StrategySymlink, filepath.Join(home, "agents", "reviewer.md"), ""},
		{"workflow", ArtifactWorkflow, "ship", Invocable, StrategySkip, "", deprecated},
		{"invocable rule", ArtifactRule, "ship", Invocable, StrategySkip, "", deprecated},
	}
	for _, c := range cases {
		d := TargetDestination(ag, c.typ, c.artName, c.sem, "", "/home/u")
		if d.Strategy != c.strategy || d.Path != c.path || d.SkipReason != c.reason {
			t.Errorf("%s: got %+v, want strategy %v path %q reason %q", c.name, d, c.strategy, c.path, c.reason)
		}
	}
}

// SPEC-014 E7, SPEC-015 A6, B1
func TestInit_DefaultTargetsNameAntigravityNotGemini(t *testing.T) {
	project := t.TempDir()
	a := &App{ProjectRoot: project, GlobalRoot: filepath.Join(t.TempDir(), ".agents"), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := a.CmdInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := a.CmdGlobalInit(); err != nil {
		t.Fatalf("global init: %v", err)
	}
	for _, c := range []struct {
		config string
		want   []string
	}{
		{filepath.Join(project, ".agents", "config"), []string{"claude", "devin", "cursor", "copilot", "antigravity"}},
		{filepath.Join(a.GlobalRoot, "config"), []string{"claude", "devin", "cursor", "copilot", "codex", "antigravity"}},
	} {
		if got := parseConfigTargets([]byte(readFile(t, c.config))); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s targets = %v, want %v", c.config, got, c.want)
		}
	}
	if got, want := AllTargets, []string{"claude", "devin", "cursor", "copilot", "antigravity"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AllTargets = %v, want %v", got, want)
	}
}

// SPEC-015 A1: status lists no project dir for antigravity, which is in
// the default targets.
func TestStatus_NoAntigravityDir(t *testing.T) {
	app, _, buf := newSyncApp(t, AllTargets...)
	if err := app.CmdStatus(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(buf.String(), ".antigravity") {
		t.Errorf("status lists a project dir for antigravity:\n%s", buf.String())
	}
}

// SPEC-015 A1: the same holds when antigravity is an active target that
// did not pass through resolveProjectTargets.
func TestStatusTargets_SkipsToolsWithoutProjectDir(t *testing.T) {
	app := &App{ActiveTargets: []string{"antigravity", "openclaw", "wave", "claude"}}
	got := app.statusTargets()
	want := []string{"claude", "devin", "cursor", "copilot", "wave"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("statusTargets() = %v, want %v", got, want)
	}
}
