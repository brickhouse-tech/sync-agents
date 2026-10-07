package agent

import (
	"strings"
	"testing"
)

// TestBuildEntriesBody_OSHeader covers SPEC-006's bundle test plan: an
// OS-scoped entry is preceded by `<!-- OS: <scope> -->`, an unscoped
// entry gets no header.
func TestBuildEntriesBody_OSHeader(t *testing.T) {
	tmp := t.TempDir()
	plain := writeArtifact(t, tmp, "rules/security.md", "security body\n")
	scoped := writeArtifact(t, tmp, "rules/macos/brew.md", "brew body\n")

	data, err := buildEntriesBody([]ConcatEntry{
		{Name: "security", SourcePath: plain},
		{Name: "macos/brew", SourcePath: scoped},
	})
	if err != nil {
		t.Fatalf("buildEntriesBody: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "<!-- OS: macos -->\n## macos/brew\n") {
		t.Errorf("scoped entry missing OS header:\n%s", s)
	}
	if strings.Count(s, "<!-- OS:") != 1 {
		t.Errorf("unscoped entry must not get an OS header:\n%s", s)
	}
}
