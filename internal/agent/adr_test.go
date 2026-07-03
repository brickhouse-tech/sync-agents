package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newADRApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &App{
		ProjectRoot: root,
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
	}
}

func writeADR(t *testing.T, a *App, status, rel, content string) string {
	t.Helper()
	path := filepath.Join(a.ProjectRoot, ".agents", "adrs", status, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCmdADR_AcceptMovesAndUpdatesStatus(t *testing.T) {
	a := newADRApp(t)
	writeADR(t, a, ADRStatusProposed, "use-postgres.md", "---\nname: use-postgres\nstatus: proposed\n---\n\n# Use Postgres\n")

	if err := a.CmdADR("accept", "use-postgres"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	dst := filepath.Join(a.ProjectRoot, ".agents", "adrs", ADRStatusAccepted, "use-postgres.md")
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("moved file missing: %v", err)
	}
	if !strings.Contains(string(data), "status: accepted") {
		t.Fatalf("status frontmatter not updated:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(a.ProjectRoot, ".agents", "adrs", ADRStatusProposed, "use-postgres.md")); !os.IsNotExist(err) {
		t.Fatal("source not removed")
	}
}

func TestCmdADR_DenyRemovesFromIndex(t *testing.T) {
	a := newADRApp(t)
	writeADR(t, a, ADRStatusProposed, "use-mongo.md", "---\nname: use-mongo\ndescription: Adopt Mongo. Use when persisting documents.\nstatus: proposed\n---\n")
	writeADR(t, a, ADRStatusAccepted, "use-postgres.md", "---\nname: use-postgres\ndescription: Adopt Postgres. Use when persisting relational data.\nstatus: accepted\n---\n")

	if err := a.CmdADR("deny", "use-mongo"); err != nil {
		t.Fatalf("deny: %v", err)
	}
	idx := readFile(t, filepath.Join(a.ProjectRoot, "AGENTS.md"))
	if !strings.Contains(idx, "## ADRs") || !strings.Contains(idx, "### Accepted") {
		t.Fatalf("ADRs section missing:\n%s", idx)
	}
	if strings.Contains(idx, "use-mongo") {
		t.Fatalf("denied ADR still indexed:\n%s", idx)
	}
	if !strings.Contains(idx, "adrs/denied") {
		t.Fatalf("denied-dir guidance note missing:\n%s", idx)
	}
}

func TestCmdADR_NestedAndErrors(t *testing.T) {
	a := newADRApp(t)
	// Nested grouping subdir resolves by basename.
	writeADR(t, a, ADRStatusProposed, "db/use-postgres.md", "---\nname: use-postgres\n---\n")
	if err := a.CmdADR("accept", "use-postgres"); err != nil {
		t.Fatalf("nested accept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.ProjectRoot, ".agents", "adrs", ADRStatusAccepted, "db", "use-postgres.md")); err != nil {
		t.Fatalf("nested path not preserved: %v", err)
	}

	// Missing name → error.
	if err := a.CmdADR("accept", "nope"); err == nil {
		t.Fatal("expected error for unknown ADR")
	}
	// Unknown action → error.
	if err := a.CmdADR("frobnicate", "use-postgres"); err == nil {
		t.Fatal("expected error for unknown action")
	}
	// Ambiguous name → error.
	writeADR(t, a, ADRStatusProposed, "dup.md", "x")
	writeADR(t, a, ADRStatusDenied, "dup.md", "x")
	if err := a.CmdADR("accept", "dup"); err == nil {
		t.Fatal("expected error for ambiguous ADR name")
	}
	// Already in target status → no-op success.
	if err := a.CmdADR("accept", "use-postgres"); err != nil {
		t.Fatalf("no-op accept should succeed: %v", err)
	}
}
