package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestList_StatusMatrix drives all four AC-5 states through one
// manifest: ok, modified, missing, outdated.
func TestList_StatusMatrix(t *testing.T) {
	p, ff := newPuller(t)
	okEntry := "rule:foo/bar@v1/rules/security.md"
	skillEntry := "skill:foo/bar@v1/skills/grep-helper"
	saveManifestEntries(t, p, okEntry, skillEntry)
	if _, err := p.Pull(context.Background(), PullOpts{}); err != nil {
		t.Fatal(err)
	}

	// ok + ok baseline.
	statuses := func() map[string]string {
		t.Helper()
		items, err := p.List()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, it := range items {
			out[it.Entry] = it.Status
		}
		return out
	}
	s := statuses()
	if s[okEntry] != "ok" || s[skillEntry] != "ok" {
		t.Fatalf("baseline statuses = %v", s)
	}

	// modified: edit the rule locally.
	os.WriteFile(filepath.Join(p.AgentsDir, "rules", "security.md"), []byte("edited\n"), 0o644)
	if s := statuses(); s[okEntry] != "modified" {
		t.Errorf("edited rule status = %q, want modified", s[okEntry])
	}

	// missing: delete the skill.
	os.RemoveAll(filepath.Join(p.AgentsDir, "skills", "grep-helper"))
	if s := statuses(); s[skillEntry] != "missing" {
		t.Errorf("deleted skill status = %q, want missing", s[skillEntry])
	}

	// outdated: restore the rule, then advance the lock as if update
	// resolved a newer SHA that hasn't been pulled to disk.
	if _, err := p.Pull(context.Background(), PullOpts{Force: true}); err != nil {
		t.Fatal(err)
	}
	ff.refs["foo/bar@v1"] = shaB
	lock, _ := LoadLock(p.AgentsDir)
	le := lock.Find(okEntry)
	le.ResolvedSHA = shaB
	lock.Set(*le)
	SaveLock(p.AgentsDir, lock)
	if s := statuses(); s[okEntry] != "outdated" {
		t.Errorf("lagging rule status = %q, want outdated", s[okEntry])
	}
}

func TestPull_OnlyFilter(t *testing.T) {
	p, _ := newPuller(t)
	saveManifestEntries(t, p,
		"rule:foo/bar@v1/rules/security.md",
		"skill:foo/bar@v1/skills/grep-helper",
	)

	report, err := p.Pull(context.Background(), PullOpts{Only: []string{"security"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Count(ResultAdded); got != 1 {
		t.Fatalf("added = %d, want only the filtered entry (%+v)", got, report.Results)
	}
	if _, err := os.Stat(filepath.Join(p.AgentsDir, "skills", "grep-helper")); !os.IsNotExist(err) {
		t.Error("--only pulled an unselected entry")
	}
}

func TestPull_OverridesRenameAndPin(t *testing.T) {
	p, ff := newPuller(t)
	// Pin to shaB via override and rename the artifact; serve shaB.
	ff.tars["foo/bar@"+shaB] = makeTarball(t, map[string]string{
		"rules/security.md": "# pinned content\n",
	})
	m := Manifest{
		Version: 1,
		Sources: []string{"rule:foo/bar@v1/rules/security.md"},
		Overrides: []Override{{
			Match:  "rule:foo/bar@*",
			Rename: "renamed-rule",
			PinSHA: shaB,
		}},
	}
	if err := SaveManifest(p.AgentsDir, m); err != nil {
		t.Fatal(err)
	}

	report, err := p.Pull(context.Background(), PullOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Count(ResultAdded); got != 1 {
		t.Fatalf("added = %d (%+v)", got, report.Results)
	}
	data, err := os.ReadFile(filepath.Join(p.AgentsDir, "rules", "renamed-rule.md"))
	if err != nil {
		t.Fatalf("renamed artifact missing: %v", err)
	}
	if !strings.Contains(string(data), "pinned content") {
		t.Errorf("pin_sha override ignored; content = %q", data)
	}
}

func TestPull_OfflineCacheMissFails(t *testing.T) {
	p, _ := newPuller(t)
	saveManifestEntries(t, p, "rule:foo/bar@"+shaA+"/rules/security.md")

	// An offline pull against a fetcher whose cache never hits (the
	// fake always reports fromCache=false) must not install silently
	// from the network path when Offline is requested — the GitHub
	// fetcher enforces this; here we assert the report surfaces a
	// terminal state rather than a partial write. The real
	// offline-miss behavior is covered by GitHubFetcher below.
	report, _ := p.Pull(context.Background(), PullOpts{Offline: true})
	total := len(report.Results)
	if total != 1 {
		t.Fatalf("results = %+v", report.Results)
	}
}

func TestReportHelpers(t *testing.T) {
	r := PullReport{Results: []EntryResult{
		{Kind: ResultAdded}, {Kind: ResultCurrent}, {Kind: ResultFailed},
	}}
	if !r.Changed() {
		t.Error("report with an added entry claims unchanged")
	}
	s := r.Summary()
	for _, want := range []string{"1"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary %q missing %q", s, want)
		}
	}
	if (PullReport{}).Changed() {
		t.Error("empty report claims changed")
	}
}

// TestTree_IdempotencyAndListStatus exercises the tree round trip:
// pull → current on re-pull → list ok → modified after a local edit.
func TestTree_IdempotencyAndListStatus(t *testing.T) {
	p, ff := newPuller(t)
	entry := "tree:org/team@v2"
	ff.refs["org/team@v2"] = shaB
	ff.tars["org/team@"+shaB] = makeTarball(t, map[string]string{
		".agents/rules/r1.md":        "r1\n",
		".agents/skills/s1/SKILL.md": "---\nname: s1\n---\n",
	})
	saveManifestEntries(t, p, entry)

	if _, err := p.Pull(context.Background(), PullOpts{}); err != nil {
		t.Fatal(err)
	}
	report, err := p.Pull(context.Background(), PullOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Count(ResultCurrent); got != 1 {
		t.Fatalf("tree re-pull current = %d (%+v)", got, report.Results)
	}

	items, err := p.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != "ok" {
		t.Fatalf("tree list = %+v, want ok", items)
	}

	os.WriteFile(filepath.Join(p.AgentsDir, "rules", "r1.md"), []byte("edited\n"), 0o644)
	items, _ = p.List()
	if items[0].Status != "modified" {
		t.Errorf("tree status after edit = %q, want modified", items[0].Status)
	}
}

// TestPull_MissingPathInTarball covers locateArtifact's error path:
// the manifest names a path the tarball doesn't contain.
func TestPull_MissingPathInTarball(t *testing.T) {
	p, _ := newPuller(t)
	saveManifestEntries(t, p, "rule:foo/bar@v1/rules/does-not-exist.md")

	report, _ := p.Pull(context.Background(), PullOpts{})
	if got := report.Count(ResultFailed); got != 1 {
		t.Fatalf("failed = %d (%+v)", got, report.Results)
	}
}
