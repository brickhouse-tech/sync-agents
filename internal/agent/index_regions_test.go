package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workspaceAgentsFixture is a representative OpenClaw workspace
// AGENTS.md: index output plus the `## Tools` section OpenClaw's
// doctor appends at the end of the file.
func workspaceAgentsFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "openclaw-workspace-AGENTS.md"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return string(data)
}

const foreignRegionFixture = "<!-- sync-agents:openclaw-rules:start -->\n" +
	"<!-- managed by sync-agents; do not edit between the markers -->\n" +
	"## git\n\nCommit small.\n\n" +
	"<!-- sync-agents:openclaw-rules:end -->\n"

func TestCmdIndex_PreservesForeignRegionAndTools(t *testing.T) {
	a, root, _ := newLocalIndexTestApp(t)
	for _, name := range []string{"git", "testing"} {
		writeArtifact(t, root, filepath.Join(".agents", "rules", name+".md"), "body\n")
	}
	agentsMD := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agentsMD, []byte(workspaceAgentsFixture(t)+"\n"+foreignRegionFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.CmdIndex(); err != nil {
		t.Fatalf("index: %v", err)
	}
	first := readFile(t, agentsMD)

	if !strings.Contains(first, foreignRegionFixture) {
		t.Errorf("foreign region lost or altered:\n%s", first)
	}
	if !strings.Contains(first, "## Tools\n\nSkills define how tools work. Keep local notes here.\n\n### Cameras\n\n- front-door: 1080p, motion alerts on\n") {
		t.Errorf("## Tools section lost:\n%s", first)
	}
	if !strings.Contains(first, "## Inherits\n\n@SOUL.md\n@IDENTITY.md\n@USER.md\n") {
		t.Errorf("## Inherits section lost:\n%s", first)
	}
	if strings.Count(first, "## git") != 1 {
		t.Errorf("region heading duplicated outside the region:\n%s", first)
	}
	if strings.Count(first, ClaudeImportsRegion.Start()) != 1 {
		t.Errorf("want exactly one claude-imports block:\n%s", first)
	}

	if err := a.CmdIndex(); err != nil {
		t.Fatalf("second index: %v", err)
	}
	if second := readFile(t, agentsMD); second != first {
		t.Errorf("index not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestCmdIndex_ToolsHeadingMatchedCaseInsensitively(t *testing.T) {
	a, root, _ := newLocalIndexTestApp(t)
	agentsMD := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agentsMD, []byte("# AGENTS\n\n## TOOLS\n\nkeep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.CmdIndex(); err != nil {
		t.Fatalf("index: %v", err)
	}
	if got := readFile(t, agentsMD); !strings.Contains(got, "## TOOLS\n\nkeep me\n") {
		t.Errorf("uppercase Tools section dropped:\n%s", got)
	}
}

func TestCmdIndex_NoPassiveRulesStillRegenerates(t *testing.T) {
	a, root, _ := newLocalIndexTestApp(t)
	agentsMD := filepath.Join(root, "AGENTS.md")
	stale := "# AGENTS\n\n## Rules\n\n- [gone](.agents/rules/gone.md)\n\n" +
		claudeImportsBlock([]string{".claude/rules/gone.md"})
	if err := os.WriteFile(agentsMD, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.CmdIndex(); err != nil {
		t.Fatalf("index: %v", err)
	}
	got := readFile(t, agentsMD)
	if strings.Contains(got, "gone") {
		t.Errorf("deleted rule survived regeneration:\n%s", got)
	}
	if !strings.Contains(got, "_No rules defined yet.") {
		t.Errorf("index not regenerated:\n%s", got)
	}
}

func TestForeignRegions(t *testing.T) {
	own := ClaudeImportsRegion.Start() + "\n@a\n" + ClaudeImportsRegion.End() + "\n"
	b := "<!-- sync-agents:b:start -->\nB\n<!-- sync-agents:b:end -->\n"
	c := "<!-- sync-agents:c:start -->\nC\n<!-- sync-agents:c:end -->"
	text := "x\n" + b + own + "<!-- sync-agents:broken:start -->\ny\n" + c
	got := foreignRegions(text, ClaudeImportsRegion)
	if len(got) != 2 || got[0] != b || got[1] != c {
		t.Errorf("got %q, want [%q %q]", got, b, c)
	}
}
