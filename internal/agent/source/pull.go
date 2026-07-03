package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// pull.go orchestrates SPEC-003 §Pull command and §Update command:
// resolve each manifest entry to a commit SHA, fetch the tarball
// (cache-first), stage + integrity-check the artifact in a temp dir,
// and only then move it into its bucket. The ordering matters — every
// validation (conflict rules, lock-hash comparison) happens against
// the staging copy so a failed pull can never leave a half-written
// artifact in .agents/.

// BucketInfo is the slice of the agent package's bucket registry that
// tree fanout needs: which directories under .agents/ hold artifacts
// and whether each artifact is a directory (skills) or a flat file.
// Passed in by the caller so this package never imports
// internal/agent (dependency direction: agent → source).
type BucketInfo struct {
	Dir            string
	DirPerArtifact bool
}

// Puller carries the per-invocation state for pull/update/list/
// bundle. Fields are plain values so tests construct one with a
// literal, mirroring the agent.App convention.
type Puller struct {
	// AgentsDir is the absolute path of the .agents/ tree being
	// operated on — project-scope or the resolved global root. All
	// destinations, the manifest, and the lock live under it.
	AgentsDir string

	Fetcher Fetcher

	// Buckets is the registered bucket set used by tree fanout. Tree
	// entries fan out to EVERY registered bucket (agents, plans,
	// specs, adrs, hooks included), not a hardcoded three — SPEC-004
	// grew the registry and pull must follow it.
	Buckets []BucketInfo

	Out io.Writer
	Err io.Writer

	// Now is injectable for deterministic fetched_at timestamps in
	// tests. Nil means time.Now.
	Now func() time.Time
}

func (p *Puller) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p *Puller) out() io.Writer {
	if p.Out != nil {
		return p.Out
	}
	return io.Discard
}

func (p *Puller) errOut() io.Writer {
	if p.Err != nil {
		return p.Err
	}
	return io.Discard
}

// PullOpts mirrors SPEC-003's PullOpts. Global scope is resolved by
// the caller (internal/agent picks AgentsDir), so it does not appear
// here — the orchestrator is scope-agnostic by design.
type PullOpts struct {
	Force   bool
	DryRun  bool
	Offline bool
	// Only restricts the operation to entries whose name or literal
	// entry string matches. Empty = all.
	Only []string
	// UpdateMode switches pull semantics to `update`: SHA-pinned
	// entries are skipped with a note, and moved tags are called out.
	UpdateMode bool
}

// ResultKind classifies one entry's outcome in a PullReport.
type ResultKind string

const (
	ResultAdded       ResultKind = "added"
	ResultUpdated     ResultKind = "updated"
	ResultCurrent     ResultKind = "current"
	ResultSkipped     ResultKind = "skipped"
	ResultFailed      ResultKind = "failed"
	ResultWouldAdd    ResultKind = "would-add"
	ResultWouldUpdate ResultKind = "would-update"
)

// EntryResult is one entry's outcome.
type EntryResult struct {
	Entry  string     `json:"entry"`
	Name   string     `json:"name"`
	Kind   ResultKind `json:"result"`
	SHA    string     `json:"sha,omitempty"`
	Detail string     `json:"detail,omitempty"`
}

// PullReport enumerates per-entry results for both human and --json
// output.
type PullReport struct {
	Results []EntryResult `json:"results"`
}

// Count returns how many results have the given kind.
func (r PullReport) Count(kind ResultKind) int {
	n := 0
	for _, res := range r.Results {
		if res.Kind == kind {
			n++
		}
	}
	return n
}

// Changed reports whether any entry wrote to the destination tree —
// the signal for regenerating AGENTS.md afterwards (SPEC-003 Q4).
func (r PullReport) Changed() bool {
	return r.Count(ResultAdded)+r.Count(ResultUpdated) > 0
}

// Summary renders the one-line human tally ("1 added, 2 already
// current, 1 failed").
func (r PullReport) Summary() string {
	parts := []string{}
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add(r.Count(ResultAdded), "added")
	add(r.Count(ResultUpdated), "updated")
	add(r.Count(ResultCurrent), "already current")
	add(r.Count(ResultWouldAdd), "would add")
	add(r.Count(ResultWouldUpdate), "would update")
	add(r.Count(ResultSkipped), "skipped")
	add(r.Count(ResultFailed), "failed")
	if len(parts) == 0 {
		return "nothing to do"
	}
	return strings.Join(parts, ", ")
}

// Pull runs the pull/update pipeline over the manifest. It returns a
// report even alongside an error so callers can render partial
// progress; the error is non-nil when any entry failed.
func (p *Puller) Pull(ctx context.Context, opts PullOpts) (PullReport, error) {
	var report PullReport

	m, exists, err := LoadManifest(p.AgentsDir)
	if err != nil {
		return report, err
	}
	if !exists {
		return report, fmt.Errorf("no %s found — declare sources with `sync-agents source add <entry>` first", ManifestPath(p.AgentsDir))
	}
	lock, err := LoadLock(p.AgentsDir)
	if err != nil {
		return report, err
	}

	lockDirty := false
	for _, raw := range m.Sources {
		if !selected(raw, opts.Only) {
			continue
		}
		res, dirty := p.pullOne(ctx, raw, m, &lock, opts)
		lockDirty = lockDirty || dirty
		report.Results = append(report.Results, res)
	}

	if lockDirty && !opts.DryRun {
		if err := SaveLock(p.AgentsDir, lock); err != nil {
			return report, err
		}
	}
	if n := report.Count(ResultFailed); n > 0 {
		return report, fmt.Errorf("%d source entr%s failed", n, plural(n, "y", "ies"))
	}
	return report, nil
}

// selected implements --only / `update NAME` matching against either
// the artifact name or the literal entry string.
func selected(raw string, only []string) bool {
	if len(only) == 0 {
		return true
	}
	name := raw
	if e, err := ParseEntry(raw); err == nil {
		name = e.Name()
	}
	for _, o := range only {
		if o == raw || o == name {
			return true
		}
	}
	return false
}

// pullOne processes a single manifest entry and returns its result
// plus whether the lock was mutated. It never returns an error —
// failures become ResultFailed with the reason in Detail, so one bad
// entry doesn't stop the rest of the manifest.
func (p *Puller) pullOne(ctx context.Context, raw string, m Manifest, lock *Lock, opts PullOpts) (EntryResult, bool) {
	e, err := ParseEntry(raw)
	if err != nil {
		return EntryResult{Entry: raw, Name: raw, Kind: ResultFailed, Detail: err.Error()}, false
	}
	name, pinSHA := applyOverrides(e, m)
	res := EntryResult{Entry: raw, Name: name}

	// Update semantics: a SHA-pinned entry can never advance, so skip
	// it before any network traffic (AC-3's "no fetch" guarantee).
	if opts.UpdateMode && (e.PinnedSHA() || pinSHA != "") {
		res.Kind = ResultSkipped
		res.Detail = "[skip] SHA-pinned"
		return res, false
	}

	// Resolve the ref to a commit SHA. Precedence: override pin >
	// inline pinned SHA > lock (offline) > GitHub API.
	le := lock.Find(raw)
	var sha string
	switch {
	case pinSHA != "":
		sha = strings.ToLower(pinSHA)
	case e.PinnedSHA():
		sha = strings.ToLower(e.Ref)
	case opts.Offline:
		if le == nil {
			res.Kind = ResultFailed
			res.Detail = "--offline: entry has never been pulled (no lock record) and its ref cannot be resolved without the network"
			return res, false
		}
		sha = le.ResolvedSHA
	default:
		sha, err = p.Fetcher.ResolveRef(ctx, e.Owner, e.Repo, e.Ref)
		if err != nil {
			res.Kind = ResultFailed
			res.Detail = err.Error()
			return res, false
		}
	}
	res.SHA = sha

	movedTag := ""
	if opts.UpdateMode && le != nil && le.ResolvedSHA != sha {
		movedTag = fmt.Sprintf("ref %q moved: %s → %s", refOrDefault(e.Ref), shortSHA(le.ResolvedSHA), shortSHA(sha))
	}

	if e.Type == EntryTree {
		res = p.syncTree(ctx, e, sha, res, le, lock, opts)
	} else {
		res = p.syncArtifact(ctx, e, name, sha, res, le, lock, opts)
	}
	if movedTag != "" && (res.Kind == ResultUpdated || res.Kind == ResultWouldUpdate) {
		res.Detail = strings.TrimSpace(movedTag + "; " + res.Detail)
	}
	dirty := res.Kind == ResultAdded || res.Kind == ResultUpdated
	return res, dirty
}

func refOrDefault(ref string) string {
	if ref == "" {
		return "HEAD"
	}
	return ref
}

// destState classifies the destination ahead of any fetch, per
// SPEC-003 §Pull command conflict handling.
type destState int

const (
	destAbsent   destState = iota // no destination → write
	destCurrent                   // origin matches SHA, content intact → no-op
	destStale                     // origin intact but older SHA → update
	destModified                  // origin present but content edited locally
	destForeign                   // destination exists without origin metadata
)

// classifyDest inspects one destination path against the resolved
// SHA. Content hashing happens here (before fetch) so the idempotent
// path — everything current — makes zero network calls and zero
// writes.
func classifyDest(dest string, dirArtifact bool, sha string) (destState, Origin, error) {
	if _, err := os.Lstat(dest); os.IsNotExist(err) {
		return destAbsent, Origin{}, nil
	} else if err != nil {
		return destAbsent, Origin{}, err
	}
	o, err := ReadOriginFor(dest, dirArtifact)
	if os.IsNotExist(err) {
		return destForeign, Origin{}, nil
	}
	if err != nil {
		return destForeign, Origin{}, err
	}
	localHash, err := HashTree(dest)
	if err != nil {
		return destForeign, o, err
	}
	if localHash != o.ContentHash {
		return destModified, o, nil
	}
	if o.SHA == sha {
		return destCurrent, o, nil
	}
	return destStale, o, nil
}

// localModDetail builds the failure message for a locally-modified
// artifact. The spec asks for a diff against upstream at the recorded
// SHA; v1 gives the hash pair plus recovery commands instead of a
// full unified diff — the actionable part (what to run next) is the
// same, without re-downloading upstream content inside an error path.
func localModDetail(name, dest string, o Origin) string {
	localHash, _ := HashTree(dest)
	return fmt.Sprintf(
		"local modification detected: content hash %s differs from recorded %s (pulled from %s/%s@%s). Re-run with --force to overwrite, or `sync-agents source detach %s` to keep your edits as a manual artifact",
		localHash, o.ContentHash, o.Owner, o.Repo, shortSHA(o.SHA), name)
}

// bucketDirFor maps an entry type to its destination bucket.
func bucketDirFor(t EntryType) string {
	switch t {
	case EntrySkill:
		return "skills"
	case EntryRule:
		return "rules"
	case EntryWorkflow:
		return "workflows"
	}
	return ""
}

// artifactDest computes the local destination for a single-artifact
// entry, honoring an override rename.
func (p *Puller) artifactDest(e Entry, name string) (dest string, dirArtifact bool) {
	bucket := bucketDirFor(e.Type)
	if e.Type == EntrySkill {
		return filepath.Join(p.AgentsDir, bucket, name), true
	}
	ext := path.Ext(e.Path)
	if ext == "" {
		ext = ".md"
	}
	return filepath.Join(p.AgentsDir, bucket, name+ext), false
}

// syncArtifact handles skill/rule/workflow entries end to end.
func (p *Puller) syncArtifact(ctx context.Context, e Entry, name, sha string, res EntryResult, le *LockEntry, lock *Lock, opts PullOpts) EntryResult {
	dest, dirArtifact := p.artifactDest(e, name)

	state, origin, err := classifyDest(dest, dirArtifact, sha)
	if err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	switch state {
	case destCurrent:
		res.Kind = ResultCurrent
		return res
	case destModified:
		if !opts.Force {
			res.Kind = ResultFailed
			res.Detail = localModDetail(name, dest, origin)
			return res
		}
	case destForeign:
		if !opts.Force {
			res.Kind = ResultFailed
			res.Detail = fmt.Sprintf("%s exists but has no origin metadata (manually created?) — re-run with --force to overwrite it", dest)
			return res
		}
	}
	if opts.DryRun {
		res.Kind = ResultWouldUpdate
		if state == destAbsent {
			res.Kind = ResultWouldAdd
		}
		return res
	}

	staged, cleanup, err := p.fetchAndExtract(ctx, e.Owner, e.Repo, sha, opts.Offline)
	if err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	defer cleanup()

	src, effPath, err := locateArtifact(staged, e)
	if err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}

	// Integrity gate (AC-10): hash the STAGED content and compare to
	// the lock before anything touches the destination. A tampered
	// tarball or corrupted cache dies here with both hashes on
	// display.
	stagedHash, err := HashTree(src)
	if err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	if le != nil && le.ResolvedSHA == sha && le.ContentHash != stagedHash {
		res.Kind = ResultFailed
		res.Detail = integrityDetail(le.ContentHash, stagedHash, sha)
		return res
	}

	if err := installArtifact(src, dest, dirArtifact); err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	fetchedAt := p.now().Format(time.RFC3339)
	if err := WriteOriginFor(dest, dirArtifact, Origin{
		Owner: e.Owner, Repo: e.Repo, Path: effPath, Ref: e.Ref,
		SHA: sha, ContentHash: stagedHash, FetchedAt: fetchedAt, Source: SourceManifest,
	}); err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	lock.Set(LockEntry{Entry: res.Entry, ResolvedSHA: sha, ContentHash: stagedHash, FetchedAt: fetchedAt})

	res.Kind = ResultUpdated
	if state == destAbsent {
		res.Kind = ResultAdded
	}
	return res
}

// integrityDetail formats the AC-10 abort message citing both hashes.
func integrityDetail(recorded, extracted, sha string) string {
	return fmt.Sprintf("integrity mismatch for commit %s: lock records %s but the fetched tarball hashes to %s — refusing to write (corrupted cache or tampered tarball; clear the cache entry and re-pull)",
		shortSHA(sha), recorded, extracted)
}

// treeArtifact is one artifact discovered by tree fanout.
type treeArtifact struct {
	bucket      BucketInfo
	rel         string // path relative to the bucket dir (dir name or file rel path)
	src         string // absolute staged source
	dest        string // absolute destination
	repoPath    string // path inside the upstream repo, recorded in origin
	dirArtifact bool
}

// syncTree handles tree: entries — one tarball fetch fanned out to
// every registered bucket found in the upstream .agents/ (or repo
// root as a fallback for repos that ARE an agents tree).
//
// Writes are all-or-nothing per entry: every artifact's conflict
// state is checked first, and a single blocked artifact fails the
// whole entry before anything is written. Partial tree installs are
// worse than failed ones — they leave the project half-upgraded with
// no record of which half.
func (p *Puller) syncTree(ctx context.Context, e Entry, sha string, res EntryResult, le *LockEntry, lock *Lock, opts PullOpts) EntryResult {
	// Tree idempotency needs the artifact list, which lives in the
	// tarball — but at an unchanged SHA the tarball is in cache, so
	// this stays offline-friendly and write-free.
	staged, cleanup, err := p.fetchAndExtract(ctx, e.Owner, e.Repo, sha, opts.Offline)
	if err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	defer cleanup()

	base := staged
	repoPrefix := ""
	if subPath := e.Path; subPath != "" {
		base = filepath.Join(staged, filepath.FromSlash(subPath))
		repoPrefix = subPath + "/"
	}
	if fi, err := os.Stat(filepath.Join(base, ".agents")); err == nil && fi.IsDir() {
		base = filepath.Join(base, ".agents")
		repoPrefix += ".agents/"
	}

	arts, err := p.discoverTreeArtifacts(base, repoPrefix)
	if err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	if len(arts) == 0 {
		res.Kind = ResultFailed
		res.Detail = fmt.Sprintf("tree %s/%s@%s contains no bucket directories (%s)", e.Owner, e.Repo, shortSHA(sha), bucketDirList(p.Buckets))
		return res
	}

	// Phase 1: classify every artifact; collect blockers.
	states := make([]destState, len(arts))
	allCurrent := true
	anyAbsent := false
	var blockers []string
	for i, a := range arts {
		st, origin, err := classifyDest(a.dest, a.dirArtifact, sha)
		if err != nil {
			res.Kind = ResultFailed
			res.Detail = err.Error()
			return res
		}
		states[i] = st
		if st != destCurrent {
			allCurrent = false
		}
		if st == destAbsent {
			anyAbsent = true
		}
		if !opts.Force {
			switch st {
			case destModified:
				blockers = append(blockers, localModDetail(path.Base(a.rel), a.dest, origin))
			case destForeign:
				blockers = append(blockers, fmt.Sprintf("%s exists but has no origin metadata — re-run with --force to overwrite it", a.dest))
			}
		}
	}
	if allCurrent && le != nil && le.ResolvedSHA == sha {
		res.Kind = ResultCurrent
		return res
	}
	if len(blockers) > 0 {
		res.Kind = ResultFailed
		res.Detail = strings.Join(blockers, "; ")
		return res
	}
	if opts.DryRun {
		res.Kind = ResultWouldUpdate
		if anyAbsent && le == nil {
			res.Kind = ResultWouldAdd
		}
		return res
	}

	// Phase 2: integrity gate over the combined staged set. The
	// entry-level lock hash covers the assembled bucket layout, so it
	// is computed from a staging mirror of exactly what will be
	// installed.
	mirror, err := os.MkdirTemp("", "sync-agents-tree-*")
	if err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	defer os.RemoveAll(mirror)
	for _, a := range arts {
		mdest := filepath.Join(mirror, a.bucket.Dir, filepath.FromSlash(a.rel))
		if err := copyPath(a.src, mdest); err != nil {
			res.Kind = ResultFailed
			res.Detail = err.Error()
			return res
		}
	}
	stagedHash, err := HashTree(mirror)
	if err != nil {
		res.Kind = ResultFailed
		res.Detail = err.Error()
		return res
	}
	if le != nil && le.ResolvedSHA == sha && le.ContentHash != stagedHash {
		res.Kind = ResultFailed
		res.Detail = integrityDetail(le.ContentHash, stagedHash, sha)
		return res
	}

	// Phase 3: install. Per-artifact origins record the in-repo path
	// so bundle/list/promote can trace each file individually.
	fetchedAt := p.now().Format(time.RFC3339)
	for i, a := range arts {
		if states[i] == destCurrent {
			continue
		}
		if err := installArtifact(a.src, a.dest, a.dirArtifact); err != nil {
			res.Kind = ResultFailed
			res.Detail = err.Error()
			return res
		}
		artHash, err := HashTree(a.dest)
		if err != nil {
			res.Kind = ResultFailed
			res.Detail = err.Error()
			return res
		}
		if err := WriteOriginFor(a.dest, a.dirArtifact, Origin{
			Owner: e.Owner, Repo: e.Repo, Path: a.repoPath, Ref: e.Ref,
			SHA: sha, ContentHash: artHash, FetchedAt: fetchedAt, Source: SourceManifest,
		}); err != nil {
			res.Kind = ResultFailed
			res.Detail = err.Error()
			return res
		}
	}
	lock.Set(LockEntry{Entry: res.Entry, ResolvedSHA: sha, ContentHash: stagedHash, FetchedAt: fetchedAt})

	res.Kind = ResultUpdated
	if le == nil {
		res.Kind = ResultAdded
	}
	res.Detail = fmt.Sprintf("%d artifact(s)", len(arts))
	return res
}

// discoverTreeArtifacts walks the staged upstream tree for every
// registered bucket directory and enumerates installable artifacts:
// subdirectories for dir-per-artifact buckets (skills), files —
// recursively, preserving relative paths — for flat buckets (plans
// and ADRs nest by convention).
func (p *Puller) discoverTreeArtifacts(base, repoPrefix string) ([]treeArtifact, error) {
	var arts []treeArtifact
	for _, b := range p.Buckets {
		srcBucket := filepath.Join(base, b.Dir)
		fi, err := os.Stat(srcBucket)
		if err != nil || !fi.IsDir() {
			continue
		}
		if b.DirPerArtifact {
			entries, err := os.ReadDir(srcBucket)
			if err != nil {
				return nil, err
			}
			for _, de := range entries {
				if !de.IsDir() {
					continue
				}
				arts = append(arts, treeArtifact{
					bucket:      b,
					rel:         de.Name(),
					src:         filepath.Join(srcBucket, de.Name()),
					dest:        filepath.Join(p.AgentsDir, b.Dir, de.Name()),
					repoPath:    repoPrefix + b.Dir + "/" + de.Name(),
					dirArtifact: true,
				})
			}
			continue
		}
		err = filepath.WalkDir(srcBucket, func(fp string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if IsOriginFile(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(srcBucket, fp)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			arts = append(arts, treeArtifact{
				bucket:      b,
				rel:         relSlash,
				src:         fp,
				dest:        filepath.Join(p.AgentsDir, b.Dir, rel),
				repoPath:    repoPrefix + b.Dir + "/" + relSlash,
				dirArtifact: false,
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].dest < arts[j].dest })
	return arts, nil
}

func bucketDirList(buckets []BucketInfo) string {
	dirs := make([]string, len(buckets))
	for i, b := range buckets {
		dirs[i] = b.Dir
	}
	return strings.Join(dirs, ", ")
}

// offlineChecker is the optional Fetcher capability --offline relies
// on: "is this SHA already cached?". GitHubFetcher implements it;
// test fakes that are inherently local may skip it and are treated as
// always-available.
type offlineChecker interface {
	CachedSHA(owner, repo, sha string) bool
}

// fetchAndExtract downloads (or cache-loads) the repo tarball and
// extracts it into a fresh temp dir. The returned cleanup removes the
// temp tree; callers defer it. With offline set, a cache miss fails
// up front instead of letting the fetcher touch the network.
func (p *Puller) fetchAndExtract(ctx context.Context, owner, repo, sha string, offline bool) (string, func(), error) {
	if offline {
		if oc, ok := p.Fetcher.(offlineChecker); ok && !oc.CachedSHA(owner, repo, sha) {
			return "", nil, fmt.Errorf("--offline: %s/%s@%s is not in the local cache", owner, repo, shortSHA(sha))
		}
	}
	rc, fromCache, err := p.Fetcher.Fetch(ctx, owner, repo, sha)
	if err != nil {
		return "", nil, err
	}
	defer rc.Close()
	if fromCache {
		fmt.Fprintf(p.out(), "[info] cache hit: %s/%s@%s\n", owner, repo, shortSHA(sha))
	}
	dir, err := os.MkdirTemp("", "sync-agents-src-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	if err := ExtractTarball(rc, dir); err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

// locateArtifact finds the entry's source path inside the extracted
// tree, applying the type-specific path defaults from the entry
// grammar. It returns the absolute staged path and the effective
// in-repo path recorded in origin metadata.
func locateArtifact(staged string, e Entry) (string, string, error) {
	if e.Path != "" {
		p := filepath.Join(staged, filepath.FromSlash(e.Path))
		if _, err := os.Stat(p); err != nil {
			return "", "", fmt.Errorf("%s/%s: path %q not found in repo at the resolved commit", e.Owner, e.Repo, e.Path)
		}
		return p, e.Path, nil
	}
	// skill: default is the repo root when SKILL.md sits there, else
	// skills/<repo-name> if that top-level dir exists.
	if e.Type == EntrySkill {
		if _, err := os.Stat(filepath.Join(staged, "SKILL.md")); err == nil {
			return staged, "", nil
		}
		alt := filepath.Join(staged, "skills", e.Repo)
		if fi, err := os.Stat(alt); err == nil && fi.IsDir() {
			return alt, "skills/" + e.Repo, nil
		}
		return "", "", fmt.Errorf("%s/%s: no SKILL.md at repo root and no skills/%s directory — specify the skill path in the entry", e.Owner, e.Repo, e.Repo)
	}
	return staged, "", nil
}

// installArtifact moves staged content into place with a swap that is
// as atomic as the platform allows: the new content is fully
// materialized as a temp sibling of the destination first, then the
// old destination is removed and the sibling renamed in.
func installArtifact(src, dest string, dirArtifact bool) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if !dirArtifact {
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return writeFileAtomic(dest, data, 0o644)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), "."+filepath.Base(dest)+"-*")
	if err != nil {
		return err
	}
	if err := copyTree(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	// Preserve existing origin? No — the caller rewrites origin right
	// after install; a stale one inside tmp would be excluded from
	// hashing anyway.
	if err := os.RemoveAll(dest); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	return nil
}

// copyPath copies a file or directory tree to dst (creating parents).
func copyPath(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		return copyTree(src, dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// copyTree copies the contents of srcDir into dstDir (which must
// exist). Regular files only — the extractor already filtered
// everything else. Files land 0644, dirs 0755, matching extraction.
func copyTree(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// -------------------------------------------------------------------------
// List (SPEC-003 §Source list)
// -------------------------------------------------------------------------

// ListItem is one entry's local status for `source list`.
type ListItem struct {
	Entry       string `json:"entry"`
	Name        string `json:"name"`
	ResolvedSHA string `json:"resolved_sha,omitempty"`
	// Status ∈ ok | outdated | modified | missing (AC-5).
	Status string `json:"status"`
}

// List reports each manifest entry's local state without touching the
// network: [missing] no destination, [modified] content differs from
// the recorded hash, [outdated] destination's origin SHA lags the
// lock, [ok] otherwise. Keeping list offline means it's safe to run
// anywhere (CI, airplanes) and instant; `update --dry-run` is the
// network-consulting counterpart.
func (p *Puller) List() ([]ListItem, error) {
	m, _, err := LoadManifest(p.AgentsDir)
	if err != nil {
		return nil, err
	}
	lock, err := LoadLock(p.AgentsDir)
	if err != nil {
		return nil, err
	}

	var items []ListItem
	for _, raw := range m.Sources {
		e, err := ParseEntry(raw)
		if err != nil {
			items = append(items, ListItem{Entry: raw, Name: raw, Status: "missing"})
			continue
		}
		name, _ := applyOverrides(e, m)
		item := ListItem{Entry: raw, Name: name}
		if le := lock.Find(raw); le != nil {
			item.ResolvedSHA = le.ResolvedSHA
		}
		item.Status = p.entryStatus(e, name, item.ResolvedSHA)
		items = append(items, item)
	}
	return items, nil
}

func (p *Puller) entryStatus(e Entry, name, lockSHA string) string {
	if e.Type == EntryTree {
		return p.treeStatus(e, lockSHA)
	}
	dest, dirArtifact := p.artifactDest(e, name)
	st, _, err := classifyDest(dest, dirArtifact, lockSHA)
	if err != nil {
		return "modified"
	}
	switch st {
	case destAbsent:
		return "missing"
	case destModified, destForeign:
		return "modified"
	case destStale:
		return "outdated"
	default:
		return "ok"
	}
}

// treeStatus aggregates the state of every locally-installed artifact
// whose origin traces back to this tree's repo. It cannot know about
// upstream artifacts that were never installed (that requires the
// tarball), so a never-pulled tree reports missing via the lock.
func (p *Puller) treeStatus(e Entry, lockSHA string) string {
	if lockSHA == "" {
		return "missing"
	}
	found := false
	worst := "ok"
	for _, a := range p.scanInstalled() {
		if a.origin.Owner != e.Owner || a.origin.Repo != e.Repo {
			continue
		}
		found = true
		localHash, err := HashTree(a.dest)
		switch {
		case err != nil || localHash != a.origin.ContentHash:
			return "modified"
		case a.origin.SHA != lockSHA:
			worst = "outdated"
		}
	}
	if !found {
		return "missing"
	}
	return worst
}

// installedArtifact is one on-disk artifact with origin metadata,
// found by scanning the buckets (used by list, bundle, and remove).
type installedArtifact struct {
	bucket      BucketInfo
	dest        string
	dirArtifact bool
	origin      Origin
}

// scanInstalled walks every registered bucket for artifacts carrying
// origin metadata. Artifacts without origin are invisible here by
// design — they're valid but untracked (SPEC-003 §Per-artifact origin
// metadata).
func (p *Puller) scanInstalled() []installedArtifact {
	var out []installedArtifact
	for _, b := range p.Buckets {
		bucketDir := filepath.Join(p.AgentsDir, b.Dir)
		if b.DirPerArtifact {
			entries, err := os.ReadDir(bucketDir)
			if err != nil {
				continue
			}
			for _, de := range entries {
				if !de.IsDir() {
					continue
				}
				dest := filepath.Join(bucketDir, de.Name())
				if o, err := ReadOriginFor(dest, true); err == nil {
					out = append(out, installedArtifact{bucket: b, dest: dest, dirArtifact: true, origin: o})
				}
			}
			continue
		}
		filepath.WalkDir(bucketDir, func(fp string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || IsOriginFile(d.Name()) {
				return nil
			}
			if o, err := ReadOriginFor(fp, false); err == nil {
				out = append(out, installedArtifact{bucket: b, dest: fp, dirArtifact: false, origin: o})
			}
			return nil
		})
	}
	return out
}
