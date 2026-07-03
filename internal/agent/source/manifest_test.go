package source

import (
	"strings"
	"testing"
)

func TestManifestAndLock_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Absent manifest: found=false, no error.
	if _, found, err := LoadManifest(dir); err != nil || found {
		t.Fatalf("empty dir: found=%v err=%v", found, err)
	}

	m := Manifest{
		Version: 1,
		Sources: []string{"rule:foo/bar@main/rules/a.md", "tree:org/repo@v2"},
		Overrides: []Override{{
			Match:  "tree:org/*",
			PinSHA: shaA,
		}},
	}
	if err := SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadManifest(dir)
	if err != nil || !found {
		t.Fatalf("reload: found=%v err=%v", found, err)
	}
	if len(got.Sources) != 2 || got.Sources[0] != m.Sources[0] {
		t.Errorf("sources round-trip = %v", got.Sources)
	}
	if len(got.Overrides) != 1 || got.Overrides[0].PinSHA != shaA {
		t.Errorf("overrides round-trip = %+v", got.Overrides)
	}

	// Lock: set, find, remove, persist.
	var lock Lock
	lock.Set(LockEntry{Entry: "e1", ResolvedSHA: shaA, ContentHash: "sha256:x"})
	lock.Set(LockEntry{Entry: "e1", ResolvedSHA: shaB, ContentHash: "sha256:y"}) // upsert
	lock.Set(LockEntry{Entry: "e2", ResolvedSHA: shaA})
	if err := SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if le := reloaded.Find("e1"); le == nil || le.ResolvedSHA != shaB {
		t.Errorf("upsert lost: %+v", le)
	}
	reloaded.Remove("e2")
	if reloaded.Find("e2") != nil {
		t.Error("remove left the entry")
	}
}

func TestOverride_Matches(t *testing.T) {
	o := Override{Match: "skill:anthropic/skill-pack@*/skills/code-review"}
	if !o.Matches("skill:anthropic/skill-pack@v9.9.9/skills/code-review") {
		t.Error("glob failed to match a versioned entry")
	}
	if o.Matches("skill:someone-else/skill-pack@v1/skills/code-review") {
		t.Error("glob matched the wrong owner")
	}
}

func TestSHAHelpers(t *testing.T) {
	if !IsCommitSHA(strings.Repeat("a", 40)) {
		t.Error("40-hex not recognised")
	}
	for _, bad := range []string{strings.Repeat("a", 39), strings.Repeat("g", 40), "v1.0.0"} {
		if IsCommitSHA(bad) {
			t.Errorf("%q wrongly recognised as SHA", bad)
		}
	}
}

func TestDefaultCacheDir_UsesXDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg-test")
	if got := DefaultCacheDir(); !strings.HasPrefix(got, "/tmp/xdg-test") {
		t.Errorf("cache dir %q ignores XDG_CACHE_HOME", got)
	}
}
