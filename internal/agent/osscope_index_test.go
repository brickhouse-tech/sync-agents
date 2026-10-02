package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRegenerateConcat_OSHeader covers SPEC-006's concat test plan:
// an OS-scoped entry is preceded by `<!-- OS: <scope> -->`, an
// unscoped entry gets no header.
func TestRegenerateConcat_OSHeader(t *testing.T) {
	tmp := t.TempDir()
	plain := writeArtifact(t, tmp, "rules/security.md", "security body\n")
	scoped := writeArtifact(t, tmp, "rules/macos/brew.md", "brew body\n")
	out := filepath.Join(tmp, "windsurf", "global_rules.md")

	if _, err := RegenerateConcat(out, []ConcatEntry{
		{Name: "security", SourcePath: plain},
		{Name: "macos/brew", SourcePath: scoped},
	}); err != nil {
		t.Fatalf("RegenerateConcat: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "<!-- OS: macos -->\n## macos/brew\n") {
		t.Errorf("scoped entry missing OS header:\n%s", s)
	}
	if strings.Count(s, "<!-- OS:") != 1 {
		t.Errorf("unscoped entry must not get an OS header:\n%s", s)
	}
}
