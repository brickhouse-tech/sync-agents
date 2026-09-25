package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCmdIndex_OSScopedEntriesBadged covers SPEC-006's index test
// plan: OS-scoped artifacts appear with an `[os]` badge, unscoped
// ones without, and — because AGENTS.md is a static, checked-in file —
// every platform's entries appear regardless of the host OS.
func TestCmdIndex_OSScopedEntriesBadged(t *testing.T) {
	a, root, _ := newLocalIndexTestApp(t)
	agents := filepath.Join(root, ".agents")
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(agents, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("rules/security.md", "---\ndescription: Locks down auth and secrets\n---\nbody\n")
	write("rules/macos/brew.md", "---\ndescription: Homebrew install audit\n---\nbody\n")
	write("rules/linux/apt.md", "---\ndescription: APT package audit\n---\nbody\n")
	write("rules/unix/posix.md", "body\n")
	write("skills/macos/xcode/SKILL.md", "---\nname: xcode\ndescription: Xcode helpers\n---\nbody\n")
	write("workflows/linux/deploy.md", "body\n")
	write("agents/windows/ps.md", "---\nname: ps\ndescription: PowerShell agent\n---\nbody\n")

	// Force a non-matching host: the index must still list macos/.
	write("config", "os = linux\n")

	if err := a.CmdIndex(); err != nil {
		t.Fatalf("CmdIndex: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	for _, want := range []string{
		"- [security](.agents/rules/security.md) — Locks down auth and secrets\n",
		"- [brew](.agents/rules/macos/brew.md) `[macos]` — Homebrew install audit\n",
		"- [apt](.agents/rules/linux/apt.md) `[linux]` — APT package audit\n",
		"- [posix](.agents/rules/unix/posix.md) `[unix]`\n",
		"- [xcode](.agents/skills/macos/xcode/SKILL.md) `[macos]` — Xcode helpers\n",
		"- [deploy](.agents/workflows/linux/deploy.md) `[linux]`\n",
		"- [ps](.agents/agents/windows/ps.md) `[windows]` — PowerShell agent\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("AGENTS.md missing %q\n---\n%s", want, s)
		}
	}
	if strings.Contains(s, "[security](.agents/rules/security.md) `[") {
		t.Errorf("unscoped entry must not carry a badge:\n%s", s)
	}
	// Badged entries sort after the unscoped ones, OS order macos→linux→unix.
	if strings.Index(s, "[brew]") > strings.Index(s, "[apt]") || strings.Index(s, "[apt]") > strings.Index(s, "[posix]") {
		t.Errorf("OS-scoped entries out of order:\n%s", s)
	}
}

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
