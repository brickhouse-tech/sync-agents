package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// audit_test.go covers the SPEC-010 Phase 1 acceptance criteria: the
// reverse sweep (foreign / orphaned) and folded-ancestor recognition.

// AC-3: an entry in a managed subdir that no artifact claims and that
// doesn't point into .agents/ reports foreign — and stays untouched.
func TestGlobalStatus_ForeignReported(t *testing.T) {
	a, root, stdout := newStatusTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "managed", "body\n")

	foreign := filepath.Join(root, ".claude", "skills", "hundred-million-offers")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(foreign, "SKILL.md")
	if err := os.WriteFile(marker, []byte("# not ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[foreign] claude/hundred-million-offers") {
		t.Errorf("expected foreign row for hundred-million-offers:\n%s", out)
	}
	// Untouched: read-only audit.
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("audit mutated a foreign entry: %v", err)
	}
}

// AC-4: a symlink into the .agents/ tree with no claiming artifact
// reports orphaned, not foreign.
func TestGlobalStatus_OrphanedSymlink(t *testing.T) {
	a, root, stdout := newStatusTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "managed", "body\n")

	commandsDir := filepath.Join(root, ".claude", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Points into .agents/ but the artifact is gone.
	gone := filepath.Join(a.ResolveGlobalRoot(), "workflows", "deleted.md")
	if err := os.Symlink(gone, filepath.Join(commandsDir, "deleted.md")); err != nil {
		t.Fatal(err)
	}

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[orphaned] claude/deleted.md") {
		t.Errorf("expected orphaned row for deleted.md:\n%s", out)
	}
}

// AC-2: a dir-level symlink into .agents/ that resolves the expected
// artifact reports folded — and the sweep does not double-report it.
func TestGlobalStatus_FoldedSkillRecognized(t *testing.T) {
	a, root, stdout := newStatusTestApp(t)
	seedSkill(t, a.ResolveGlobalRoot(), "decision-helper",
		"---\nname: decision-helper\n---\nbody\n", nil)

	skillsDir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The fold: dir-level link instead of the per-SKILL.md link a
	// fresh sync would create.
	src := filepath.Join(a.ResolveGlobalRoot(), "skills", "decision-helper")
	if err := os.Symlink(src, filepath.Join(skillsDir, "decision-helper")); err != nil {
		t.Fatal(err)
	}

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[folded] claude/skill/decision-helper") {
		t.Errorf("expected folded row for decision-helper:\n%s", out)
	}
	if strings.Contains(out, "[foreign] claude/decision-helper") ||
		strings.Contains(out, "[orphaned] claude/decision-helper") {
		t.Errorf("folded skill double-reported by the sweep:\n%s", out)
	}
	if strings.Contains(out, "[not-a-symlink] claude/skill/decision-helper") {
		t.Errorf("folded skill misclassified as conflict:\n%s", out)
	}
}

// AC-1: a real dir shadowing a claimed artifact is still a conflict
// (not-a-symlink), never folded/foreign.
func TestGlobalStatus_ConflictStillReported(t *testing.T) {
	a, root, stdout := newStatusTestApp(t)
	seedSkill(t, a.ResolveGlobalRoot(), "find-skills",
		"---\nname: find-skills\n---\nbody\n", nil)

	shadow := filepath.Join(root, ".claude", "skills", "find-skills")
	if err := os.MkdirAll(shadow, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "SKILL.md"), []byte("# local fork\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[not-a-symlink] claude/skill/find-skills") {
		t.Errorf("expected conflict row for find-skills:\n%s", out)
	}
}

// AC-5: tool app state (files outside the managed subdirs) never
// appears in output.
func TestGlobalStatus_AppStateNeverSwept(t *testing.T) {
	a, root, stdout := newStatusTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "managed", "body\n")

	claude := filepath.Join(root, ".claude")
	if err := os.MkdirAll(filepath.Join(claude, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".credentials.json", "history.jsonl", "settings.json"} {
		if err := os.WriteFile(filepath.Join(claude, f), []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	for _, banned := range []string{".credentials.json", "history.jsonl", "sessions", "settings.json"} {
		if strings.Contains(out, banned) {
			t.Errorf("app-state entry %q leaked into audit output:\n%s", banned, out)
		}
	}
}

// The summary line counts every state across forward pass + sweep.
func TestGlobalStatus_SummaryLine(t *testing.T) {
	a, root, stdout := newStatusTestApp(t)
	seedRule(t, a.ResolveGlobalRoot(), "managed", "body\n")
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills", "stray"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := a.CmdGlobalStatus(GlobalStatusOpts{}); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "audit:") {
		t.Errorf("expected audit summary line:\n%s", out)
	}
	if !strings.Contains(out, "1 foreign") {
		t.Errorf("expected '1 foreign' in summary:\n%s", out)
	}
}
