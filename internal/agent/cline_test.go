package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// SPEC-016: the cline target.

// SPEC-016 E1
func TestClineTool_Dirs(t *testing.T) {
	cline := asTool(t, "cline")
	if got, want := cline.DirForScope(ScopeLocal, "/proj"), filepath.Join("/proj", ".cline"); got != want {
		t.Errorf("cline local dir = %q, want %q", got, want)
	}
	if got, want := cline.DirForScope(ScopeGlobal, "/home/u"), filepath.Join("/home/u", ".cline"); got != want {
		t.Errorf("cline global dir = %q, want %q", got, want)
	}
}

// SPEC-016 E1: $CLINE_DIR replaces ~/.cline.
func TestClineTool_ClineDir(t *testing.T) {
	cases := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"default", nil, filepath.Join("/home/u", ".cline")},
		{"CLINE_DIR", map[string]string{"CLINE_DIR": "/opt/cline"}, "/opt/cline"},
		{"CLINE_DIR with tilde", map[string]string{"CLINE_DIR": "~/cline-data"}, filepath.Join("/home/u", "cline-data")},
	}
	resolve := asTool(t, "cline").ResolveGlobalDir
	if resolve == nil {
		t.Fatal("cline has no ResolveGlobalDir; $CLINE_DIR would be ignored")
	}
	for _, c := range cases {
		got, err := resolve("/home/u", mapEnv(c.vars, nil))
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

// SPEC-016 E2, E4, E5, E6
func TestSync_ClineFoldsRulesAndWorkflows(t *testing.T) {
	app, root, buf := newSyncApp(t, "cline")
	writeFile(t, filepath.Join(root, ".agents", "workflows", "ship.md"), "workflow")
	writeFile(t, filepath.Join(root, ".agents", "agents", "reviewer.md"), "agent")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	assertResolvesTo(t, filepath.Join(root, ".cline", "rules"), filepath.Join(root, ".agents", "rules"))
	assertResolvesTo(t, filepath.Join(root, ".cline", "workflows"), filepath.Join(root, ".agents", "workflows"))
	for _, dir := range []string{"skills", "agents"} {
		if exists(filepath.Join(root, ".cline", dir)) {
			t.Errorf(".cline/%s was linked; Cline must not receive it", dir)
		}
	}
	for _, want := range []string{
		"cline skills: skipped: Cline loads .agents/skills natively",
		"cline agents: skipped: Cline's agents/ format is undocumented",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}

// SPEC-016 E2, E4: at user scope each rule and workflow is linked into
// ~/.cline/rules and ~/.cline/workflows.
func TestGlobalSync_ClineLinksRulesAndWorkflows(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	writeArtifact(t, a.GlobalRoot, filepath.Join("workflows", "ship.md"), "Ship it.\n")
	seedSkill(t, a.GlobalRoot, "deploy", "---\nname: deploy\n---\nDeploy.\n", nil)
	globalSync(t, a, "cline")

	assertResolvesTo(t, filepath.Join(root, ".cline", "rules", "security.md"), filepath.Join(a.GlobalRoot, "rules", "security.md"))
	assertResolvesTo(t, filepath.Join(root, ".cline", "workflows", "ship.md"), filepath.Join(a.GlobalRoot, "workflows", "ship.md"))
	if exists(filepath.Join(root, ".cline", "skills")) {
		t.Error("~/.cline/skills was created; Cline loads ~/.agents/skills itself")
	}
}

// SPEC-016 E1: global sync writes under $CLINE_DIR, not ~/.cline.
func TestGlobalSync_ClineHonorsClineDir(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	home := filepath.Join(root, "cline-data")
	a.ToolEnv = mapEnv(map[string]string{"CLINE_DIR": home}, nil)
	globalSync(t, a, "cline")

	assertResolvesTo(t, filepath.Join(home, "rules", "security.md"), filepath.Join(a.GlobalRoot, "rules", "security.md"))
	if exists(filepath.Join(root, ".cline")) {
		t.Error("~/.cline created although $CLINE_DIR points elsewhere")
	}
}

// SPEC-016 E2-E6
func TestTargetDestination_Cline(t *testing.T) {
	cline := asTool(t, "cline")
	home := filepath.Join("/home/u", ".cline")
	cases := []struct {
		name     string
		typ      ArtifactType
		art      string
		sem      Semantic
		strategy DestinationStrategy
		path     string
		reason   string
	}{
		{"passive rule", ArtifactRule, "security", Passive, StrategySymlink, filepath.Join(home, "rules", "security.md"), ""},
		{"invocable rule", ArtifactRule, "security", Invocable, StrategySymlink, filepath.Join(home, "rules", "security.md"), ""},
		{"workflow", ArtifactWorkflow, "ship", Invocable, StrategySymlink, filepath.Join(home, "workflows", "ship.md"), ""},
		// SPEC-016 E3: Cline scans one level deep.
		{"OS-scoped rule", ArtifactRule, "macos/brew", Passive, StrategySymlink, filepath.Join(home, "rules", "macos-brew.md"), ""},
		{"skill", ArtifactSkill, "deploy", Invocable, StrategySkip, "", "Cline loads ~/.agents/skills natively"},
		{"agent", ArtifactAgent, "reviewer", Invocable, StrategySkip, "", "Cline's agents/ format is undocumented"},
	}
	for _, c := range cases {
		d := TargetDestination(cline, c.typ, c.art, c.sem, "", "/home/u")
		if d.Strategy != c.strategy || d.Path != c.path || d.SkipReason != c.reason {
			t.Errorf("%s: got %+v, want strategy %v path %q reason %q", c.name, d, c.strategy, c.path, c.reason)
		}
	}
}

// SPEC-016 E7: cline is opt-in, like opencode.
func TestInit_DefaultTargetsOmitCline(t *testing.T) {
	for _, target := range AllTargets {
		if target == "cline" {
			t.Fatal("cline must not be in AllTargets; it is opt-in via config or --targets")
		}
	}
	app, root, _ := newSyncApp(t)
	if err := os.Remove(filepath.Join(root, ".agents", "rules", "r.md")); err != nil {
		t.Fatal(err)
	}
	if err := app.CmdInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	for name, body := range map[string]string{
		".agents/config":   readFile(t, filepath.Join(root, ".agents", "config")),
		"~/.agents/config": GlobalConfigContent,
	} {
		if targets := parseConfigTargets([]byte(body)); slices.Contains(targets, "cline") {
			t.Errorf("%s targets = %v; cline must be opt-in", name, targets)
		}
	}
}

// SPEC-016 E7: a global tree with no targets line does not enroll
// cline either, so ~/.cline is never created uninvited.
func TestGlobalSync_ClineOptIn(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	globalSync(t, a)
	if exists(filepath.Join(root, ".cline")) {
		t.Error("global sync without targets created ~/.cline")
	}
	writeFile(t, filepath.Join(a.GlobalRoot, "config"), "targets = cline\n")
	globalSync(t, a)
	assertResolvesTo(t, filepath.Join(root, ".cline", "rules", "security.md"), filepath.Join(a.GlobalRoot, "rules", "security.md"))
}

// SPEC-016 E2: global status reports each link, and a link whose rule
// is gone as orphaned; global clean removes them.
func TestGlobalStatusAndClean_ClineLinks(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	seedRule(t, a.GlobalRoot, "gone", "Old.\n")
	globalSync(t, a, "cline")
	if err := os.Remove(filepath.Join(a.GlobalRoot, "rules", "gone.md")); err != nil {
		t.Fatal(err)
	}

	out := globalStatusOut(t, a, "cline")
	for _, want := range []string{"[synced] cline/rule/security", "[orphaned] cline/gone.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	if err := a.CmdGlobalClean(GlobalCleanOpts{Targets: []string{"cline"}}); err != nil {
		t.Fatalf("global clean: %v", err)
	}
	if exists(filepath.Join(root, ".cline")) {
		t.Errorf("global clean left %s", filepath.Join(root, ".cline"))
	}
}
