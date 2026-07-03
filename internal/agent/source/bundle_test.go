package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBundle_RoundTrip(t *testing.T) {
	p, _ := newPuller(t)

	// A manually-imported rule carrying origin metadata (what
	// `import` writes when it can infer provenance).
	dest := filepath.Join(p.AgentsDir, "rules", "manual.md")
	os.MkdirAll(filepath.Dir(dest), 0o755)
	os.WriteFile(dest, []byte("# manual\n"), 0o644)
	if err := WriteOriginFor(dest, false, Origin{
		Schema: 1, Owner: "foo", Repo: "bar", Path: "rules/manual.md",
		Ref: "main", SHA: shaA, Source: SourceManual,
	}); err != nil {
		t.Fatal(err)
	}

	report, err := p.Bundle()
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if len(report.Added) != 1 {
		t.Fatalf("added = %v, want 1 entry", report.Added)
	}
	if len(report.Flipped) != 1 {
		t.Fatalf("flipped = %v, want 1 artifact", report.Flipped)
	}

	m, found, err := LoadManifest(p.AgentsDir)
	if err != nil || !found {
		t.Fatalf("manifest after bundle: found=%v err=%v", found, err)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("manifest sources = %v", m.Sources)
	}
	if _, err := ParseEntry(m.Sources[0]); err != nil {
		t.Fatalf("bundled entry %q does not round-trip through the parser: %v", m.Sources[0], err)
	}

	o, err := ReadOriginFor(dest, false)
	if err != nil {
		t.Fatal(err)
	}
	if o.Source != SourceManifest {
		t.Errorf("origin source = %q, want flipped to manifest", o.Source)
	}
}

func TestRemove_DeletesUnlessKeep(t *testing.T) {
	p, _ := newPuller(t)
	entry := "rule:foo/bar@v1/rules/security.md"
	saveManifestEntries(t, p, entry)
	if _, err := p.Pull(context.Background(), PullOpts{}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(p.AgentsDir, "rules", "security.md")

	// --keep leaves the artifact as a manual one.
	if err := p.Remove("security", true); err != nil {
		t.Fatalf("remove --keep: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal("--keep deleted the artifact")
	}
	o, err := ReadOriginFor(dest, false)
	if err != nil {
		t.Fatal(err)
	}
	if o.Source != SourceManual {
		t.Errorf("kept artifact source = %q, want manual", o.Source)
	}
	m, _, _ := LoadManifest(p.AgentsDir)
	if len(m.Sources) != 0 {
		t.Errorf("manifest still lists %v", m.Sources)
	}

	// Re-add and remove without --keep: artifact gone.
	saveManifestEntries(t, p, entry)
	if _, err := p.Pull(context.Background(), PullOpts{Force: true}); err != nil {
		t.Fatal(err)
	}
	if err := p.Remove("security", false); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("remove left the artifact behind")
	}
	lock, _ := LoadLock(p.AgentsDir)
	if lock.Find(entry) != nil {
		t.Error("remove left the lock entry behind")
	}
}

func TestDetach_FlipsToManualAndDropsEntry(t *testing.T) {
	p, _ := newPuller(t)
	entry := "rule:foo/bar@v1/rules/security.md"
	saveManifestEntries(t, p, entry)
	if _, err := p.Pull(context.Background(), PullOpts{}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(p.AgentsDir, "rules", "security.md")

	if err := p.Detach("security"); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal("detach deleted the artifact")
	}
	o, _ := ReadOriginFor(dest, false)
	if o.Source != SourceManual {
		t.Errorf("detached source = %q, want manual", o.Source)
	}
	m, _, _ := LoadManifest(p.AgentsDir)
	if len(m.Sources) != 0 {
		t.Errorf("detach left manifest entry %v", m.Sources)
	}
}

func TestAdd_AppendsAndPulls(t *testing.T) {
	p, _ := newPuller(t)
	saveManifestEntries(t, p, "rule:foo/bar@v1/rules/security.md")

	report, err := p.Add(context.Background(), "skill:foo/bar@v1/skills/grep-helper", PullOpts{})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := report.Count(ResultAdded); got != 1 {
		t.Fatalf("added = %d (%+v)", got, report.Results)
	}
	m, _, _ := LoadManifest(p.AgentsDir)
	if len(m.Sources) != 2 {
		t.Fatalf("manifest sources = %v, want 2", m.Sources)
	}
	if _, err := os.Stat(filepath.Join(p.AgentsDir, "skills", "grep-helper", "SKILL.md")); err != nil {
		t.Errorf("added skill not installed: %v", err)
	}

	// Invalid entry: rejected with the valid prefixes named.
	if _, err := p.Add(context.Background(), "notatype:foo/bar", PullOpts{}); err == nil {
		t.Fatal("invalid entry accepted")
	}
}
