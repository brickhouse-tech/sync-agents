package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClaude is a CommandRunner standing in for `claude --version`: it
// returns out, or err when err is non-nil. It fails the run if asked to
// start anything else.
func fakeClaude(out string, err error) CommandRunner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "claude" || len(args) != 1 || args[0] != "--version" {
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
		if err != nil {
			return nil, err
		}
		return []byte(out), nil
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want Version
		ok   bool
	}{
		{"2.1.286 (Claude Code)", Version{2, 1, 286}, true},
		{"2.1.286 (Claude Code)\n", Version{2, 1, 286}, true},
		{"claude v2.1.276", Version{2, 1, 276}, true},
		{"  10.20.30-beta.1", Version{10, 20, 30}, true},
		{"2.1 (Claude Code)", Version{}, false},
		{"", Version{}, false},
		{"Claude Code", Version{}, false},
		{"99999999999999999999.1.1", Version{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseVersion(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestVersionLess(t *testing.T) {
	tests := []struct {
		a, b Version
		want bool
	}{
		{Version{2, 1, 280}, Version{2, 1, 281}, true},
		{Version{2, 1, 281}, Version{2, 1, 281}, false},
		{Version{2, 1, 282}, Version{2, 1, 281}, false},
		{Version{2, 0, 999}, Version{2, 1, 0}, true},
		{Version{1, 9, 999}, Version{2, 0, 0}, true},
		{Version{3, 0, 0}, Version{2, 9, 9}, false},
	}
	for _, tt := range tests {
		if got := tt.a.Less(tt.b); got != tt.want {
			t.Errorf("%v.Less(%v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestProbeClaude(t *testing.T) {
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	blocking := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	tests := []struct {
		name    string
		ctx     context.Context
		run     CommandRunner
		want    Version
		known   bool
		errPart string
	}{
		{"nil runner", context.Background(), nil, Version{}, false, "no command runner"},
		{"version", context.Background(), fakeClaude("2.1.286 (Claude Code)\n", nil), Version{2, 1, 286}, true, ""},
		{"not on PATH", context.Background(), fakeClaude("", errors.New(`exec: "claude": executable file not found in $PATH`)), Version{}, false, "not found"},
		{"exit 1", context.Background(), fakeClaude("", errors.New("exit status 1")), Version{}, false, "exit status 1"},
		{"unparseable", context.Background(), fakeClaude("Claude Code\n", nil), Version{}, false, "printed no version"},
		{"timeout", expired, blocking, Version{}, false, "timed out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := probeClaude(tt.ctx, tt.run)
			if p.Known != tt.known || p.Version != tt.want {
				t.Fatalf("probe = %+v, want known=%v version=%v", p, tt.known, tt.want)
			}
			if tt.errPart == "" {
				if p.Err != nil {
					t.Fatalf("unexpected err %v", p.Err)
				}
				return
			}
			if p.Err == nil || !strings.Contains(p.Err.Error(), tt.errPart) {
				t.Fatalf("err = %v, want it to mention %q", p.Err, tt.errPart)
			}
		})
	}
}

// TestRunCommand_HonorsContext drives runCommand itself against a
// program that outlives the deadline, so the exec wrapper's kill path is
// exercised, not just the fake's.
func TestRunCommand_HonorsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := runCommand(ctx, "sleep", "5"); err == nil {
		t.Fatal("runCommand returned no error after its context expired")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("runCommand took %v; the context did not stop it", d)
	}
}

var (
	probeOld      = ClaudeProbe{Version: Version{2, 1, 276}, Known: true}
	probeBoundary = ClaudeProbe{Version: Version{2, 1, 280}, Known: true}
	probeNative   = ClaudeProbe{Version: Version{2, 1, 281}, Known: true}
	probeNew      = ClaudeProbe{Version: Version{2, 1, 286}, Known: true}
	probeUnknown  = ClaudeProbe{Err: errors.New("exit status 1")}
)

// TestDecideClaudeMD_Rows pins one case per row of the SPEC-013 policy
// table, in the order the rows are checked.
func TestDecideClaudeMD_Rows(t *testing.T) {
	base := claudeMDFacts{Mode: ClaudeMDAuto, ClaudeActive: true, AgentsMD: true}
	with := func(edit func(*claudeMDFacts)) claudeMDFacts {
		f := base
		edit(&f)
		return f
	}

	tests := []struct {
		name    string
		facts   claudeMDFacts
		probe   ClaudeProbe
		action  ClaudeMDAction
		warn    bool
		managed bool
		probed  bool
		reason  string
	}{
		{"claude inactive, even with link mode", with(func(f *claudeMDFacts) { f.ClaudeActive = false; f.Mode = ClaudeMDLink }), probeOld, ClaudeMDKeep, false, false, false, "not an active target"},
		{"off over a real file", with(func(f *claudeMDFacts) { f.Mode = ClaudeMDOff; f.Current = ClaudeMDRealFile }), probeOld, ClaudeMDKeep, false, false, false, "claude-md = off"},
		{"off on an old version", with(func(f *claudeMDFacts) { f.Mode = ClaudeMDOff }), probeOld, ClaudeMDKeep, false, false, false, "claude-md = off"},
		{"real file, link mode", with(func(f *claudeMDFacts) { f.Mode = ClaudeMDLink; f.Current = ClaudeMDRealFile }), probeOld, ClaudeMDKeep, true, true, false, "@AGENTS.md"},
		{"real file, auto old version", with(func(f *claudeMDFacts) { f.Current = ClaudeMDRealFile }), probeOld, ClaudeMDKeep, true, true, false, "real file"},
		{"foreign link", with(func(f *claudeMDFacts) { f.Current = ClaudeMDForeignLink; f.Mode = ClaudeMDLink }), probeOld, ClaudeMDKeep, true, true, false, "does not point at AGENTS.md"},
		{"no AGENTS.md, link mode", with(func(f *claudeMDFacts) { f.AgentsMD = false; f.Mode = ClaudeMDLink }), probeOld, ClaudeMDKeep, false, true, false, "AGENTS.md does not exist"},
		{"link mode, new version", with(func(f *claudeMDFacts) { f.Mode = ClaudeMDLink }), probeNew, ClaudeMDLinkIt, false, true, false, "claude-md = link"},
		{"link mode, unknown version", with(func(f *claudeMDFacts) { f.Mode = ClaudeMDLink }), probeUnknown, ClaudeMDLinkIt, false, true, false, "claude-md = link"},
		{"CLAUDE.local.md, new version", with(func(f *claudeMDFacts) { f.LocalMD = true }), probeNew, ClaudeMDLinkIt, false, true, false, "CLAUDE.local.md"},
		{"CLAUDE.local.md, unknown version", with(func(f *claudeMDFacts) { f.LocalMD = true }), probeUnknown, ClaudeMDLinkIt, false, true, false, "CLAUDE.local.md"},
		{"old version, absent", base, probeOld, ClaudeMDLinkIt, false, true, true, "2.1.276 reads CLAUDE.md"},
		{"2.1.280 still links", base, probeBoundary, ClaudeMDLinkIt, false, true, true, "2.1.280"},
		{"old version, our link", with(func(f *claudeMDFacts) { f.Current = ClaudeMDOurLink }), probeOld, ClaudeMDLinkIt, false, true, true, "2.1.276"},
		{"2.1.281, absent", base, probeNative, ClaudeMDKeep, false, true, true, "not created"},
		{"new version, absent", base, probeNew, ClaudeMDKeep, false, true, true, "not created: Claude Code 2.1.286"},
		{"new version, our link is removed", with(func(f *claudeMDFacts) { f.Current = ClaudeMDOurLink }), probeNew, ClaudeMDUnlink, false, true, true, "removed"},
		{"2.1.281, our link is removed", with(func(f *claudeMDFacts) { f.Current = ClaudeMDOurLink }), probeNative, ClaudeMDUnlink, false, true, true, "subdirectories"},
		{"parent CLAUDE.md, new version", with(func(f *claudeMDFacts) { f.Shadowing = "/home/u/CLAUDE.md" }), probeNew, ClaudeMDLinkIt, false, true, false, "/home/u/CLAUDE.md stops"},
		{"parent CLAUDE.md, unknown version", with(func(f *claudeMDFacts) { f.Shadowing = "/home/u/CLAUDE.md" }), probeUnknown, ClaudeMDLinkIt, false, true, false, "/home/u/CLAUDE.md"},
		{"parent CLAUDE.md keeps our link", with(func(f *claudeMDFacts) { f.Shadowing = "/w/CLAUDE.local.md"; f.Current = ClaudeMDOurLink }), probeNew, ClaudeMDLinkIt, false, true, false, "CLAUDE.local.md"},
		{"parent CLAUDE.md, off", with(func(f *claudeMDFacts) { f.Shadowing = "/w/CLAUDE.md"; f.Mode = ClaudeMDOff }), probeNew, ClaudeMDKeep, false, false, false, "claude-md = off"},
		{"unknown version, absent", base, probeUnknown, ClaudeMDKeep, true, true, true, "claude-md = link"},
		{"unknown version, our link", with(func(f *claudeMDFacts) { f.Current = ClaudeMDOurLink }), probeUnknown, ClaudeMDKeep, true, true, true, "exit status 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probed := false
			d := decideClaudeMD(tt.facts, func() ClaudeProbe { probed = true; return tt.probe })
			if d.Action != tt.action || d.Warn != tt.warn || d.Managed != tt.managed || probed != tt.probed {
				t.Fatalf("got action=%v warn=%v managed=%v probed=%v (%q); want action=%v warn=%v managed=%v probed=%v",
					d.Action, d.Warn, d.Managed, probed, d.Reason, tt.action, tt.warn, tt.managed, tt.probed)
			}
			if d.Current != tt.facts.Current {
				t.Errorf("Current = %v, want %v", d.Current, tt.facts.Current)
			}
			if !strings.Contains(d.Reason, tt.reason) {
				t.Errorf("reason %q does not mention %q", d.Reason, tt.reason)
			}
		})
	}
}

// TestDecideClaudeMD_TruthTable walks every combination of inputs and
// checks the invariants the policy promises, independent of row order.
func TestDecideClaudeMD_TruthTable(t *testing.T) {
	modes := []ClaudeMDMode{ClaudeMDAuto, ClaudeMDLink, ClaudeMDOff}
	states := []ClaudeMDState{ClaudeMDAbsent, ClaudeMDOurLink, ClaudeMDForeignLink, ClaudeMDRealFile}
	probes := []ClaudeProbe{probeOld, probeBoundary, probeNative, probeNew, probeUnknown}
	bools := []bool{false, true}
	n := 0
	for _, mode := range modes {
		for _, active := range bools {
			for _, agentsMD := range bools {
				for _, local := range bools {
					for _, shadowed := range bools {
						for _, cur := range states {
							for _, p := range probes {
								n++
								f := claudeMDFacts{Mode: mode, ClaudeActive: active, AgentsMD: agentsMD, LocalMD: local, Current: cur}
								if shadowed {
									f.Shadowing = "/parent/CLAUDE.md"
								}
								probed := false
								d := decideClaudeMD(f, func() ClaudeProbe { probed = true; return p })
								label := fmt.Sprintf("%+v probe=%+v -> %+v", f, p, d)

								userOwned := cur == ClaudeMDRealFile || cur == ClaudeMDForeignLink
								mayLink := active && mode != ClaudeMDOff && !userOwned && agentsMD
								wantProbe := mayLink && mode == ClaudeMDAuto && !local && !shadowed
								if probed != wantProbe {
									t.Errorf("probed=%v, want %v: %s", probed, wantProbe, label)
								}
								if d.Action == ClaudeMDLinkIt && !mayLink {
									t.Errorf("links where it may not: %s", label)
								}
								if d.Managed != (active && mode != ClaudeMDOff) {
									t.Errorf("managed wrong: %s", label)
								}
								if d.Warn && d.Action != ClaudeMDKeep {
									t.Errorf("warns but acts: %s", label)
								}
								wantWarn := d.Managed && (userOwned || (wantProbe && !p.Known))
								if d.Warn != wantWarn {
									t.Errorf("warn=%v, want %v: %s", d.Warn, wantWarn, label)
								}
								var wantLink bool
								switch {
								case !mayLink:
								case mode == ClaudeMDLink, local, shadowed:
									wantLink = true
								default:
									wantLink = p.Known && p.Version.Less(ClaudeNativeAgentsMD)
								}
								if (d.Action == ClaudeMDLinkIt) != wantLink {
									t.Errorf("link=%v, want %v: %s", d.Action == ClaudeMDLinkIt, wantLink, label)
								}
								// Only our own link is ever removed, and only when
								// a native Claude Code would read AGENTS.md here.
								wantUnlink := wantProbe && p.Known && !p.Version.Less(ClaudeNativeAgentsMD) && cur == ClaudeMDOurLink
								if (d.Action == ClaudeMDUnlink) != wantUnlink {
									t.Errorf("unlink=%v, want %v: %s", d.Action == ClaudeMDUnlink, wantUnlink, label)
								}
								if d.Reason == "" {
									t.Errorf("empty reason: %s", label)
								}
							}
						}
					}
				}
			}
		}
	}
	if n != 960 {
		t.Fatalf("walked %d combinations, want 960", n)
	}
}

func TestReadConfigClaudeMD(t *testing.T) {
	tests := []struct {
		config  string
		want    ClaudeMDMode
		wantErr bool
	}{
		{"", ClaudeMDAuto, false},
		{"targets = claude\n", ClaudeMDAuto, false},
		{"# claude-md = off\n", ClaudeMDAuto, false},
		{"claude-md = auto\n", ClaudeMDAuto, false},
		{"claude-md=link\n", ClaudeMDLink, false},
		{"  claude-md = OFF  \n", ClaudeMDOff, false},
		{"claude-md = always\n", ClaudeMDAuto, true},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config"), []byte(tt.config), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ReadConfigClaudeMD(dir)
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("config %q: got %v, %v; want %v, err=%v", tt.config, got, err, tt.want, tt.wantErr)
		}
	}
	if got, err := ReadConfigClaudeMD(t.TempDir()); got != ClaudeMDAuto || err != nil {
		t.Errorf("missing config: got %v, %v; want auto, nil", got, err)
	}
}

func TestClassifyClaudeMD(t *testing.T) {
	tests := []struct {
		name  string
		setup func(dir string) error
		want  ClaudeMDState
	}{
		{"absent", func(string) error { return nil }, ClaudeMDAbsent},
		{"relative link", func(d string) error { return os.Symlink("AGENTS.md", filepath.Join(d, "CLAUDE.md")) }, ClaudeMDOurLink},
		{"absolute link", func(d string) error {
			if err := os.WriteFile(filepath.Join(d, "AGENTS.md"), nil, 0o644); err != nil {
				return err
			}
			return os.Symlink(filepath.Join(d, "AGENTS.md"), filepath.Join(d, "CLAUDE.md"))
		}, ClaudeMDOurLink},
		{"foreign link", func(d string) error { return os.Symlink("docs/CLAUDE.md", filepath.Join(d, "CLAUDE.md")) }, ClaudeMDForeignLink},
		{"real file", func(d string) error { return os.WriteFile(filepath.Join(d, "CLAUDE.md"), []byte("mine"), 0o644) }, ClaudeMDRealFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := tt.setup(dir); err != nil {
				t.Fatal(err)
			}
			got, err := classifyClaudeMD(dir)
			if err != nil || got != tt.want {
				t.Fatalf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

// newClaudeMDApp is a project with .agents/rules, AGENTS.md, claude as
// the only target, and run as the `claude --version` runner.
func newClaudeMDApp(t *testing.T, run CommandRunner) (*App, string, *strings.Builder) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# AGENTS.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	app := &App{ProjectRoot: dir, ActiveTargets: []string{"claude"}, Stdout: &buf, Stderr: &buf}
	app.ToolEnv.Run = run
	return app, dir, &buf
}

func readLinkOrEmpty(path string) string {
	s, _ := os.Readlink(path)
	return s
}

func TestCmdSync_ClaudeMD(t *testing.T) {
	t.Run("old claude links it and gitignores it", func(t *testing.T) {
		app, dir, _ := newClaudeMDApp(t, fakeClaude("2.1.276 (Claude Code)\n", nil))
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if got := readLinkOrEmpty(filepath.Join(dir, "CLAUDE.md")); got != "AGENTS.md" {
			t.Fatalf("CLAUDE.md -> %q, want AGENTS.md", got)
		}
		if gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); !containsExactLine(string(gi), "CLAUDE.md") {
			t.Errorf(".gitignore lacks CLAUDE.md:\n%s", gi)
		}
	})

	t.Run("new claude creates nothing and does not gitignore it", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.286 (Claude Code)\n", nil))
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md exists (err %v)", err)
		}
		if gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); containsExactLine(string(gi), "CLAUDE.md") {
			t.Errorf(".gitignore lists CLAUDE.md:\n%s", gi)
		}
		if !strings.Contains(out.String(), "[info] CLAUDE.md not created: Claude Code 2.1.286") {
			t.Errorf("missing info line:\n%s", out)
		}
	})

	t.Run("new claude removes our link", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.286 (Claude Code)\n", nil))
		if err := os.Symlink("AGENTS.md", filepath.Join(dir, "CLAUDE.md")); err != nil {
			t.Fatal(err)
		}
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md still exists (err %v)", err)
		}
		if gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); containsExactLine(string(gi), "CLAUDE.md") {
			t.Errorf(".gitignore lists CLAUDE.md after the link was removed:\n%s", gi)
		}
		if !strings.Contains(out.String(), "CLAUDE.md -> AGENTS.md removed") {
			t.Errorf("missing removal line:\n%s", out)
		}
	})

	t.Run("dry run reports but keeps our link", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.286 (Claude Code)\n", nil))
		app.DryRun = true
		if err := os.Symlink("AGENTS.md", filepath.Join(dir, "CLAUDE.md")); err != nil {
			t.Fatal(err)
		}
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if got := readLinkOrEmpty(filepath.Join(dir, "CLAUDE.md")); got != "AGENTS.md" {
			t.Fatalf("dry run removed CLAUDE.md (-> %q)", got)
		}
		if !strings.Contains(out.String(), "would remove CLAUDE.md") {
			t.Errorf("missing dry-run line:\n%s", out)
		}
	})

	t.Run("a CLAUDE.md in a parent directory forces the link", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.286 (Claude Code)\n", nil))
		parentMD := filepath.Join(filepath.Dir(dir), "CLAUDE.md")
		if err := os.WriteFile(parentMD, []byte("# parent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(parentMD) })
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if got := readLinkOrEmpty(filepath.Join(dir, "CLAUDE.md")); got != "AGENTS.md" {
			t.Fatalf("CLAUDE.md -> %q, want AGENTS.md", got)
		}
		if !strings.Contains(out.String(), parentMD+" stops Claude Code") {
			t.Errorf("reason does not name the parent file:\n%s", out)
		}
	})

	t.Run("unknown version changes nothing and warns once", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("", errors.New("exit status 1")))
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md exists (err %v)", err)
		}
		if n := strings.Count(out.String(), "cannot tell the Claude Code version"); n != 1 {
			t.Errorf("warning printed %d times, want 1:\n%s", n, out)
		}
	})

	t.Run("nil runner is unknown", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, nil)
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md exists (err %v)", err)
		}
		if !strings.Contains(out.String(), "no command runner") {
			t.Errorf("warning does not name the cause:\n%s", out)
		}
	})

	t.Run("CLAUDE.local.md forces the link without probing", func(t *testing.T) {
		app, dir, _ := newClaudeMDApp(t, func(context.Context, string, ...string) ([]byte, error) {
			t.Error("probe ran although CLAUDE.local.md decides")
			return nil, errors.New("unreachable")
		})
		if err := os.WriteFile(filepath.Join(dir, "CLAUDE.local.md"), []byte("local\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if got := readLinkOrEmpty(filepath.Join(dir, "CLAUDE.md")); got != "AGENTS.md" {
			t.Fatalf("CLAUDE.md -> %q, want AGENTS.md", got)
		}
	})

	t.Run("real CLAUDE.md survives --overwrite", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.276 (Claude Code)\n", nil))
		app.Overwrite = true
		claudeMD := filepath.Join(dir, "CLAUDE.md")
		if err := os.WriteFile(claudeMD, []byte("mine\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(claudeMD); string(got) != "mine\n" {
			t.Fatalf("CLAUDE.md = %q, want it untouched", got)
		}
		if matches, _ := filepath.Glob(claudeMD + BackupSuffix + "*"); len(matches) != 0 {
			t.Fatalf("CLAUDE.md was moved aside: %v", matches)
		}
		if !strings.Contains(out.String(), "@AGENTS.md") {
			t.Errorf("no warning suggesting @AGENTS.md:\n%s", out)
		}
		if gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); containsExactLine(string(gi), "CLAUDE.md") {
			t.Errorf(".gitignore hides the user's CLAUDE.md:\n%s", gi)
		}
	})

	t.Run("claude not active means no CLAUDE.md handling", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.276 (Claude Code)\n", nil))
		app.ActiveTargets = []string{"windsurf"}
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md exists (err %v)", err)
		}
		if strings.Contains(out.String(), "] CLAUDE.md") {
			t.Errorf("sync mentioned CLAUDE.md:\n%s", out)
		}
	})

	t.Run("claude-md = link with no claude installed", func(t *testing.T) {
		app, dir, _ := newClaudeMDApp(t, nil)
		if err := os.WriteFile(filepath.Join(dir, ".agents", "config"), []byte("claude-md = link\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if got := readLinkOrEmpty(filepath.Join(dir, "CLAUDE.md")); got != "AGENTS.md" {
			t.Fatalf("CLAUDE.md -> %q, want AGENTS.md", got)
		}
	})

	t.Run("claude-md = off leaves a broken setup alone", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.276 (Claude Code)\n", nil))
		if err := os.WriteFile(filepath.Join(dir, ".agents", "config"), []byte("claude-md = off\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md exists (err %v)", err)
		}
		if strings.Contains(out.String(), "] CLAUDE.md") {
			t.Errorf("sync mentioned CLAUDE.md:\n%s", out)
		}
	})

	t.Run("invalid claude-md value warns and changes nothing", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.276 (Claude Code)\n", nil))
		if err := os.WriteFile(filepath.Join(dir, ".agents", "config"), []byte("claude-md = always\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md exists (err %v)", err)
		}
		if !strings.Contains(out.String(), `claude-md = "always"`) {
			t.Errorf("warning does not name the bad value:\n%s", out)
		}
	})

	t.Run("dry-run writes nothing", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.276 (Claude Code)\n", nil))
		app.DryRun = true
		if err := app.CmdSync(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("dry-run created CLAUDE.md (err %v)", err)
		}
		if !strings.Contains(out.String(), "would link") || !strings.Contains(out.String(), "would add to .gitignore: CLAUDE.md") {
			t.Errorf("dry-run did not report the link:\n%s", out)
		}
	})
}

func TestCmdFix_ClaudeMD(t *testing.T) {
	t.Run("creates the link for an old claude", func(t *testing.T) {
		app, dir, _ := newClaudeMDApp(t, fakeClaude("2.1.276 (Claude Code)\n", nil))
		if err := app.CmdFix("", false); err != nil {
			t.Fatal(err)
		}
		if got := readLinkOrEmpty(filepath.Join(dir, "CLAUDE.md")); got != "AGENTS.md" {
			t.Fatalf("CLAUDE.md -> %q, want AGENTS.md", got)
		}
	})

	t.Run("leaves a foreign link in place", func(t *testing.T) {
		app, dir, out := newClaudeMDApp(t, fakeClaude("2.1.276 (Claude Code)\n", nil))
		if err := os.Symlink("docs/CLAUDE.md", filepath.Join(dir, "CLAUDE.md")); err != nil {
			t.Fatal(err)
		}
		if err := app.CmdFix("", false); err != nil {
			t.Fatal(err)
		}
		if got := readLinkOrEmpty(filepath.Join(dir, "CLAUDE.md")); got != "docs/CLAUDE.md" {
			t.Fatalf("CLAUDE.md -> %q, want docs/CLAUDE.md", got)
		}
		if !strings.Contains(out.String(), "[warn] CLAUDE.md is a symlink") {
			t.Errorf("no warning:\n%s", out)
		}
	})

	t.Run("creates nothing for a new claude", func(t *testing.T) {
		app, dir, _ := newClaudeMDApp(t, fakeClaude("2.1.286 (Claude Code)\n", nil))
		if err := app.CmdFix("", false); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md exists (err %v)", err)
		}
	})
}

func TestCmdStatus_ClaudeMD(t *testing.T) {
	tests := []struct {
		name string
		run  CommandRunner
		link bool
		want string
	}{
		{"old, missing", fakeClaude("2.1.276 (Claude Code)", nil), false, "[missing] CLAUDE.md -> AGENTS.md: Claude Code 2.1.276"},
		{"old, linked", fakeClaude("2.1.276 (Claude Code)", nil), true, "[ok] CLAUDE.md -> AGENTS.md: Claude Code 2.1.276"},
		{"new, linked", fakeClaude("2.1.286 (Claude Code)", nil), true, "[stale] CLAUDE.md -> AGENTS.md removed"},
		{"new, absent", fakeClaude("2.1.286 (Claude Code)", nil), false, "[info] CLAUDE.md not created"},
		{"unknown", fakeClaude("", errors.New("exit status 1")), false, "[warn] CLAUDE.md left unchanged"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, dir, out := newClaudeMDApp(t, tt.run)
			if tt.link {
				if err := os.Symlink("AGENTS.md", filepath.Join(dir, "CLAUDE.md")); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.CmdStatus(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("status lacks %q:\n%s", tt.want, out)
			}
		})
	}
}

func TestCmdClean_ClaudeMD(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(path string) error
		wantGone bool
	}{
		{"our link is removed", func(p string) error { return os.Symlink("AGENTS.md", p) }, true},
		{"foreign link stays", func(p string) error { return os.Symlink("docs/CLAUDE.md", p) }, false},
		{"real file stays", func(p string) error { return os.WriteFile(p, []byte("mine\n"), 0o644) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, dir, _ := newClaudeMDApp(t, nil)
			claudeMD := filepath.Join(dir, "CLAUDE.md")
			if err := tt.setup(claudeMD); err != nil {
				t.Fatal(err)
			}
			if err := app.CmdClean(); err != nil {
				t.Fatal(err)
			}
			_, err := os.Lstat(claudeMD)
			if gone := os.IsNotExist(err); gone != tt.wantGone {
				t.Fatalf("CLAUDE.md gone=%v, want %v", gone, tt.wantGone)
			}
		})
	}
}

func TestShadowingClaudeMD(t *testing.T) {
	write := func(t *testing.T, path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name  string
		files []string // relative to the temp root; the project is root/home/code/proj
		want  string   // relative to the temp root, "" for none
	}{
		{"none", nil, ""},
		{"home CLAUDE.md", []string{"home/CLAUDE.md"}, "home/CLAUDE.md"},
		{"user-level home/.claude/CLAUDE.md does not count", []string{"home/.claude/CLAUDE.md"}, ""},
		{"nested .claude/CLAUDE.md counts", []string{"home/code/.claude/CLAUDE.md"}, "home/code/.claude/CLAUDE.md"},
		{"CLAUDE.local.md counts", []string{"home/code/CLAUDE.local.md"}, "home/code/CLAUDE.local.md"},
		{"nearest wins", []string{"home/CLAUDE.md", "home/code/CLAUDE.md"}, "home/code/CLAUDE.md"},
		{"the project's own CLAUDE.md is not a parent", []string{"home/code/proj/CLAUDE.md"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, f := range tt.files {
				write(t, filepath.Join(root, f))
			}
			proj := filepath.Join(root, "home", "code", "proj")
			if err := os.MkdirAll(proj, 0o755); err != nil {
				t.Fatal(err)
			}
			got, err := shadowingClaudeMD(proj, filepath.Join(root, "home"))
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if tt.want != "" {
				want = filepath.Join(root, tt.want)
			}
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
