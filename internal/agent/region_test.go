package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSpliceRegion(t *testing.T) {
	r := ManagedRegion{Name: "x"}
	block := r.Start() + "\nNEW\n" + r.End() + "\n"
	cases := []struct {
		name, in, want string
	}{
		{"empty file becomes the block", "", block},
		{"appended after a blank line", "# T\n", "# T\n\n" + block},
		{"unterminated file gets its newline first", "# T", "# T\n\n" + block},
		{"replaced in place", "a\n" + r.Start() + "\nOLD\n" + r.End() + "\nz\n", "a\n" + block + "z\n"},
		{"start without end appends", "a\n" + r.Start() + "\n", "a\n" + r.Start() + "\n\n" + block},
		{"other region untouched", "<!-- sync-agents:y:start -->\nY\n<!-- sync-agents:y:end -->\n",
			"<!-- sync-agents:y:start -->\nY\n<!-- sync-agents:y:end -->\n\n" + block},
	}
	for _, c := range cases {
		if got := spliceRegion(c.in, r, block); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestStripRegion_InvertsSplice(t *testing.T) {
	r := ManagedRegion{Name: "x"}
	block := r.Start() + "\nNEW\n" + r.End() + "\n"
	for _, orig := range []string{"# T\n", "# T\n\n## Tools\n\nkeep\n"} {
		got, found := stripRegion(spliceRegion(orig, r, block), r)
		if !found || got != orig {
			t.Errorf("strip(splice(%q)) = %q, found=%v", orig, got, found)
		}
	}
	mid := "a\n" + block + "z\n"
	if got, _ := stripRegion(mid, r); got != "a\nz\n" {
		t.Errorf("mid-file strip = %q", got)
	}
	if got, found := stripRegion("plain\n", r); found || got != "plain\n" {
		t.Errorf("no region: got %q found=%v", got, found)
	}
}

func TestWriteIfChanged_SkipsEqualBytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.md")
	if err := os.WriteFile(p, []byte("same\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	changed, err := writeIfChanged(p, []byte("same\n"))
	if err != nil || changed {
		t.Fatalf("equal bytes: changed=%v err=%v", changed, err)
	}
	fi, _ := os.Stat(p)
	if !fi.ModTime().Equal(old) {
		t.Errorf("mtime moved on a no-op write: %v -> %v", old, fi.ModTime())
	}

	if changed, err := writeIfChanged(p, []byte("new\n")); err != nil || !changed {
		t.Fatalf("new bytes: changed=%v err=%v", changed, err)
	}
	fi, _ = os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode not preserved: %v", fi.Mode().Perm())
	}
}

func TestWriteIfChanged_WritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.md")
	link := filepath.Join(dir, "link.md")
	if err := os.WriteFile(real, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := writeIfChanged(link, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced by a regular file")
	}
	if got, _ := os.ReadFile(real); string(got) != "new\n" {
		t.Errorf("target not updated: %q", got)
	}
}

func writeEntry(t *testing.T, dir, name, body string) ConcatEntry {
	t.Helper()
	p := filepath.Join(dir, name+".md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return ConcatEntry{Name: name, SourcePath: p}
}

func TestRenderRegion_LimitDemotesLargestFirstAndKeepsHeadroom(t *testing.T) {
	dir := t.TempDir()
	entries := []ConcatEntry{
		writeEntry(t, dir, "small", "Small body.\n"),
		writeEntry(t, dir, "big", "Big opening.\n\n"+strings.Repeat("x", 3000)+"\n"),
		writeEntry(t, dir, "mid", "Mid opening.\n\n"+strings.Repeat("y", 1000)+"\n"),
	}
	unlimited, err := renderRegion("", OpenClawRulesRegion, entries, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unlimited, strings.Repeat("x", 3000)) {
		t.Fatal("limit 0 must inline everything")
	}

	limit := 2000
	got, err := renderRegion("", OpenClawRulesRegion, entries, limit)
	if err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(got); n > limit-limit*regionHeadroomPct/100 {
		t.Errorf("region is %d chars, want within the headroom target", n)
	}
	if strings.Contains(got, strings.Repeat("x", 100)) {
		t.Error("largest entry was not summarized")
	}
	if !strings.Contains(got, "Small body.") {
		t.Error("smallest entry should stay inline")
	}
}

func TestRenderRegion_ImpossibleLimitReturnsTypedError(t *testing.T) {
	dir := t.TempDir()
	entries := []ConcatEntry{writeEntry(t, dir, "a", "Body.\n")}
	got, err := renderRegion("host text\n", OpenClawRulesRegion, entries, 20)
	var over *RegionOverBudgetError
	if !errors.As(err, &over) || over.Limit != 20 || over.Chars <= 20 {
		t.Fatalf("want *RegionOverBudgetError, got %v", err)
	}
	if got != "" {
		t.Errorf("no text may be returned on failure, got %q", got)
	}
}
