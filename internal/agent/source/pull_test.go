package source

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeFetcher implements Fetcher entirely in memory: refs maps
// "owner/repo@ref" → sha, tars maps "owner/repo@sha" → tarball bytes.
type fakeFetcher struct {
	refs    map[string]string
	tars    map[string][]byte
	fetches int
}

func (f *fakeFetcher) ResolveRef(_ context.Context, owner, repo, ref string) (string, error) {
	if isHexSHA(ref) {
		return ref, nil
	}
	if ref == "" {
		ref = "HEAD"
	}
	sha, ok := f.refs[owner+"/"+repo+"@"+ref]
	if !ok {
		return "", fmt.Errorf("fake: unknown ref %s/%s@%s", owner, repo, ref)
	}
	return sha, nil
}

func (f *fakeFetcher) Fetch(_ context.Context, owner, repo, sha string) (io.ReadCloser, bool, error) {
	data, ok := f.tars[owner+"/"+repo+"@"+sha]
	if !ok {
		return nil, false, fmt.Errorf("fake: no tarball for %s/%s@%s", owner, repo, sha)
	}
	f.fetches++
	return io.NopCloser(bytes.NewReader(data)), false, nil
}

var testBuckets = []BucketInfo{
	{Dir: "rules"},
	{Dir: "skills", DirPerArtifact: true},
	{Dir: "workflows"},
	{Dir: "plans"},
}

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// newPuller returns a Puller over a fresh temp .agents dir plus its
// fake fetcher, pre-loaded with one repo serving a skill and a rule
// at shaA for the "v1" tag.
func newPuller(t *testing.T) (*Puller, *fakeFetcher) {
	t.Helper()
	agentsDir := filepath.Join(t.TempDir(), ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tarball := makeTarball(t, map[string]string{
		"skills/grep-helper/SKILL.md": "---\nname: grep-helper\n---\n# grep\n",
		"rules/security.md":           "# security rule\n",
	})
	ff := &fakeFetcher{
		refs: map[string]string{"foo/bar@v1": shaA},
		tars: map[string][]byte{"foo/bar@" + shaA: tarball},
	}
	p := &Puller{
		AgentsDir: agentsDir,
		Fetcher:   ff,
		Buckets:   testBuckets,
		Out:       &bytes.Buffer{},
		Err:       &bytes.Buffer{},
		Now:       func() time.Time { return time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC) },
	}
	return p, ff
}

func saveManifestEntries(t *testing.T, p *Puller, entries ...string) {
	t.Helper()
	if err := SaveManifest(p.AgentsDir, Manifest{Version: 1, Sources: entries}); err != nil {
		t.Fatal(err)
	}
}

func TestPull_AddsSkillAndRule(t *testing.T) {
	p, _ := newPuller(t)
	saveManifestEntries(t, p,
		"skill:foo/bar@v1/skills/grep-helper",
		"rule:foo/bar@v1/rules/security.md",
	)

	report, err := p.Pull(context.Background(), PullOpts{})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if got := report.Count(ResultAdded); got != 2 {
		t.Fatalf("added = %d, want 2 (%+v)", got, report.Results)
	}

	skill := filepath.Join(p.AgentsDir, "skills", "grep-helper", "SKILL.md")
	if _, err := os.Stat(skill); err != nil {
		t.Fatalf("skill not installed: %v", err)
	}
	o, err := ReadOriginFor(filepath.Join(p.AgentsDir, "skills", "grep-helper"), true)
	if err != nil {
		t.Fatalf("skill origin: %v", err)
	}
	if o.Owner != "foo" || o.Repo != "bar" || o.Ref != "v1" || o.SHA != shaA || o.Source != "manifest" || o.Schema != 1 {
		t.Errorf("skill origin = %+v", o)
	}

	rule := filepath.Join(p.AgentsDir, "rules", "security.md")
	if _, err := os.Stat(rule); err != nil {
		t.Fatalf("rule not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.AgentsDir, "rules", "security.origin.json")); err != nil {
		t.Fatalf("rule origin sibling missing: %v", err)
	}

	lock, err := LoadLock(p.AgentsDir)
	if err != nil {
		t.Fatal(err)
	}
	le := lock.Find("skill:foo/bar@v1/skills/grep-helper")
	if le == nil || le.ResolvedSHA != shaA || le.ContentHash == "" {
		t.Errorf("lock entry = %+v", le)
	}
}

func TestPull_Idempotent(t *testing.T) {
	p, _ := newPuller(t)
	saveManifestEntries(t, p, "rule:foo/bar@v1/rules/security.md")

	if _, err := p.Pull(context.Background(), PullOpts{}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(p.AgentsDir, "rules", "security.md")
	before, _ := os.Stat(dest)

	report, err := p.Pull(context.Background(), PullOpts{})
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if got := report.Count(ResultCurrent); got != 1 {
		t.Fatalf("current = %d, want 1 (%+v)", got, report.Results)
	}
	after, _ := os.Stat(dest)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("idempotent pull rewrote the artifact (mtime changed)")
	}
}

func TestPull_LocalModificationBlocksWithoutForce(t *testing.T) {
	p, _ := newPuller(t)
	saveManifestEntries(t, p, "rule:foo/bar@v1/rules/security.md")
	if _, err := p.Pull(context.Background(), PullOpts{}); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(p.AgentsDir, "rules", "security.md")
	if err := os.WriteFile(dest, []byte("locally edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, _ := p.Pull(context.Background(), PullOpts{})
	if got := report.Count(ResultFailed); got != 1 {
		t.Fatalf("failed = %d, want 1 (%+v)", got, report.Results)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "locally edited\n" {
		t.Error("local modification was overwritten without --force")
	}

	// --force replaces the local edit.
	report, err := p.Pull(context.Background(), PullOpts{Force: true})
	if err != nil {
		t.Fatalf("forced pull: %v", err)
	}
	if report.Count(ResultUpdated)+report.Count(ResultAdded) != 1 {
		t.Fatalf("forced pull did not rewrite (%+v)", report.Results)
	}
	data, _ = os.ReadFile(dest)
	if string(data) != "# security rule\n" {
		t.Errorf("forced pull content = %q", data)
	}
}

func TestPull_DryRunWritesNothing(t *testing.T) {
	p, _ := newPuller(t)
	saveManifestEntries(t, p, "rule:foo/bar@v1/rules/security.md")

	report, err := p.Pull(context.Background(), PullOpts{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Count(ResultWouldAdd); got != 1 {
		t.Fatalf("would-add = %d, want 1 (%+v)", got, report.Results)
	}
	if _, err := os.Stat(filepath.Join(p.AgentsDir, "rules", "security.md")); !os.IsNotExist(err) {
		t.Error("dry-run wrote the artifact")
	}
}

func TestPull_IntegrityMismatchAborts(t *testing.T) {
	p, _ := newPuller(t)
	entry := "rule:foo/bar@v1/rules/security.md"
	saveManifestEntries(t, p, entry)

	// Prime the lock with a bogus recorded hash for shaA: the fetched
	// tarball will hash differently, which must abort before any
	// destination write (AC-10).
	lock := Lock{Version: 1}
	lock.Set(LockEntry{Entry: entry, ResolvedSHA: shaA, ContentHash: "sha256:deadbeef"})
	if err := SaveLock(p.AgentsDir, lock); err != nil {
		t.Fatal(err)
	}

	report, _ := p.Pull(context.Background(), PullOpts{})
	if got := report.Count(ResultFailed); got != 1 {
		t.Fatalf("failed = %d, want 1 (%+v)", got, report.Results)
	}
	if _, err := os.Stat(filepath.Join(p.AgentsDir, "rules", "security.md")); !os.IsNotExist(err) {
		t.Fatal("integrity mismatch still wrote the destination")
	}
}

func TestPull_TreeFanout(t *testing.T) {
	p, ff := newPuller(t)
	tarball := makeTarball(t, map[string]string{
		".agents/rules/r1.md":          "r1\n",
		".agents/skills/s1/SKILL.md":   "---\nname: s1\n---\n",
		".agents/workflows/w1.md":      "w1\n",
		".agents/plans/roadmap.md":     "plan\n", // SPEC-004 bucket, not just the classic three
		".agents/unrelated/ignored.md": "not a bucket\n",
		"README.md":                    "outside .agents entirely\n",
	})
	ff.refs["org/team@v2"] = shaB
	ff.tars["org/team@"+shaB] = tarball
	saveManifestEntries(t, p, "tree:org/team@v2")

	report, err := p.Pull(context.Background(), PullOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Count(ResultAdded); got != 1 {
		t.Fatalf("added = %d, want 1 tree entry (%+v)", got, report.Results)
	}
	for _, rel := range []string{"rules/r1.md", "skills/s1/SKILL.md", "workflows/w1.md", "plans/roadmap.md"} {
		if _, err := os.Stat(filepath.Join(p.AgentsDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("tree fanout missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(p.AgentsDir, "unrelated")); !os.IsNotExist(err) {
		t.Error("tree fanout installed a non-bucket directory")
	}
}

func TestUpdate_MovedTagRefetches(t *testing.T) {
	p, ff := newPuller(t)
	saveManifestEntries(t, p, "rule:foo/bar@v1/rules/security.md")
	if _, err := p.Pull(context.Background(), PullOpts{}); err != nil {
		t.Fatal(err)
	}

	// Upstream retags v1 to shaB with new content.
	ff.refs["foo/bar@v1"] = shaB
	ff.tars["foo/bar@"+shaB] = makeTarball(t, map[string]string{
		"rules/security.md": "# security rule v2\n",
	})

	report, err := p.Pull(context.Background(), PullOpts{UpdateMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Count(ResultUpdated); got != 1 {
		t.Fatalf("updated = %d, want 1 (%+v)", got, report.Results)
	}
	data, _ := os.ReadFile(filepath.Join(p.AgentsDir, "rules", "security.md"))
	if string(data) != "# security rule v2\n" {
		t.Errorf("content after update = %q", data)
	}
	lock, _ := LoadLock(p.AgentsDir)
	if le := lock.Find("rule:foo/bar@v1/rules/security.md"); le == nil || le.ResolvedSHA != shaB {
		t.Errorf("lock not advanced: %+v", le)
	}
}

func TestUpdate_SHAPinnedSkipped(t *testing.T) {
	p, ff := newPuller(t)
	entry := "rule:foo/bar@" + shaA + "/rules/security.md"
	saveManifestEntries(t, p, entry)
	if _, err := p.Pull(context.Background(), PullOpts{}); err != nil {
		t.Fatal(err)
	}
	fetchesBefore := ff.fetches

	report, err := p.Pull(context.Background(), PullOpts{UpdateMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Count(ResultSkipped); got != 1 {
		t.Fatalf("skipped = %d, want 1 (%+v)", got, report.Results)
	}
	if ff.fetches != fetchesBefore {
		t.Error("SHA-pinned update still fetched a tarball")
	}
}
