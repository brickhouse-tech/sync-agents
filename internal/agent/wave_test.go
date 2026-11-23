package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// SPEC-017: the wave target, which mirrors Claude's delivery.

// SPEC-017 E1
func TestWaveTool_Dirs(t *testing.T) {
	wave := asTool(t, "wave")
	if got, want := wave.DirForScope(ScopeLocal, "/proj"), filepath.Join("/proj", ".wave"); got != want {
		t.Errorf("wave local dir = %q, want %q", got, want)
	}
	if got, want := wave.DirForScope(ScopeGlobal, "/home/u"), filepath.Join("/home/u", ".wave"); got != want {
		t.Errorf("wave global dir = %q, want %q", got, want)
	}
}

// SPEC-017 E2, E3
func TestSync_WaveLinksClaudeBuckets(t *testing.T) {
	app, root, buf := newSyncApp(t, "wave")
	for _, rel := range []string{
		filepath.Join("workflows", "ship.md"),
		filepath.Join("agents", "reviewer.md"),
		filepath.Join("plans", "p.md"),
		filepath.Join("specs", "s.md"),
	} {
		writeFile(t, filepath.Join(root, ".agents", rel), "body")
	}
	writeFile(t, filepath.Join(root, ".agents", "hooks", "lint.json"),
		`{"event":"PreToolUse","matcher":"Bash","hooks":[{"type":"command","command":"echo lint"}]}`)
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v\n%s", err, buf)
	}
	for _, b := range []string{"rules", "skills", "workflows", "agents"} {
		assertResolvesTo(t, filepath.Join(root, ".wave", b), filepath.Join(root, ".agents", b))
	}
	for _, p := range []string{
		filepath.Join(".wave", "plans"),
		filepath.Join(".wave", "specs"),
		filepath.Join(".wave", "settings.json"),
		filepath.Join(".claude", "settings.json"),
		"CLAUDE.md",
	} {
		assertAbsent(t, filepath.Join(root, p))
	}
}

// SPEC-017 E4
func TestTargetDestination_Wave(t *testing.T) {
	wave := asTool(t, "wave")
	home := filepath.Join("/home/u", ".wave")
	cases := []struct {
		name     string
		typ      ArtifactType
		art      string
		sem      Semantic
		strategy DestinationStrategy
		path     string
	}{
		{"passive rule", ArtifactRule, "security", Passive, StrategySymlink, filepath.Join(home, "rules", "security.md")},
		{"passive workflow", ArtifactWorkflow, "notes", Passive, StrategySymlink, filepath.Join(home, "rules", "notes.md")},
		{"invocable rule", ArtifactRule, "onboarding", Invocable, StrategySymlink, filepath.Join(home, "commands", "onboarding.md")},
		{"invocable workflow", ArtifactWorkflow, "ship", Invocable, StrategySymlink, filepath.Join(home, "commands", "ship.md")},
		{"invocable skill", ArtifactSkill, "deploy", Invocable, StrategySymlink, filepath.Join(home, "skills", "deploy", "SKILL.md")},
		{"agent", ArtifactAgent, "reviewer", Invocable, StrategySymlink, filepath.Join(home, "agents", "reviewer.md")},
		{"passive skill", ArtifactSkill, "x", Passive, StrategySkip, ""},
	}
	for _, c := range cases {
		d := TargetDestination(wave, c.typ, c.art, c.sem, "", "/home/u")
		if d.Strategy != c.strategy || d.Path != c.path {
			t.Errorf("%s: got %+v, want strategy %v path %q", c.name, d, c.strategy, c.path)
		}
	}
}

// SPEC-017 E4: Claude and Wave route every artifact to the same place
// relative to their own dir.
func TestTargetDestination_WaveMatchesClaude(t *testing.T) {
	claude, wave := asTool(t, "claude"), asTool(t, "wave")
	for _, typ := range []ArtifactType{ArtifactRule, ArtifactSkill, ArtifactWorkflow, ArtifactAgent} {
		for _, sem := range []Semantic{Passive, Invocable} {
			c := TargetDestination(claude, typ, "x", sem, "", "/home/u")
			w := TargetDestination(wave, typ, "x", sem, "", "/home/u")
			cRel, _ := filepath.Rel(filepath.Join("/home/u", ".claude"), c.Path)
			wRel, _ := filepath.Rel(filepath.Join("/home/u", ".wave"), w.Path)
			if c.Strategy != w.Strategy || cRel != wRel {
				t.Errorf("%s %v: claude %v %q, wave %v %q", typ, sem, c.Strategy, cRel, w.Strategy, wRel)
			}
		}
	}
}

// SPEC-017 E4
func TestGlobalSync_WaveRoutesLikeClaude(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	seedRule(t, a.GlobalRoot, "onboarding", "---\ninvocable: true\n---\nOnboard.\n")
	seedWorkflow(t, a.GlobalRoot, "ship", "Ship it.\n")
	seedSkill(t, a.GlobalRoot, "deploy", "---\nname: deploy\n---\nDeploy.\n", nil)
	writeArtifact(t, a.GlobalRoot, filepath.Join("agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.\n")
	globalSync(t, a, "wave")

	wave := filepath.Join(root, ".wave")
	for link, src := range map[string]string{
		filepath.Join(wave, "rules", "security.md"):         filepath.Join("rules", "security.md"),
		filepath.Join(wave, "commands", "onboarding.md"):    filepath.Join("rules", "onboarding.md"),
		filepath.Join(wave, "commands", "ship.md"):          filepath.Join("workflows", "ship.md"),
		filepath.Join(wave, "skills", "deploy", "SKILL.md"): filepath.Join("skills", "deploy", "SKILL.md"),
		filepath.Join(wave, "agents", "reviewer.md"):        filepath.Join("agents", "reviewer.md"),
	} {
		assertResolvesTo(t, link, filepath.Join(a.GlobalRoot, src))
	}
	assertAbsent(t, filepath.Join(root, ".claude"))
}

// SPEC-017 E5
func TestGlobalSync_WaveGetsNoClaudeOnlyArtifacts(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	writeArtifact(t, a.GlobalRoot, filepath.Join("plans", "p.md"), "plan")
	writeArtifact(t, a.GlobalRoot, filepath.Join("specs", "s.md"), "spec")
	writeArtifact(t, a.GlobalRoot, filepath.Join("hooks", "lint.json"),
		`{"event":"PreToolUse","matcher":"Bash","hooks":[{"type":"command","command":"echo lint"}]}`)
	globalSync(t, a, "wave")

	wave := filepath.Join(root, ".wave")
	assertResolvesTo(t, filepath.Join(wave, "rules", "security.md"), filepath.Join(a.GlobalRoot, "rules", "security.md"))
	for _, p := range []string{"plans", "specs", "settings.json", "CLAUDE.md", "AGENTS.md"} {
		assertAbsent(t, filepath.Join(wave, p))
	}
	assertAbsent(t, filepath.Join(root, ".claude"))
	if _, ok := channelSpecs["wave"]; ok {
		t.Error("wave has a channelSpecs row; it reads ~/.wave/rules natively")
	}
}

// SPEC-017 E6
func TestGlobalStatusAndClean_WaveLinks(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	seedRule(t, a.GlobalRoot, "gone", "Old.\n")
	seedSkill(t, a.GlobalRoot, "deploy", "---\nname: deploy\n---\nDeploy.\n", nil)
	writeArtifact(t, a.GlobalRoot, filepath.Join("agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.\n")
	globalSync(t, a, "wave")
	if err := os.Remove(filepath.Join(a.GlobalRoot, "rules", "gone.md")); err != nil {
		t.Fatal(err)
	}

	out := globalStatusOut(t, a, "wave")
	for _, want := range []string{
		"[synced] wave/rule/security",
		"[synced] wave/skill/deploy",
		"[synced] wave/agent/reviewer",
		"[orphaned] wave/gone.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	if err := a.CmdGlobalClean(GlobalCleanOpts{Targets: []string{"wave"}}); err != nil {
		t.Fatalf("global clean: %v", err)
	}
	for _, p := range []string{
		filepath.Join("rules", "security.md"),
		filepath.Join("rules", "gone.md"),
		filepath.Join("skills", "deploy", "SKILL.md"),
		filepath.Join("agents", "reviewer.md"),
	} {
		assertAbsent(t, filepath.Join(root, ".wave", p))
	}
}

// SPEC-017 E7
func TestWave_OptIn(t *testing.T) {
	if slices.Contains(AllTargets, "wave") {
		t.Fatal("wave must not be in AllTargets; it is opt-in via config or --targets")
	}
	if !asTool(t, "wave").OptIn {
		t.Error("wave.OptIn = false; registering it must not enroll it in default global sync")
	}
	app, root, _ := newSyncApp(t)
	if err := app.CmdInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	for name, body := range map[string]string{
		".agents/config":   readFile(t, filepath.Join(root, ".agents", "config")),
		"~/.agents/config": GlobalConfigContent,
	} {
		if targets := parseConfigTargets([]byte(body)); slices.Contains(targets, "wave") {
			t.Errorf("%s targets = %v; wave must be opt-in", name, targets)
		}
		if !strings.Contains(body, "available: ") || !strings.Contains(body, "wave") {
			t.Errorf("%s does not name wave among the available targets:\n%s", name, body)
		}
	}

	a, home, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	globalSync(t, a)
	assertAbsent(t, filepath.Join(home, ".wave"))
	writeFile(t, filepath.Join(a.GlobalRoot, "config"), "targets = wave\n")
	globalSync(t, a)
	assertResolvesTo(t, filepath.Join(home, ".wave", "rules", "security.md"), filepath.Join(a.GlobalRoot, "rules", "security.md"))
}
