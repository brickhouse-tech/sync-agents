package source

import (
	"strings"
	"testing"
)

func TestParseEntry_Forms(t *testing.T) {
	cases := []struct {
		in   string
		want Entry
		name string
	}{
		{
			in:   "skill:anthropic/skill-pack@v1.2.0/skills/code-review",
			want: Entry{Type: EntrySkill, Owner: "anthropic", Repo: "skill-pack", Ref: "v1.2.0", Path: "skills/code-review"},
			name: "code-review",
		},
		{
			in:   "rule:my-org/agent-norms@main/rules/security.md",
			want: Entry{Type: EntryRule, Owner: "my-org", Repo: "agent-norms", Ref: "main", Path: "rules/security.md"},
			name: "security",
		},
		{
			in:   "workflow:my-org/agent-norms@v0.4.1/workflows/release.md",
			want: Entry{Type: EntryWorkflow, Owner: "my-org", Repo: "agent-norms", Ref: "v0.4.1", Path: "workflows/release.md"},
			name: "release",
		},
		{
			in:   "tree:my-org/team-agents@v2.0.0",
			want: Entry{Type: EntryTree, Owner: "my-org", Repo: "team-agents", Ref: "v2.0.0"},
			name: "my-org/team-agents",
		},
		{
			// No ref → default branch; skill with no path → repo root.
			in:   "skill:foo/bar",
			want: Entry{Type: EntrySkill, Owner: "foo", Repo: "bar"},
			name: "bar",
		},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseEntry(c.in)
			if err != nil {
				t.Fatalf("ParseEntry(%q): %v", c.in, err)
			}
			if got.Type != c.want.Type || got.Owner != c.want.Owner || got.Repo != c.want.Repo ||
				got.Ref != c.want.Ref || got.Path != c.want.Path {
				t.Errorf("ParseEntry(%q) = %+v, want %+v", c.in, got, c.want)
			}
			if got.Name() != c.name {
				t.Errorf("Name() = %q, want %q", got.Name(), c.name)
			}
		})
	}
}

func TestParseEntry_PinnedSHA(t *testing.T) {
	sha := strings.Repeat("ab", 20) // 40 hex chars
	e, err := ParseEntry("skill:foo/bar@" + sha + "/skills/x")
	if err != nil {
		t.Fatal(err)
	}
	if !e.PinnedSHA() {
		t.Errorf("40-hex ref not recognised as pinned SHA: %q", e.Ref)
	}
	tag, _ := ParseEntry("skill:foo/bar@v1.0.0/skills/x")
	if tag.PinnedSHA() {
		t.Error("tag ref wrongly treated as pinned SHA")
	}
}

func TestParseEntry_Invalid(t *testing.T) {
	cases := []string{
		"notatype:foo/bar",  // bad prefix
		"rule:foo/bar@main", // rule requires a path
		"workflow:foo/bar",  // workflow requires a path
		"skill:justowner",   // missing repo
		"",                  // empty
		"skill:",            // nothing after prefix
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := ParseEntry(in)
			if err == nil {
				t.Fatalf("ParseEntry(%q) succeeded, want error", in)
			}
		})
	}

	// The bad-prefix error must name the valid prefixes (spec
	// scenario: "stderr names the valid type prefixes").
	_, err := ParseEntry("notatype:foo/bar")
	for _, prefix := range ValidEntryPrefixes() {
		if !strings.Contains(err.Error(), prefix) {
			t.Errorf("error %q does not name valid prefix %q", err, prefix)
		}
	}
}
