package source

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// link.go implements SPEC-007: linked (editable) sources — the
// npm-link / go-replace analogue for .agents artifacts. Instead of a
// SHA-pinned tarball snapshot, a linked source symlinks the artifact in
// .agents/ at a live local git checkout, so edits flow both ways and
// `git pull` in the checkout reaps upstream updates with no re-fetch.
//
// Everything here keeps persisted paths RELATIVE (the file: scheme,
// resolved against the .agents/ directory) and creates the on-disk
// symlink relative too — an absolute path in the git-tracked
// sources.lock breaks the instant another machine clones the repo.

// linkScheme is npm's file: convention. The literal after it is a path
// relative to the .agents/ directory (where sources.yaml lives).
const linkScheme = "file:"

// managedSourcesDir is the dot-dir under .agents/ that holds clones
// sync-agents owns for bare `--link <entry>`. Dot-prefixed so sync and
// the AGENTS.md index skip it, same convention as .quarantine.
const managedSourcesDir = ".sources"

// ParseLinkPath validates and normalizes a file: link value from a
// manifest override or the lock. It returns the cleaned RELATIVE path
// (scheme stripped). Absolute paths are a parse error — they do not
// round-trip through git — and the error names the fix.
func ParseLinkPath(raw string) (string, error) {
	if !strings.HasPrefix(raw, linkScheme) {
		return "", fmt.Errorf("link %q must use the file: scheme (e.g. file:../foo-skill)", raw)
	}
	rel := strings.TrimPrefix(raw, linkScheme)
	if rel == "" {
		return "", fmt.Errorf("link %q has an empty path", raw)
	}
	// Reject absolute before and after cleaning. A leading slash, or a
	// Windows volume path, cannot be shared across machines.
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("link path %q is absolute — use a relative file:../ path so the lock stays portable across machines", rel)
	}
	clean := path.Clean(rel)
	if path.IsAbs(clean) {
		return "", fmt.Errorf("link path %q resolves to an absolute path — use a relative file:../ path", rel)
	}
	return clean, nil
}

// Now implements the injectable clock for link lock records.
func (p *Puller) workDir() (string, error) {
	if p.WorkDir != "" {
		return p.WorkDir, nil
	}
	return os.Getwd()
}

// cloneURL returns the git URL to clone a managed source from. Tests
// override p.CloneURL to point at a local repo; production defaults to
// GitHub over HTTPS (gh/https auth is the fetcher's concern elsewhere).
func (p *Puller) cloneURL(owner, repo string) string {
	if p.CloneURL != nil {
		return p.CloneURL(owner, repo)
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
}

// linkArtifactTarget returns the absolute path the symlink should point
// at: the in-repo artifact inside the checkout root. Mirrors
// locateArtifact's defaults — a path-less skill is the checkout root
// itself (SKILL.md at repo root).
func linkArtifactTarget(checkoutRoot string, e Entry) string {
	if e.Path != "" {
		return filepath.Join(checkoutRoot, filepath.FromSlash(e.Path))
	}
	return checkoutRoot
}

// createRelSymlink wires dest → targetAbs as a RELATIVE symlink
// (computed from dest's parent), creating parent dirs first and
// replacing any existing symlink atomically-ish (remove + create).
func createRelSymlink(dest, targetAbs string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rel, err := filepath.Rel(filepath.Dir(dest), targetAbs)
	if err != nil {
		return err
	}
	// Remove a pre-existing symlink so os.Symlink doesn't EEXIST. A
	// real dir/file is the caller's responsibility to reject first.
	if fi, err := os.Lstat(dest); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(dest); err != nil {
			return err
		}
	}
	return os.Symlink(rel, dest)
}

// AddLink implements `source add --link` in its three forms (SPEC-007
// §Behavior). Exactly one of these shapes is expected:
//
//	linkPath!="" entry!=""  → link an existing checkout you own
//	linkPath==""  entry!="" → managed clone under .agents/.sources/
//	linkPath!="" entry==""  → derive owner/repo from the checkout's remote
//
// It writes the upstream identity to sources.yaml, a link override, the
// relative symlink, and the lock — then returns a one-entry report for
// uniform rendering.
func (p *Puller) AddLink(ctx context.Context, linkPath, entry string, opts PullOpts) (PullReport, error) {
	managed := linkPath == "" && entry != ""

	// Form 3: derive the entry from the checkout's git remote + layout.
	if linkPath != "" && entry == "" {
		derived, err := p.deriveEntryFromCheckout(linkPath)
		if err != nil {
			return PullReport{}, err
		}
		entry = derived
	}
	if entry == "" {
		return PullReport{}, fmt.Errorf("source add --link needs an entry (or a --link=<path> to a checkout with a github remote)")
	}

	e, err := ParseEntry(entry)
	if err != nil {
		return PullReport{}, err
	}
	if e.Type == EntryTree {
		return PullReport{}, fmt.Errorf("tree: entries cannot be linked — link a specific skill/rule/workflow instead")
	}

	m, _, err := LoadManifest(p.AgentsDir)
	if err != nil {
		return PullReport{}, err
	}

	// Resolve the checkout root and the relative file: path stored in
	// the override/lock, then the symlink target and destination.
	var linkRel string      // relative to .agents/, stored as file:<linkRel>
	var checkoutRoot string // absolute
	if managed {
		linkRel = path.Join(managedSourcesDir, e.Owner+"-"+e.Repo)
		checkoutRoot = filepath.Join(p.AgentsDir, filepath.FromSlash(linkRel))
	} else {
		abs, err := p.absCheckout(linkPath)
		if err != nil {
			return PullReport{}, err
		}
		checkoutRoot = abs
		rel, err := filepath.Rel(p.AgentsDir, abs)
		if err != nil {
			return PullReport{}, err
		}
		relSlash := filepath.ToSlash(rel)
		if filepath.IsAbs(relSlash) || strings.HasPrefix(relSlash, "/") {
			return PullReport{}, fmt.Errorf("checkout %q cannot be expressed relative to %s — link a checkout on the same filesystem", linkPath, p.AgentsDir)
		}
		linkRel = relSlash
	}

	name, _ := applyOverrides(e, m)
	dest, dirArtifact := p.artifactDest(e, name)
	res := EntryResult{Entry: e.Raw, Name: name}

	// Refuse to clobber a real (non-symlink) artifact.
	if fi, err := os.Lstat(dest); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 && !opts.Force {
			return PullReport{}, fmt.Errorf("%s already exists as a real %s — remove it or re-run with --force to replace it with a link",
				dest, artifactKind(dirArtifact))
		}
	}

	// Managed clone: fetch remote code, then scan once (SPEC-005).
	if managed {
		if err := p.cloneManaged(ctx, e, checkoutRoot); err != nil {
			return PullReport{}, err
		}
		target := linkArtifactTarget(checkoutRoot, e)
		if _, err := os.Stat(target); err != nil {
			os.RemoveAll(checkoutRoot)
			return PullReport{}, fmt.Errorf("cloned %s/%s but the artifact path %q is not present in it", e.Owner, e.Repo, refArtifactPath(e))
		}
		findings := ScanTree(target)
		if p.Quarantine && !opts.Trust && len(findings) > 0 {
			p.printFindings(name, findings)
			os.RemoveAll(checkoutRoot)
			return PullReport{}, fmt.Errorf("managed clone %s/%s has %d scan finding(s) — refusing to link; review the source and re-run with --trust to link anyway", e.Owner, e.Repo, len(findings))
		}
		p.printFindings(name, findings)
	} else {
		// User checkout: trusted by default, no scan. Just confirm the
		// artifact is actually there.
		target := linkArtifactTarget(checkoutRoot, e)
		if _, err := os.Stat(target); err != nil {
			return PullReport{}, fmt.Errorf("checkout %s has no artifact at %q (expected the %s there)", checkoutRoot, refArtifactPath(e), e.Type)
		}
	}

	if opts.DryRun {
		res.Kind = ResultWouldAdd
		return PullReport{Results: []EntryResult{res}}, nil
	}

	// Wire the symlink.
	if err := createRelSymlink(dest, linkArtifactTarget(checkoutRoot, e)); err != nil {
		return PullReport{}, err
	}

	// Persist: sources entry (identity), link override, lock record.
	if !containsString(m.Sources, e.Raw) {
		m.Sources = append(m.Sources, e.Raw)
	}
	m.Overrides = upsertLinkOverride(m.Overrides, e, linkScheme+linkRel)
	if err := SaveManifest(p.AgentsDir, m); err != nil {
		return PullReport{}, err
	}

	sha, _ := gitHEAD(checkoutRoot) // informational; empty if not a git repo
	lock, err := LoadLock(p.AgentsDir)
	if err != nil {
		return PullReport{}, err
	}
	lock.Set(LockEntry{
		Entry:        e.Raw,
		ResolvedSHA:  sha,
		ContentHash:  "",
		Link:         linkScheme + linkRel,
		ManagedClone: managed,
		FetchedAt:    p.now().Format(time.RFC3339),
	})
	if err := SaveLock(p.AgentsDir, lock); err != nil {
		return PullReport{}, err
	}

	res.Kind = ResultAdded
	res.SHA = sha
	res.Detail = "link → " + linkScheme + linkRel
	if managed {
		res.Detail += " (managed clone)"
	}
	return PullReport{Results: []EntryResult{res}}, nil
}

// absCheckout resolves a user-supplied --link path (relative to the
// working directory, or absolute) to an absolute, symlink-cleaned path.
func (p *Puller) absCheckout(linkPath string) (string, error) {
	abs := linkPath
	if !filepath.IsAbs(abs) {
		wd, err := p.workDir()
		if err != nil {
			return "", err
		}
		abs = filepath.Join(wd, linkPath)
	}
	abs = filepath.Clean(abs)
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("checkout path %q not found: %w", linkPath, err)
	}
	return abs, nil
}

// cloneManaged clones the upstream repo into checkoutRoot (a managed
// dir under .agents/.sources/). A pre-existing clone is reused.
func (p *Puller) cloneManaged(ctx context.Context, e Entry, checkoutRoot string) error {
	if fi, err := os.Stat(filepath.Join(checkoutRoot, ".git")); err == nil && fi.IsDir() {
		return nil // already cloned
	}
	if err := os.MkdirAll(filepath.Dir(checkoutRoot), 0o755); err != nil {
		return err
	}
	url := p.cloneURL(e.Owner, e.Repo)
	args := []string{"clone", url, checkoutRoot}
	if e.Ref != "" {
		args = []string{"clone", "--branch", e.Ref, url, checkoutRoot}
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(checkoutRoot)
		return fmt.Errorf("git clone %s failed: %s", url, strings.TrimSpace(string(out)))
	}
	return nil
}

// pullLinked handles a linked entry during pull/update: it skips the
// HTTP fetcher entirely. On pull it verifies (and self-heals) the
// symlink; on update it advances a managed clone with git pull --ff-only
// and refreshes the informational resolved_sha. It never re-fetches a
// remote snapshot — a broken dev checkout surfaces as a warning, not a
// silent revert.
func (p *Puller) pullLinked(e Entry, name, linkRel string, le *LockEntry, lock *Lock, opts PullOpts) (EntryResult, bool) {
	res := EntryResult{Entry: e.Raw, Name: name}
	dest, dirArtifact := p.artifactDest(e, name)
	managed := le != nil && le.ManagedClone
	checkoutRoot := filepath.Join(p.AgentsDir, filepath.FromSlash(linkRel))
	target := linkArtifactTarget(checkoutRoot, e)

	// Update mode: advance a managed clone. A user checkout is the
	// operator's to drive — we only refresh the recorded SHA below.
	if opts.UpdateMode && managed && !opts.DryRun {
		if out, err := gitPullFFOnly(checkoutRoot); err != nil {
			res.Kind = ResultFailed
			res.Detail = fmt.Sprintf("git pull --ff-only in managed clone failed: %s", strings.TrimSpace(out))
			return res, false
		}
	}

	// Verify / establish the symlink.
	fi, lerr := os.Lstat(dest)
	created := false
	switch {
	case os.IsNotExist(lerr):
		if opts.DryRun {
			res.Kind = ResultWouldAdd
			return res, false
		}
		if _, err := os.Stat(target); err != nil {
			res.Kind = ResultFailed
			res.Detail = fmt.Sprintf("linked checkout %s is missing the artifact at %q — clone/restore it, then re-run", checkoutRoot, refArtifactPath(e))
			return res, false
		}
		if err := createRelSymlink(dest, target); err != nil {
			res.Kind = ResultFailed
			res.Detail = err.Error()
			return res, false
		}
		created = true
		res.Kind = ResultAdded
	case lerr != nil:
		res.Kind = ResultFailed
		res.Detail = lerr.Error()
		return res, false
	case fi.Mode()&os.ModeSymlink == 0:
		res.Kind = ResultFailed
		res.Detail = fmt.Sprintf("%s exists but is not a symlink — a linked source expects a symlink here; remove the real %s or `source detach` it",
			dest, artifactKind(dirArtifact))
		return res, false
	default:
		// Symlink present: confirm it resolves; a dangling target is a
		// warning, never a silent re-fetch.
		if _, serr := os.Stat(dest); serr != nil {
			fmt.Fprintf(p.errOut(), "[warn] linked source %s: target %s is missing (checkout moved or deleted) — leaving the dangling link, not re-fetching\n", name, target)
			res.Kind = ResultCurrent
			res.Detail = "dangling link — target missing"
		} else {
			res.Kind = ResultCurrent
		}
	}

	// Refresh the informational SHA (best-effort) and decide whether the
	// lock needs rewriting. Plain pull is a no-op unless it had to
	// establish the link or record it for the first time; update rewrites
	// when HEAD advanced.
	sha := ""
	if s, err := gitHEAD(checkoutRoot); err == nil {
		sha = s
	} else if le != nil {
		sha = le.ResolvedSHA
	}
	res.SHA = sha

	needLock := le == nil || created
	if opts.UpdateMode && le != nil && sha != "" && sha != le.ResolvedSHA {
		needLock = true
		if res.Kind == ResultCurrent {
			res.Kind = ResultUpdated
			res.Detail = fmt.Sprintf("checkout advanced to %s", shortSHA(sha))
		}
	}
	if needLock && !opts.DryRun {
		lock.Set(LockEntry{
			Entry:        e.Raw,
			ResolvedSHA:  sha,
			ContentHash:  "",
			Link:         linkScheme + linkRel,
			ManagedClone: managed,
			FetchedAt:    p.now().Format(time.RFC3339),
		})
	}
	return res, needLock
}

// deriveEntryFromCheckout reads a checkout's git remote and layout to
// synthesize a source entry (SPEC-007 form 3). v1 infers skill: from a
// SKILL.md at the repo root; anything else asks for an explicit entry.
func (p *Puller) deriveEntryFromCheckout(linkPath string) (string, error) {
	abs, err := p.absCheckout(linkPath)
	if err != nil {
		return "", err
	}
	url, err := gitRemoteURL(abs)
	if err != nil {
		return "", fmt.Errorf("cannot derive owner/repo from %s: %w (pass an explicit entry instead)", linkPath, err)
	}
	owner, repo, err := parseGitHubRemote(url)
	if err != nil {
		return "", fmt.Errorf("%s: %w — pass an explicit entry (e.g. skill:%s)", linkPath, err, filepath.Base(abs))
	}
	if _, err := os.Stat(filepath.Join(abs, "SKILL.md")); err == nil {
		return fmt.Sprintf("skill:%s/%s", owner, repo), nil
	}
	return "", fmt.Errorf("cannot infer artifact type for %s/%s (no SKILL.md at the checkout root) — pass an explicit entry like rule:%s/%s@main/rules/name.md", owner, repo, owner, repo)
}

// upsertLinkOverride adds or replaces the link override for an entry.
// The match glob is anchored to the entry string with a trailing * so a
// versioned/pathful entry still matches (mirrors the spec example).
func upsertLinkOverride(overrides []Override, e Entry, link string) []Override {
	match := e.Raw
	for i := range overrides {
		if overrides[i].Match == match || overrides[i].Match == match+"*" {
			overrides[i].Link = link
			overrides[i].PinSHA = "" // link and pin_sha are exclusive
			return overrides
		}
	}
	return append(overrides, Override{Match: match, Link: link})
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// artifactKind labels a destination for error messages.
func artifactKind(dirArtifact bool) string {
	if dirArtifact {
		return "directory"
	}
	return "file"
}

// refArtifactPath describes the in-repo artifact path for messages.
func refArtifactPath(e Entry) string {
	if e.Path != "" {
		return e.Path
	}
	if e.Type == EntrySkill {
		return "SKILL.md (repo root)"
	}
	return "(repo root)"
}

// -------------------------------------------------------------------------
// git helpers (SPEC-007 uses git only for managed clones + provenance)
// -------------------------------------------------------------------------

func gitHEAD(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitRemoteURL(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return "", fmt.Errorf("no origin remote")
	}
	url := strings.TrimSpace(string(out))
	if url == "" {
		return "", fmt.Errorf("origin remote is empty")
	}
	return url, nil
}

func gitPullFFOnly(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "pull", "--ff-only").CombinedOutput()
	return string(out), err
}

var githubRemoteRe = regexp.MustCompile(`github\.com[:/]+([^/]+)/([^/]+?)(?:\.git)?/?$`)

// parseGitHubRemote extracts owner/repo from a github.com remote URL in
// any of git's common forms (https, ssh scp-style, ssh://).
func parseGitHubRemote(url string) (owner, repo string, err error) {
	m := githubRemoteRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", "", fmt.Errorf("remote %q is not a github.com URL", url)
	}
	return m[1], m[2], nil
}
