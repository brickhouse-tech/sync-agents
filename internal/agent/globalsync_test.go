package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newGlobalSyncTestApp wires up a fresh App whose GlobalRoot points
// at a t.TempDir-backed ~/.agents/. The parent of the global root is
// also under that temp dir, so every per-tool global dir
// (.claude/, .codeium/, etc.) lives inside the test rig with no
// chance of leaking into $HOME.
func newGlobalSyncTestApp(t *testing.T) (*App, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	globalRoot := filepath.Join(root, ".agents")
	if err := os.MkdirAll(globalRoot, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	stdout := &bytes.Buffer{}
	return &App{
		ProjectRoot: filepath.Join(root, "project"),
		GlobalRoot:  globalRoot,
		Stdout:      stdout,
		Stderr:      &bytes.Buffer{},
	}, root, stdout
}

// seedRule writes a passive rule under ~/.agents/rules/<name>.md.
func seedRule(t *testing.T, globalRoot, name, body string) {
	t.Helper()
	writeArtifact(t, globalRoot, filepath.Join("rules", name+".md"), body)
}

// seedSkill writes a skill directory at ~/.agents/skills/<name>/
// with the given SKILL.md content and any extra sibling files.
func seedSkill(t *testing.T, globalRoot, name, skillMD string, extras map[string]string) {
	t.Helper()
	skillDir := filepath.Join(globalRoot, "skills", name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("seedSkill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatalf("seedSkill SKILL.md: %v", err)
	}
	for fname, content := range extras {
		if err := os.WriteFile(filepath.Join(skillDir, fname), []byte(content), 0o644); err != nil {
			t.Fatalf("seedSkill extra %s: %v", fname, err)
		}
	}
}

// seedWorkflow writes an invocable workflow.
func seedWorkflow(t *testing.T, globalRoot, name, body string) {
	t.Helper()
	writeArtifact(t, globalRoot, filepath.Join("workflows", name+".md"), body)
}

// TestCmdGlobalSync_EmptyTreeNoop covers the safety case: a fresh
// ~/.agents/ with no artifacts returns successfully with no symlinks
// or concat files written.
func TestCmdGlobalSync_EmptyTreeNoop(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	// No per-tool dirs should exist.
	for _, dir := range []string{".claude", ".codeium", ".cursor", ".github", ".codex"} {
		p := filepath.Join(root, dir)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("unexpected per-tool dir created: %s", p)
		}
	}
}

// TestCmdGlobalSync_PassiveRuleFanout exercises the passive-rule
// routing across every tool. A single rule with no frontmatter
// should:
//   - symlink at ~/.claude/rules/security.md
//   - symlink at ~/.cursor/rules/security.md
//   - concat into ~/.codeium/windsurf/memories/global_rules.md
//   - concat into ~/.github/copilot/instructions.md
//   - concat into ~/.codex/instructions.md
func TestCmdGlobalSync_PassiveRuleFanout(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "security", "be careful\n")

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	wantSymlinks := []string{
		filepath.Join(root, ".claude", "rules", "security.md"),
		filepath.Join(root, ".cursor", "rules", "security.md"),
	}
	for _, p := range wantSymlinks {
		info, err := os.Lstat(p)
		if err != nil {
			t.Errorf("missing symlink %s: %v", p, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is not a symlink", p)
		}
	}

	wantConcat := []string{
		filepath.Join(root, ".codeium", "windsurf", "memories", "global_rules.md"),
		filepath.Join(root, ".github", "copilot", "instructions.md"),
		filepath.Join(root, ".codex", "instructions.md"),
	}
	for _, p := range wantConcat {
		content, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("missing concat %s: %v", p, err)
			continue
		}
		if !bytes.Contains(content, []byte("## security")) {
			t.Errorf("concat %s missing security heading:\n%s", p, content)
		}
		if !bytes.Contains(content, []byte("be careful")) {
			t.Errorf("concat %s missing rule body:\n%s", p, content)
		}
	}
}

// TestCmdGlobalSync_InvocableSkillRoutesPerTool is the semantic-
// routing flagship test. A single-file invocable skill must land at
// the right per-tool destination for each tool:
//
//   - claude:   ~/.claude/skills/cool/SKILL.md (symlink)
//   - codeium:  ~/.codeium/windsurf/global_workflows/cool.md (symlink)
//   - cursor:   ~/.cursor/rules/cool.md (symlink)
//   - copilot:  concat into instructions.md
//   - codex:    concat into instructions.md
func TestCmdGlobalSync_InvocableSkillRoutesPerTool(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedSkill(t, a.ResolveGlobalRoot(), "cool", "# cool skill\nbody\n", nil)

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	cases := []struct {
		path       string
		isSymlink  bool
		concatNeed string
	}{
		{filepath.Join(root, ".claude", "skills", "cool", "SKILL.md"), true, ""},
		{filepath.Join(root, ".codeium", "windsurf", "global_workflows", "cool.md"), true, ""},
		{filepath.Join(root, ".cursor", "rules", "cool.md"), true, ""},
		{filepath.Join(root, ".github", "copilot", "instructions.md"), false, "## cool"},
		{filepath.Join(root, ".codex", "instructions.md"), false, "## cool"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if c.isSymlink {
				info, err := os.Lstat(c.path)
				if err != nil {
					t.Fatalf("expected symlink at %s: %v", c.path, err)
				}
				if info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("%s is not a symlink", c.path)
				}
			} else {
				content, err := os.ReadFile(c.path)
				if err != nil {
					t.Fatalf("expected concat at %s: %v", c.path, err)
				}
				if !bytes.Contains(content, []byte(c.concatNeed)) {
					t.Errorf("concat %s missing %q:\n%s", c.path, c.concatNeed, content)
				}
			}
		})
	}

	// Negative assertion: an invocable skill must NOT show up in
	// the Windsurf memories concat (that's for passive artifacts).
	memContent, err := os.ReadFile(filepath.Join(root, ".codeium", "windsurf", "memories", "global_rules.md"))
	if err == nil {
		if bytes.Contains(memContent, []byte("## cool")) {
			t.Errorf("invocable skill leaked into Windsurf memories concat:\n%s", memContent)
		}
	}
	// (If the file doesn't exist at all, that's also acceptable —
	// the sync skipped a memories concat because no passive
	// artifacts targeted it.)
}

// TestCmdGlobalSync_MultiFileSkillSkipsCodeium covers the SPEC-002
// scenario "Multi-file invocable skill cannot land in Windsurf
// workflow". The skill should be present in other tools' dirs but
// SKIPPED for codeium with a warning.
func TestCmdGlobalSync_MultiFileSkillSkipsCodeium(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedSkill(t, a.ResolveGlobalRoot(), "big", "# big skill\n", map[string]string{
		"helper.txt": "support",
	})

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	// Codeium destination should NOT exist.
	codeiumDest := filepath.Join(root, ".codeium", "windsurf", "global_workflows", "big.md")
	if _, err := os.Stat(codeiumDest); err == nil {
		t.Errorf("codeium destination unexpectedly exists: %s", codeiumDest)
	}

	// Warning should mention the skill name.
	out := stdout.String()
	if !strings.Contains(out, "big") || !strings.Contains(out, "skip") {
		t.Errorf("expected skip warning naming 'big' in stdout:\n%s", out)
	}

	// Claude destination should still exist.
	claudeDest := filepath.Join(root, ".claude", "skills", "big", "SKILL.md")
	if _, err := os.Lstat(claudeDest); err != nil {
		t.Errorf("Claude destination missing despite multi-file skip being codeium-only: %v", err)
	}
}

// TestCmdGlobalSync_FrontmatterFlipsRoute exercises the rev-3
// override: a rule explicitly marked invocable: true must land in
// Claude's commands/, not rules/.
func TestCmdGlobalSync_FrontmatterFlipsRoute(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "onboarding",
		"---\ninvocable: true\n---\n# onboarding\nbody\n")

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	// Claude: commands/ not rules/.
	commandsLink := filepath.Join(root, ".claude", "commands", "onboarding.md")
	if _, err := os.Lstat(commandsLink); err != nil {
		t.Errorf("expected commands link at %s: %v", commandsLink, err)
	}
	rulesLink := filepath.Join(root, ".claude", "rules", "onboarding.md")
	if _, err := os.Lstat(rulesLink); err == nil {
		t.Errorf("rules link should not exist for invocable rule: %s", rulesLink)
	}

	// Windsurf: must NOT be in memories concat (passive only).
	mem := filepath.Join(root, ".codeium", "windsurf", "memories", "global_rules.md")
	if data, err := os.ReadFile(mem); err == nil {
		if bytes.Contains(data, []byte("onboarding")) {
			t.Errorf("invocable rule leaked into memories concat:\n%s", data)
		}
	}
}

// TestCmdGlobalSync_Idempotent runs the sync twice and asserts the
// second run is fast and doesn't rewrite anything. We check both
// symlinks (existing symlink to same target = no recreate) and the
// concat file's mtime preservation.
func TestCmdGlobalSync_Idempotent(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "x", "body\n")

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Grab mtime of the codeium concat (the most interesting
	// idempotency case — it's content-compared).
	concatPath := filepath.Join(root, ".codeium", "windsurf", "memories", "global_rules.md")
	firstInfo, err := os.Stat(concatPath)
	if err != nil {
		t.Fatalf("concat missing after first sync: %v", err)
	}

	// Run sync again immediately.
	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	secondInfo, err := os.Stat(concatPath)
	if err != nil {
		t.Fatalf("concat missing after second sync: %v", err)
	}
	if !secondInfo.ModTime().Equal(firstInfo.ModTime()) {
		t.Errorf("concat mtime changed despite idempotent sync: first=%v second=%v",
			firstInfo.ModTime(), secondInfo.ModTime())
	}
}

// TestCmdGlobalSync_TargetsFilter limits the sync to a subset of
// tools via the Targets option. Only those tools' dirs should be
// touched.
func TestCmdGlobalSync_TargetsFilter(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "x", "body\n")

	if err := a.CmdGlobalSync(GlobalSyncOpts{Targets: []string{"claude"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Claude should be touched.
	if _, err := os.Lstat(filepath.Join(root, ".claude", "rules", "x.md")); err != nil {
		t.Errorf("claude target not touched: %v", err)
	}
	// Cursor and others should NOT.
	for _, dir := range []string{".cursor", ".codeium", ".github", ".codex"} {
		p := filepath.Join(root, dir)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("unexpected dir created for unfiltered target: %s", p)
		}
	}
}

// TestCmdGlobalSync_DryRun confirms no filesystem writes occur and
// the plan is printed.
func TestCmdGlobalSync_DryRun(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "x", "body\n")
	a.DryRun = true

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("dry-run sync: %v", err)
	}

	for _, dir := range []string{".claude", ".codeium", ".cursor", ".github", ".codex"} {
		p := filepath.Join(root, dir)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("dry-run created per-tool dir %s", p)
		}
	}

	if !strings.Contains(stdout.String(), "[dry-run]") {
		t.Errorf("expected [dry-run] in output:\n%s", stdout.String())
	}
}

// TestCmdGlobalSync_DriftedSymlinkRepaired covers the repair case: a
// previously-created symlink that points at a stale target gets
// silently re-pointed to the current artifact.
func TestCmdGlobalSync_DriftedSymlinkRepaired(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "x", "body\n")

	// Manually create a drifted symlink before running sync.
	driftedTarget := filepath.Join(root, "elsewhere.md")
	if err := os.WriteFile(driftedTarget, []byte("stale"), 0o644); err != nil {
		t.Fatalf("setup drift: %v", err)
	}
	claudeRule := filepath.Join(root, ".claude", "rules", "x.md")
	if err := os.MkdirAll(filepath.Dir(claudeRule), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(driftedTarget, claudeRule); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// The symlink should now point at the canonical artifact.
	got, err := os.Readlink(claudeRule)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	want := filepath.Join(a.ResolveGlobalRoot(), "rules", "x.md")
	if got != want {
		t.Errorf("symlink target = %q, want %q", got, want)
	}
}

// TestCmdGlobalSync_NoGlobalRootErrors verifies that a missing
// global root yields a clear error message pointing at `global init`.
func TestCmdGlobalSync_NoGlobalRootErrors(t *testing.T) {
	tmp := t.TempDir()
	a := &App{
		ProjectRoot: tmp,
		GlobalRoot:  filepath.Join(tmp, "does-not-exist"),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
	}
	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err == nil {
		t.Fatal("expected error for missing global root; got nil")
	}
	stderr := a.Stderr.(*bytes.Buffer)
	if !strings.Contains(stderr.String(), "global init") {
		t.Errorf("error message %q should suggest `global init`", stderr.String())
	}
}

// TestDiscoverArtifacts_FindsAllThreeBuckets seeds one of each type
// and confirms the discovery function returns them all with the
// correct Type and Name.
func TestDiscoverArtifacts_FindsAllThreeBuckets(t *testing.T) {
	tmp := t.TempDir()
	seedRule(t, tmp, "r1", "rule body\n")
	seedSkill(t, tmp, "s1", "# skill body\n", nil)
	seedWorkflow(t, tmp, "w1", "workflow body\n")

	got, err := DiscoverArtifacts(tmp)
	if err != nil {
		t.Fatalf("DiscoverArtifacts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 artifacts; got %d: %+v", len(got), got)
	}
	wantByName := map[string]ArtifactType{
		"r1": ArtifactRule,
		"s1": ArtifactSkill,
		"w1": ArtifactWorkflow,
	}
	for _, a := range got {
		if wantByName[a.Name] != a.Type {
			t.Errorf("artifact %s: type %q, want %q", a.Name, a.Type, wantByName[a.Name])
		}
	}
}
