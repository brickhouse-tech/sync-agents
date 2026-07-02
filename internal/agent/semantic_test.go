package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBucketDefaultSemantic asserts the rev-3 frontmatter contract
// from SPEC-002: rules default to passive; skills and workflows
// default to invocable. Changes here must round-trip through the
// spec.
func TestBucketDefaultSemantic(t *testing.T) {
	cases := []struct {
		typ  ArtifactType
		want Semantic
	}{
		{ArtifactRule, Passive},
		{ArtifactSkill, Invocable},
		{ArtifactWorkflow, Invocable},
		{ArtifactAgent, Invocable},
		{ArtifactPlan, Reference},
		{ArtifactSpec, Reference},
	}
	for _, c := range cases {
		t.Run(string(c.typ), func(t *testing.T) {
			if got := BucketDefaultSemantic(c.typ); got != c.want {
				t.Errorf("BucketDefaultSemantic(%q) = %q, want %q", c.typ, got, c.want)
			}
		})
	}
}

// TestBucketDefaultSemantic_Unknown documents the "unknown type ⇒
// passive" safe default. If we ever add a new artifact type, this
// is a tripwire that points at semantic.go's switch.
func TestBucketDefaultSemantic_Unknown(t *testing.T) {
	if got := BucketDefaultSemantic(ArtifactType("not-a-real-type")); got != Passive {
		t.Errorf("unknown type returned %q, want passive (safe default)", got)
	}
}

// TestParseFrontmatterInvocable_NoFrontmatter covers files that
// don't start with `---`. The parser must return present=false with
// no error so callers fall back to bucket defaults.
func TestParseFrontmatterInvocable_NoFrontmatter(t *testing.T) {
	cases := []string{
		"# just a markdown title\n",
		"\n---\nleading blank line disqualifies\n---\n",
		"plain text with no markers\n",
		"",
	}
	for _, in := range cases {
		t.Run(in[:min(len(in), 20)], func(t *testing.T) {
			val, present, err := ParseFrontmatterInvocable([]byte(in))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if present {
				t.Errorf("expected present=false, got val=%v present=true", val)
			}
		})
	}
}

// TestParseFrontmatterInvocable_TrueAndFalse covers the happy path
// for both boolean values. Quoted forms are also accepted.
func TestParseFrontmatterInvocable_TrueAndFalse(t *testing.T) {
	cases := []struct {
		input   string
		wantVal bool
	}{
		{"---\ninvocable: true\n---\nbody\n", true},
		{"---\ninvocable: false\n---\nbody\n", false},
		{"---\ninvocable: \"true\"\n---\n", true},
		{"---\ninvocable: 'false'\n---\n", false},
		// trailing inline comment
		{"---\ninvocable: true   # because reasons\n---\n", true},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			val, present, err := ParseFrontmatterInvocable([]byte(c.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !present {
				t.Fatal("expected present=true")
			}
			if val != c.wantVal {
				t.Errorf("val = %v, want %v", val, c.wantVal)
			}
		})
	}
}

// TestParseFrontmatterInvocable_OtherKeysOnly verifies that
// frontmatter without `invocable:` returns present=false. The
// rationale: callers should fall back to bucket defaults, not infer
// from sibling keys (like `trigger:`).
func TestParseFrontmatterInvocable_OtherKeysOnly(t *testing.T) {
	in := "---\ntrigger: always_on\nname: foo\n---\nbody\n"
	val, present, err := ParseFrontmatterInvocable([]byte(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if present {
		t.Errorf("present should be false when invocable: key absent; got val=%v", val)
	}
}

// TestParseFrontmatterInvocable_MalformedUnterminated covers the case
// where the file starts with `---` but never closes the frontmatter.
// The parser must return an error so we don't silently apply a
// bucket default to a malformed file.
func TestParseFrontmatterInvocable_MalformedUnterminated(t *testing.T) {
	in := "---\ninvocable: true\nbody text but no closing dashes\n"
	_, _, err := ParseFrontmatterInvocable([]byte(in))
	if err == nil {
		t.Error("expected error for unterminated frontmatter; got nil")
	}
}

// TestParseFrontmatterInvocable_InvalidValue rejects non-boolean
// values under `invocable:`. The contract is strict: `yes`, `1`,
// `maybe` are typos, not aliases.
func TestParseFrontmatterInvocable_InvalidValue(t *testing.T) {
	cases := []string{
		"---\ninvocable: yes\n---\n",
		"---\ninvocable: 1\n---\n",
		"---\ninvocable: maybe\n---\n",
		"---\ninvocable:\n---\n", // empty value
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, _, err := ParseFrontmatterInvocable([]byte(in))
			if err == nil {
				t.Errorf("expected error for input %q; got nil", in)
			}
		})
	}
}

// TestParseFrontmatterInvocable_CRLF confirms Windows line endings
// don't break parsing. Important because rules authored on Windows
// often round-trip with CRLF.
func TestParseFrontmatterInvocable_CRLF(t *testing.T) {
	in := "---\r\ninvocable: true\r\n---\r\nbody\r\n"
	val, present, err := ParseFrontmatterInvocable([]byte(in))
	if err != nil {
		t.Fatalf("CRLF parsing failed: %v", err)
	}
	if !present || !val {
		t.Errorf("CRLF input did not parse correctly: val=%v present=%v", val, present)
	}
}

// TestParseFrontmatterInvocable_WithCommentsAndBlanks confirms that
// `#` comment lines and blank lines inside frontmatter are skipped
// rather than treated as syntax errors.
func TestParseFrontmatterInvocable_WithCommentsAndBlanks(t *testing.T) {
	in := "---\n# leading comment\n\ninvocable: false\n\n# trailing comment\n---\n"
	val, present, err := ParseFrontmatterInvocable([]byte(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !present {
		t.Fatal("expected present=true")
	}
	if val {
		t.Errorf("val = true, want false")
	}
}

// TestResolveSemantic_RuleNoFrontmatter exercises the most common
// case: a rule .md file with no frontmatter should resolve to passive
// via the bucket default.
func TestResolveSemantic_RuleNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "security.md")
	if err := os.WriteFile(path, []byte("# security\nrule body\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := ResolveSemantic(path, ArtifactRule)
	if err != nil {
		t.Fatalf("ResolveSemantic: %v", err)
	}
	if got != Passive {
		t.Errorf("got %q, want passive", got)
	}
}

// TestResolveSemantic_SkillNoFrontmatter exercises the inverse: a
// skill with no frontmatter resolves to invocable.
func TestResolveSemantic_SkillNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "cool")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# cool\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := ResolveSemantic(skillDir, ArtifactSkill)
	if err != nil {
		t.Fatalf("ResolveSemantic: %v", err)
	}
	if got != Invocable {
		t.Errorf("got %q, want invocable", got)
	}
}

// TestResolveSemantic_FrontmatterOverridesBucket is the key behavior
// test. A skill explicitly marked invocable: false must resolve to
// passive despite its bucket. This is what makes the routing flexible
// for "long-form rule disguised as a skill" cases.
func TestResolveSemantic_FrontmatterOverridesBucket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "onboarding.md")
	if err := os.WriteFile(path, []byte("---\ninvocable: true\n---\n# onboarding\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := ResolveSemantic(path, ArtifactRule)
	if err != nil {
		t.Fatalf("ResolveSemantic: %v", err)
	}
	// Bucket default for rule is passive; frontmatter flips it.
	if got != Invocable {
		t.Errorf("got %q, want invocable (frontmatter override on a rule)", got)
	}
}

// TestResolveSemantic_SkillDirOrSKILLmd ensures both path forms work
// for skills — passing the dir or the SKILL.md file inside both
// resolve correctly.
func TestResolveSemantic_SkillDirOrSKILLmd(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "thing")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMD, []byte("---\ninvocable: false\n---\nbody\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Pass the directory.
	got1, err := ResolveSemantic(skillDir, ArtifactSkill)
	if err != nil {
		t.Fatalf("dir form: %v", err)
	}
	// Pass the SKILL.md directly.
	got2, err := ResolveSemantic(skillMD, ArtifactSkill)
	if err != nil {
		t.Fatalf("file form: %v", err)
	}
	if got1 != Passive || got2 != Passive {
		t.Errorf("expected both forms to return passive; got dir=%q file=%q", got1, got2)
	}
}

// TestResolveSemantic_MissingFile documents the error path. Callers
// that want "default on missing" should detect not-exist themselves.
func TestResolveSemantic_MissingFile(t *testing.T) {
	_, err := ResolveSemantic("/does/not/exist.md", ArtifactRule)
	if err == nil {
		t.Error("expected error for missing file; got nil")
	}
}

// TestResolveSemantic_MalformedFrontmatter ensures parsing errors
// propagate from ResolveSemantic, with the path included so the user
// can find the file.
func TestResolveSemantic_MalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(path, []byte("---\ninvocable: maybe\n---\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := ResolveSemantic(path, ArtifactRule)
	if err == nil {
		t.Fatal("expected error for malformed frontmatter; got nil")
	}
	if !contains(err.Error(), "bad.md") {
		t.Errorf("error message %q does not mention file path", err.Error())
	}
}

// helpers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (indexOf(haystack, needle) != -1)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
