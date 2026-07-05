package agent

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestOSScopes is the SPEC-006 table test: each scope name matches the
// right runtime.GOOS values; unix = darwin+linux but not windows/other.
func TestOSScopes(t *testing.T) {
	cases := []struct {
		scope string
		goos  string
		want  bool
	}{
		{"macos", "darwin", true}, {"macos", "linux", false}, {"macos", "windows", false},
		{"linux", "linux", true}, {"linux", "darwin", false},
		{"unix", "darwin", true}, {"unix", "linux", true}, {"unix", "windows", false}, {"unix", "freebsd", false},
		{"windows", "windows", true}, {"windows", "darwin", false},
	}
	for _, c := range cases {
		if got := osScopes[c.scope](c.goos); got != c.want {
			t.Errorf("osScopes[%q](%q) = %v, want %v", c.scope, c.goos, got, c.want)
		}
	}
}

func names(arts []Artifact) []string {
	var out []string
	for _, a := range arts {
		out = append(out, string(a.Type)+":"+a.Name)
	}
	sort.Strings(out)
	return out
}

// TestDiscoverArtifacts_OSScoped seeds root + macos/linux/unix rules
// and asserts each host OS discovers the right subset. Root files are
// always present; non-matching OS subdirs are skipped entirely.
func TestDiscoverArtifacts_OSScoped(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("# rule\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("rules/security.md")     // always
	mk("rules/macos/brew.md")   // darwin only
	mk("rules/linux/apt.md")    // linux only
	mk("rules/unix/posix.md")   // darwin + linux

	tests := []struct {
		goos string
		want []string
	}{
		{"darwin", []string{"rule:macos/brew", "rule:security", "rule:unix/posix"}},
		{"linux", []string{"rule:linux/apt", "rule:security", "rule:unix/posix"}},
		{"windows", []string{"rule:security"}},
	}
	for _, tc := range tests {
		arts, err := discoverArtifactsForOS(dir, tc.goos)
		if err != nil {
			t.Fatalf("%s: %v", tc.goos, err)
		}
		got := names(arts)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.goos, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %v, want %v", tc.goos, got, tc.want)
				break
			}
		}
		// Scoped source paths point inside the OS subdir.
		for _, a := range arts {
			if a.Name == "macos/brew" && filepath.Base(filepath.Dir(a.SourcePath)) != "macos" {
				t.Errorf("scoped SourcePath not under macos/: %s", a.SourcePath)
			}
		}
	}
}

// TestDiscoverArtifacts_ConfigOSOverride: `os = linux` in .agents/config
// makes discovery behave as if on Linux regardless of the host.
func TestDiscoverArtifacts_ConfigOSOverride(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("# rule\n"), 0o644)
	}
	mk("rules/macos/brew.md")
	mk("rules/linux/apt.md")
	mk("rules/unix/posix.md")
	os.WriteFile(filepath.Join(dir, "config"), []byte("os = linux\n"), 0o644)

	if got := effectiveGOOS(dir); got != "linux" {
		t.Fatalf("effectiveGOOS = %q, want linux", got)
	}
	arts, err := DiscoverArtifacts(dir) // uses the config override
	if err != nil {
		t.Fatal(err)
	}
	got := names(arts)
	want := []string{"rule:linux/apt", "rule:unix/posix"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("with os=linux: got %v, want %v", got, want)
	}
}
