package agent

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var updateAgentsMDGoldens = flag.Bool("update-agentsmd", false, "rewrite testdata/agentsmd/*.golden from migrateAgentsMD")

// agentsMDFixtures returns every migration input under
// testdata/agentsmd (the *.md files; each has a *.golden sibling).
// Three are real generated files copied from the author's machine; the
// rest were written by the pre-SPEC-013 generator from synthetic trees,
// or by hand for shapes no generator wrote.
func agentsMDFixtures(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "agentsmd", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	return files
}

func TestMigrateAgentsMD_Golden(t *testing.T) {
	for _, in := range agentsMDFixtures(t) {
		t.Run(filepath.Base(in), func(t *testing.T) {
			got, _ := migrateAgentsMD(readFile(t, in))
			golden := strings.TrimSuffix(in, ".md") + ".golden"
			if *updateAgentsMDGoldens {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if want := readFile(t, golden); got != want {
				t.Errorf("migrate(%s) mismatch\n--- got ---\n%s\n--- want ---\n%s", in, got, want)
			}
		})
	}
}

// TestMigrateAgentsMD_Reports pins what each fixture reports, which is
// also what the log line says.
func TestMigrateAgentsMD_Reports(t *testing.T) {
	std := []string{"Rules", "Skills", "Workflows", "State"}
	tests := map[string]AgentsMDMigration{
		"org-stub-only.md":              {Header: true, Removed: std},
		"inherits-placeholders.md":      {Header: true, Removed: std},
		"inherits-state-imports.md":     {Header: true, Removed: std, ClaudeImports: true},
		"bash-era-leading-blank.md":     {Header: true, Removed: std},
		"descriptions-specs-imports.md": {Header: true, Removed: []string{"Rules", "Skills", "Workflows", "Specs", "State"}, ClaudeImports: true},
		"all-sections.md": {Header: true, ClaudeImports: true,
			Removed: []string{"Rules", "Skills", "Workflows", "Agents", "Plans", "Specs", "ADRs", "Hooks", "State"}},
		"openclaw-workspace.md":       {Header: true, Removed: std, ClaudeImports: true},
		"openclaw-workspace-tools.md": {Header: true, Removed: std, ClaudeImports: true},
		"hand-line-in-rules.md":       {Header: true, Removed: []string{"Skills", "Workflows", "State"}, Kept: []string{"Rules"}},
		"not-generated-imports.md":    {ClaudeImports: true},
		"not-generated.md":            {},
	}
	files := agentsMDFixtures(t)
	if len(files) != len(tests) {
		t.Fatalf("%d fixtures, %d expectations: keep them in step", len(files), len(tests))
	}
	for _, in := range files {
		want, ok := tests[filepath.Base(in)]
		if !ok {
			t.Errorf("no expectation for %s", in)
			continue
		}
		_, got := migrateAgentsMD(readFile(t, in))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: report %+v, want %+v", filepath.Base(in), got, want)
		}
	}
}

func TestMigrateAgentsMD_Idempotent(t *testing.T) {
	for _, in := range agentsMDFixtures(t) {
		once, _ := migrateAgentsMD(readFile(t, in))
		twice, m := migrateAgentsMD(once)
		if twice != once {
			t.Errorf("%s: migrate(migrate(x)) != migrate(x)\n--- once ---\n%s\n--- twice ---\n%s", in, once, twice)
		}
		if m.Changed() {
			t.Errorf("%s: second migration reports a change: %+v", in, m)
		}
	}
}

// TestMigrateAgentsMD_HandWrittenLinesSurvive is the survival property:
// every non-blank line the old generator could not have written appears
// in the output, in the original order. "Could have written" is judged
// line by line, independently of the section logic: header lines,
// generated-section headings, generatedLine matches, and the inside of
// the claude-imports block.
func TestMigrateAgentsMD_HandWrittenLinesSurvive(t *testing.T) {
	header := map[string]bool{}
	for _, l := range generatedHeader {
		header[l] = true
	}
	for _, in := range agentsMDFixtures(t) {
		src := readFile(t, in)
		out, _ := migrateAgentsMD(src)
		outLines := strings.Split(out, "\n")
		pos := 0
		inImports := false
		for _, l := range strings.Split(src, "\n") {
			switch {
			case l == legacyClaudeImportsRegion.Start():
				inImports = true
				continue
			case l == legacyClaudeImportsRegion.End():
				inImports = false
				continue
			}
			title, isHeading := strings.CutPrefix(l, "## ")
			if inImports || strings.TrimSpace(l) == "" || header[l] ||
				(isHeading && generatedSections[title]) || generatedLine.MatchString(l) {
				continue
			}
			for pos < len(outLines) && outLines[pos] != l {
				pos++
			}
			if pos == len(outLines) {
				t.Errorf("%s: hand-written line lost or reordered: %q", filepath.Base(in), l)
				break
			}
			pos++
		}
	}
}

// TestMigrateAgentsMD_GeneratedReducesToStub checks the spec's E13
// outcome on every generated fixture: the stub comes first, and nothing
// the generator wrote is left.
func TestMigrateAgentsMD_GeneratedReducesToStub(t *testing.T) {
	for _, in := range agentsMDFixtures(t) {
		out, m := migrateAgentsMD(readFile(t, in))
		if !m.Header {
			continue
		}
		if !strings.HasPrefix(out, AgentsMDStub) {
			t.Errorf("%s: output does not start with the stub:\n%s", in, out)
		}
		for _, gone := range []string{"Auto-generated by", "trigger: always_on", "sync-agents:claude-imports"} {
			if strings.Contains(out, gone) {
				t.Errorf("%s: %q survived:\n%s", in, gone, out)
			}
		}
	}
}

func TestMigrateAgentsMD_Edges(t *testing.T) {
	gen := strings.Join([]string{
		"---", "trigger: always_on", "---", "", "# AGENTS", "",
		"> Auto-generated by [sync-agents](https://github.com/brickhouse-tech/sync-agents). Do not edit manually.",
		"> Run `sync-agents index` to regenerate.", "",
		"This file indexes all rules, skills, and workflows defined in `.agents/`.", "",
	}, "\n") + "\n"
	tests := []struct {
		name, in, want string
	}{
		{"empty file", "", ""},
		{"header only", gen, AgentsMDStub},
		{"header without trailing newline", strings.TrimSuffix(gen, "\n\n"), AgentsMDStub},
		{"truncated header is not generated", "---\ntrigger: always_on\n---\n\n# AGENTS\n", "---\ntrigger: always_on\n---\n\n# AGENTS\n"},
		{"region with a ## line is opaque", gen + "## Rules\n\n- [a](.agents/rules/a.md)\n\n" +
			"<!-- sync-agents:openclaw-rules:start -->\n## Rules\n\nbody\n<!-- sync-agents:openclaw-rules:end -->\n",
			AgentsMDStub + "\n<!-- sync-agents:openclaw-rules:start -->\n## Rules\n\nbody\n<!-- sync-agents:openclaw-rules:end -->\n"},
		{"unterminated region marker is an ordinary line", gen + "## Rules\n\n<!-- sync-agents:x:start -->\n",
			AgentsMDStub + "\n## Rules\n\n<!-- sync-agents:x:start -->\n"},
		{"heading case differs: not generated", gen + "## rules\n\n- [a](.agents/rules/a.md)\n",
			AgentsMDStub + "\n## rules\n\n- [a](.agents/rules/a.md)\n"},
		{"link outside .agents is hand-written", gen + "## Skills\n\n- [a](skills/a.md)\n",
			AgentsMDStub + "\n## Skills\n\n- [a](skills/a.md)\n"},
		{"unknown badge is hand-written", gen + "## Rules\n\n- [a](.agents/rules/bsd/a.md) `[bsd]`\n",
			AgentsMDStub + "\n## Rules\n\n- [a](.agents/rules/bsd/a.md) `[bsd]`\n"},
		{"imports block mid-file in a user file leaves one blank line",
			"a\n\n" + legacyImportsBlock("x") + "\nb\n", "a\n\nb\n"},
		{"imports block at EOF in a user file", "a\n\n" + legacyImportsBlock("x"), "a\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := migrateAgentsMD(tt.in); got != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// newAgentsMDApp returns an App over a fresh project with .agents/ and
// the given AGENTS.md.
func newAgentsMDApp(t *testing.T, agentsMD string) (*App, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	return &App{ProjectRoot: root, GlobalRoot: filepath.Join(root, ".agents"), ActiveTargets: []string{"claude"},
		Stdout: out, Stderr: out}, root, out
}

func TestMigrateProjectAgentsMD_WritesBackupOnceAndThenNothing(t *testing.T) {
	src := readFile(t, filepath.Join("testdata", "agentsmd", "openclaw-workspace.md"))
	a, root, out := newAgentsMDApp(t, src)
	path := filepath.Join(root, "AGENTS.md")
	backup := filepath.Join(root, ".agents", agentsMDBackupRel)

	if _, err := a.migrateProjectAgentsMD(); err != nil {
		t.Fatal(err)
	}
	want, _ := migrateAgentsMD(src)
	if got := readFile(t, path); got != want {
		t.Errorf("AGENTS.md after migration:\n%s", got)
	}
	if got := readFile(t, backup); got != src {
		t.Errorf("backup is not the original bytes")
	}
	if !strings.Contains(out.String(), "Backup: .agents/"+agentsMDBackupRel) {
		t.Errorf("summary line missing:\n%s", out.String())
	}

	// Second run: nothing to do, so the file keeps its mtime.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if m, err := a.migrateProjectAgentsMD(); err != nil || m.Changed() {
		t.Fatalf("second run: %+v, %v", m, err)
	}
	if fi, _ := os.Stat(path); !fi.ModTime().Equal(old) {
		t.Errorf("second run touched AGENTS.md (mtime %v, want %v)", fi.ModTime(), old)
	}

	// A later generated file (say, from a checkout of an old branch)
	// migrates again, but the first backup is never overwritten.
	if err := os.WriteFile(path, []byte(readFile(t, filepath.Join("testdata", "agentsmd", "org-stub-only.md"))), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.migrateProjectAgentsMD(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, backup); got != src {
		t.Errorf("backup overwritten by a later migration")
	}
}

func TestMigrateProjectAgentsMD_DryRunWritesNothing(t *testing.T) {
	src := readFile(t, filepath.Join("testdata", "agentsmd", "org-stub-only.md"))
	a, root, out := newAgentsMDApp(t, src)
	a.DryRun = true
	if _, err := a.migrateProjectAgentsMD(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, "AGENTS.md")); got != src {
		t.Errorf("dry-run rewrote AGENTS.md")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", agentsMDBackupRel)); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote the backup (stat err %v)", err)
	}
	if !strings.Contains(out.String(), "[dry-run] would migrate AGENTS.md") {
		t.Errorf("dry-run report missing:\n%s", out.String())
	}
}

func TestMigrateProjectAgentsMD_MissingFileIsNotAnError(t *testing.T) {
	a, root, _ := newAgentsMDApp(t, "")
	os.Remove(filepath.Join(root, "AGENTS.md"))
	if m, err := a.migrateProjectAgentsMD(); err != nil || m.Changed() {
		t.Fatalf("got %+v, %v", m, err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("migration created AGENTS.md")
	}
}

// TestUserOwnedAgentsMDIsNeverRewritten is the SPEC-013 invariant:
// after migration no command writes a project AGENTS.md. The file here
// has a ## Rules section of index-shaped lines, which the old generator
// would have replaced.
func TestUserOwnedAgentsMDIsNeverRewritten(t *testing.T) {
	src := "# Mine\n\n## Rules\n\n- [git](.agents/rules/git.md)\n\nMy own notes.\n"
	a, root, _ := newAgentsMDApp(t, src)
	writeArtifact(t, root, filepath.Join(".agents", "rules", "git.md"), "body\n")

	steps := map[string]func() error{
		"index": a.CmdIndex,
		"sync":  a.CmdSync,
		"add":   func() error { return a.CmdAdd("rule", "testing", AddOpts{}) },
		"fix":   func() error { return a.CmdFix("all", false) },
	}
	for _, name := range []string{"index", "sync", "add", "fix"} {
		if err := steps[name](); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := readFile(t, filepath.Join(root, "AGENTS.md")); got != src {
			t.Fatalf("%s rewrote a user-owned AGENTS.md:\n%s", name, got)
		}
	}
}

func TestCmdIndex_MigratesGeneratedAgentsMD(t *testing.T) {
	src := readFile(t, filepath.Join("testdata", "agentsmd", "inherits-placeholders.md"))
	a, root, _ := newAgentsMDApp(t, src)
	if err := a.CmdIndex(); err != nil {
		t.Fatal(err)
	}
	want := AgentsMDStub + "\n## Inherits\n\n- [org](../.agents/AGENTS.md)\n"
	if got := readFile(t, filepath.Join(root, "AGENTS.md")); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestWriteIfUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeIfUnchanged(path, []byte("stale"), []byte("new")); !errors.Is(err, errConcurrentEdit) {
		t.Fatalf("mismatched prev: err %v, want errConcurrentEdit", err)
	}
	if got := readFile(t, path); got != "old" {
		t.Fatalf("file changed on a failed swap: %q", got)
	}

	if err := writeIfUnchanged(path, []byte("old"), []byte("new")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new" {
		t.Fatalf("swap did not write: %q", got)
	}

	gone := filepath.Join(t.TempDir(), "gone.md")
	if err := writeIfUnchanged(gone, []byte("old"), []byte("new")); !errors.Is(err, errConcurrentEdit) {
		t.Fatalf("deleted file: err %v, want errConcurrentEdit", err)
	}
}

// TestWriteAtomic_RecheckBeforeRename covers the second half of the
// compare-and-swap: a write that lands after the first check but before
// the rename still aborts the swap.
func TestWriteAtomic_RecheckBeforeRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, err := writeAtomic(path, []byte("new"), func(cur []byte) error {
		calls++
		if calls == 1 {
			// Simulate an editor save between the checks.
			return os.WriteFile(path, []byte("edited"), 0o644)
		}
		if string(cur) != "old" {
			return errConcurrentEdit
		}
		return nil
	})
	if !errors.Is(err, errConcurrentEdit) {
		t.Fatalf("err %v, want errConcurrentEdit", err)
	}
	if got := readFile(t, path); got != "edited" {
		t.Fatalf("concurrent edit undone: %q", got)
	}
}

func TestCmdInherit_IsARemovedStub(t *testing.T) {
	a, _, out := newAgentsMDApp(t, "")
	if err := a.CmdInherit(); !errors.Is(err, errInheritRemoved) {
		t.Fatalf("err %v", err)
	}
	if !strings.Contains(out.String(), "docs/inheritance.md") {
		t.Errorf("stub does not point at the docs:\n%s", out.String())
	}
}
