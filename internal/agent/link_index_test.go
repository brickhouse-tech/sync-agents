package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// link_index_test.go covers the SPEC-007 §index guarantee: a linked
// skill — a symlink to a live checkout dir — must enumerate in AGENTS.md
// and DiscoverArtifacts exactly like a vendored one. os.DirEntry.IsDir()
// reports the symlink, not its target, so the discovery paths have to
// stat through the link.

func TestLinkedSkillIsIndexed(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".agents")
	if err := os.MkdirAll(filepath.Join(agentsDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Real checkout living outside the .agents tree.
	checkout := filepath.Join(root, "checkout", "linked-skill")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "SKILL.md"),
		[]byte("---\nname: linked-skill\ndescription: A linked skill. Use when testing SPEC-007.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Symlink the skill into the bucket (relative, as SPEC-007 wires it).
	dest := filepath.Join(agentsDir, "skills", "linked-skill")
	rel, err := filepath.Rel(filepath.Dir(dest), checkout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rel, dest); err != nil {
		t.Fatal(err)
	}

	// DiscoverArtifacts follows the symlink.
	arts, err := DiscoverArtifacts(agentsDir)
	if err != nil {
		t.Fatalf("DiscoverArtifacts: %v", err)
	}
	found := false
	for _, a := range arts {
		if a.Name == "linked-skill" && a.Type == ArtifactSkill {
			found = true
		}
	}
	if !found {
		t.Fatalf("linked skill not discovered: %+v", arts)
	}

	// AGENTS.md enumerates it.
	a := &App{ProjectRoot: root, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	a.generateAgentsMD()
	idx := readFile(t, filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(idx, "linked-skill") {
		t.Fatalf("linked skill missing from AGENTS.md:\n%s", idx)
	}
}
