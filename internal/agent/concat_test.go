package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// TestRegenerateConcat_BasicOrdering creates two rules and verifies
// the concat output has the banner, sorted headings, and bodies in
// alphabetical order regardless of input order.
func TestRegenerateConcat_BasicOrdering(t *testing.T) {
	tmp := t.TempDir()
	srcA := writeArtifact(t, tmp, "rules/zeta.md", "zeta body\n")
	srcB := writeArtifact(t, tmp, "rules/alpha.md", "alpha body\n")
	out := filepath.Join(tmp, "out", "concat.md")

	// Intentionally pass them in reverse-alpha order to exercise
	// the sort.
	entries := []ConcatEntry{
		{Name: "zeta", SourcePath: srcA},
		{Name: "alpha", SourcePath: srcB},
	}
	changed, err := RegenerateConcat(out, entries)
	if err != nil {
		t.Fatalf("RegenerateConcat: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on first write")
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read concat: %v", err)
	}
	got := string(content)

	if !strings.HasPrefix(got, ConcatBanner) {
		t.Errorf("concat should start with banner; got:\n%s", got[:min(len(got), 100)])
	}

	// alpha must appear before zeta.
	idxAlpha := strings.Index(got, "## alpha")
	idxZeta := strings.Index(got, "## zeta")
	if idxAlpha == -1 || idxZeta == -1 {
		t.Fatalf("missing heading(s): idxAlpha=%d idxZeta=%d in:\n%s", idxAlpha, idxZeta, got)
	}
	if idxAlpha >= idxZeta {
		t.Errorf("alpha (%d) should come before zeta (%d)", idxAlpha, idxZeta)
	}

	// Bodies present.
	if !strings.Contains(got, "alpha body") || !strings.Contains(got, "zeta body") {
		t.Errorf("bodies missing from concat:\n%s", got)
	}
}

// TestRegenerateConcat_StripsFrontmatter verifies that the source
// artifact's YAML frontmatter does not appear in the concat output.
// Only the body content should leak through.
func TestRegenerateConcat_StripsFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	src := writeArtifact(t, tmp, "rules/x.md",
		"---\ntrigger: always_on\ninvocable: false\n---\nactual body line\n")
	out := filepath.Join(tmp, "concat.md")

	_, err := RegenerateConcat(out, []ConcatEntry{{Name: "x", SourcePath: src}})
	if err != nil {
		t.Fatalf("RegenerateConcat: %v", err)
	}
	content, _ := os.ReadFile(out)
	got := string(content)

	if strings.Contains(got, "trigger: always_on") {
		t.Errorf("concat leaked frontmatter content:\n%s", got)
	}
	if !strings.Contains(got, "actual body line") {
		t.Errorf("concat missing body content:\n%s", got)
	}
}

// TestRegenerateConcat_IdempotentMtimePreserved is the SPEC-002
// idempotency contract: when the content is unchanged, the file is
// not rewritten. We check this by comparing mtime before and after a
// second RegenerateConcat call.
func TestRegenerateConcat_IdempotentMtimePreserved(t *testing.T) {
	tmp := t.TempDir()
	src := writeArtifact(t, tmp, "rules/x.md", "body\n")
	out := filepath.Join(tmp, "concat.md")

	if _, err := RegenerateConcat(out, []ConcatEntry{{Name: "x", SourcePath: src}}); err != nil {
		t.Fatalf("first regen: %v", err)
	}
	firstInfo, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Sleep a moment so any rewrite would produce a measurably
	// different mtime. Filesystem mtime resolution is typically
	// nanoseconds on Linux/macOS but second-level on some FUSE
	// setups; 1.1s comfortably exceeds anything we'd see in CI.
	time.Sleep(1100 * time.Millisecond)

	changed, err := RegenerateConcat(out, []ConcatEntry{{Name: "x", SourcePath: src}})
	if err != nil {
		t.Fatalf("second regen: %v", err)
	}
	if changed {
		t.Error("expected changed=false on second regen with same content")
	}
	secondInfo, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !secondInfo.ModTime().Equal(firstInfo.ModTime()) {
		t.Errorf("mtime changed despite identical content: first=%v second=%v",
			firstInfo.ModTime(), secondInfo.ModTime())
	}
}

// TestRegenerateConcat_ContentChangeWrites verifies the inverse: when
// content does change, the file is actually rewritten.
func TestRegenerateConcat_ContentChangeWrites(t *testing.T) {
	tmp := t.TempDir()
	src := writeArtifact(t, tmp, "rules/x.md", "original\n")
	out := filepath.Join(tmp, "concat.md")

	if _, err := RegenerateConcat(out, []ConcatEntry{{Name: "x", SourcePath: src}}); err != nil {
		t.Fatalf("first regen: %v", err)
	}

	// Change the source content.
	if err := os.WriteFile(src, []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}

	changed, err := RegenerateConcat(out, []ConcatEntry{{Name: "x", SourcePath: src}})
	if err != nil {
		t.Fatalf("second regen: %v", err)
	}
	if !changed {
		t.Error("expected changed=true when content changed")
	}

	got, _ := os.ReadFile(out)
	if !bytes.Contains(got, []byte("changed")) {
		t.Errorf("concat does not reflect new content:\n%s", got)
	}
	if bytes.Contains(got, []byte("original")) {
		t.Errorf("concat still contains old content:\n%s", got)
	}
}

// TestRegenerateConcat_MissingSourceAborts verifies that an
// unreadable source aborts the regeneration AND leaves any existing
// destination intact. This is the spec's "no destination writes on
// integrity failure" pattern applied to concat regen.
func TestRegenerateConcat_MissingSourceAborts(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "concat.md")

	// Seed the destination with known content so we can verify it's
	// preserved.
	if err := os.WriteFile(out, []byte("PREEXISTING\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := RegenerateConcat(out, []ConcatEntry{
		{Name: "missing", SourcePath: filepath.Join(tmp, "does-not-exist.md")},
	})
	if err == nil {
		t.Fatal("expected error from missing source; got nil")
	}

	got, _ := os.ReadFile(out)
	if string(got) != "PREEXISTING\n" {
		t.Errorf("destination was modified despite failed regen; got %q", got)
	}
}

// TestRegenerateConcat_CreatesParentDir confirms the destination
// dir is created if missing. Concat files often live a few levels
// deep (e.g. windsurf/memories/) and should not require a separate
// MkdirAll from the caller.
func TestRegenerateConcat_CreatesParentDir(t *testing.T) {
	tmp := t.TempDir()
	src := writeArtifact(t, tmp, "rules/x.md", "body\n")
	out := filepath.Join(tmp, "deep", "nested", "concat.md")

	if _, err := RegenerateConcat(out, []ConcatEntry{{Name: "x", SourcePath: src}}); err != nil {
		t.Fatalf("RegenerateConcat: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("expected file at %s: %v", out, err)
	}
}
