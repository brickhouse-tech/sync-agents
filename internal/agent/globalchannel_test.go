package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Global delivery channels (SPEC-013 U6): one fixture per tool, each
// built on newGlobalSyncTestApp, whose temp root stands in for $HOME.

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func globalSync(t *testing.T, a *App, targets ...string) {
	t.Helper()
	if err := a.CmdGlobalSync(GlobalSyncOpts{Targets: targets}); err != nil {
		t.Fatalf("global sync %v: %v", targets, err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// globalStatusOut runs global status and returns its output.
func globalStatusOut(t *testing.T, a *App, targets ...string) string {
	t.Helper()
	buf := a.Stdout.(interface {
		Reset()
		String() string
	})
	buf.Reset()
	if err := a.CmdGlobalStatus(GlobalStatusOpts{Targets: targets}); err != nil {
		t.Fatalf("global status: %v", err)
	}
	return buf.String()
}

func TestGlobalChannel_UninstalledToolsGetNothing(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	seedSkill(t, a.GlobalRoot, "cool", "# cool\n", nil)
	globalSync(t, a)

	for _, p := range []string{".codex", ".copilot", ".codeium", ".config", ".github", ".openclaw", ".agents/index"} {
		if exists(filepath.Join(root, p)) {
			t.Errorf("global sync created %s for a tool that is not installed", p)
		}
	}
	for _, tool := range []string{"[codex]", "[copilot]", "[codeium]", "[opencode]", "[openclaw]"} {
		if strings.Contains(stdout.String(), tool) {
			t.Errorf("output talks about an uninstalled tool %s:\n%s", tool, stdout)
		}
	}
	if !exists(filepath.Join(root, ".claude", "rules", "security.md")) {
		t.Error("Claude delivery must not depend on the channel gate")
	}
}

func TestGlobalChannel_CodexCreatesRegionHostAndIsIdempotent(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	mkdirs(t, filepath.Join(root, ".codex"))
	globalSync(t, a)

	host := filepath.Join(root, ".codex", "AGENTS.md")
	want := CodexRulesRegion.Start() + "\n" + globalBanner + "\n\n## security\n\nNever commit secrets.\n\n" + CodexRulesRegion.End() + "\n"
	if got := readFile(t, host); got != want {
		t.Fatalf("host:\n got %q\nwant %q", got, want)
	}
	idx := filepath.Join(a.GlobalRoot, "index", "codex.md")
	if got := readFile(t, idx); got != globalBanner+"\n\n## security\n\nNever commit secrets.\n\n" {
		t.Errorf("index = %q", got)
	}

	hostOld, idxOld := setOld(t, host), setOld(t, idx)
	globalSync(t, a)
	if !mtime(t, host).Equal(hostOld) || !mtime(t, idx).Equal(idxOld) {
		t.Error("second global sync rewrote unchanged files")
	}
}

func TestGlobalChannel_CodexUserFileNeedsConsentThenKeepsOutsideBytes(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	host := filepath.Join(root, ".codex", "AGENTS.md")
	user := "# My Codex notes\n\nPrefer small diffs.\n"
	writeArtifact(t, root, filepath.Join(".codex", "AGENTS.md"), user)

	globalSync(t, a)
	if got := readFile(t, host); got != user {
		t.Fatalf("plain global sync edited a user file without consent:\n%s", got)
	}
	if out := globalStatusOut(t, a); !strings.Contains(out, "[unmounted] codex -> "+host) || !strings.Contains(out, "run once with --targets codex") {
		t.Errorf("status should explain the missing consent:\n%s", out)
	}

	stdout.Reset()
	globalSync(t, a, "codex")
	got := readFile(t, host)
	if !strings.HasPrefix(got, user+"\n"+CodexRulesRegion.Start()+"\n") || !strings.Contains(got, "Never commit secrets.") {
		t.Fatalf("consented sync did not append the region after the user text:\n%s", got)
	}

	// The markers are consent from now on: a plain sync refreshes the
	// region and keeps edits made outside it.
	edited := "Top line added later.\n" + got + "Bottom line added later.\n"
	if err := os.WriteFile(host, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	seedRule(t, a.GlobalRoot, "security", "Rotate keys.\n")
	globalSync(t, a)
	after := readFile(t, host)
	if !strings.HasPrefix(after, "Top line added later.\n"+user) || !strings.HasSuffix(after, CodexRulesRegion.End()+"\nBottom line added later.\n") {
		t.Errorf("bytes outside the region changed:\n%s", after)
	}
	if !strings.Contains(after, "Rotate keys.") || strings.Contains(after, "Never commit secrets.") {
		t.Errorf("region not refreshed:\n%s", after)
	}
}

func TestGlobalChannel_CodexShadowedByOverride(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	writeArtifact(t, root, filepath.Join(".codex", "AGENTS.override.md"), "override text\n")
	globalSync(t, a)

	override := filepath.Join(root, ".codex", "AGENTS.override.md")
	if !strings.Contains(stdout.String(), "codex reads "+override+" instead") {
		t.Errorf("sync should warn that the override hides the region:\n%s", stdout)
	}
	if out := globalStatusOut(t, a); !strings.Contains(out, "[shadowed] codex -> "+filepath.Join(root, ".codex", "AGENTS.md")) {
		t.Errorf("status should report the channel as shadowed:\n%s", out)
	}
}

func TestGlobalChannel_CodexHomeFromEnv(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	home := filepath.Join(root, "elsewhere", "codex")
	mkdirs(t, home)
	a.ToolEnv = mapEnv(map[string]string{"CODEX_HOME": home}, nil)
	globalSync(t, a)

	if !strings.Contains(readFile(t, filepath.Join(home, "AGENTS.md")), CodexRulesRegion.Start()) {
		t.Error("region not delivered to $CODEX_HOME/AGENTS.md")
	}
	if exists(filepath.Join(root, ".codex")) {
		t.Error("~/.codex created although $CODEX_HOME points elsewhere")
	}
}

// TestGlobalChannel_CodexOverCapDemotesWithReserved: the user's text
// outside the region counts against project_doc_max_bytes, rules that
// no longer fit become pointers, the warning names the knob, and the
// whole file stays within the cap.
func TestGlobalChannel_CodexOverCapDemotesWithReserved(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "big", strings.Repeat("big rule text. ", 40)+"\n")
	seedRule(t, a.GlobalRoot, "small", "Small rule.\n")
	user := strings.Repeat("user line\n", 30)
	writeArtifact(t, root, filepath.Join(".codex", "AGENTS.md"), user)
	writeArtifact(t, root, filepath.Join(".codex", "config.toml"), "project_doc_max_bytes = 1000\n")
	globalSync(t, a, "codex")

	host := readFile(t, filepath.Join(root, ".codex", "AGENTS.md"))
	if len(host) > 1000 {
		t.Errorf("host is %d bytes, over the 1000-byte cap", len(host))
	}
	if !strings.HasPrefix(host, user) {
		t.Error("user text changed")
	}
	if !strings.Contains(host, "## small\n\nSmall rule.") || !strings.Contains(host, "- big (~/.agents/rules/big.md)") {
		t.Errorf("want small inlined and big as a pointer:\n%s", host)
	}
	if !strings.Contains(stdout.String(), "1 rule(s) did not fit in 1000 bytes and are pointers instead: big. Raise project_doc_max_bytes in "+filepath.Join(root, ".codex", "config.toml")) {
		t.Errorf("missing demotion warning naming the knob:\n%s", stdout)
	}
}

func TestGlobalChannel_WindsurfLegacyConcatBecomesRegion(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	host := filepath.Join(root, ".codeium", "windsurf", "memories", "global_rules.md")
	writeArtifact(t, root, filepath.Join(".codeium", "windsurf", "memories", "global_rules.md"),
		legacyConcatBanner+" — do not edit by hand.\nSource: ~/.agents/\n-->\n\n## security\n\nold text\n\n")

	globalSync(t, a)
	want := CodeiumRulesRegion.Start() + "\n" + globalBanner + "\n\n## security\n\nNever commit secrets.\n\n" + CodeiumRulesRegion.End() + "\n"
	if got := readFile(t, host); got != want {
		t.Fatalf("legacy concat not rewritten as the region:\n%s", got)
	}
	if !strings.Contains(stdout.String(), "Rewrote the old generated file as region codeium-rules") {
		t.Errorf("rewrite not reported:\n%s", stdout)
	}

	// A memory the Windsurf UI adds outside the markers survives.
	if err := os.WriteFile(host, []byte("Remember: tabs.\n\n"+want), 0o644); err != nil {
		t.Fatal(err)
	}
	seedRule(t, a.GlobalRoot, "testing", "Run the tests.\n")
	globalSync(t, a)
	got := readFile(t, host)
	if !strings.HasPrefix(got, "Remember: tabs.\n\n"+CodeiumRulesRegion.Start()) || !strings.Contains(got, "## testing") {
		t.Errorf("UI edit lost or region not refreshed:\n%s", got)
	}
}

func TestGlobalChannel_CopilotLinkNeedsHome(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	link := filepath.Join(root, ".copilot", "instructions", "sync-agents.instructions.md")
	idx := filepath.Join(a.GlobalRoot, "index", "copilot.md")

	globalSync(t, a)
	if exists(filepath.Join(root, ".copilot")) {
		t.Fatal("global sync created ~/.copilot without Copilot installed")
	}

	globalSync(t, a, "copilot")
	target, err := os.Readlink(link)
	if err != nil || target != idx {
		t.Fatalf("link = %q (%v), want an absolute link to %s", target, err, idx)
	}
	if got := readFile(t, link); !strings.HasPrefix(got, "---\napplyTo: \"**\"\n---\n"+globalBanner) {
		t.Errorf("Copilot reads %q", got)
	}
}

func TestGlobalChannel_OpencodeEntry(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	dir := filepath.Join(root, ".config", "opencode")
	mkdirs(t, dir)
	cfg := filepath.Join(dir, "opencode.json")
	entry := filepath.Join(a.GlobalRoot, "index", "opencode.md")

	globalSync(t, a)
	if got := readFile(t, cfg); got != "{\n  \"instructions\": [\""+entry+"\"]\n}\n" {
		t.Fatalf("created config = %q", got)
	}

	// An existing config is the user's: untouched until --targets.
	user := "{\n  \"theme\": \"dark\",\n  \"model\": \"x\"\n}\n"
	if err := os.WriteFile(cfg, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	globalSync(t, a)
	if got := readFile(t, cfg); got != user {
		t.Fatalf("plain sync edited opencode.json:\n%s", got)
	}
	globalSync(t, a, "opencode")
	got := readFile(t, cfg)
	if !strings.Contains(got, `"theme": "dark",`) || !strings.Contains(got, `"instructions": ["`+entry+`"]`) {
		t.Errorf("entry not inserted or formatting lost:\n%s", got)
	}
}

func TestGlobalChannel_CursorGapRow(t *testing.T) {
	a, _, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	out := globalStatusOut(t, a, "cursor")
	if !strings.Contains(out, "[gap] cursor  (Cursor keeps user rules in app settings") {
		t.Errorf("want one gap row for Cursor:\n%s", out)
	}
	if strings.Contains(out, "cursor/rule/security") {
		t.Errorf("passive rules must not get per-artifact Cursor rows:\n%s", out)
	}
}

func TestGlobalChannel_LegacyPlacements(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	src := filepath.Join(a.GlobalRoot, "rules", "security.md")
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	banner := legacyConcatBanner + " — do not edit by hand.\n-->\n\n## security\n\nold\n"
	writeArtifact(t, root, filepath.Join(".github", "copilot", "instructions.md"), banner)
	writeArtifact(t, root, filepath.Join(".codex", "instructions.md"), "my own codex notes\n")
	writeArtifact(t, root, filepath.Join(".cursor", "rules", "mine.mdc"), "user rule\n")
	mkdirs(t, filepath.Join(root, ".cursor", "rules", "macos"))
	for _, l := range []struct{ at, to string }{
		{filepath.Join(".cursor", "rules", "security.md"), src},
		{filepath.Join(".cursor", "rules", "macos", "brew.md"), filepath.Join(a.GlobalRoot, "rules", "macos", "brew.md")},
		{filepath.Join(".cursor", "rules", "elsewhere.md"), filepath.Join(root, "notes.md")},
	} {
		if err := os.Symlink(l.to, filepath.Join(root, l.at)); err != nil {
			t.Fatal(err)
		}
	}

	globalSync(t, a)
	for _, gone := range []string{
		filepath.Join(".github"),
		filepath.Join(".cursor", "rules", "security.md"),
		filepath.Join(".cursor", "rules", "macos"),
	} {
		if exists(filepath.Join(root, gone)) {
			t.Errorf("legacy %s not removed", gone)
		}
	}
	for _, kept := range []string{
		filepath.Join(".codex", "instructions.md"),
		filepath.Join(".cursor", "rules", "mine.mdc"),
		filepath.Join(".cursor", "rules", "elsewhere.md"),
	} {
		if !exists(filepath.Join(root, kept)) {
			t.Errorf("%s is the user's and must stay", kept)
		}
	}
	if !strings.Contains(stdout.String(), "left "+filepath.Join(root, ".codex", "instructions.md")+": it does not start with the sync-agents banner") {
		t.Errorf("the kept non-bannered file is not reported:\n%s", stdout)
	}
}

func TestGlobalChannel_CleanUndoesEveryChannel(t *testing.T) {
	a, root, _ := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	// Installed tools keep state in their homes; clean must leave it.
	for _, home := range []string{".codex", ".copilot", filepath.Join(".config", "opencode")} {
		writeArtifact(t, root, filepath.Join(home, "state.json"), "{}\n")
	}
	windsurf := "Remember: tabs.\n"
	writeArtifact(t, root, filepath.Join(".codeium", "windsurf", "memories", "global_rules.md"), windsurf)
	globalSync(t, a, "claude", "codex", "copilot", "opencode", "windsurf")

	if err := a.CmdGlobalClean(GlobalCleanOpts{}); err != nil {
		t.Fatalf("clean: %v", err)
	}
	for _, gone := range []string{
		filepath.Join(".codex", "AGENTS.md"),
		filepath.Join(".copilot", "instructions"),
		filepath.Join(".config", "opencode", "opencode.json"),
		filepath.Join(".agents", "index"),
	} {
		if exists(filepath.Join(root, gone)) {
			t.Errorf("clean left %s", gone)
		}
	}
	for _, home := range []string{".codex", ".copilot", filepath.Join(".config", "opencode")} {
		if !exists(filepath.Join(root, home)) {
			t.Errorf("clean removed the tool home %s", home)
		}
	}
	if got := readFile(t, filepath.Join(root, ".codeium", "windsurf", "memories", "global_rules.md")); got != windsurf {
		t.Errorf("clean did not restore global_rules.md: %q", got)
	}
}

func TestGlobalChannel_DryRunWritesNothing(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	mkdirs(t, filepath.Join(root, ".codex"), filepath.Join(root, ".copilot"))
	a.DryRun = true
	globalSync(t, a)
	for _, p := range []string{filepath.Join(".codex", "AGENTS.md"), filepath.Join(".copilot", "instructions"), filepath.Join(".agents", "index")} {
		if exists(filepath.Join(root, p)) {
			t.Errorf("dry run created %s", p)
		}
	}
	if !strings.Contains(stdout.String(), "would edit: "+filepath.Join(root, ".codex", "AGENTS.md")) {
		t.Errorf("dry run does not show the planned region edit:\n%s", stdout)
	}
}

// TestGlobalChannel_LinkConflictIsCountedNotClobbered: a real file at
// the Copilot link path is the user's. Sync leaves it, warns, and
// exits non-zero.
func TestGlobalChannel_LinkConflictIsCountedNotClobbered(t *testing.T) {
	a, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, a.GlobalRoot, "security", "Never commit secrets.\n")
	mine := writeArtifact(t, root, filepath.Join(".copilot", "instructions", "sync-agents.instructions.md"), "mine\n")

	err := a.CmdGlobalSync(GlobalSyncOpts{})
	if err == nil || !strings.Contains(err.Error(), "1 delivery channel(s) blocked") {
		t.Fatalf("err = %v; want the conflict counted in the exit status", err)
	}
	if got := readFile(t, mine); got != "mine\n" {
		t.Errorf("conflicting file changed: %q", got)
	}
	if !strings.Contains(stdout.String(), "conflict: copilot") {
		t.Errorf("conflict not warned:\n%s", stdout)
	}
}
