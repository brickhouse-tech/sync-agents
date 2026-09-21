package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newAddTestApp returns an App rooted in a temp project with an
// initialized .agents/ tree, plus the captured stdout buffer.
func newAddTestApp(t *testing.T) (*App, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "rules"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	stdout := &bytes.Buffer{}
	return &App{
		ProjectRoot: root,
		GlobalRoot:  filepath.Join(root, "global-agents"),
		Stdout:      stdout,
		Stderr:      &bytes.Buffer{},
	}, root, stdout
}

// writeSrc writes a source artifact outside the .agents/ tree.
func writeSrc(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return p
}

const validAgent = "---\nname: old-name\ndescription: Reviews diffs adversarially.\nmodel: opus\ntools: Read, Grep, Bash(git diff *)\n---\n\nYou are the reviewer.\n"

// TestCmdAdd_TemplateModeUnchanged is the no-regression guard for
// SPEC-011 Part C: adding the import flags must not alter the
// behavior of a plain `add`.
func TestCmdAdd_TemplateModeUnchanged(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	if err := a.CmdAdd("agent", "reviewer", AddOpts{}); err != nil {
		t.Fatalf("add: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".agents", "agents", "reviewer.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := strings.ReplaceAll(templatesAgentForTest(), "${NAME}", "reviewer")
	if string(got) != want {
		t.Errorf("template output changed:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestCmdAdd_ImportCopyRewritesName covers the copy path: the name is
// corrected to the imported-as name, every other key survives, and
// the source file is untouched.
func TestCmdAdd_ImportCopyRewritesName(t *testing.T) {
	a, root, out := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, "outside"), "rev.md", validAgent)

	if err := a.CmdAdd("agent", "reviewer", AddOpts{From: src}); err != nil {
		t.Fatalf("add --from: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, ".agents", "agents", "reviewer.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "name: reviewer") {
		t.Errorf("name not rewritten:\n%s", body)
	}
	if strings.Contains(body, "old-name") {
		t.Errorf("stale name survived:\n%s", body)
	}
	for _, keep := range []string{"model: opus", "tools: Read, Grep, Bash(git diff *)", "You are the reviewer."} {
		if !strings.Contains(body, keep) {
			t.Errorf("import dropped %q:\n%s", keep, body)
		}
	}
	if !strings.Contains(out.String(), "rewriting name") {
		t.Error("a silent name rewrite is not acceptable; expected a warning")
	}

	// The source is not ours to edit.
	orig, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if string(orig) != validAgent {
		t.Errorf("source file was modified:\n%s", orig)
	}
}

// TestCmdAdd_ImportCopyMatchingNameIsQuiet: no rewrite, no warning.
func TestCmdAdd_ImportCopyMatchingNameIsQuiet(t *testing.T) {
	a, root, out := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, "outside"), "reviewer.md",
		strings.Replace(validAgent, "old-name", "reviewer", 1))

	if err := a.CmdAdd("agent", "reviewer", AddOpts{From: src}); err != nil {
		t.Fatalf("add --from: %v", err)
	}
	if strings.Contains(out.String(), "rewriting name") {
		t.Error("matching name must not produce a rewrite warning")
	}
}

// TestCmdAdd_ImportRejectsMissingDescription: a subagent with no
// description installs fine and is then unreachable, so the import
// fails loudly instead.
func TestCmdAdd_ImportRejectsMissingDescription(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, "outside"), "bad.md", "---\nname: x\nmodel: opus\n---\n\nBody.\n")

	if err := a.CmdAdd("agent", "reviewer", AddOpts{From: src}); err == nil {
		t.Fatal("expected an error for missing description:")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "agents", "reviewer.md")); !os.IsNotExist(err) {
		t.Error("a rejected import must not leave a partial artifact behind")
	}
}

// TestCmdAdd_ImportRejectsNoFrontmatter guards the other malformed
// shape: a plain markdown file with no frontmatter at all.
func TestCmdAdd_ImportRejectsNoFrontmatter(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, "outside"), "plain.md", "Just prose, no frontmatter.\n")

	if err := a.CmdAdd("agent", "reviewer", AddOpts{From: src}); err == nil {
		t.Fatal("expected an error for missing frontmatter")
	}
}

// TestCmdAdd_ImportByLink covers the link path: the canonical path is
// a symlink, the content is not duplicated, and edits at the source
// are visible through it.
func TestCmdAdd_ImportByLink(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, "personas"), "tars.md",
		"---\nname: tars\ndescription: Chief of staff.\n---\n\nBody v1.\n")

	if err := a.CmdAdd("agent", "tars", AddOpts{From: src, Link: true}); err != nil {
		t.Fatalf("add --link: %v", err)
	}

	dest := filepath.Join(root, ".agents", "agents", "tars.md")
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("--link must produce a symlink, not a copy")
	}

	// An edit at the source is live through the link.
	if err := os.WriteFile(src, []byte("---\nname: tars\ndescription: Chief of staff.\n---\n\nBody v2.\n"), 0o644); err != nil {
		t.Fatalf("edit source: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read through link: %v", err)
	}
	if !strings.Contains(string(got), "Body v2.") {
		t.Errorf("link is not live; got:\n%s", got)
	}
}

// TestCmdAdd_LinkRejectsNameMismatch: link mode cannot rewrite the
// source, so a mismatch it could only propagate is a hard error — and
// the message has to name the escape hatch.
func TestCmdAdd_LinkRejectsNameMismatch(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, "personas"), "rev.md", validAgent)

	err := a.CmdAdd("agent", "reviewer", AddOpts{From: src, Link: true})
	if err == nil {
		t.Fatal("expected an error for a name mismatch under --link")
	}
	if !strings.Contains(err.Error(), "--link") || !strings.Contains(err.Error(), "old-name") {
		t.Errorf("error must name the conflict and the way out; got: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, ".agents", "agents", "reviewer.md")); !os.IsNotExist(statErr) {
		t.Error("a rejected link must not leave anything behind")
	}
}

// TestCmdAdd_LinkRejectsSourceInsideTree: linking .agents/ into
// itself would create a cycle that sync would then fan out.
func TestCmdAdd_LinkRejectsSourceInsideTree(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, ".agents", "agents"), "other.md",
		"---\nname: other\ndescription: d\n---\n\nb\n")

	if err := a.CmdAdd("agent", "other-copy", AddOpts{From: src, Link: true}); err == nil {
		t.Fatal("expected an error linking a source already inside .agents/")
	}
}

// TestCmdAdd_LinkWithoutFromRejected: --link alone has nothing to
// point at, and silently scaffolding a template would be surprising.
func TestCmdAdd_LinkWithoutFromRejected(t *testing.T) {
	a, _, _ := newAddTestApp(t)
	if err := a.CmdAdd("agent", "x", AddOpts{Link: true}); err == nil {
		t.Fatal("expected --link without --from to be rejected")
	}
}

// TestCmdAdd_ImportMissingSource surfaces a bad path as a clear
// error rather than an empty artifact.
func TestCmdAdd_ImportMissingSource(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	err := a.CmdAdd("agent", "x", AddOpts{From: filepath.Join(root, "nope.md")})
	if err == nil {
		t.Fatal("expected an error for a nonexistent --from path")
	}
}

// TestResolveAddSource_SourceRefIsRedirected: the "<source>:<path>"
// form is recognized and redirected to `source add` + `pull` rather
// than failing as a missing file, because that route applies the
// quarantine gate this one cannot.
func TestResolveAddSource_SourceRefIsRedirected(t *testing.T) {
	a, _, _ := newAddTestApp(t)
	_, err := a.resolveAddSource("team-agents:agents/critic.md")
	if err == nil {
		t.Fatal("expected a source reference to be rejected with guidance")
	}
	if !strings.Contains(err.Error(), "source add") {
		t.Errorf("error must point at the supported route; got: %v", err)
	}
}

// TestResolveAddSource_WindowsStyleAbsolutePath: a drive-letter path
// has a colon but is a path, not a source reference. The separator
// check is what tells them apart.
func TestResolveAddSource_WindowsStyleAbsolutePath(t *testing.T) {
	a, _, _ := newAddTestApp(t)
	_, err := a.resolveAddSource(`./rel:with-colon/x.md`)
	if err != nil && strings.Contains(err.Error(), "source add") {
		t.Errorf("a path containing a separator before ':' must not be read as a source ref; got: %v", err)
	}
}

// TestCmdAdd_ImportSkillDirectory: a dir-per-artifact bucket accepts a
// directory and brings the supporting files with it.
func TestCmdAdd_ImportSkillDirectory(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	srcDir := filepath.Join(root, "outside", "helper")
	writeSrc(t, srcDir, "SKILL.md", "---\nname: old\ndescription: Does a thing.\n---\n\nBody.\n")
	writeSrc(t, srcDir, "reference.md", "supporting material\n")

	if err := a.CmdAdd("skill", "helper", AddOpts{From: srcDir}); err != nil {
		t.Fatalf("add skill --from dir: %v", err)
	}

	destDir := filepath.Join(root, ".agents", "skills", "helper")
	got, err := os.ReadFile(filepath.Join(destDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(got), "name: helper") {
		t.Errorf("SKILL.md name not normalized:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(destDir, "reference.md")); err != nil {
		t.Errorf("supporting file not copied: %v", err)
	}
}

// TestCmdAdd_ImportSkillDirectoryWithoutEntrypointRejected: copy mode
// must refuse a skill directory that lacks its SKILL.md entrypoint,
// the same way link mode does — otherwise it would produce an
// entrypoint-less, undeliverable artifact.
func TestCmdAdd_ImportSkillDirectoryWithoutEntrypointRejected(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	srcDir := filepath.Join(root, "outside", "helper")
	writeSrc(t, srcDir, "reference.md", "supporting material\n")

	if err := a.CmdAdd("skill", "helper", AddOpts{From: srcDir}); err == nil {
		t.Fatal("expected a skill dir without SKILL.md to be rejected under copy mode")
	}
}

// TestCmdAdd_ImportDirectoryIntoFlatBucketRejected: a rule is one
// file; handing it a directory is a mistake worth naming.
func TestCmdAdd_ImportDirectoryIntoFlatBucketRejected(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	srcDir := filepath.Join(root, "outside", "dir")
	writeSrc(t, srcDir, "x.md", "---\nname: x\n---\n\nb\n")

	if err := a.CmdAdd("rule", "x", AddOpts{From: srcDir}); err == nil {
		t.Fatal("expected a directory --from into a flat bucket to be rejected")
	}
}

// TestCmdAdd_ImportRefusesExistingWithoutForce keeps --from on the
// same overwrite contract as template mode.
func TestCmdAdd_ImportRefusesExistingWithoutForce(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, "outside"), "reviewer.md",
		strings.Replace(validAgent, "old-name", "reviewer", 1))

	if err := a.CmdAdd("agent", "reviewer", AddOpts{From: src}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := a.CmdAdd("agent", "reviewer", AddOpts{From: src}); err == nil {
		t.Fatal("expected the second add to refuse without --force")
	}

	a.Force = true
	if err := a.CmdAdd("agent", "reviewer", AddOpts{From: src}); err != nil {
		t.Fatalf("forced add: %v", err)
	}
}

// TestCmdAdd_LinkRejectsMissingDescription: link mode cannot add a
// description the source lacks, and an agent with no description is
// silently unreachable, so the import is refused rather than propagated.
func TestCmdAdd_LinkRejectsMissingDescription(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	src := writeSrc(t, filepath.Join(root, "personas"), "tars.md",
		"---\nname: tars\n---\n\nNo description here.\n")

	err := a.CmdAdd("agent", "tars", AddOpts{From: src, Link: true})
	if err == nil {
		t.Fatal("expected a missing description to be rejected under --link")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error must name the missing field; got: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, ".agents", "agents", "tars.md")); !os.IsNotExist(statErr) {
		t.Error("a rejected link must not leave anything behind")
	}
}

// TestCmdAdd_LinkSkillRejectsNameMismatch: for a dir-per-artifact
// bucket the identity lives in SKILL.md, so link mode must reject a
// name mismatch there the same way it does for a single-file artifact.
func TestCmdAdd_LinkSkillRejectsNameMismatch(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	srcDir := filepath.Join(root, "outside", "helper")
	writeSrc(t, srcDir, "SKILL.md", "---\nname: old\ndescription: Does a thing.\n---\n\nBody.\n")

	err := a.CmdAdd("skill", "helper", AddOpts{From: srcDir, Link: true})
	if err == nil {
		t.Fatal("expected a SKILL.md name mismatch to be rejected under --link")
	}
	if !strings.Contains(err.Error(), "--link") || !strings.Contains(err.Error(), "old") {
		t.Errorf("error must name the conflict and the way out; got: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, ".agents", "skills", "helper")); !os.IsNotExist(statErr) {
		t.Error("a rejected link must not leave anything behind")
	}
}

// TestCmdAdd_LinkSkillMatchingNameSucceeds is the positive companion:
// a SKILL.md whose name already matches links cleanly.
func TestCmdAdd_LinkSkillMatchingNameSucceeds(t *testing.T) {
	a, root, _ := newAddTestApp(t)
	srcDir := filepath.Join(root, "outside", "helper")
	writeSrc(t, srcDir, "SKILL.md", "---\nname: helper\ndescription: Does a thing.\n---\n\nBody.\n")

	if err := a.CmdAdd("skill", "helper", AddOpts{From: srcDir, Link: true}); err != nil {
		t.Fatalf("add skill --link: %v", err)
	}
	fi, err := os.Lstat(filepath.Join(root, ".agents", "skills", "helper"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("--link must produce a symlink at the skill directory")
	}
}

// templatesAgentForTest mirrors the bucket's template accessor so the
// no-regression test compares against the real scaffold rather than a
// copy that could drift.
func templatesAgentForTest() string {
	b, _ := BucketForArtifact(ArtifactAgent)
	return b.NewTemplate()
}
