package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newLintApp returns an App rooted at a temp project with an
// initialized .agents/skills tree, plus the skills dir path.
func newLintApp(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	skills := filepath.Join(root, ".agents", "skills")
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}
	return &App{
		ProjectRoot: root,
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
	}, skills
}

func writeSkill(t *testing.T, skillsDir, name, content string) string {
	t.Helper()
	dir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// findingCodes extracts the finding codes a lintSkill call produced.
func lintOnce(t *testing.T, a *App, dir, path string, fix bool) []LintFinding {
	t.Helper()
	fs, err := a.lintSkill(dir, path, fix)
	if err != nil {
		t.Fatalf("lintSkill: %v", err)
	}
	return fs
}

func hasCode(fs []LintFinding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestLint_CompliantSkillIsClean(t *testing.T) {
	a, skills := newLintApp(t)
	path := writeSkill(t, skills, "pdf-processing", `---
name: pdf-processing
description: Extracts text and tables from PDF files. Use when working with PDFs.
---

# PDF Processing
`)
	fs := lintOnce(t, a, "pdf-processing", path, false)
	if len(fs) != 0 {
		t.Fatalf("expected no findings, got %+v", fs)
	}
}

func TestLint_MissingFrontmatter_FixInjects(t *testing.T) {
	a, skills := newLintApp(t)
	body := "# My Skill\n\nDoes useful things when asked.\n"
	path := writeSkill(t, skills, "my-skill", body)

	fs := lintOnce(t, a, "my-skill", path, false)
	if !hasCode(fs, "E001") {
		t.Fatalf("expected E001, got %+v", fs)
	}

	lintOnce(t, a, "my-skill", path, true)
	out := readFile(t, path)
	if !strings.HasPrefix(out, "---\n") {
		t.Fatalf("frontmatter not injected:\n%s", out)
	}
	if !strings.Contains(out, "name: my-skill") {
		t.Fatalf("name not set:\n%s", out)
	}
	if !strings.Contains(out, "description: Does useful things when asked.") {
		t.Fatalf("description not derived from body:\n%s", out)
	}
	if !strings.HasSuffix(out, body) {
		t.Fatalf("body not preserved:\n%s", out)
	}
}

func TestLint_NameChecks(t *testing.T) {
	cases := []struct {
		testName string
		dir      string
		fm       string
		wantCode string
		fixedVal string // expected `name:` value after --fix
	}{
		{"missing name", "alpha", "invocable: true", "E002", "alpha"},
		{"uppercase name", "beta", "name: Beta_Skill", "E003", "beta"},
		{"mismatched name", "gamma", "name: other-name", "E006", "gamma"},
		{"xml in name", "delta", "name: <b>delta</b>", "E009", "delta"},
	}
	for _, tc := range cases {
		t.Run(tc.testName, func(t *testing.T) {
			a, skills := newLintApp(t)
			path := writeSkill(t, skills, tc.dir, "---\n"+tc.fm+"\ndescription: Does a thing. Use when needed.\n---\n\nBody.\n")

			fs := lintOnce(t, a, tc.dir, path, false)
			if !hasCode(fs, tc.wantCode) {
				t.Fatalf("expected %s, got %+v", tc.wantCode, fs)
			}

			lintOnce(t, a, tc.dir, path, true)
			out := readFile(t, path)
			if !strings.Contains(out, "name: "+tc.fixedVal+"\n") {
				t.Fatalf("expected fixed name %q in:\n%s", tc.fixedVal, out)
			}
		})
	}
}

func TestLint_NameTooLong_TruncatedAtHyphen(t *testing.T) {
	a, skills := newLintApp(t)
	long := strings.Repeat("verylong-", 10) // 90 chars
	path := writeSkill(t, skills, long, "---\nname: "+long+"\ndescription: Does things. Use when needed.\n---\n")

	fs := lintOnce(t, a, long, path, false)
	if !hasCode(fs, "E004") {
		t.Fatalf("expected E004, got %+v", fs)
	}
	lintOnce(t, a, long, path, true)
	name, _ := mustFM(t, path).get("name")
	if len(name) > skillNameMaxLen || strings.HasSuffix(name, "-") || !skillNameRe.MatchString(name) {
		t.Fatalf("bad truncated name %q", name)
	}
}

func TestLint_ReservedWord_NotAutoFixed(t *testing.T) {
	a, skills := newLintApp(t)
	path := writeSkill(t, skills, "claude-helper", "---\nname: claude-helper\ndescription: Helps. Use when needed.\n---\n")

	fs := lintOnce(t, a, "claude-helper", path, true)
	if !hasCode(fs, "E005") {
		t.Fatalf("expected E005, got %+v", fs)
	}
	for _, f := range fs {
		if f.Code == "E005" && f.Fixed {
			t.Fatal("E005 must not be auto-fixed")
		}
	}
}

func TestLint_DescriptionChecks(t *testing.T) {
	a, skills := newLintApp(t)

	// E007: missing description, derived from body prose.
	p1 := writeSkill(t, skills, "no-desc", "---\nname: no-desc\n---\n\n# Title\n\nExtracts data from reports when asked.\n")
	fs := lintOnce(t, a, "no-desc", p1, true)
	if !hasCode(fs, "E007") {
		t.Fatalf("expected E007, got %+v", fs)
	}
	desc, _ := mustFM(t, p1).get("description")
	if desc != "Extracts data from reports when asked." {
		t.Fatalf("derived description = %q", desc)
	}

	// E008: overlong description truncated.
	longDesc := strings.Repeat("word ", 300) // ~1500 chars
	p2 := writeSkill(t, skills, "long-desc", "---\nname: long-desc\ndescription: "+longDesc+"\n---\n")
	fs = lintOnce(t, a, "long-desc", p2, true)
	if !hasCode(fs, "E008") {
		t.Fatalf("expected E008, got %+v", fs)
	}
	desc, _ = mustFM(t, p2).get("description")
	if len(desc) > skillDescriptionMaxLen {
		t.Fatalf("description still %d chars", len(desc))
	}

	// E009: XML stripped from description.
	p3 := writeSkill(t, skills, "xml-desc", "---\nname: xml-desc\ndescription: Uses <tool>stuff</tool> when needed.\n---\n")
	fs = lintOnce(t, a, "xml-desc", p3, true)
	if !hasCode(fs, "E009") {
		t.Fatalf("expected E009, got %+v", fs)
	}
	desc, _ = mustFM(t, p3).get("description")
	if strings.Contains(desc, "<") {
		t.Fatalf("XML not stripped: %q", desc)
	}
}

func TestLint_Warnings(t *testing.T) {
	a, skills := newLintApp(t)

	// W101 first person + W102 no when-clause.
	p := writeSkill(t, skills, "warny", "---\nname: warny\ndescription: I can help you with stuff.\n---\n")
	fs := lintOnce(t, a, "warny", p, false)
	if !hasCode(fs, "W101") || !hasCode(fs, "W102") {
		t.Fatalf("expected W101+W102, got %+v", fs)
	}

	// W103 body too long.
	long := "---\nname: longbody\ndescription: Does things. Use when needed.\n---\n" + strings.Repeat("line\n", 600)
	p2 := writeSkill(t, skills, "longbody", long)
	fs = lintOnce(t, a, "longbody", p2, false)
	if !hasCode(fs, "W103") {
		t.Fatalf("expected W103, got %+v", fs)
	}
}

func TestLint_FixIsIdempotent_AndPreservesUnknownKeys(t *testing.T) {
	a, skills := newLintApp(t)
	path := writeSkill(t, skills, "keeper", `---
trigger: always_on
invocable: true
# a comment
name: Wrong Name
---

Processes widgets. Use when widgets appear.
`)
	lintOnce(t, a, "keeper", path, true)
	first := readFile(t, path)

	for _, keep := range []string{"trigger: always_on", "invocable: true", "# a comment", "name: keeper"} {
		if !strings.Contains(first, keep) {
			t.Fatalf("lost %q in:\n%s", keep, first)
		}
	}

	fs := lintOnce(t, a, "keeper", path, true)
	if len(fs) != 0 {
		t.Fatalf("second lint not clean: %+v", fs)
	}
	if second := readFile(t, path); second != first {
		t.Fatalf("fix not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestCmdLint_ExitBehavior(t *testing.T) {
	a, skills := newLintApp(t)
	writeSkill(t, skills, "broken", "# no frontmatter\n\nSome prose here when used.\n")

	if err := a.CmdLint("skills", false); err == nil {
		t.Fatal("expected error exit for unfixed E-level findings")
	}
	if err := a.CmdLint("skills", true); err != nil {
		t.Fatalf("fix run should exit clean, got %v", err)
	}
	if err := a.CmdLint("skills", false); err != nil {
		t.Fatalf("post-fix lint should be clean, got %v", err)
	}

	// Warnings only → exit zero.
	a2, skills2 := newLintApp(t)
	writeSkill(t, skills2, "warny", "---\nname: warny\ndescription: I can do stuff.\n---\n")
	if err := a2.CmdLint("", false); err != nil {
		t.Fatalf("warnings must not fail lint, got %v", err)
	}

	// Unsupported type → error.
	if err := a2.CmdLint("rules", false); err == nil {
		t.Fatal("expected error for unsupported lint type")
	}
}

func TestSlugifySkillName(t *testing.T) {
	cases := map[string]string{
		"My_Cool Skill":  "my-cool-skill",
		"PDF--Utils!":    "pdf-utils",
		"-trim-me-":      "trim-me",
		"already-fine-1": "already-fine-1",
	}
	for in, want := range cases {
		if got := slugifySkillName(in); got != want {
			t.Errorf("slugifySkillName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuoteYAMLScalar(t *testing.T) {
	if got := quoteYAMLScalar("plain value"); got != "plain value" {
		t.Errorf("plain value quoted: %q", got)
	}
	if got := quoteYAMLScalar("has: colon"); got != `"has: colon"` {
		t.Errorf("colon value = %q", got)
	}
	if got := quoteYAMLScalar(`say "hi"`); got != `"say \"hi\""` {
		t.Errorf("quote escaping = %q", got)
	}
}

func mustFM(t *testing.T, path string) fmBlock {
	t.Helper()
	block, err := parseFMBlock(readFile(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if !block.present {
		t.Fatalf("no frontmatter in %s", path)
	}
	return block
}

func TestWriteFileAtomic_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "nested", "file.md")
	data := []byte("hello world\n")

	if err := writeFileAtomic(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	read, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(read) != string(data) {
		t.Errorf("got %q, want %q", read, data)
	}
}

func TestWriteFileAtomic_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	os.WriteFile(path, []byte("old"), 0o644)

	if err := writeFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	read, _ := os.ReadFile(path)
	if string(read) != "new" {
		t.Errorf("got %q, want 'new'", read)
	}
}
