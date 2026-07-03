package source

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// fetcher.go implements SPEC-003 §Tarball fetch: ref resolution and
// content download via the GitHub REST API over net/http. It NEVER
// shells out to git or curl — that keeps the tool usable in CI
// containers without git and lets us cache immutable tarballs by SHA.
// (The one permitted subprocess is `gh auth token`, which the spec
// names explicitly as the last rung of the token ladder.)

// DefaultBaseURL is the production GitHub REST API endpoint. Tests
// inject an httptest.Server URL here; a future GitHub Enterprise
// story would too.
const DefaultBaseURL = "https://api.github.com"

// EnvToken is the sync-agents-specific token variable, consulted
// before the generic GITHUB_TOKEN/GH_TOKEN so a user can give this
// tool a narrower-scoped token than their general-purpose one.
const EnvToken = "SYNC_AGENTS_GITHUB_TOKEN"

// Fetcher resolves refs and fetches content tarballs. Pull/update
// depend on this interface (not GitHubFetcher) so tests can substitute
// a fake and a future GitLab/Bitbucket fetcher slots in cleanly.
type Fetcher interface {
	// ResolveRef returns the commit SHA for a tag/branch/SHA ref.
	// An empty ref means the repo's default branch.
	ResolveRef(ctx context.Context, owner, repo, ref string) (string, error)
	// Fetch returns a tarball reader for repo@sha, hitting the local
	// cache first. fromCache reports whether the network was avoided.
	Fetch(ctx context.Context, owner, repo, sha string) (io.ReadCloser, bool, error)
}

// GitHubFetcher is the production Fetcher. Zero-value fields fall
// back to sane defaults at call time (nil client → shared client with
// timeout; empty BaseURL → api.github.com; empty CacheDir →
// DefaultCacheDir; nil TokenFn → DefaultTokenFn), so constructing one
// is just &GitHubFetcher{} in production and a two-field literal in
// tests.
type GitHubFetcher struct {
	HTTPClient *http.Client
	// BaseURL is the API root without trailing slash. Both the
	// commits endpoint (ref resolution) and the tarball endpoint are
	// derived from it, so one httptest.Server can serve both in tests.
	BaseURL  string
	CacheDir string
	TokenFn  func() (string, error)
}

func (g *GitHubFetcher) client() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	// Generous timeout: tarballs of large repos over slow links.
	// Ref resolution shares the client; its payloads are tiny so the
	// ceiling never matters there.
	return &http.Client{Timeout: 5 * time.Minute}
}

func (g *GitHubFetcher) baseURL() string {
	if g.BaseURL != "" {
		return strings.TrimRight(g.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (g *GitHubFetcher) cacheDir() string {
	if g.CacheDir != "" {
		return g.CacheDir
	}
	return DefaultCacheDir()
}

// DefaultCacheDir resolves $XDG_CACHE_HOME/sync-agents/sources,
// falling back to ~/.cache/sync-agents/sources. The path embeds the
// host segment ("github.com") at the per-repo level (see cachePath)
// to leave room for other SCM hosts.
func DefaultCacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			// No home: fall back to the OS temp dir rather than
			// failing — the cache is an optimization, not a
			// requirement.
			return filepath.Join(os.TempDir(), "sync-agents", "sources")
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "sync-agents", "sources")
}

// DefaultTokenFn implements the SPEC-003 / skillfish token ladder:
// SYNC_AGENTS_GITHUB_TOKEN > GITHUB_TOKEN > GH_TOKEN > `gh auth
// token`. Absence of every rung is non-fatal (anonymous API access is
// allowed; rate-limit errors later hint at auth), so this never
// returns an error for "no token" — only "" with nil error.
func DefaultTokenFn() (string, error) {
	for _, env := range []string{EnvToken, "GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, nil
		}
	}
	// Best-effort shellout to gh. LookPath first so machines without
	// gh don't pay for a failed exec; any gh failure (not logged in,
	// broken config) degrades to anonymous access.
	if _, err := exec.LookPath("gh"); err == nil {
		out, err := exec.Command("gh", "auth", "token").Output()
		if err == nil {
			if tok := strings.TrimSpace(string(out)); tok != "" {
				return tok, nil
			}
		}
	}
	return "", nil
}

func (g *GitHubFetcher) token() string {
	fn := g.TokenFn
	if fn == nil {
		fn = DefaultTokenFn
	}
	tok, err := fn()
	if err != nil {
		return "" // token acquisition is best-effort by contract
	}
	return tok
}

func (g *GitHubFetcher) authorize(req *http.Request) {
	if tok := g.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
}

// ResolveRef resolves a tag/branch to its commit SHA via
// GET /repos/{owner}/{repo}/commits/{ref} with
// Accept: application/vnd.github.sha (the response body is the bare
// SHA). Already-pinned 40-hex refs return immediately with no network
// — pull and update rely on that for the "SHA-pinned entries make no
// resolution call" guarantee.
func (g *GitHubFetcher) ResolveRef(ctx context.Context, owner, repo, ref string) (string, error) {
	if isHexSHA(ref) {
		return strings.ToLower(ref), nil
	}
	if ref == "" {
		// HEAD resolves the repository's default branch on the
		// commits endpoint — one request instead of a repo-metadata
		// lookup followed by a branch lookup.
		ref = "HEAD"
	}
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", g.baseURL(), owner, repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.sha")
	g.authorize(req)

	resp, err := g.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve %s/%s@%s: %w", owner, repo, ref, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("resolve %s/%s@%s: ref not found (HTTP 404)", owner, repo, ref)
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		return "", fmt.Errorf("resolve %s/%s@%s: HTTP %d — likely a GitHub rate limit; set %s (or GITHUB_TOKEN/GH_TOKEN, or `gh auth login`) to authenticate", owner, repo, ref, resp.StatusCode, EnvToken)
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("resolve %s/%s@%s: HTTP %d", owner, repo, ref, resp.StatusCode)
	}

	sha := strings.TrimSpace(string(body))
	if isHexSHA(sha) {
		return strings.ToLower(sha), nil
	}
	// Fallback: a proxy or mock that ignores the Accept header
	// returns the full commit JSON; pull the sha field out of it.
	var payload struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && isHexSHA(payload.SHA) {
		return strings.ToLower(payload.SHA), nil
	}
	return "", fmt.Errorf("resolve %s/%s@%s: unrecognized response (neither bare SHA nor commit JSON)", owner, repo, ref)
}

// cachePath is $CACHE/github.com/<owner>/<repo>/<sha>.tar.gz —
// keyed by SHA because tarballs of a commit are immutable, which is
// what makes "cache hit ⇒ zero network" safe.
func (g *GitHubFetcher) cachePath(owner, repo, sha string) string {
	return filepath.Join(g.cacheDir(), "github.com", owner, repo, sha+".tar.gz")
}

// Fetch returns a reader for the repo@sha tarball. Cache hits avoid
// the network entirely (AC-7). Downloads stream to a temp file that
// is renamed into the cache only on success, so an interrupted
// download can never poison future cache hits.
func (g *GitHubFetcher) Fetch(ctx context.Context, owner, repo, sha string) (io.ReadCloser, bool, error) {
	cp := g.cachePath(owner, repo, sha)
	if f, err := os.Open(cp); err == nil {
		return f, true, nil
	}

	url := fmt.Sprintf("%s/repos/%s/%s/tarball/%s", g.baseURL(), owner, repo, sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	g.authorize(req)

	resp, err := g.client().Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("fetch %s/%s@%s: %w", owner, repo, shortSHA(sha), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			return nil, false, fmt.Errorf("fetch %s/%s@%s: HTTP %d — likely a GitHub rate limit; set %s (or GITHUB_TOKEN/GH_TOKEN, or `gh auth login`) to authenticate", owner, repo, shortSHA(sha), resp.StatusCode, EnvToken)
		}
		return nil, false, fmt.Errorf("fetch %s/%s@%s: HTTP %d", owner, repo, shortSHA(sha), resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(cp), 0o755); err != nil {
		return nil, false, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(cp), ".download-*")
	if err != nil {
		return nil, false, err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return nil, false, fmt.Errorf("fetch %s/%s@%s: %w", owner, repo, shortSHA(sha), err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return nil, false, err
	}
	if err := os.Rename(tmpName, cp); err != nil {
		os.Remove(tmpName)
		return nil, false, err
	}
	f, err := os.Open(cp)
	if err != nil {
		return nil, false, err
	}
	return f, false, nil
}

// CachedSHA reports whether repo@sha is already in the cache —
// used by --offline to answer "can this be served locally?" without
// opening the file.
func (g *GitHubFetcher) CachedSHA(owner, repo, sha string) bool {
	_, err := os.Stat(g.cachePath(owner, repo, sha))
	return err == nil
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// -------------------------------------------------------------------------
// Tarball extraction (SPEC-005 Part A groundwork)
// -------------------------------------------------------------------------

// Extraction hard limits. Upstream repos are untrusted input — a
// hostile tarball must not be able to exhaust disk or fill the inode
// table before per-file validation gets a chance to reject anything.
const (
	maxTarEntries       = 10_000
	maxTarExtractedSize = 100 << 20 // 100 MiB
)

// ExtractTarball unpacks a GitHub-style .tar.gz (single top-level
// directory prefix, which is stripped) into dest, defensively:
//
//   - path traversal (.., absolute paths) aborts extraction;
//   - symlinks/hardlinks whose target escapes dest abort extraction;
//     in-tree links are skipped rather than recreated (nothing in an
//     .agents/ tree legitimately needs them, and recreating links is
//     how later files get smuggled through them);
//   - regular files are written 0644 and directories 0755 — upstream
//     mode bits (setuid, exec) are never propagated;
//   - entry count and total extracted bytes are capped.
//
// dest must be a fresh private directory (the puller uses a temp
// staging dir); ExtractTarball assumes it can create paths under it
// freely.
func ExtractTarball(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	entries := 0
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("extract: %w", err)
		}
		entries++
		if entries > maxTarEntries {
			return fmt.Errorf("extract: tarball has more than %d entries — refusing", maxTarEntries)
		}

		rel, ok := stripTopLevel(hdr.Name)
		if !ok {
			continue // the top-level dir itself, or pax metadata
		}
		clean := path.Clean(rel)
		if clean == "." || clean == "" {
			continue
		}
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("extract: entry %q escapes the extraction root — refusing", hdr.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(clean))

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += hdr.Size
			if total > maxTarExtractedSize {
				return fmt.Errorf("extract: tarball exceeds %d MiB extracted — refusing", maxTarExtractedSize>>20)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			// LimitReader belt-and-suspenders: a lying header Size
			// can't sneak extra bytes past the total cap.
			if _, err := io.Copy(f, io.LimitReader(tr, maxTarExtractedSize+1)); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			if linkEscapes(clean, hdr.Linkname) {
				return fmt.Errorf("extract: link entry %q → %q points outside the extraction root — refusing", hdr.Name, hdr.Linkname)
			}
			// In-tree links are intentionally NOT recreated; see the
			// function doc for why.
		default:
			// Char/block devices, FIFOs, etc. have no business in an
			// artifact tree; skip silently.
		}
	}
}

// stripTopLevel removes the tarball's single top-level directory
// component ("owner-repo-shortsha/..."). Entries without a slash are
// the top-level dir itself or global pax headers — not content.
func stripTopLevel(name string) (string, bool) {
	name = strings.TrimPrefix(name, "./")
	i := strings.IndexByte(name, '/')
	if i < 0 {
		return "", false
	}
	rest := name[i+1:]
	if rest == "" {
		return "", false
	}
	return rest, true
}

// linkEscapes reports whether a link at entry path rel (already
// cleaned, slash-separated, relative) pointing at linkname resolves
// outside the extraction root.
func linkEscapes(rel, linkname string) bool {
	if linkname == "" {
		return true
	}
	if path.IsAbs(linkname) || filepath.IsAbs(linkname) {
		return true
	}
	joined := path.Clean(path.Join(path.Dir(rel), linkname))
	return joined == ".." || strings.HasPrefix(joined, "../")
}
