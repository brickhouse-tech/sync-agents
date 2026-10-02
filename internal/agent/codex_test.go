package agent

import (
	"path/filepath"
	"testing"
)

// TestCodexBudget_Layout resolves Codex's home, limit, and global file
// the way Codex does: $CODEX_HOME over ~/.codex, project_doc_max_bytes
// from top-level config.toml only, and a non-empty AGENTS.override.md
// over AGENTS.md.
func TestCodexBudget_Layout(t *testing.T) {
	const parent = "/home/u"
	home := filepath.Join(parent, ".codex")
	cases := []struct {
		name     string
		vars     map[string]string
		files    map[string]string
		home     string
		maxBytes int
		global   string
		wantErr  bool
	}{
		{name: "defaults", home: home, maxBytes: 32768, global: filepath.Join(home, "AGENTS.md")},
		{
			name: "CODEX_HOME", vars: map[string]string{"CODEX_HOME": "/opt/codex"},
			home: "/opt/codex", maxBytes: 32768, global: "/opt/codex/AGENTS.md",
		},
		{
			name:  "top-level limit with underscores and a comment",
			files: map[string]string{filepath.Join(home, "config.toml"): "model = \"o4\"\nproject_doc_max_bytes = 65_536 # raised\n"},
			home:  home, maxBytes: 65536, global: filepath.Join(home, "AGENTS.md"),
		},
		{
			name:  "a key inside a table is not the top-level limit",
			files: map[string]string{filepath.Join(home, "config.toml"): "[profiles.x]\nproject_doc_max_bytes = 10\n"},
			home:  home, maxBytes: 32768, global: filepath.Join(home, "AGENTS.md"),
		},
		{
			name:    "a bad limit is an error",
			files:   map[string]string{filepath.Join(home, "config.toml"): "project_doc_max_bytes = lots\n"},
			wantErr: true,
		},
		{
			name:  "non-empty override is the global file",
			files: map[string]string{filepath.Join(home, "AGENTS.override.md"): "mine\n"},
			home:  home, maxBytes: 32768, global: filepath.Join(home, "AGENTS.override.md"),
		},
		{
			name:  "blank override is skipped",
			files: map[string]string{filepath.Join(home, "AGENTS.override.md"): " \n"},
			home:  home, maxBytes: 32768, global: filepath.Join(home, "AGENTS.md"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := resolveCodex(parent, mapEnv(tc.vars, tc.files))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveCodex: %v", err)
			}
			if l.Home != tc.home || l.MaxBytes != tc.maxBytes || l.Global != tc.global {
				t.Errorf("got %+v, want home=%s max=%d global=%s", l, tc.home, tc.maxBytes, tc.global)
			}
		})
	}
}

// TestCodexBudget_ProjectReservesGlobalFile charges the global file
// against the project override's budget, because Codex loads both under
// one combined limit; the global budget reserves nothing itself.
func TestCodexBudget_ProjectReservesGlobalFile(t *testing.T) {
	const parent = "/home/u"
	global := filepath.Join(parent, ".codex", "AGENTS.md")
	tc := ToolContext{Parent: parent, Env: mapEnv(nil, map[string]string{global: "0123456789"})}

	p, err := codexProjectBudget(tc)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if p.Reserved != 10 || p.Cap.Limit != 32768 || p.Cap.Unit != CapBytes || p.Cap.Knob == "" {
		t.Errorf("project = %+v", p)
	}

	g, err := codexGlobalBudget(tc)
	if err != nil {
		t.Fatalf("global: %v", err)
	}
	if g.Reserved != 0 || g.Cap != p.Cap {
		t.Errorf("global = %+v", g)
	}

	empty, err := codexProjectBudget(ToolContext{Parent: parent, Env: mapEnv(nil, nil)})
	if err != nil || empty.Reserved != 0 {
		t.Errorf("no global file: %+v, %v", empty, err)
	}
}

// TestCodexBudget_GlobalOverrideShadows reports a non-empty
// ~/.codex/AGENTS.override.md, which Codex reads instead of the
// AGENTS.md that carries the codex-rules region.
func TestCodexBudget_GlobalOverrideShadows(t *testing.T) {
	home := "/home/u/.codex"
	override := filepath.Join(home, "AGENTS.override.md")
	if got := codexGlobalOverride(ToolContext{Home: home, Env: mapEnv(nil, nil)}); got != "" {
		t.Errorf("absent override: %q", got)
	}
	if got := codexGlobalOverride(ToolContext{Home: home, Env: mapEnv(nil, map[string]string{override: "x"})}); got != override {
		t.Errorf("present override: %q", got)
	}
}
