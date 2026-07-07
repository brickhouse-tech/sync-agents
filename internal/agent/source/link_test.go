package source

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// link_test.go covers SPEC-007 linked sources: file: parse rules, the
// three `source add --link` forms, pull no-op/dangling behavior, the
// index-follows-symlink guarantee, detach-freezes-to-snapshot, and the
// portability invariant (no absolute paths persisted).

func TestParseLinkPath(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"file:../foo-skill", "../foo-skill", false},
		{"file:./vendor/foo-skill", "vendor/foo-skill", false},
		{"file:foo/bar", "foo/bar", false},
		{"file:../../a/b", "../../a/b", false},
		{"file:/Users/tars/foo", "", true}, // absolute rejected
		{"file:/abs", "", true},
		{"file:", "", true},        // empty
		{"../no-scheme", "", true}, // missing file: scheme
		{"skill:me/foo", "", true}, // wrong scheme
	}
	for _, c := range cases {
		got, err := ParseLinkPath(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseLinkPath(%q): expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseLinkPath(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseLinkPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLoadManifest_LinkPinExclusive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "version: 1\n" +
		"sources:\n  - skill:me/foo\n" +
		"overrides:\n  - match: skill:me/foo*\n    link: file:../foo\n    pin_sha: " + shaA + "\n"
	if err := os.WriteFile(ManifestPath(dir), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadManifest(dir); err == nil || !strings.Contains(err.Error(), "cannot also be SHA-pinned") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}

func TestLoadManifest_LinkAbsoluteRejected(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "version: 1\nsources:\n  - skill:me/foo\noverrides:\n  - match: skill:me/foo*\n    link: file:/abs/foo\n"
	if err := os.WriteFile(ManifestPath(dir), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadManifest(dir); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path error, got %v", err)
	}
}

// makeSkillCheckout creates a git repo at <parent>/<name> with a
// SKILL.md at its root and (optionally) an origin remote, returning the
// checkout path. git is required; the test skips if it is absent.
func makeSkillCheckout(t *testing.T, parent, name, remote string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "t@example.com")
	runGit(t, dir, "config", "user.name", "t")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "init")
	if remote != "" {
		runGit(t, dir, "remote", "add", "origin", remote)
	}
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func newLinkPuller(t *testing.T) *Puller {
	t.Helper()
	agentsDir := filepath.Join(t.TempDir(), "proj", ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &Puller{
		AgentsDir:  agentsDir,
		Buckets:    testBuckets,
		Quarantine: true,
		Out:        &bytes.Buffer{},
		Err:        &bytes.Buffer{},
		Now:        func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) },
	}
}

func TestAddLink_UserCheckout(t *testing.T) {
	p := newLinkPuller(t)
	// checkout sits as a sibling of the project root (../foo-skill from
	// the project dir, ../../foo-skill from .agents).
	projRoot := filepath.Dir(p.AgentsDir)
	checkout := makeSkillCheckout(t, filepath.Dir(projRoot), "foo-skill", "")

	rep, err := p.AddLink(context.Background(), checkout, "skill:me/foo-skill", PullOpts{})
	if err != nil {
		t.Fatalf("AddLink: %v", err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Kind != ResultAdded {
		t.Fatalf("expected one added result, got %+v", rep.Results)
	}

	dest := filepath.Join(p.AgentsDir, "skills", "foo-skill")
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("lstat dest: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("dest is not a symlink")
	}
	linkVal, _ := os.Readlink(dest)
	if filepath.IsAbs(linkVal) {
		t.Fatalf("symlink is absolute (%q) — must be relative for portability", linkVal)
	}
	// Resolves to the real SKILL.md.
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Fatalf("symlink does not resolve to SKILL.md: %v", err)
	}

	// Manifest: source entry + link override.
	m, _, err := LoadManifest(p.AgentsDir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if !containsString(m.Sources, "skill:me/foo-skill") {
		t.Errorf("sources missing entry: %+v", m.Sources)
	}
	if link := linkFor(Entry{Raw: "skill:me/foo-skill"}, m); link == "" {
		t.Errorf("no link override recorded: %+v", m.Overrides)
	}

	// Lock: link recorded, managed_clone false, no content hash.
	lock, _ := LoadLock(p.AgentsDir)
	le := lock.Find("skill:me/foo-skill")
	if le == nil || le.Link == "" || le.ManagedClone || le.ContentHash != "" {
		t.Fatalf("lock entry wrong: %+v", le)
	}

	// Portability: nothing persisted contains the absolute checkout path.
	assertNoAbsPaths(t, p.AgentsDir, checkout)

	// Re-pull is a no-op and does not clobber the link.
	saveManifestForPull(t, p) // ensure manifest is what pull reads
	rep2, err := p.Pull(context.Background(), PullOpts{})
	if err != nil {
		t.Fatalf("re-pull: %v", err)
	}
	if k := rep2.Results[0].Kind; k != ResultCurrent {
		t.Errorf("re-pull expected current, got %s", k)
	}
	fi2, _ := os.Lstat(dest)
	if fi2.Mode()&os.ModeSymlink == 0 {
		t.Fatal("re-pull clobbered the symlink")
	}
}

// saveManifestForPull is a no-op placeholder documenting that AddLink
// already persisted the manifest; Pull reads it directly.
func saveManifestForPull(t *testing.T, _ *Puller) { t.Helper() }

func TestAddLink_DeriveFromRemote(t *testing.T) {
	p := newLinkPuller(t)
	projRoot := filepath.Dir(p.AgentsDir)
	checkout := makeSkillCheckout(t, filepath.Dir(projRoot), "widget", "https://github.com/acme/widget.git")

	rep, err := p.AddLink(context.Background(), checkout, "", PullOpts{})
	if err != nil {
		t.Fatalf("AddLink derive: %v", err)
	}
	if rep.Results[0].Entry != "skill:acme/widget" {
		t.Fatalf("derived entry = %q, want skill:acme/widget", rep.Results[0].Entry)
	}
	if _, err := os.Stat(filepath.Join(p.AgentsDir, "skills", "widget", "SKILL.md")); err != nil {
		t.Fatalf("linked skill not resolvable: %v", err)
	}
}

func TestAddLink_ManagedClone(t *testing.T) {
	p := newLinkPuller(t)
	// A local "upstream" git repo stands in for github.com/acme/widget.
	upstream := makeSkillCheckout(t, t.TempDir(), "upstream", "")
	p.CloneURL = func(owner, repo string) string { return "file://" + upstream }

	rep, err := p.AddLink(context.Background(), "", "skill:acme/widget", PullOpts{})
	if err != nil {
		t.Fatalf("AddLink managed: %v", err)
	}
	if rep.Results[0].Kind != ResultAdded {
		t.Fatalf("managed link result: %+v", rep.Results[0])
	}
	clone := filepath.Join(p.AgentsDir, managedSourcesDir, "acme-widget")
	if fi, err := os.Stat(filepath.Join(clone, ".git")); err != nil || !fi.IsDir() {
		t.Fatalf("managed clone missing .git: %v", err)
	}
	lock, _ := LoadLock(p.AgentsDir)
	le := lock.Find("skill:acme/widget")
	if le == nil || !le.ManagedClone {
		t.Fatalf("expected managed_clone true, got %+v", le)
	}
	if _, err := os.Stat(filepath.Join(p.AgentsDir, "skills", "widget", "SKILL.md")); err != nil {
		t.Fatalf("managed-clone skill not resolvable: %v", err)
	}
	assertNoAbsPaths(t, p.AgentsDir, clone)
}

func TestPullLinked_DanglingWarns(t *testing.T) {
	p := newLinkPuller(t)
	projRoot := filepath.Dir(p.AgentsDir)
	checkout := makeSkillCheckout(t, filepath.Dir(projRoot), "foo-skill", "")
	if _, err := p.AddLink(context.Background(), checkout, "skill:me/foo-skill", PullOpts{}); err != nil {
		t.Fatal(err)
	}
	// Move the checkout away → the symlink dangles.
	if err := os.RemoveAll(checkout); err != nil {
		t.Fatal(err)
	}
	errBuf := &bytes.Buffer{}
	p.Err = errBuf
	rep, err := p.Pull(context.Background(), PullOpts{})
	if err != nil {
		t.Fatalf("pull over dangling link should not error: %v", err)
	}
	if rep.Results[0].Kind == ResultFailed {
		t.Fatalf("dangling link should not fail: %+v", rep.Results[0])
	}
	if !strings.Contains(errBuf.String(), "dangling") && !strings.Contains(errBuf.String(), "missing") {
		t.Errorf("expected a dangling-target warning, got %q", errBuf.String())
	}
	// The symlink is left in place (not re-fetched into a real dir).
	fi, _ := os.Lstat(filepath.Join(p.AgentsDir, "skills", "foo-skill"))
	if fi == nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("dangling link was replaced instead of preserved")
	}
}

func TestDetachLink_FreezesToSnapshot(t *testing.T) {
	p := newLinkPuller(t)
	projRoot := filepath.Dir(p.AgentsDir)
	checkout := makeSkillCheckout(t, filepath.Dir(projRoot), "foo-skill", "")
	if _, err := p.AddLink(context.Background(), checkout, "skill:me/foo-skill", PullOpts{}); err != nil {
		t.Fatal(err)
	}

	frozen, err := p.Detach("foo-skill")
	if err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if !frozen {
		t.Fatal("Detach of a link should report frozen=true")
	}

	dest := filepath.Join(p.AgentsDir, "skills", "foo-skill")
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("lstat after detach: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("detach left a symlink — expected a real vendored copy")
	}
	if !fi.IsDir() {
		t.Fatal("detached artifact is not a directory")
	}
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Fatalf("materialized copy missing SKILL.md: %v", err)
	}
	// No .git leaked into the vendored copy.
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		t.Fatal(".git leaked into the detached vendored copy")
	}

	// Origin written with a content hash, source: manifest.
	o, err := ReadOriginFor(dest, true)
	if err != nil {
		t.Fatalf("origin after detach: %v", err)
	}
	if o.ContentHash == "" {
		t.Error("detached origin has empty content_hash")
	}

	// Manifest: link override gone, sources identity kept.
	m, _, _ := LoadManifest(p.AgentsDir)
	if linkFor(Entry{Raw: "skill:me/foo-skill"}, m) != "" {
		t.Error("link override survived detach")
	}
	if !containsString(m.Sources, "skill:me/foo-skill") {
		t.Error("detach dropped the sources identity entry")
	}

	// Lock: snapshot entry with content hash, no link.
	lock, _ := LoadLock(p.AgentsDir)
	le := lock.Find("skill:me/foo-skill")
	if le == nil || le.Link != "" || le.ContentHash == "" {
		t.Fatalf("post-detach lock not a snapshot: %+v", le)
	}
}

// assertNoAbsPaths fails if sources.yaml, sources.lock, or the on-disk
// symlinks contain the given absolute path — the SPEC-007 portability
// invariant.
func assertNoAbsPaths(t *testing.T, agentsDir, absPath string) {
	t.Helper()
	for _, f := range []string{ManifestPath(agentsDir), LockPath(agentsDir)} {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), absPath) {
			t.Errorf("%s contains absolute path %q", filepath.Base(f), absPath)
		}
	}
	filepath.WalkDir(filepath.Join(agentsDir, "skills"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if v, _ := os.Readlink(path); filepath.IsAbs(v) {
				t.Errorf("symlink %s is absolute: %q", path, v)
			}
		}
		return nil
	})
}
