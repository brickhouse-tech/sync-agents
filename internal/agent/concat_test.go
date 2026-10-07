package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeArtifact creates a file at the given relative path under
// tmpDir, with the supplied content. Returns the absolute path.
func writeArtifact(t *testing.T, tmpDir, rel, content string) string {
	t.Helper()
	abs := filepath.Join(tmpDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("setup parent: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("setup file: %v", err)
	}
	return abs
}

// TestBuildEntriesBody_SortsByName: sections come out in name order
// regardless of input order, each heading followed by its body.
func TestBuildEntriesBody_SortsByName(t *testing.T) {
	tmp := t.TempDir()
	srcZ := writeArtifact(t, tmp, "rules/zeta.md", "zeta body\n")
	srcA := writeArtifact(t, tmp, "rules/alpha.md", "alpha body\n")

	got, err := buildEntriesBody([]ConcatEntry{
		{Name: "zeta", SourcePath: srcZ},
		{Name: "alpha", SourcePath: srcA},
	})
	if err != nil {
		t.Fatalf("buildEntriesBody: %v", err)
	}
	want := "## alpha\n\nalpha body\n\n## zeta\n\nzeta body\n\n"
	if string(got) != want {
		t.Errorf("body:\n got %q\nwant %q", got, want)
	}
}

// TestBuildEntriesBody_StripsFrontmatter: the source's YAML
// frontmatter is sync-agents metadata and never reaches the tool.
func TestBuildEntriesBody_StripsFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	src := writeArtifact(t, tmp, "rules/x.md",
		"---\ntrigger: always_on\ninvocable: false\n---\nactual body line\n")

	got, err := buildEntriesBody([]ConcatEntry{{Name: "x", SourcePath: src}})
	if err != nil {
		t.Fatalf("buildEntriesBody: %v", err)
	}
	if string(got) != "## x\n\nactual body line\n\n" {
		t.Errorf("body = %q", got)
	}
}

// TestBuildEntriesBody_MissingSourceErrors: an unreadable source is an
// error naming the path, so a channel is never written with a rule
// silently missing.
func TestBuildEntriesBody_MissingSourceErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	_, err := buildEntriesBody([]ConcatEntry{{Name: "missing", SourcePath: missing}})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("err = %v; want an error naming %s", err, missing)
	}
}
