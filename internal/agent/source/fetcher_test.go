package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// makeTarball builds a gzipped tarball with GitHub's single
// top-level directory prefix, which ExtractTarball strips.
func makeTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)

	const prefix = "owner-repo-abc123/"
	for _, name := range names {
		content := files[name]
		if err := tw.WriteHeader(&tar.Header{
			Name: prefix + name,
			Mode: 0o644,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// rawTarball builds a gzipped tarball from explicit headers, for
// crafting hostile archives (traversal, symlinks).
func rawTarball(t *testing.T, entries []tar.Header, bodies map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for i := range entries {
		hdr := entries[i]
		body := bodies[hdr.Name]
		hdr.Size = int64(len(body))
		if hdr.Typeflag == tar.TypeSymlink {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(&hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag != tar.TypeSymlink {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

const testSHA = "abc1234567890abcdef0123456789abcdef01234"

// fakeGitHub serves the two endpoints GitHubFetcher uses.
func fakeGitHub(t *testing.T, sha string, tarball []byte, hits *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		switch {
		case strings.Contains(r.URL.Path, "/commits/"):
			fmt.Fprint(w, sha)
		case strings.Contains(r.URL.Path, "/tarball/"):
			w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestFetcher_RefResolution(t *testing.T) {
	hits := 0
	srv := fakeGitHub(t, testSHA, nil, &hits)
	defer srv.Close()
	g := &GitHubFetcher{BaseURL: srv.URL, CacheDir: t.TempDir()}

	for _, ref := range []string{"v1.0.0", "main", ""} {
		sha, err := g.ResolveRef(context.Background(), "foo", "bar", ref)
		if err != nil {
			t.Fatalf("ResolveRef(%q): %v", ref, err)
		}
		if sha != testSHA {
			t.Errorf("ResolveRef(%q) = %q, want %q", ref, sha, testSHA)
		}
	}

	// SHA-form refs resolve locally with no network round trip.
	before := hits
	sha, err := g.ResolveRef(context.Background(), "foo", "bar", strings.ToUpper(testSHA))
	if err != nil {
		t.Fatal(err)
	}
	if sha != testSHA {
		t.Errorf("SHA passthrough = %q, want lowercase %q", sha, testSHA)
	}
	if hits != before {
		t.Error("SHA-form ref hit the network")
	}
}

func TestFetcher_CacheHitAvoidsNetwork(t *testing.T) {
	tarball := makeTarball(t, map[string]string{"SKILL.md": "# x\n"})
	hits := 0
	srv := fakeGitHub(t, testSHA, tarball, &hits)
	defer srv.Close()
	g := &GitHubFetcher{BaseURL: srv.URL, CacheDir: t.TempDir()}

	rc, fromCache, err := g.Fetch(context.Background(), "foo", "bar", testSHA)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()
	if fromCache {
		t.Error("first fetch reported fromCache=true")
	}
	netHits := hits

	rc, fromCache, err = g.Fetch(context.Background(), "foo", "bar", testSHA)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if !fromCache {
		t.Error("second fetch missed the cache")
	}
	if hits != netHits {
		t.Errorf("cache hit still touched the network (%d → %d hits)", netHits, hits)
	}
	if !bytes.Equal(data, tarball) {
		t.Error("cached tarball bytes differ from the original")
	}
}

func TestExtractTarball_WritesFiles(t *testing.T) {
	tarball := makeTarball(t, map[string]string{
		"SKILL.md":       "# skill\n",
		"scripts/run.py": "print(1)\n",
	})
	dest := t.TempDir()
	if err := ExtractTarball(bytes.NewReader(tarball), dest); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"SKILL.md", "scripts/run.py"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing extracted file %s: %v", rel, err)
		}
	}
}

func TestExtractTarball_RejectsTraversal(t *testing.T) {
	evil := rawTarball(t, []tar.Header{
		{Name: "top/../../escape.md", Mode: 0o644, Typeflag: tar.TypeReg},
	}, map[string]string{"top/../../escape.md": "pwned"})

	parent := t.TempDir()
	dest := filepath.Join(parent, "extract")
	os.MkdirAll(dest, 0o755)
	err := ExtractTarball(bytes.NewReader(evil), dest)
	if _, statErr := os.Stat(filepath.Join(parent, "escape.md")); statErr == nil {
		t.Fatal("path traversal escaped the extraction root")
	}
	if err == nil {
		// Silently skipping the entry is acceptable; writing outside
		// the root (checked above) is not.
		t.Log("traversal entry skipped without error")
	}
}

func TestExtractTarball_RejectsEscapingSymlinks(t *testing.T) {
	// A link pointing outside the extraction root fails the WHOLE
	// tarball (fail-closed): hostile input is refused, not partially
	// installed.
	evil := rawTarball(t, []tar.Header{
		{Name: "top/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "top/ok.md", Mode: 0o644, Typeflag: tar.TypeReg},
	}, map[string]string{"top/ok.md": "fine"})

	dest := t.TempDir()
	if err := ExtractTarball(bytes.NewReader(evil), dest); err == nil {
		t.Fatal("escaping symlink did not abort extraction")
	}
	if fi, err := os.Lstat(filepath.Join(dest, "link")); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("symlink from tarball was materialized")
	}
}

func TestTokenResolution_Order(t *testing.T) {
	t.Setenv("SYNC_AGENTS_GITHUB_TOKEN", "aaa")
	t.Setenv("GITHUB_TOKEN", "bbb")
	t.Setenv("GH_TOKEN", "ccc")
	if tok, _ := DefaultTokenFn(); tok != "aaa" {
		t.Errorf("token = %q, want SYNC_AGENTS_GITHUB_TOKEN to win", tok)
	}
	t.Setenv("SYNC_AGENTS_GITHUB_TOKEN", "")
	if tok, _ := DefaultTokenFn(); tok != "bbb" {
		t.Errorf("token = %q, want GITHUB_TOKEN second", tok)
	}
	t.Setenv("GITHUB_TOKEN", "")
	if tok, _ := DefaultTokenFn(); tok != "ccc" {
		t.Errorf("token = %q, want GH_TOKEN third", tok)
	}
}

func TestFetcher_OfflineCacheMissAndCachedSHA(t *testing.T) {
	tarball := makeTarball(t, map[string]string{"a.md": "x\n"})
	hits := 0
	srv := fakeGitHub(t, testSHA, tarball, &hits)
	defer srv.Close()
	g := &GitHubFetcher{BaseURL: srv.URL, CacheDir: t.TempDir()}

	if g.CachedSHA("foo", "bar", testSHA) {
		t.Error("CachedSHA true before any fetch")
	}
	rc, _, err := g.Fetch(context.Background(), "foo", "bar", testSHA)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()
	if !g.CachedSHA("foo", "bar", testSHA) {
		t.Error("CachedSHA false after a fetch populated the cache")
	}
}
