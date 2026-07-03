package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHashTree_Deterministic(t *testing.T) {
	files := map[string]string{
		"SKILL.md":       "# skill\n",
		"scripts/run.py": "print('hi')\n",
		"reference/a.md": "ref\n",
	}
	d1, d2 := t.TempDir(), t.TempDir()
	writeTree(t, d1, files)
	writeTree(t, d2, files)

	h1, err := HashTree(d1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashTree(d2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("identical trees hash differently: %s vs %s", h1, h2)
	}
	if !strings.HasPrefix(h1, HashPrefix) {
		t.Errorf("hash %q missing %q prefix", h1, HashPrefix)
	}
}

func TestHashTree_ContentSensitive(t *testing.T) {
	d1, d2 := t.TempDir(), t.TempDir()
	writeTree(t, d1, map[string]string{"a.md": "one\n"})
	writeTree(t, d2, map[string]string{"a.md": "two\n"})
	h1, _ := HashTree(d1)
	h2, _ := HashTree(d2)
	if h1 == h2 {
		t.Error("different content produced identical hashes")
	}
}

func TestHashTree_ExcludesOriginFiles(t *testing.T) {
	d1, d2 := t.TempDir(), t.TempDir()
	writeTree(t, d1, map[string]string{"SKILL.md": "x\n"})
	writeTree(t, d2, map[string]string{
		"SKILL.md":     "x\n",
		"_origin.json": `{"schema":1}`,
	})
	h1, _ := HashTree(d1)
	h2, _ := HashTree(d2)
	if h1 != h2 {
		t.Error("_origin.json affected the content hash (chicken-and-egg: origin records the hash)")
	}

	if !IsOriginFile("_origin.json") || !IsOriginFile("security.origin.json") {
		t.Error("IsOriginFile misses origin filenames")
	}
	if IsOriginFile("SKILL.md") {
		t.Error("IsOriginFile false-positive on SKILL.md")
	}
}

func TestHashTree_SingleFile(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"rule.md": "content\n"})
	h, err := HashTree(filepath.Join(dir, "rule.md"))
	if err != nil {
		t.Fatalf("HashTree on a single file: %v", err)
	}
	if !strings.HasPrefix(h, HashPrefix) {
		t.Errorf("single-file hash %q missing prefix", h)
	}
}
