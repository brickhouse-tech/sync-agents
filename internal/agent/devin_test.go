package agent

import (
	"os"
	"path/filepath"
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
