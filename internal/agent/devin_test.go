package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// SPEC-014 Part A: the devin target.

// SPEC-014 E1
func TestDevinTool_Dirs(t *testing.T) {
	devin := asTool(t, "devin")
	if got, want := devin.DirForScope(ScopeLocal, "/proj"), filepath.Join("/proj", ".devin"); got != want {
		t.Errorf("devin local dir = %q, want %q", got, want)
	}
	if got, want := devin.DirForScope(ScopeGlobal, "/home/u"), filepath.Join("/home/u", ".config", "devin"); got != want {
		t.Errorf("devin global dir = %q, want %q", got, want)
	}
}

// SPEC-014 E2
func TestSync_DevinFoldsRulesOnly(t *testing.T) {
	app, root, _ := newSyncApp(t, "devin")
	writeFile(t, filepath.Join(root, ".agents", "agents", "reviewer.md"), "agent")
	writeFile(t, filepath.Join(root, ".agents", "workflows", "ship.md"), "workflow")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	assertResolvesTo(t, filepath.Join(root, ".devin", "rules"), filepath.Join(root, ".agents", "rules"))
	// SPEC-014 E4, E5, E6: Devin reads .agents/skills and .agents/agents
	// itself and has no workflow surface.
	for _, dir := range []string{"skills", "agents", "workflows"} {
		if exists(filepath.Join(root, ".devin", dir)) {
			t.Errorf(".devin/%s was linked; Devin must not receive it at project scope", dir)
		}
	}
}

// SPEC-014 E2a
func TestSync_DevinRulesServedByLegacyWindsurfLink(t *testing.T) {
	app, root, buf := newSyncApp(t, "devin")
	if err := os.MkdirAll(filepath.Join(root, ".windsurf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", ".agents", "rules"), filepath.Join(root, ".windsurf", "rules")); err != nil {
		t.Fatal(err)
	}
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if exists(filepath.Join(root, ".devin", "rules")) {
		t.Error(".devin/rules was folded although .windsurf/rules already serves the rules to Devin")
	}
	if !strings.Contains(buf.String(), "devin rules served by legacy .windsurf/rules link") {
		t.Errorf("missing E2a report:\n%s", buf.String())
	}
	// SPEC-014 E10: the legacy link stays.
	assertResolvesTo(t, filepath.Join(root, ".windsurf", "rules"), filepath.Join(root, ".agents", "rules"))
}

// SPEC-014 E2a: only a link into .agents/ withholds the fold.
func TestSync_DevinFoldsRulesBesideForeignWindsurfRules(t *testing.T) {
	app, root, _ := newSyncApp(t, "devin")
	writeFile(t, filepath.Join(root, ".windsurf", "rules", "mine.md"), "hand-written")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	assertResolvesTo(t, filepath.Join(root, ".devin", "rules"), filepath.Join(root, ".agents", "rules"))
}

// SPEC-014 E3
func TestGlobalSync_DevinRulesRegion(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	big := strings.Repeat("Never commit secrets. ", 400) + "\n"
	seedRule(t, a.GlobalRoot, "security", big)
	globalSync(t, a, "devin")

	host := filepath.Join(root, ".config", "devin", "AGENTS.md")
	got := readFile(t, host)
	want := DevinRulesRegion.Start() + "\n" + globalBanner + "\n\n## security\n\n" + big + "\n" + DevinRulesRegion.End() + "\n"
	if got != want {
		t.Errorf("%s =\n%q\nwant\n%q", host, got, want)
	}
	if DevinRulesRegion.Name != "devin-rules" {
		t.Errorf("region name = %q, want devin-rules", DevinRulesRegion.Name)
	}
}

// SPEC-014 E3: the host is the user's file; text outside the region
// survives.
func TestGlobalSync_DevinRegionKeepsUserText(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	host := filepath.Join(root, ".config", "devin", "AGENTS.md")
	writeFile(t, host, "Prefer tabs.\n")
	globalSync(t, a, "devin")

	got := readFile(t, host)
	if !strings.HasPrefix(got, "Prefer tabs.\n") || !strings.Contains(got, "Never commit secrets.") {
		t.Errorf("host = %q", got)
	}
}

// SPEC-014 E4, E5, E6
func TestTargetDestination_Devin(t *testing.T) {
	devin := asTool(t, "devin")
	cases := []struct {
		name     string
		typ      ArtifactType
		sem      Semantic
		strategy DestinationStrategy
		path     string
		reason   string
	}{
		{"skill", ArtifactSkill, Invocable, StrategySkip, "", "Devin loads ~/.agents/skills natively"},
		{"agent", ArtifactAgent, Invocable, StrategySymlink, filepath.Join("/home/u", ".config", "devin", "agents", "reviewer.md"), ""},
		{"workflow", ArtifactWorkflow, Invocable, StrategySkip, "", "Devin has no workflow surface; make it a skill"},
		{"invocable rule", ArtifactRule, Invocable, StrategySkip, "", "Devin has no workflow surface; make it a skill"},
		{"passive rule", ArtifactRule, Passive, StrategyChannel, "", ""},
	}
	for _, c := range cases {
		d := TargetDestination(devin, c.typ, "reviewer", c.sem, "", "/home/u")
		if d.Strategy != c.strategy || d.Path != c.path || d.SkipReason != c.reason {
			t.Errorf("%s: got %+v, want strategy %v path %q reason %q", c.name, d, c.strategy, c.path, c.reason)
		}
	}
}

// SPEC-014 E5
func TestGlobalSync_DevinAgentsLink(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	writeArtifact(t, a.GlobalRoot, filepath.Join("agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.\n")
	globalSync(t, a, "devin")
	assertResolvesTo(t, filepath.Join(root, ".config", "devin", "agents", "reviewer.md"), filepath.Join(a.GlobalRoot, "agents", "reviewer.md"))
}

// SPEC-014 E3: like every tool with a global channel, devin is left
// alone until it is installed or named.
func TestGlobalSync_DevinNotInstalledGetsNothing(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	globalSync(t, a)
	if exists(filepath.Join(root, ".config", "devin")) {
		t.Error("global sync created ~/.config/devin for a Devin that is not installed")
	}
	mkdirs(t, filepath.Join(root, ".config", "devin"))
	globalSync(t, a)
	if !exists(filepath.Join(root, ".config", "devin", "AGENTS.md")) {
		t.Error("installed Devin did not get ~/.config/devin/AGENTS.md")
	}
}

// SPEC-014 Part B: windsurf and codeium are deprecated names for devin.

func deprecationWarning(name string) string {
	return `warning: target "` + name + `" is deprecated; Windsurf is now Devin. Use "devin". Removed in 5.0.0.`
}

// stderrLines splits captured stderr into non-empty lines.
func stderrLines(buf *bytes.Buffer) []string {
	var out []string
	for _, l := range strings.Split(buf.String(), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// SPEC-014 E8, E9
func TestCanonicalTargets(t *testing.T) {
	got, deprecated := canonicalTargets([]string{"claude", "windsurf", "devin", "codeium", "cursor", "windsurf"})
	if want := []string{"claude", "devin", "cursor"}; !reflect.DeepEqual(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}
	if want := []string{"windsurf", "codeium"}; !reflect.DeepEqual(deprecated, want) {
		t.Errorf("deprecated = %v, want %v", deprecated, want)
	}
}

// SPEC-014 E8: a deprecated name in .agents/config syncs devin, with
// one warning on stderr.
func TestSync_DeprecatedConfigTargetSyncsDevin(t *testing.T) {
	app, root, _ := newSyncApp(t)
	writeFile(t, filepath.Join(root, ".agents", "config"), "targets = claude,windsurf\n")
	app.ActiveTargets = ReadConfigTargets(root)
	var stderr bytes.Buffer
	app.Stderr = &stderr
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	assertResolvesTo(t, filepath.Join(root, ".devin", "rules"), filepath.Join(root, ".agents", "rules"))
	if exists(filepath.Join(root, ".windsurf")) {
		t.Error("sync created .windsurf/ for a deprecated target")
	}
	if got, want := stderrLines(&stderr), []string{deprecationWarning("windsurf")}; !reflect.DeepEqual(got, want) {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// SPEC-014 E9: devin named twice over, through aliases and its own
// name, syncs once, and each alias warns once.
func TestSync_DevinAndAliasesSyncOnce(t *testing.T) {
	app, root, stdout := newSyncApp(t, "devin", "codeium", "windsurf", "codeium")
	var stderr bytes.Buffer
	app.Stderr = &stderr
	app.TargetsFromFlag = true
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n := strings.Count(stdout.String(), "Syncing to .devin/"); n != 1 {
		t.Errorf("devin synced %d times, want 1:\n%s", n, stdout.String())
	}
	want := []string{deprecationWarning("codeium"), deprecationWarning("windsurf")}
	if got := stderrLines(&stderr); !reflect.DeepEqual(got, want) {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	assertResolvesTo(t, filepath.Join(root, ".devin", "rules"), filepath.Join(root, ".agents", "rules"))
}

// SPEC-014 E8: --targets windsurf on global sync names devin, which
// opens devin's home gate as naming devin would.
func TestGlobalSync_DeprecatedFlagTargetSyncsDevin(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	var stderr bytes.Buffer
	a.Stderr = &stderr
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	globalSync(t, a, "windsurf")

	if !strings.Contains(readFile(t, filepath.Join(root, ".config", "devin", "AGENTS.md")), "Never commit secrets.") {
		t.Error("global sync --targets windsurf did not deliver to devin")
	}
	if exists(filepath.Join(root, ".codeium")) {
		t.Error("global sync --targets windsurf wrote ~/.codeium")
	}
	if got, want := stderrLines(&stderr), []string{deprecationWarning("windsurf")}; !reflect.DeepEqual(got, want) {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// SPEC-014 E8: the same for the targets line of ~/.agents/config.
func TestGlobalSync_DeprecatedConfigTargetSyncsDevin(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	var stderr bytes.Buffer
	a.Stderr = &stderr
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	writeFile(t, filepath.Join(a.GlobalRoot, "config"), "targets = claude,codeium\n")
	mkdirs(t, filepath.Join(root, ".config", "devin"))
	globalSync(t, a)

	if !strings.Contains(readFile(t, filepath.Join(root, ".config", "devin", "AGENTS.md")), "Never commit secrets.") {
		t.Error("targets = codeium did not deliver to devin")
	}
	if got, want := stderrLines(&stderr), []string{deprecationWarning("codeium")}; !reflect.DeepEqual(got, want) {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// seedCodeiumLeftovers writes what global sync placed for codeium
// before SPEC-014: the codeium-rules region and one workflow link.
func seedCodeiumLeftovers(t *testing.T, a *App, root string) (host, link string) {
	t.Helper()
	seedWorkflow(t, a.GlobalRoot, "ship", "---\ninvocable: true\n---\nShip it.\n")
	host = filepath.Join(root, ".codeium", "windsurf", "memories", "global_rules.md")
	writeFile(t, host, "Remember: tabs.\n\n"+CodeiumRulesRegion.Start()+"\nold rules\n"+CodeiumRulesRegion.End()+"\n")
	link = filepath.Join(root, ".codeium", "windsurf", "global_workflows", "ship.md")
	mkdirs(t, filepath.Dir(link))
	if err := os.Symlink(filepath.Join(a.GlobalRoot, "workflows", "ship.md"), link); err != nil {
		t.Fatal(err)
	}
	return host, link
}

// SPEC-014 E10: no run touches what earlier versions wrote for codeium.
func TestGlobal_CodeiumLeftoversStay(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	host, link := seedCodeiumLeftovers(t, a, root)
	before := readFile(t, host)

	globalSync(t, a)
	globalSync(t, a, "codeium")
	if err := a.CmdGlobalClean(GlobalCleanOpts{}); err != nil {
		t.Fatalf("global clean: %v", err)
	}
	if err := a.CmdGlobalClean(GlobalCleanOpts{Targets: []string{"windsurf"}}); err != nil {
		t.Fatalf("global clean: %v", err)
	}

	if got := readFile(t, host); got != before {
		t.Errorf("codeium region host changed:\n%q\nwant\n%q", got, before)
	}
	assertResolvesTo(t, link, filepath.Join(a.GlobalRoot, "workflows", "ship.md"))
}

// SPEC-014 E7
func TestInit_DefaultTargetsNameDevin(t *testing.T) {
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
		{filepath.Join(project, ".agents", "config"), []string{"claude", "devin", "cursor", "copilot"}},
		{filepath.Join(a.GlobalRoot, "config"), []string{"claude", "devin", "cursor", "copilot", "codex"}},
	} {
		if got := parseConfigTargets([]byte(readFile(t, c.config))); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s targets = %v, want %v", c.config, got, c.want)
		}
	}
	if got, want := AllTargets, []string{"claude", "devin", "cursor", "copilot"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AllTargets = %v, want %v", got, want)
	}
}
