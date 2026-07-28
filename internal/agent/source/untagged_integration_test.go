package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// untagged_integration_test.go is a sanity integration test for
// `sync-agents source add` with an UNTAGGED source (no @ref). It drives
// the real Puller.Add pipeline (parse → append manifest → resolve →
// fetch → extract → vendor → lock) against a real GitHubFetcher pointed
// at an httptest server, so the only thing faked is GitHub itself.
//
// The property under test: an untagged entry resolves against the
// repo's DEFAULT BRANCH, whatever it's named (main OR master). The
// client proves this by requesting the commits/HEAD endpoint and never
// a hardcoded branch name — GitHub's HEAD follows the default branch
// server-side, so main-vs-master is not the client's concern. Each case
// below models a different default branch by having HEAD resolve to a
// different tip SHA; both must round-trip identically.

// recordingGitHub is a fake GitHub that serves the two endpoints
// GitHubFetcher uses and records every request path. commits/HEAD (and
// only HEAD) resolves; a request for commits/main or commits/master is
// a 404 — that 404 is what would fail the test if the client ever
// guessed a branch name instead of asking for HEAD.
func recordingGitHub(t *testing.T, headSHA string, tarball []byte) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits/HEAD"):
			fmt.Fprint(w, headSHA) // Accept: application/vnd.github.sha ⇒ bare SHA
		case strings.Contains(r.URL.Path, "/tarball/"):
			w.Write(tarball)
		default:
			// Any /commits/main, /commits/master, or unknown ref lands
			// here — resolution fails loudly rather than silently
			// picking a branch.
			http.NotFound(w, r)
		}
	}))
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(paths))
		copy(out, paths)
		return out
	}
}

func TestSourceAdd_Untagged_FollowsDefaultBranch(t *testing.T) {
	// Same untagged entry, two repos whose default branch differs only
	// in name/tip. Both must resolve via HEAD and pin the tip SHA.
	cases := []struct {
		name       string
		defaultTip string // the SHA that HEAD (the default branch) points at
	}{
		{name: "default branch main", defaultTip: shaA},
		{name: "default branch master", defaultTip: shaB},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tarball := makeTarball(t, map[string]string{
				"skills/grep-helper/SKILL.md": "---\nname: grep-helper\ndescription: demo. Use when testing.\n---\n# grep\n",
			})
			srv, requests := recordingGitHub(t, tc.defaultTip, tarball)
			defer srv.Close()

			agentsDir := filepath.Join(t.TempDir(), ".agents")
			if err := os.MkdirAll(agentsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			p := &Puller{
				AgentsDir: agentsDir,
				Fetcher: &GitHubFetcher{
					BaseURL:  srv.URL,
					CacheDir: t.TempDir(),
					// Hermetic: never shell out to `gh auth token`.
					TokenFn: func() (string, error) { return "", nil },
				},
				Buckets: testBuckets,
				Out:     &strBuf{},
				Err:     &strBuf{},
				Now:     func() time.Time { return time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC) },
			}

			// The entry under test: no @ref. Path is present only so
			// artifact location is on the known-good route; the variable
			// being exercised is the missing ref.
			const entry = "skill:foo/bar/skills/grep-helper"
			rep, err := p.Add(context.Background(), entry, PullOpts{})
			if err != nil {
				t.Fatalf("source add (untagged): %v", err)
			}
			if got := rep.Count(ResultAdded); got != 1 {
				t.Fatalf("added = %d, want 1 (%+v)", got, rep.Results)
			}

			// 1. Manifest stored the untagged entry verbatim (no ref
			//    synthesized into the text).
			m, _, err := LoadManifest(agentsDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Sources) != 1 || m.Sources[0] != entry {
				t.Fatalf("manifest sources = %v, want [%q]", m.Sources, entry)
			}

			// 2. The artifact was actually vendored.
			if _, err := os.Stat(filepath.Join(agentsDir, "skills", "grep-helper", "SKILL.md")); err != nil {
				t.Errorf("untagged skill not installed: %v", err)
			}

			// 3. The lock pinned the default-branch tip SHA.
			lock, err := LoadLock(agentsDir)
			if err != nil {
				t.Fatal(err)
			}
			le := lock.Find(entry)
			if le == nil {
				t.Fatalf("no lock entry for %q", entry)
			}
			if le.ResolvedSHA != tc.defaultTip {
				t.Errorf("locked SHA = %q, want default-branch tip %q", le.ResolvedSHA, tc.defaultTip)
			}

			// 4. THE point: resolution went through commits/HEAD and the
			//    client never guessed a branch name.
			var sawHEAD bool
			for _, path := range requests() {
				if strings.HasSuffix(path, "/commits/HEAD") {
					sawHEAD = true
				}
				if strings.HasSuffix(path, "/commits/main") || strings.HasSuffix(path, "/commits/master") {
					t.Errorf("client requested a hardcoded branch (%s) instead of HEAD", path)
				}
			}
			if !sawHEAD {
				t.Errorf("untagged entry never resolved via commits/HEAD; requests = %v", requests())
			}
		})
	}
}

// strBuf is a tiny io.Writer sink so the test doesn't depend on the
// bytes.Buffer import already used elsewhere in the package.
type strBuf struct{ b strings.Builder }

func (s *strBuf) Write(p []byte) (int, error) { return s.b.Write(p) }
