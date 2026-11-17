package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

// SPEC-015 C4

const triggerWarning = "rules without a valid `trigger:` (always_on, model_decision, glob, manual) are dropped by "

func seedTriggerRules(t *testing.T, rules string) {
	t.Helper()
	writeFile(t, filepath.Join(rules, "ok.md"), "---\ntrigger: always_on\n---\nFine.\n")
	writeFile(t, filepath.Join(rules, "bad.md"), "---\ntrigger: sometimes\n---\nInvalid value.\n")
	writeFile(t, filepath.Join(rules, "macos", "brew.md"), "Use brew.\n")
}

// SPEC-015 C4: local sync lists every failing rule once, naming the
// tools that read the rule files.
func TestSync_WarnsOnRulesWithoutTrigger(t *testing.T) {
	for _, c := range []struct {
		targets []string
		want    string
	}{
		{[]string{"antigravity"}, triggerWarning + "antigravity: bad, macos/brew, r."},
		{[]string{"claude", "antigravity", "devin"}, triggerWarning + "devin, antigravity: bad, macos/brew, r."},
		{[]string{"devin"}, triggerWarning + "devin: bad, macos/brew, r."},
	} {
		app, root, buf := newSyncApp(t, c.targets...)
		seedTriggerRules(t, filepath.Join(root, ".agents", "rules"))
		if err := app.CmdSync(); err != nil {
			t.Fatalf("sync %v: %v", c.targets, err)
		}
		out := buf.String()
		if n := strings.Count(out, triggerWarning); n != 1 {
			t.Errorf("%v: %d trigger warnings, want 1:\n%s", c.targets, n, out)
		}
		if !strings.Contains(out, "[warn] "+c.want) {
			t.Errorf("%v: missing %q in:\n%s", c.targets, c.want, out)
		}
		if !strings.Contains(out, "`sync-agents -d "+root+" lint --fix`") {
			t.Errorf("%v: the warning does not name the fix:\n%s", c.targets, out)
		}
	}
}

// SPEC-015 C4: tools that do not read rule files get no warning.
func TestSync_NoTriggerWarningForOtherTools(t *testing.T) {
	app, root, buf := newSyncApp(t, "claude", "cursor")
	seedTriggerRules(t, filepath.Join(root, ".agents", "rules"))
	if err := app.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if strings.Contains(buf.String(), triggerWarning) {
		t.Errorf("trigger warning without antigravity or devin:\n%s", buf.String())
	}
}

// SPEC-015 C4: at user scope Antigravity reads the linked rule files.
// Devin's rules arrive inlined in its devin-rules region, where the
// trigger key plays no part, so devin is not named there.
func TestGlobalSync_WarnsOnRulesWithoutTrigger(t *testing.T) {
	a, root, buf := newGlobalSyncTestApp(t)
	writeFile(t, filepath.Join(a.GlobalRoot, "config"), "os = darwin\n")
	seedTriggerRules(t, filepath.Join(a.GlobalRoot, "rules"))
	mkdirs(t, filepath.Join(root, ".config", "devin"))

	globalSync(t, a, "antigravity", "devin")
	if want := "[warn] " + triggerWarning + "antigravity: bad, macos/brew."; !strings.Contains(buf.String(), want) {
		t.Errorf("missing %q in:\n%s", want, buf.String())
	}

	buf.Reset()
	globalSync(t, a, "devin")
	if strings.Contains(buf.String(), triggerWarning) {
		t.Errorf("trigger warning for devin's inlined region:\n%s", buf.String())
	}
}
