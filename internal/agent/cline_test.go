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

// seedOSScopedRules writes one rule per OS scope and pins discovery to
// darwin, so macos/ and unix/ match and linux/ does not.
func seedOSScopedRules(t *testing.T, agentsDir string) {
	t.Helper()
	writeFile(t, filepath.Join(agentsDir, "config"), "os = darwin\n")
	writeFile(t, filepath.Join(agentsDir, "rules", "macos", "brew.md"), "brew")
	writeFile(t, filepath.Join(agentsDir, "rules", "unix", "posix.md"), "posix")
	writeFile(t, filepath.Join(agentsDir, "rules", "linux", "apt.md"), "apt")
}

// SPEC-016 E3: Cline scans .cline/rules one level deep, so a fold would
// hide macos/brew.md. The rules are linked flat into a real directory.
func TestSync_ClineLinksOSScopedRulesFlat(t *testing.T) {
	app, root, _ := newSyncApp(t, "cline")
	agents := filepath.Join(root, ".agents")
	seedOSScopedRules(t, agents)
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	dest := filepath.Join(root, ".cline", "rules")
	if fi, err := os.Lstat(dest); err != nil || !fi.IsDir() {
		t.Fatalf(".cline/rules is not a real directory: %v", err)
	}
	assertResolvesTo(t, filepath.Join(dest, "r.md"), filepath.Join(agents, "rules", "r.md"))
	assertResolvesTo(t, filepath.Join(dest, "macos-brew.md"), filepath.Join(agents, "rules", "macos", "brew.md"))
	assertResolvesTo(t, filepath.Join(dest, "unix-posix.md"), filepath.Join(agents, "rules", "unix", "posix.md"))
	for _, name := range []string{"linux-apt.md", "macos", "unix", "linux"} {
		if exists(filepath.Join(dest, name)) {
			t.Errorf(".cline/rules/%s was linked", name)
		}
	}
}

// SPEC-016 E3: a fold made before the first OS-scoped rule is replaced
// by the flat directory, and the next sync changes nothing.
func TestSync_ClineUnfoldsWhenOSScopedRuleAppears(t *testing.T) {
	app, root, buf := newSyncApp(t, "cline")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	agents := filepath.Join(root, ".agents")
	seedOSScopedRules(t, agents)
	if err := app.CmdSync(); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	assertResolvesTo(t, filepath.Join(root, ".cline", "rules", "macos-brew.md"), filepath.Join(agents, "rules", "macos", "brew.md"))

	buf.Reset()
	if err := app.CmdSync(); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	if strings.Contains(buf.String(), "Linked") || strings.Contains(buf.String(), "Removed") {
		t.Errorf("third sync changed links:\n%s", buf.String())
	}
}

// SPEC-016 E3: a real .cline/rules is the user's; flat links go in
// beside their files.
func TestSync_ClineFlatLinksBesideUserRules(t *testing.T) {
	app, root, _ := newSyncApp(t, "cline")
	agents := filepath.Join(root, ".agents")
	seedOSScopedRules(t, agents)
	writeFile(t, filepath.Join(root, ".cline", "rules", "mine.md"), "hand-written")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, filepath.Join(root, ".cline", "rules", "mine.md")); got != "hand-written" {
		t.Errorf("mine.md = %q", got)
	}
	assertResolvesTo(t, filepath.Join(root, ".cline", "rules", "macos-brew.md"), filepath.Join(agents, "rules", "macos", "brew.md"))
}

// SPEC-016 E3: status counts the flat links, not the scope directories.
func TestStatus_ClineFlatRulesMerged(t *testing.T) {
	app, root, buf := newSyncApp(t, "cline")
	seedOSScopedRules(t, filepath.Join(root, ".agents"))
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	buf.Reset()
	if err := app.CmdStatus(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if want := "[merged] rules (3/3 linked)"; !strings.Contains(buf.String(), want) {
		t.Errorf("missing %q in:\n%s", want, buf.String())
	}
}

// SPEC-016 E3: a scope that does not match the host hides nothing Cline
// would read, so the fold stays.
func TestSync_ClineFoldsWhenNoScopeMatchesHost(t *testing.T) {
	app, root, _ := newSyncApp(t, "cline")
	agents := filepath.Join(root, ".agents")
	writeFile(t, filepath.Join(agents, "config"), "os = darwin\n")
	writeFile(t, filepath.Join(agents, "rules", "linux", "apt.md"), "apt")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	assertResolvesTo(t, filepath.Join(root, ".cline", "rules"), filepath.Join(agents, "rules"))
}

// SPEC-016 E3 at user scope: the host's OS-scoped rules are linked flat.
func TestGlobalSync_ClineOSScopedRulesFlat(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedOSScopedRules(t, a.GlobalRoot)
	globalSync(t, a, "cline")
	assertResolvesTo(t, filepath.Join(root, ".cline", "rules", "macos-brew.md"), filepath.Join(a.GlobalRoot, "rules", "macos", "brew.md"))
	assertResolvesTo(t, filepath.Join(root, ".cline", "rules", "unix-posix.md"), filepath.Join(a.GlobalRoot, "rules", "unix", "posix.md"))
	if exists(filepath.Join(root, ".cline", "rules", "linux-apt.md")) {
		t.Error("a linux rule was linked on darwin")
	}
}

// SPEC-016 E3: a fold that hides an OS-scoped rule from Cline is not
// reported as synced.
func TestStatus_ClineFoldHidingScopedRulesIsStale(t *testing.T) {
	app, root, buf := newSyncApp(t, "cline")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	seedOSScopedRules(t, filepath.Join(root, ".agents"))
	buf.Reset()
	if err := app.CmdStatus(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if want := "[stale] rules -> ../.agents/rules (cline reads one level deep; sync links the OS-scoped entries flat)"; !strings.Contains(buf.String(), want) {
		t.Errorf("missing %q in:\n%s", want, buf.String())
	}
}

// In $HOME the project tree is the global tree, and $HOME/.cline is
// Cline's user dir, which global sync links each rule and workflow
// into. A fold there would give the dir two owners, and with $CLINE_DIR
// set Cline would load every rule twice when it opens $HOME (once as
// workspace rules, once as user rules). Local sync defers to global.
func TestSync_ClineDefersToGlobalWhenProjectIsGlobalRoot(t *testing.T) {
	app, root, buf := newSyncApp(t, "cline")
	app.GlobalRoot = filepath.Join(root, ".agents")
	writeFile(t, filepath.Join(root, ".agents", "workflows", "ship.md"), "workflow")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	for _, dir := range []string{"rules", "workflows"} {
		if exists(filepath.Join(root, ".cline", dir)) {
			t.Errorf("sync placed ~/.cline/%s although global sync delivers it", dir)
		}
		want := "cline " + dir + ": skipped at project scope: this project's .agents/ is the global root, so `sync-agents global sync` delivers it"
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}

const (
	clinerulesNote = ".clinerules/ is yours and left alone; Cline reads it as well as .cline/rules"
	dupSkillsNote  = "Cline reads .claude/skills as well as .agents/skills, so each skill linked for claude loads twice"
)

// SPEC-016 E8
func TestSync_ClineLeavesClinerulesAlone(t *testing.T) {
	app, root, buf := newSyncApp(t, "cline")
	writeFile(t, filepath.Join(root, ".clinerules", "mine.md"), "hand-written")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, filepath.Join(root, ".clinerules", "mine.md")); got != "hand-written" {
		t.Errorf(".clinerules/mine.md = %q", got)
	}
	if fi, err := os.Lstat(filepath.Join(root, ".clinerules")); err != nil || !fi.IsDir() {
		t.Errorf(".clinerules is no longer a real directory: %v", err)
	}
	if want := "cline: " + clinerulesNote; !strings.Contains(buf.String(), want) {
		t.Errorf("missing %q in:\n%s", want, buf.String())
	}
	buf.Reset()
	if err := app.CmdStatus(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if want := "[note] " + clinerulesNote; !strings.Contains(buf.String(), want) {
		t.Errorf("missing %q in:\n%s", want, buf.String())
	}
}

// SPEC-016 E8: nothing to report without a .clinerules/.
func TestSync_ClineNoClinerulesNoNote(t *testing.T) {
	app, _, buf := newSyncApp(t, "cline")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if strings.Contains(buf.String(), ".clinerules") {
		t.Errorf("unexpected .clinerules report:\n%s", buf.String())
	}
}

// SPEC-016 edge audit: Cline reads .claude/skills and .agents/skills,
// so a project targeting claude and cline shows each skill twice.
func TestStatus_ClineDuplicateSkills(t *testing.T) {
	for _, tc := range []struct {
		targets []string
		want    bool
	}{
		{[]string{"claude", "cline"}, true},
		{[]string{"cline", "claude"}, true},
		{[]string{"cline"}, false},
		{[]string{"claude"}, false},
	} {
		app, _, buf := newSyncApp(t, tc.targets...)
		if err := app.CmdSync(); err != nil {
			t.Fatalf("%v sync: %v", tc.targets, err)
		}
		buf.Reset()
		if err := app.CmdStatus(); err != nil {
			t.Fatalf("%v status: %v", tc.targets, err)
		}
		if got := strings.Contains(buf.String(), "[note] "+dupSkillsNote); got != tc.want {
			t.Errorf("%v: duplicate-skills note = %v, want %v:\n%s", tc.targets, got, tc.want, buf.String())
		}
	}
}
