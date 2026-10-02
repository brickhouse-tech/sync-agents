package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var openClawEnvKeys = []string{
	"OPENCLAW_HOME", "OPENCLAW_STATE_DIR", "OPENCLAW_CONFIG_PATH",
	"OPENCLAW_WORKSPACE_DIR", "OPENCLAW_PROFILE",
}

// TestMain clears OpenClaw's env vars for the whole package. A zero
// App already ignores the environment; this also covers any test that
// builds an App with OSToolEnv, so a developer's exports can never
// point a test at a live workspace.
// workspaceAgentsFixture is a representative OpenClaw workspace
// AGENTS.md: old index output plus the `## Tools` section OpenClaw's
// doctor appends at the end of the file. It doubles as an AGENTS.md
// migration fixture.
func workspaceAgentsFixture(t *testing.T) string {
	t.Helper()
	return readFile(t, filepath.Join("testdata", "agentsmd", "openclaw-workspace-tools.md"))
}

func TestMain(m *testing.M) {
	for _, k := range openClawEnvKeys {
		os.Unsetenv(k)
	}
	os.Exit(m.Run())
}

func mapEnv(vars map[string]string, files map[string]string) ToolEnv {
	return ToolEnv{
		Getenv: func(k string) string { return vars[k] },
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := files[p]; ok {
				return []byte(s), nil
			}
			return nil, os.ErrNotExist
		},
	}
}

func TestResolveOpenClaw_Precedence(t *testing.T) {
	const parent = "/home/u"
	cfg := func(ws string) string {
		return `{"agents":{"defaults":{"workspace":"` + ws + `"}}}`
	}
	cases := []struct {
		name  string
		vars  map[string]string
		files map[string]string
		want  string
	}{
		{"default under parent", nil, nil, "/home/u/.openclaw/workspace"},
		{"OPENCLAW_HOME moves home", map[string]string{"OPENCLAW_HOME": "/oc"}, nil, "/oc/.openclaw/workspace"},
		{"profile state dir", map[string]string{"OPENCLAW_PROFILE": "work"}, nil, "/home/u/.openclaw-work/workspace"},
		{"default profile is the plain dir", map[string]string{"OPENCLAW_PROFILE": "default"}, nil, "/home/u/.openclaw/workspace"},
		{"state dir wins over profile", map[string]string{"OPENCLAW_PROFILE": "work", "OPENCLAW_STATE_DIR": "/s"}, nil, "/s/workspace"},
		{"state dir tilde", map[string]string{"OPENCLAW_STATE_DIR": "~/st"}, nil, "/home/u/st/workspace"},
		{"workspace env wins over state dir", map[string]string{"OPENCLAW_STATE_DIR": "/s", "OPENCLAW_WORKSPACE_DIR": "/w"}, nil, "/w"},
		{"config wins over workspace env",
			map[string]string{"OPENCLAW_WORKSPACE_DIR": "/w"},
			map[string]string{"/home/u/.openclaw/openclaw.json": cfg("/from-config")}, "/from-config"},
		{"config tilde",
			nil, map[string]string{"/home/u/.openclaw/openclaw.json": cfg("~/ws")}, "/home/u/ws"},
		{"config read from state dir",
			map[string]string{"OPENCLAW_STATE_DIR": "/s"},
			map[string]string{"/s/openclaw.json": cfg("/via-state")}, "/via-state"},
		{"config path env",
			map[string]string{"OPENCLAW_CONFIG_PATH": "/etc/oc.json"},
			map[string]string{"/etc/oc.json": cfg("/via-path"), "/home/u/.openclaw/openclaw.json": cfg("/ignored")}, "/via-path"},
		{"config without workspace falls through",
			nil, map[string]string{"/home/u/.openclaw/openclaw.json": `{"agents":{}}`}, "/home/u/.openclaw/workspace"},
	}
	for _, c := range cases {
		got, err := resolveOpenClawWorkspace(parent, mapEnv(c.vars, c.files))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolveOpenClaw_InvalidConfigFailsClosed(t *testing.T) {
	env := mapEnv(nil, map[string]string{"/home/u/.openclaw/openclaw.json": "{ not json"})
	if _, err := resolveOpenClawWorkspace("/home/u", env); err == nil {
		t.Fatal("invalid config must be an error, not a guessed workspace")
	}
}

func TestResolveOpenClaw_BootstrapMaxChars(t *testing.T) {
	if n, _ := openClawRegionCharCap("/home/u", mapEnv(nil, nil)); n != 20000 {
		t.Errorf("default cap = %d, want 20000", n)
	}
	env := mapEnv(nil, map[string]string{"/home/u/.openclaw/openclaw.json": `{"agents":{"defaults":{"bootstrapMaxChars":500}}}`})
	if n, _ := openClawRegionCharCap("/home/u", env); n != 500 {
		t.Errorf("configured cap = %d, want 500", n)
	}
}

func TestOpenClawDestination(t *testing.T) {
	tool, _ := ResolveTool("openclaw")
	cases := []struct {
		typ      ArtifactType
		sem      Semantic
		strategy DestinationStrategy
	}{
		{ArtifactRule, Passive, StrategyRegion},
		{ArtifactWorkflow, Passive, StrategyRegion},
		{ArtifactRule, Invocable, StrategySkip},
		{ArtifactWorkflow, Invocable, StrategySkip},
		{ArtifactSkill, Invocable, StrategySkip},
		{ArtifactSkill, Passive, StrategySkip},
		{ArtifactAgent, Passive, StrategySkip},
	}
	for _, c := range cases {
		d := TargetDestination(tool, c.typ, "x", c.sem, "/src/x", "/home/u")
		if d.Strategy != c.strategy {
			t.Errorf("%s/%v: strategy %v, want %v", c.typ, c.sem, d.Strategy, c.strategy)
		}
		if c.strategy == StrategyRegion {
			if d.Path != "/home/u/.openclaw/workspace/AGENTS.md" || d.Region != OpenClawRulesRegion {
				t.Errorf("%s/%v: got %q region %q", c.typ, c.sem, d.Path, d.Region.Name)
			}
		}
	}
}

// openClawRig is a temp global root whose parent holds an OpenClaw
// workspace with a representative AGENTS.md.
type openClawRig struct {
	app       *App
	parent    string
	workspace string
	host      string // workspace AGENTS.md
	fixture   string
	stdout    *bytes.Buffer
}

func newOpenClawRig(t *testing.T) *openClawRig {
	t.Helper()
	app, root, stdout := newGlobalSyncTestApp(t)
	ws := filepath.Join(root, ".openclaw", "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := workspaceAgentsFixture(t)
	host := filepath.Join(ws, "AGENTS.md")
	if err := os.WriteFile(host, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	seedRule(t, app.GlobalRoot, "git", "---\ndescription: git hygiene\n---\n\nCommit small.\n")
	seedRule(t, app.GlobalRoot, "testing", "Run the tests.\n")
	seedSkill(t, app.GlobalRoot, "weather", "---\nname: weather\n---\n\nForecast.\n", nil)
	return &openClawRig{app: app, parent: root, workspace: ws, host: host, fixture: fixture, stdout: stdout}
}

func (r *openClawRig) sync(t *testing.T, targets ...string) {
	t.Helper()
	if err := r.app.CmdGlobalSync(GlobalSyncOpts{Targets: targets}); err != nil {
		t.Fatalf("global sync: %v", err)
	}
}

func setOld(t *testing.T, path string) time.Time {
	t.Helper()
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	return old
}

func mtime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime()
}

func TestGlobalSync_OpenClaw_InsertsRegionAndPreservesHost(t *testing.T) {
	r := newOpenClawRig(t)
	r.sync(t, "openclaw")

	got := readFile(t, r.host)
	if !strings.HasPrefix(got, r.fixture) {
		t.Fatalf("content outside the region changed:\n%s", got)
	}
	region := strings.TrimPrefix(got, r.fixture)
	want := "\n" + OpenClawRulesRegion.Start() + "\n" + regionBanner + "\n" +
		"## git\n\nCommit small.\n\n" +
		"## testing\n\nRun the tests.\n\n" +
		OpenClawRulesRegion.End() + "\n"
	if region != want {
		t.Errorf("region:\n got %q\nwant %q", region, want)
	}
	if !strings.Contains(r.stdout.String(), "OpenClaw loads ~/.agents/skills natively") {
		t.Errorf("skill skip reason not reported:\n%s", r.stdout)
	}
}

func TestGlobalSync_OpenClaw_RerunIsByteIdenticalAndKeepsMtime(t *testing.T) {
	r := newOpenClawRig(t)
	r.sync(t, "openclaw")
	first := readFile(t, r.host)
	old := setOld(t, r.host)

	r.sync(t)
	if got := readFile(t, r.host); got != first {
		t.Errorf("rerun changed bytes:\n%s", got)
	}
	if !mtime(t, r.host).Equal(old) {
		t.Error("rerun rewrote an unchanged host file")
	}
}

func TestGlobalSync_OpenClaw_DefaultSyncWithoutMarkersLeavesHostUntouched(t *testing.T) {
	r := newOpenClawRig(t)
	old := setOld(t, r.host)
	r.sync(t)
	if got := readFile(t, r.host); got != r.fixture {
		t.Errorf("default sync edited a host file without consent:\n%s", got)
	}
	if !mtime(t, r.host).Equal(old) {
		t.Error("default sync touched the host file")
	}
	if strings.Contains(r.stdout.String(), "openclaw") {
		t.Errorf("non-consenting sync should say nothing about openclaw:\n%s", r.stdout)
	}
}

func TestGlobalSync_OpenClaw_ExplicitTargetWithoutAgentsMDSkips(t *testing.T) {
	app, root, stdout := newGlobalSyncTestApp(t)
	seedRule(t, app.GlobalRoot, "git", "Commit small.\n")
	if err := app.CmdGlobalSync(GlobalSyncOpts{Targets: []string{"openclaw"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".openclaw")); !os.IsNotExist(err) {
		t.Errorf("sync created OpenClaw state it does not own (err=%v)", err)
	}
	if !strings.Contains(stdout.String(), "run openclaw once to create it") {
		t.Errorf("missing skip warning:\n%s", stdout)
	}
}

func TestGlobalSync_OpenClaw_DeletingLastRuleEmptiesRegion(t *testing.T) {
	r := newOpenClawRig(t)
	r.sync(t, "openclaw")
	for _, n := range []string{"git", "testing"} {
		if err := os.Remove(filepath.Join(r.app.GlobalRoot, "rules", n+".md")); err != nil {
			t.Fatal(err)
		}
	}
	r.sync(t)
	want := r.fixture + "\n" + OpenClawRulesRegion.Start() + "\n" + regionBanner + "\n" + OpenClawRulesRegion.End() + "\n"
	if got := readFile(t, r.host); got != want {
		t.Errorf("region not emptied:\n%s", got)
	}
}

func TestGlobalSync_OpenClaw_SizeCapWarning(t *testing.T) {
	r := newOpenClawRig(t)
	cfg := filepath.Join(r.parent, ".openclaw", "openclaw.json")
	if err := os.WriteFile(cfg, []byte(`{"agents":{"defaults":{"bootstrapMaxChars":100}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r.sync(t, "openclaw")
	if !strings.Contains(r.stdout.String(), "over the 100-char bootstrap cap (agents.defaults.bootstrapMaxChars)") {
		t.Errorf("no size-cap warning:\n%s", r.stdout)
	}
}

func TestGlobalSync_OpenClaw_NoSizeCapWarningUnderDefault(t *testing.T) {
	r := newOpenClawRig(t)
	r.sync(t, "openclaw")
	if strings.Contains(r.stdout.String(), "bootstrap cap") {
		t.Errorf("spurious size-cap warning:\n%s", r.stdout)
	}
}

func TestGlobalClean_OpenClaw_StripsOnlyRegion(t *testing.T) {
	r := newOpenClawRig(t)
	r.sync(t, "openclaw")
	other := filepath.Join(r.workspace, "SOUL.md")
	if err := os.WriteFile(other, []byte("soul\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := r.app.CmdGlobalClean(GlobalCleanOpts{}); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if got := readFile(t, r.host); got != r.fixture {
		t.Errorf("clean did not restore the host file exactly:\n%s", got)
	}
	if got := readFile(t, other); got != "soul\n" {
		t.Errorf("clean touched another workspace file: %q", got)
	}
}

func TestGlobalStatus_OpenClaw_RegionRow(t *testing.T) {
	r := newOpenClawRig(t)
	r.sync(t, "openclaw")
	status := func() string {
		r.stdout.Reset()
		if err := r.app.CmdGlobalStatus(GlobalStatusOpts{Targets: []string{"openclaw"}}); err != nil {
			t.Fatalf("status: %v", err)
		}
		return r.stdout.String()
	}
	if out := status(); !strings.Contains(out, "[region synced] "+r.host) {
		t.Errorf("want synced region row:\n%s", out)
	}
	seedRule(t, r.app.GlobalRoot, "git", "Commit smaller.\n")
	if out := status(); !strings.Contains(out, "[region stale] "+r.host) {
		t.Errorf("want stale region row:\n%s", out)
	}
}

func TestOpenClaw_IndexPreservesRegionAcrossSyncs(t *testing.T) {
	r := newOpenClawRig(t)
	writeArtifact(t, r.workspace, filepath.Join(".agents", "rules", "git.md"), "local\n")
	idx := &App{ProjectRoot: r.workspace, GlobalRoot: r.app.GlobalRoot, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}

	r.sync(t, "openclaw")
	if err := idx.CmdIndex(); err != nil {
		t.Fatalf("index: %v", err)
	}
	afterIndex := readFile(t, r.host)
	for _, want := range []string{
		OpenClawRulesRegion.Start() + "\n" + regionBanner + "\n## git\n\nCommit small.\n",
		"## Tools\n\nSkills define how tools work.",
		"### Cameras\n",
	} {
		if !strings.Contains(afterIndex, want) {
			t.Errorf("index dropped %q:\n%s", want, afterIndex)
		}
	}

	r.sync(t)
	if got := readFile(t, r.host); got != afterIndex {
		t.Errorf("sync -> index -> sync not byte-identical:\nafter index:\n%s\nafter sync:\n%s", afterIndex, got)
	}
}

func TestLocalSync_OpenClawTargetDoesNotCreateDir(t *testing.T) {
	a, root, stdout := newLocalIndexTestApp(t)
	writeArtifact(t, root, filepath.Join(".agents", "rules", "git.md"), "body\n")
	a.ActiveTargets = []string{"claude", "openclaw"}
	if err := a.CmdSync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".openclaw")); !os.IsNotExist(err) {
		t.Errorf("local sync created .openclaw/ in the project (err=%v)", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "rules")); err != nil {
		t.Errorf("other targets must still sync: %v", err)
	}
	if !strings.Contains(stdout.String(), `target "openclaw" is global-only`) {
		t.Errorf("missing skip warning:\n%s", stdout)
	}
	if gi := readFile(t, filepath.Join(root, ".gitignore")); strings.Contains(gi, ".openclaw") {
		t.Errorf(".gitignore gained .openclaw/:\n%s", gi)
	}
}
