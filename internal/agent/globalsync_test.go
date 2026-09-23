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
// symlink we own — it points into the canonical tree, just at the
// wrong artifact after a rename — gets silently re-pointed. No
// --force needed, because nothing outside the tree is at risk.
func TestCmdGlobalSync_DriftedSymlinkRepaired(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	gr := a.ResolveGlobalRoot()
	seedRule(t, gr, "x", "body\n")
	seedRule(t, gr, "old-name", "stale\n")

	// A stale link into our own tree: the artifact was renamed
	// old-name -> x, leaving .claude/rules/x.md aimed at the old file.
	claudeRule := filepath.Join(root, ".claude", "rules", "x.md")
	if err := os.MkdirAll(filepath.Dir(claudeRule), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(filepath.Join(gr, "rules", "old-name.md"), claudeRule); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got, err := os.Readlink(claudeRule)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	want := filepath.Join(gr, "rules", "x.md")
	if got != want {
		t.Errorf("symlink target = %q, want %q", got, want)
	}
}

// TestCmdGlobalSync_ForeignSymlinkPreserved is the SPEC-011 Part A
// hazard regression: a symlink pointing OUTSIDE the canonical tree is
// somebody else's wiring (the motivating case is a personas directory
// hand-linked into ~/.claude/agents/). Before the ownership check,
// this fell into the drift branch and was deleted outright — no
// backup, no --force gate, no way to tell it had happened.
func TestCmdGlobalSync_ForeignSymlinkPreserved(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "x", "body\n")

	foreignTarget := filepath.Join(root, "elsewhere.md")
	if err := os.WriteFile(foreignTarget, []byte("hand-managed"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	claudeRule := filepath.Join(root, ".claude", "rules", "x.md")
	if err := os.MkdirAll(filepath.Dir(claudeRule), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(foreignTarget, claudeRule); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Sync keeps going — one conflict must not abort a whole run — but
	// it has to say so, both inline and in the end-of-run summary.
	_, _, out := a, root, a.Stdout.(*bytes.Buffer)
	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("sync should warn and continue, not fail: %v", err)
	}
	log := out.String()
	if !strings.Contains(log, "outside the sync-agents tree") {
		t.Errorf("expected an inline warning naming the foreign link; got:\n%s", log)
	}
	if !strings.Contains(log, "left untouched because sync-agents did not create them") {
		t.Errorf("expected the end-of-run conflict summary; got:\n%s", log)
	}

	// The user's link is exactly as they left it.
	got, err := os.Readlink(claudeRule)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if got != foreignTarget {
		t.Errorf("foreign symlink was modified: target = %q, want %q", got, foreignTarget)
	}
	if _, err := os.Stat(foreignTarget); err != nil {
		t.Errorf("foreign symlink's target must survive: %v", err)
	}
}

// TestCmdGlobalSync_ForeignSymlinkForcedIsRecoverable: --force may
// take the path, but never by deletion. The displaced link is renamed
// to a timestamped sibling so the user can put it back.
func TestCmdGlobalSync_ForeignSymlinkForcedIsRecoverable(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	gr := a.ResolveGlobalRoot()
	seedRule(t, gr, "x", "body\n")

	foreignTarget := filepath.Join(root, "elsewhere.md")
	if err := os.WriteFile(foreignTarget, []byte("hand-managed"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	claudeRule := filepath.Join(root, ".claude", "rules", "x.md")
	if err := os.MkdirAll(filepath.Dir(claudeRule), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(foreignTarget, claudeRule); err != nil {
		t.Fatalf("setup: %v", err)
	}

	a.Force = true
	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("forced sync: %v", err)
	}

	if got, want := readlinkOrFail(t, claudeRule), filepath.Join(gr, "rules", "x.md"); got != want {
		t.Errorf("symlink target = %q, want %q", got, want)
	}

	entries, err := os.ReadDir(filepath.Dir(claudeRule))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var backups int
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "x.md"+BackupSuffix) {
			backups++
			if got := readlinkOrFail(t, filepath.Join(filepath.Dir(claudeRule), e.Name())); got != foreignTarget {
				t.Errorf("backup points at %q, want the original %q", got, foreignTarget)
			}
		}
	}
	if backups != 1 {
		t.Errorf("found %d backups, want exactly 1", backups)
	}
}

// TestPointsIntoGlobalTree covers the ownership predicate directly,
// including the relative-target form the kernel resolves against the
// link's own directory, and the prefix-collision case where a
// sibling path merely starts with the root's characters.
func TestPointsIntoGlobalTree(t *testing.T) {
	a := &App{GlobalRoot: "/home/u/.agents"}
	cases := []struct {
		name     string
		linkPath string
		target   string
		want     bool
	}{
		{"absolute inside", "/home/u/.claude/rules/x.md", "/home/u/.agents/rules/x.md", true},
		{"the root itself", "/home/u/.claude/rules", "/home/u/.agents", true},
		{"absolute outside", "/home/u/.claude/agents/tars.md", "/home/u/personas/tars.md", false},
		{"relative into tree", "/home/u/.claude/rules/x.md", "../../.agents/rules/x.md", true},
		{"relative outside", "/home/u/.claude/rules/x.md", "../../personas/x.md", false},
		{"prefix collision", "/home/u/.claude/rules/x.md", "/home/u/.agents-backup/rules/x.md", false},
		{"empty target", "/home/u/.claude/rules/x.md", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.pointsIntoGlobalTree(tc.linkPath, tc.target); got != tc.want {
				t.Errorf("pointsIntoGlobalTree(%q, %q) = %v, want %v", tc.linkPath, tc.target, got, tc.want)
			}
		})
	}
}

// readlinkOrFail reads a symlink or fails the test.
func readlinkOrFail(t *testing.T, path string) string {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}
	return got
}

// TestCmdGlobalSync_ForceSkipsFoldedAncestor is the issue #90
// regression: when a tool's per-artifact dir is itself a symlink into
// ~/.agents/ (a "fold"), the destination path resolves through that
// ancestor symlink straight to the canonical source file. Force must
// not treat that as a conflicting non-symlink and rename it away —
// doing so moves the real file to a `.replaced-by-sync-agents`
// backup and then symlinks the (now-empty) original path to itself,
// producing "too many levels of symbolic links".
func TestCmdGlobalSync_ForceSkipsFoldedAncestor(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	a.Force = true
	seedRule(t, a.ResolveGlobalRoot(), "state", "be careful\n")

	// The fold: ~/.claude/rules -> ~/.agents/rules, so
	// ~/.claude/rules/state.md resolves to the same file a fresh sync
	// would otherwise symlink individually.
	rulesDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	src := filepath.Join(a.ResolveGlobalRoot(), "rules")
	if err := os.Symlink(src, filepath.Join(rulesDir, "rules")); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// The canonical source must still be a real, readable file — not
	// a symlink pointing at itself.
	sourcePath := filepath.Join(a.ResolveGlobalRoot(), "rules", "state.md")
	info, err := os.Lstat(sourcePath)
	if err != nil {
		t.Fatalf("source file gone: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("source file was replaced with a symlink: %s", sourcePath)
	}
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("source file unreadable: %v", err)
	}
	if string(content) != "be careful\n" {
		t.Errorf("source content = %q, want %q", content, "be careful\n")
	}

	// No backup should have been created anywhere under root.
	err = filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(fi.Name(), ".replaced-by-sync-agents") {
			t.Errorf("unexpected backup file: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
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

// TestCmdGlobalSync_WritesClaudeImportsBlock verifies the bridge
// between "sync placed the rule at ~/.claude/rules/X.md" and "Claude
// actually loads rule X": a managed @-import block is regenerated in
// ~/.claude/CLAUDE.md listing every passive rule routed to Claude.
// See issue #46 — Claude does not auto-scan rules/*.md.
func TestCmdGlobalSync_WritesClaudeImportsBlock(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "security", "be careful\n")
	seedRule(t, a.ResolveGlobalRoot(), "no-secrets", "no PII\n")
	seedWorkflow(t, a.ResolveGlobalRoot(), "passive-wf", "`\n```\n")

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	claudeMD := filepath.Join(root, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)

	if !HasManagedImportBlock(content) {
		t.Fatalf("missing managed block in CLAUDE.md:\n%s", content)
	}

	imports := ExtractManagedImports(content)
	wantImports := map[string]bool{
		filepath.Join(root, ".claude", "rules", "no-secrets.md"): true,
		filepath.Join(root, ".claude", "rules", "security.md"):   true,
	}
	if len(imports) != len(wantImports) {
		t.Errorf("got %d imports (%v), want %d: %v", len(imports), imports, len(wantImports), wantImports)
	}
	for _, imp := range imports {
		if !wantImports[imp] {
			t.Errorf("unexpected import %q", imp)
		}
	}
}

// TestCmdGlobalSync_ClaudeImportsOmitsInvocables verifies that
// invocable skills (which Claude auto-discovers at ~/.claude/skills/)
// and invocable workflows (which Claude auto-registers at
// ~/.claude/commands/) do NOT contribute to the @-import block — they
// don't need it.
func TestCmdGlobalSync_ClaudeImportsOmitsInvocables(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedSkill(t, a.ResolveGlobalRoot(), "cool", "skill MD\n", nil) // invocable by default
	seedWorkflow(t, a.ResolveGlobalRoot(), "wf", "workflow MD\n")  // invocable by default

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	claudeMD := filepath.Join(root, ".claude", "CLAUDE.md")
	// File may not exist if there were no passive rules — that's
	// correct behavior; the sync shouldn't create an empty block
	// when nothing needs it.
	if _, err := os.Stat(claudeMD); err == nil {
		t.Errorf("CLAUDE.md exists but should not (no passive rules):\n%s", func() string {
			b, _ := os.ReadFile(claudeMD)
			return string(b)
		}())
	}
}

// TestCmdGlobalSync_ClaudeImportsPreservesUserContent confirms that
// any user-authored content at ~/.claude/CLAUDE.md survives the
// managed-block rewrite.
func TestCmdGlobalSync_ClaudeImportsPreservesUserContent(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	claudeDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	claudeMD := filepath.Join(claudeDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte("# Custom Claude config\n\nUser keeps this.\n"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	seedRule(t, a.ResolveGlobalRoot(), "security", "rule body\n")

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	content, _ := os.ReadFile(claudeMD)
	s := string(content)

	if !strings.Contains(s, "# Custom Claude config") {
		t.Errorf("user content lost:\n%s", s)
	}
	if !HasManagedImportBlock(s) {
		t.Errorf("managed block not added:\n%s", s)
	}
}

// TestCmdGlobalSync_ClaudeImportsDryRun verifies dry-run doesn't
// touch the filesystem but still reports the plan.
func TestCmdGlobalSync_ClaudeImportsDryRun(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "security", "body\n")
	a.DryRun = true

	if err := a.CmdGlobalSync(GlobalSyncOpts{}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	claudeMD := filepath.Join(root, ".claude", "CLAUDE.md")
	if _, err := os.Stat(claudeMD); err == nil {
		t.Errorf("dry-run created CLAUDE.md")
	}
	if !strings.Contains(stdout.String(), "[dry-run]") {
		t.Errorf("expected [dry-run] in output:\n%s", stdout.String())
	}
}

// TestCmdGlobalSync_ClaudeImportsTargetsFilter verifies that when
// --targets excludes claude, no @-import block is written for
// Claude.
func TestCmdGlobalSync_ClaudeImportsTargetsFilter(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "security", "body\n")

	// Only sync cursor — claude should be skipped.
	if err := a.CmdGlobalSync(GlobalSyncOpts{Targets: []string{"cursor"}}); err != nil {
		t.Fatalf("CmdGlobalSync: %v", err)
	}

	claudeMD := filepath.Join(root, ".claude", "CLAUDE.md")
	if _, err := os.Stat(claudeMD); err == nil {
		data, _ := os.ReadFile(claudeMD)
		t.Errorf("CLAUDE.md written when claude target filtered:\n%s", string(data))
	}
}
