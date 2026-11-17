package agent

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// SPEC-015 Part B: the gemini target (Gemini CLI), opt-in.

// SPEC-015 B1
func TestGeminiTool_Dirs(t *testing.T) {
	g := asTool(t, "gemini")
	if got, want := g.DirForScope(ScopeLocal, "/proj"), filepath.Join("/proj", ".gemini"); got != want {
		t.Errorf("gemini local dir = %q, want %q", got, want)
	}
	if got, want := g.DirForScope(ScopeGlobal, "/home/u"), filepath.Join("/home/u", ".gemini"); got != want {
		t.Errorf("gemini global dir = %q, want %q", got, want)
	}
}

// SPEC-015 B1: gemini is selected only by name.
func TestGemini_NotInDefaultTargets(t *testing.T) {
	a, _, _ := newGlobalSyncTestApp(t)
	tools, err := a.resolveSyncTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, tool := range tools {
		ids = append(ids, tool.ID)
	}
	if slices.Contains(ids, "gemini") {
		t.Errorf("default global tools %v include gemini", ids)
	}
	if !slices.Contains(ids, "antigravity") {
		t.Errorf("default global tools %v lack antigravity", ids)
	}
	if slices.Contains(AllTargets, "gemini") {
		t.Errorf("AllTargets %v include gemini", AllTargets)
	}
	named, err := a.resolveSyncTools([]string{"gemini"})
	if err != nil || len(named) != 1 || named[0].ID != "gemini" {
		t.Errorf("--targets gemini resolved to %v, %v", named, err)
	}
}

// SPEC-015 B2
func TestGlobalSync_GeminiRulesRegion(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	body := "---\ntrigger: always_on\n---\nNever commit secrets.\n"
	seedRule(t, a.GlobalRoot, "security", body)
	globalSync(t, a, "gemini")

	host := filepath.Join(root, ".gemini", "GEMINI.md")
	got := readFile(t, host)
	want := GeminiRulesRegion.Start() + "\n" + globalBanner + "\n\n## security\n\nNever commit secrets.\n\n" + GeminiRulesRegion.End() + "\n"
	if got != want {
		t.Errorf("%s =\n%q\nwant\n%q", host, got, want)
	}
	if GeminiRulesRegion.Name != "gemini-rules" {
		t.Errorf("region name = %q, want gemini-rules", GeminiRulesRegion.Name)
	}
}

// SPEC-015 B3: without a GEMINI.md, Gemini CLI reads no rules file
// sync-agents may edit, so nothing is written.
func TestSync_GeminiWithoutGeminiMDWritesNothing(t *testing.T) {
	app, root, buf := newSyncApp(t, "gemini")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if exists(filepath.Join(root, "GEMINI.md")) {
		t.Error("sync created GEMINI.md")
	}
	if exists(filepath.Join(root, ".agents", "index", "gemini.md")) {
		t.Error("sync wrote .agents/index/gemini.md for a channel it cannot mount")
	}
	if want := "Gemini CLI reads AGENTS.md only when context.fileName lists it"; !strings.Contains(buf.String(), want) {
		t.Errorf("missing %q in:\n%s", want, buf.String())
	}
}

// SPEC-015 B3: an existing GEMINI.md gets the gemini-rules region; the
// user's text stays.
func TestSync_GeminiRegionInExistingGeminiMD(t *testing.T) {
	app, root, _ := newSyncApp(t, "gemini")
	app.TargetsFromFlag = true
	host := filepath.Join(root, "GEMINI.md")
	writeFile(t, host, "Prefer tabs.\n")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got := readFile(t, host)
	if !strings.HasPrefix(got, "Prefer tabs.\n") {
		t.Errorf("the user's text was not kept:\n%s", got)
	}
	if !strings.Contains(got, GeminiRulesRegion.Start()) || !strings.Contains(got, "agents rule") {
		t.Errorf("GEMINI.md lacks the gemini-rules region with the rule:\n%s", got)
	}
}

// SPEC-015 B4, B5
func TestTargetDestination_Gemini(t *testing.T) {
	g := asTool(t, "gemini")
	cases := []struct {
		name     string
		typ      ArtifactType
		sem      Semantic
		strategy DestinationStrategy
		path     string
		reason   string
	}{
		{"agent", ArtifactAgent, Invocable, StrategySymlink, filepath.Join("/home/u", ".gemini", "agents", "reviewer.md"), ""},
		{"skill", ArtifactSkill, Invocable, StrategySkip, "", "Gemini CLI loads ~/.agents/skills natively"},
		{"workflow", ArtifactWorkflow, Invocable, StrategySkip, "", "TOML commands are not generated"},
		{"invocable rule", ArtifactRule, Invocable, StrategySkip, "", "TOML commands are not generated"},
		{"passive rule", ArtifactRule, Passive, StrategyChannel, "", ""},
	}
	for _, c := range cases {
		d := TargetDestination(g, c.typ, "reviewer", c.sem, "", "/home/u")
		if d.Strategy != c.strategy || d.Path != c.path || d.SkipReason != c.reason {
			t.Errorf("%s: got %+v, want strategy %v path %q reason %q", c.name, d, c.strategy, c.path, c.reason)
		}
	}
}

// SPEC-015 B4: at project scope only agents are linked; Gemini CLI
// reads .agents/skills natively and has no Markdown workflows.
func TestSync_GeminiLinksAgentsOnly(t *testing.T) {
	app, root, _ := newSyncApp(t, "gemini")
	writeFile(t, filepath.Join(root, ".agents", "agents", "reviewer.md"), "agent")
	writeFile(t, filepath.Join(root, ".agents", "workflows", "ship.md"), "workflow")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	assertResolvesTo(t, filepath.Join(root, ".gemini", "agents"), filepath.Join(root, ".agents", "agents"))
	for _, dir := range []string{"rules", "skills", "workflows"} {
		if exists(filepath.Join(root, ".gemini", dir)) {
			t.Errorf(".gemini/%s was linked; Gemini CLI must not receive it", dir)
		}
	}
}

// SPEC-015 B4
func TestGlobalSync_GeminiAgentsLink(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	writeArtifact(t, a.GlobalRoot, filepath.Join("agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.\n")
	globalSync(t, a, "gemini")
	assertResolvesTo(t, filepath.Join(root, ".gemini", "agents", "reviewer.md"), filepath.Join(a.GlobalRoot, "agents", "reviewer.md"))
}

// SPEC-015 edge audit: ~/.gemini/config is Antigravity's home inside
// Gemini CLI's, so cleaning gemini leaves Antigravity's links.
func TestGlobalClean_GeminiLeavesAntigravityLinks(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "---\ntrigger: always_on\n---\nNever commit secrets.\n")
	globalSync(t, a, "antigravity", "gemini")
	if err := a.CmdGlobalClean(GlobalCleanOpts{Targets: []string{"gemini"}}); err != nil {
		t.Fatalf("global clean: %v", err)
	}
	assertResolvesTo(t, filepath.Join(root, ".gemini", "config", "rules", "security.md"), filepath.Join(a.GlobalRoot, "rules", "security.md"))
	if exists(filepath.Join(root, ".gemini", "GEMINI.md")) {
		t.Error("global clean --targets gemini left the GEMINI.md it created")
	}
}

// SPEC-015 edge audit: Antigravity CLI also reads ~/.gemini/GEMINI.md,
// so targeting both loads global rules twice.
func TestGlobalSync_WarnsWhenAntigravityAndGeminiBothTargeted(t *testing.T) {
	const both = "antigravity and gemini are both targeted"
	a, _, buf := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "---\ntrigger: always_on\n---\nNever commit secrets.\n")
	globalSync(t, a, "antigravity", "gemini")
	if n := strings.Count(buf.String(), both); n != 1 {
		t.Errorf("%d double-load warnings, want 1:\n%s", n, buf.String())
	}
	buf.Reset()
	globalSync(t, a, "antigravity")
	globalSync(t, a, "gemini")
	if strings.Contains(buf.String(), both) {
		t.Errorf("double-load warning with one of the two targeted:\n%s", buf.String())
	}
}
