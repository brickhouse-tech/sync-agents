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

// Every ADR entry point warns that the bucket is going away (v4).

func TestADRDeprecation_ADRCommandWarns(t *testing.T) {
	a := newADRApp(t)
	writeADR(t, a, ADRStatusProposed, "use-postgres.md", "# Use Postgres\n")
	if err := a.CmdADR("accept", "use-postgres"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if out := a.Stdout.(*bytes.Buffer).String(); !strings.Contains(out, adrDeprecation) {
		t.Errorf("adr command output missing deprecation warning:\n%s", out)
	}
}

func TestADRDeprecation_AddADRWarns(t *testing.T) {
	a := newADRApp(t)
	if err := a.CmdAdd("adr", "use-postgres", AddOpts{}); err != nil {
		t.Fatalf("add adr: %v", err)
	}
	if out := a.Stdout.(*bytes.Buffer).String(); !strings.Contains(out, adrDeprecation) {
		t.Errorf("add adr output missing deprecation warning:\n%s", out)
	}
}

func TestADRDeprecation_AddOtherTypeSilent(t *testing.T) {
	a := newADRApp(t)
	if err := a.CmdAdd("rule", "style", AddOpts{}); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if out := a.Stdout.(*bytes.Buffer).String(); strings.Contains(out, "ADRs are deprecated") {
		t.Errorf("add rule should not warn about ADRs:\n%s", out)
	}
}

func TestADRDeprecation_SyncWarnsOnlyWhenBucketExists(t *testing.T) {
	app, _, buf := newSyncApp(t, "claude")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if strings.Contains(buf.String(), "ADRs are deprecated") {
		t.Fatalf("sync without adrs/ should not warn:\n%s", buf)
	}

	app, root, buf := newSyncApp(t, "claude")
	writeFile(t, filepath.Join(root, ".agents", "adrs", "accepted", "db.md"), "# DB\n")
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !strings.Contains(buf.String(), adrDeprecation) {
		t.Errorf("sync with adrs/ missing deprecation warning:\n%s", buf)
	}
}

func TestADRDeprecation_GlobalSyncWarnsWhenBucketExists(t *testing.T) {
	a, root, buf := newGlobalSyncTestApp(t)
	writeArtifact(t, filepath.Join(root, ".agents"), filepath.Join("adrs", "accepted", "db.md"), "# DB\n")
	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("global sync: %v", err)
	}
	if !strings.Contains(buf.String(), adrDeprecation) {
		t.Errorf("global sync with adrs/ missing deprecation warning:\n%s", buf)
	}
}
