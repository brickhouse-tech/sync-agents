package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/brickhouse-tech/sync-agents/internal/agent/source"
	"gopkg.in/yaml.v3"
)

// integrity.go implements SPEC-008: the full-context integrity lock.
//
// Two machine-written, git-committed files sit at the .agents/ root
// and together answer, for every artifact in every bucket, the two
// supply-chain questions:
//
//   - agents.lock (provenance, YAML) — where did each artifact come
//     from? One entry per artifact: path, bucket, name, origin
//     (local / source:<entry> / linked:<file:path> / imported:<url>),
//     resolved SHA where one exists, and the SPEC-003 tree hash.
//   - agents.sum (integrity, go.sum-style text) — is it still exactly
//     what we locked? One line per file, "<path> sha256:<hex>", plus a
//     "<path> link:<target>" line for each symlink so a re-pointed
//     link is caught even when target contents match.
//
// SPEC-003's sources.lock covers only remotely-sourced manifest
// entries; SPEC-008 closes the gap for locally-authored rules,
// hand-written skills, imports, linked checkouts, hooks, agents,
// plans, specs and ADRs, so a single `verify` proves the whole tree
// checks out. See specs/SPEC-008-context-integrity-lock.md.

const (
	// AgentsLockFileName is the provenance record at the .agents/ root.
	AgentsLockFileName = "agents.lock"
	// AgentsSumFileName is the go.sum-style integrity record.
	AgentsSumFileName = "agents.sum"
	// integritySchemaVersion is the agents.lock schema this build
	// writes and understands.
	integritySchemaVersion = 1
)

// integrityNow returns lock-generation time; overridable in tests so
// the generated_at field is deterministic.
var integrityNow = func() time.Time { return time.Now().UTC() }

// lockedArtifact is one entry in agents.lock. Field order controls
// YAML emission order, which keeps regeneration diffs stable.
type lockedArtifact struct {
	Path        string `yaml:"path"`
	Bucket      string `yaml:"bucket"`
	Name        string `yaml:"name"`
	Origin      string `yaml:"origin"`
	ResolvedSHA string `yaml:"resolved_sha,omitempty"`
	TreeHash    string `yaml:"tree_hash"`
}

// agentsLock is the parsed agents.lock document.
type agentsLock struct {
	Version     int              `yaml:"version"`
	GeneratedAt string           `yaml:"generated_at"`
	Artifacts   []lockedArtifact `yaml:"artifacts"`
}

// sumLine is one agents.sum record: a file path (relative to the
// .agents/ dir, forward slashes) and its value — "sha256:<hex>" for a
// regular file or "link:<relative-target>" for a symlink.
type sumLine struct {
	Path  string
	Value string
}

// integrityState is the whole-tree snapshot both `lock` and `verify`
// compute from disk, so the two commands can never disagree about what
// the tree contains.
type integrityState struct {
	// artifacts, sorted lexicographically by Path.
	artifacts []lockedArtifact
	// sums covers every tree file plus the root inputs
	// (sources.yaml/sources.lock/config); it excludes agents.lock and
	// agents.sum (agents.lock is summed separately, agents.sum is the
	// trust root and cannot contain itself). Sorted by Path.
	sums []sumLine
	// fileOrigin maps a sum Path to the origin of its owning artifact,
	// or "" for root inputs — used to pick a drift severity.
	fileOrigin map[string]string
	// artFiles maps an artifact Path to its per-file sum lines, for
	// `verify --explain`.
	artFiles map[string][]sumLine
}

// AgentsLockPath returns .agents/agents.lock for the given agents dir.
func AgentsLockPath(agentsDir string) string {
	return filepath.Join(agentsDir, AgentsLockFileName)
}

// AgentsSumPath returns .agents/agents.sum for the given agents dir.
func AgentsSumPath(agentsDir string) string {
	return filepath.Join(agentsDir, AgentsSumFileName)
}

// ExitError carries a specific process exit code up to main. `verify`
// distinguishes drift (1) from usage/IO failure (2); everything else
// exits 1 by default.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// ExitCode reports the process exit code main should use.
func (e *ExitError) ExitCode() int { return e.Code }

// ---------------------------------------------------------------------
// State computation (shared by lock + verify)
// ---------------------------------------------------------------------

// computeIntegrityState walks the .agents/ tree per SPEC-008 Coverage
// rules and returns the artifacts + per-file sums. It never filters by
// host OS — the lock is a content record, not a routing record, so a
// teammate on another platform can still verify the files they don't
// sync (SPEC-008 §Coverage).
func computeIntegrityState(agentsDir string) (integrityState, error) {
	st := integrityState{
		fileOrigin: map[string]string{},
		artFiles:   map[string][]sumLine{},
	}
	srcLock, _ := source.LoadLock(agentsDir) // best-effort; empty on absence

	for _, b := range Buckets {
		bucketDir := filepath.Join(agentsDir, b.Dir)
		entries, err := os.ReadDir(bucketDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return integrityState{}, err
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") || source.IsOriginFile(name) {
				continue
			}
			// OS-scoped subdir (SPEC-006): locked regardless of host OS,
			// artifacts keep the subdir as a name prefix ("macos/brew").
			if e.IsDir() && isOSScopeDir(name) {
				sub, err := os.ReadDir(filepath.Join(bucketDir, name))
				if err != nil {
					return integrityState{}, err
				}
				for _, se := range sub {
					if strings.HasPrefix(se.Name(), ".") || source.IsOriginFile(se.Name()) {
						continue
					}
					if err := st.addArtifact(agentsDir, b, filepath.Join(bucketDir, name), se, name+"/", srcLock); err != nil {
						return integrityState{}, err
					}
				}
				continue
			}
			if err := st.addArtifact(agentsDir, b, bucketDir, e, "", srcLock); err != nil {
				return integrityState{}, err
			}
		}
	}

	// Root inputs: hashed but not artifacts (SPEC-008 §Coverage). config
	// toggles quarantine, so it is security-relevant and summed too.
	for _, root := range []string{source.ManifestFileName, source.LockFileName, "config"} {
		full := filepath.Join(agentsDir, root)
		if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
			sum, err := sha256File(full)
			if err != nil {
				return integrityState{}, err
			}
			st.sums = append(st.sums, sumLine{Path: root, Value: sum})
			st.fileOrigin[root] = ""
		}
	}

	sort.Slice(st.artifacts, func(i, j int) bool { return st.artifacts[i].Path < st.artifacts[j].Path })
	sort.Slice(st.sums, func(i, j int) bool { return st.sums[i].Path < st.sums[j].Path })
	return st, nil
}

// addArtifact resolves one directory entry into a lockedArtifact plus
// its per-file sum lines and appends them to the state. ok-less: stray
// files (skill dir without SKILL.md, wrong extension) are silently
// skipped, matching DiscoverArtifacts.
func (st *integrityState) addArtifact(agentsDir string, b Bucket, dir string, e os.DirEntry, prefix string, srcLock source.Lock) error {
	abs := filepath.Join(dir, e.Name())
	rel, err := filepath.Rel(agentsDir, abs)
	if err != nil {
		return err
	}
	relSlash := filepath.ToSlash(rel)

	if b.DirPerArtifact {
		if !entryIsDir(abs, e) {
			return nil
		}
		if _, err := os.Stat(filepath.Join(abs, "SKILL.md")); err != nil {
			return nil
		}
	} else {
		if entryIsDir(abs, e) || !strings.HasSuffix(e.Name(), b.FileExt()) {
			return nil
		}
	}

	// STATE snapshots are per-engineer scratch unless they opt in with
	// `shared: true` frontmatter (same convention as the AGENTS.md
	// indexer). Applies to flat .md artifacts.
	if !b.DirPerArtifact && strings.HasPrefix(e.Name(), "STATE_") && strings.HasSuffix(e.Name(), ".md") {
		if !stateSnapshotIsShared(abs) {
			return nil
		}
	}

	baseName := e.Name()
	if !b.DirPerArtifact {
		baseName = strings.TrimSuffix(baseName, b.FileExt())
	}
	la := lockedArtifact{
		Path:   relSlash,
		Bucket: b.Dir,
		Name:   prefix + baseName,
	}

	// Origin + tree hash + files.
	isLink := e.Type()&fs.ModeSymlink != 0
	var files []sumLine
	if isLink {
		target, terr := os.Readlink(abs)
		la.Origin = "linked:file:" + filepath.ToSlash(target)
		// A repointed link must be detectable even when contents match:
		// the symlink itself contributes a link: line.
		files = append(files, sumLine{Path: relSlash, Value: "link:" + filepath.ToSlash(target)})
		resolved, rerr := filepath.EvalSymlinks(abs)
		if terr == nil && rerr == nil {
			th, herr := source.HashTree(resolved)
			if herr != nil {
				return herr
			}
			la.TreeHash = th
			fl, ferr := fileSums(agentsDir, resolved, relSlash)
			if ferr != nil {
				return ferr
			}
			files = append(files, fl...)
		}
	} else {
		la.Origin = resolveNonLinkOrigin(abs, b, &la)
		th, herr := source.HashTree(abs)
		if herr != nil {
			return herr
		}
		la.TreeHash = th
		fl, ferr := fileSums(agentsDir, abs, relSlash)
		if ferr != nil {
			return ferr
		}
		files = append(files, fl...)
	}

	st.artifacts = append(st.artifacts, la)
	st.artFiles[relSlash] = files
	for _, f := range files {
		st.sums = append(st.sums, f)
		st.fileOrigin[f.Path] = la.Origin
	}
	return nil
}

// resolveNonLinkOrigin classifies a non-symlink artifact by reading its
// SPEC-003 _origin.json (manifest → source:, manual → imported:),
// falling back to local. It also fills resolved_sha on la.
func resolveNonLinkOrigin(abs string, b Bucket, la *lockedArtifact) string {
	o, err := source.ReadOriginFor(abs, b.DirPerArtifact)
	if err != nil {
		return "local"
	}
	la.ResolvedSHA = o.SHA
	switch o.Source {
	case source.SourceManifest:
		return "source:" + reconstructEntry(b, o)
	case source.SourceManual:
		return "imported:" + reconstructBlobURL(o)
	default:
		return "local"
	}
}

// reconstructEntry rebuilds the SPEC-003 entry string an artifact was
// pulled from, from its origin metadata: <type>:<owner>/<repo>[@ref][/path].
func reconstructEntry(b Bucket, o source.Origin) string {
	typ := entryPrefixForBucket(b)
	s := typ + ":" + o.Owner + "/" + o.Repo
	if o.Ref != "" {
		s += "@" + o.Ref
	}
	if o.Path != "" {
		s += "/" + o.Path
	}
	return s
}

// reconstructBlobURL rebuilds a github blob URL from origin metadata,
// used for imported (manual) artifacts.
func reconstructBlobURL(o source.Origin) string {
	ref := o.Ref
	if ref == "" {
		ref = "HEAD"
	}
	return fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", o.Owner, o.Repo, ref, o.Path)
}

// entryPrefixForBucket maps a bucket to the SPEC-003 entry type prefix.
// Only rules/skills/workflows are pullable; other buckets never carry a
// source: origin, so their prefix is informational only.
func entryPrefixForBucket(b Bucket) string {
	switch b.Dir {
	case "rules":
		return "rule"
	case "skills":
		return "skill"
	case "workflows":
		return "workflow"
	default:
		return string(b.Artifact)
	}
}

// fileSums returns the sha256 sum lines for every file under root
// (a single file or an artifact directory), keyed by relBase (the
// artifact's path relative to the .agents/ dir). Origin files and
// nested symlinks are excluded, matching HashTree.
func fileSums(agentsDir, root, relBase string) ([]sumLine, error) {
	fi, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		sum, err := sha256File(root)
		if err != nil {
			return nil, err
		}
		return []sumLine{{Path: relBase, Value: sum}}, nil
	}
	var out []sumLine
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || source.IsOriginFile(d.Name()) {
			return nil
		}
		relIn, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		sum, err := sha256File(p)
		if err != nil {
			return err
		}
		out = append(out, sumLine{Path: relBase + "/" + filepath.ToSlash(relIn), Value: sum})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// sha256File returns "sha256:<hex>" over a file's raw bytes.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

// ---------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------

// renderSum renders sum lines to the go.sum-style text body: one
// "<path> <value>\n" per line, path-sorted, LF-joined.
func renderSum(lines []sumLine) []byte {
	sorted := append([]sumLine(nil), lines...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var b strings.Builder
	for _, l := range sorted {
		b.WriteString(l.Path)
		b.WriteByte(' ')
		b.WriteString(l.Value)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// ---------------------------------------------------------------------
// lock command
// ---------------------------------------------------------------------

// CmdLock implements `sync-agents lock`: generate/refresh agents.lock +
// agents.sum. Idempotent — an unchanged tree yields byte-identical
// files, and generated_at is only advanced when some other byte moved.
func (a *App) CmdLock(opts SourceCmdOpts) error {
	agentsDir := a.sourceAgentsDir(opts.Global)
	st, err := computeIntegrityState(agentsDir)
	if err != nil {
		a.Error(err.Error())
		return &ExitError{Code: 2, Err: err}
	}

	// Preserve generated_at when nothing else changed (keeps no-op runs
	// diff-free).
	generatedAt := integrityNow().Format(time.RFC3339)
	if existing, err := loadAgentsLock(agentsDir); err == nil {
		if artifactsEqual(existing.Artifacts, st.artifacts) {
			generatedAt = existing.GeneratedAt
		}
	}
	lock := agentsLock{Version: integritySchemaVersion, GeneratedAt: generatedAt, Artifacts: st.artifacts}
	lockBytes, err := yaml.Marshal(lock)
	if err != nil {
		return &ExitError{Code: 2, Err: err}
	}

	// The sum covers agents.lock itself (written first, then hashed into
	// the sum) — the sum is the single root of trust.
	sums := append([]sumLine(nil), st.sums...)
	sums = append(sums, sumLine{Path: AgentsLockFileName, Value: sha256Bytes(lockBytes)})
	sumBytes := renderSum(sums)

	if a.DryRun {
		a.reportLockDelta(agentsDir, st)
		return nil
	}
	if err := writeFileAtomicAgent(AgentsLockPath(agentsDir), lockBytes); err != nil {
		a.Error(err.Error())
		return &ExitError{Code: 2, Err: err}
	}
	if err := writeFileAtomicAgent(AgentsSumPath(agentsDir), sumBytes); err != nil {
		a.Error(err.Error())
		return &ExitError{Code: 2, Err: err}
	}
	if opts.JSON {
		byOrigin := map[string]int{"source": 0, "linked": 0, "imported": 0, "local": 0}
		for _, la := range st.artifacts {
			byOrigin[originClass(la.Origin)]++
		}
		data, _ := json.MarshalIndent(map[string]any{
			"artifacts": len(st.artifacts),
			"files":     len(sums),
			"origins":   byOrigin,
		}, "", "  ")
		fmt.Fprintln(a.Stdout, string(data))
		return nil
	}
	a.Info(lockSummary(st))
	return nil
}

// reportLockDelta prints what a --dry-run lock would add/remove/rehash
// relative to the on-disk lock.
func (a *App) reportLockDelta(agentsDir string, st integrityState) {
	existing, _ := loadAgentsLock(agentsDir)
	old := map[string]lockedArtifact{}
	for _, la := range existing.Artifacts {
		old[la.Path] = la
	}
	cur := map[string]bool{}
	for _, la := range st.artifacts {
		cur[la.Path] = true
		prev, ok := old[la.Path]
		if !ok {
			a.Info("+ " + la.Path + " (" + la.Origin + ")")
		} else if prev.TreeHash != la.TreeHash {
			a.Info("~ " + la.Path + " (rehashed)")
		}
	}
	for _, la := range existing.Artifacts {
		if !cur[la.Path] {
			a.Info("- " + la.Path)
		}
	}
	a.Info(lockSummary(st))
}

// lockSummary is the one-line "locked N artifacts (M files) …" report.
func lockSummary(st integrityState) string {
	byOrigin := map[string]int{}
	files := 0
	for _, la := range st.artifacts {
		byOrigin[originClass(la.Origin)]++
	}
	for _, s := range st.sums {
		if !strings.HasPrefix(s.Value, "link:") {
			files++
		}
	}
	buckets := map[string]bool{}
	for _, la := range st.artifacts {
		buckets[la.Bucket] = true
	}
	return fmt.Sprintf("locked %d artifacts (%d files) across %d buckets; %d source, %d linked, %d imported, %d local",
		len(st.artifacts), files, len(buckets),
		byOrigin["source"], byOrigin["linked"], byOrigin["imported"], byOrigin["local"])
}

// originClass reduces a full origin string to its leading class word.
func originClass(origin string) string {
	if i := strings.IndexByte(origin, ':'); i >= 0 {
		return origin[:i]
	}
	return origin
}

// ---------------------------------------------------------------------
// verify command
// ---------------------------------------------------------------------

// Finding is one verify result; JSON tags match the documented schema.
type Finding struct {
	Path     string `json:"path"`
	Origin   string `json:"origin"`
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Want     string `json:"want,omitempty"`
	Got      string `json:"got,omitempty"`
}

// verifyResult is the --json envelope.
type verifyResult struct {
	Status   string         `json:"status"`
	Findings []Finding      `json:"findings"`
	Counts   map[string]int `json:"counts"`
}

const (
	sevError = "ERROR"
	sevWarn  = "WARN"
	sevInfo  = "INFO"
)

// CmdVerify implements `sync-agents verify`.
func (a *App) CmdVerify(opts SourceCmdOpts, strict bool, explain string) error {
	agentsDir := a.sourceAgentsDir(opts.Global)

	lockPath := AgentsLockPath(agentsDir)
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		a.Info("not locked — run 'sync-agents lock'")
		return nil
	}
	stored, err := loadAgentsLock(agentsDir)
	if err != nil {
		a.Error(err.Error())
		return &ExitError{Code: 2, Err: err}
	}
	storedSum, err := loadAgentsSum(agentsDir)
	if err != nil {
		a.Error(err.Error())
		return &ExitError{Code: 2, Err: err}
	}
	st, err := computeIntegrityState(agentsDir)
	if err != nil {
		a.Error(err.Error())
		return &ExitError{Code: 2, Err: err}
	}

	if explain != "" {
		return a.explainOne(agentsDir, explain, stored, st)
	}

	findings := verifyFindings(agentsDir, stored, storedSum, st, strict)

	if opts.JSON {
		return a.emitVerifyJSON(findings)
	}
	return a.emitVerifyText(findings)
}

// verifyFindings compares the stored lock+sum against the freshly
// walked tree and returns findings, most-severe ordering left to the
// caller. Pure (no I/O beyond hashing agents.lock) for testability.
func verifyFindings(agentsDir string, stored agentsLock, storedSum map[string]string, st integrityState, strict bool) []Finding {
	var findings []Finding
	fresh := map[string]string{}
	for _, s := range st.sums {
		fresh[s.Path] = s.Value
	}

	// 1. Sum check — file-level content/coverage.
	consumed := map[string]bool{}
	for _, s := range st.sums {
		want, ok := storedSum[s.Path]
		if !ok {
			// On disk, covered, but never locked = bypassed every gate.
			findings = append(findings, Finding{
				Path: s.Path, Origin: st.fileOrigin[s.Path], Severity: sevError,
				Kind: "unlocked-file", Got: s.Value,
			})
			continue
		}
		consumed[s.Path] = true
		if want == s.Value {
			continue
		}
		origin := st.fileOrigin[s.Path]
		kind := "content-mismatch"
		sev := driftSeverity(origin, strict)
		if strings.HasPrefix(want, "link:") || strings.HasPrefix(s.Value, "link:") {
			// A swapped symlink is a tamper signal regardless of origin
			// (SPEC-008 §Security framing lists it explicitly).
			kind = "link-repointed"
			sev = sevError
		}
		findings = append(findings, Finding{
			Path: s.Path, Origin: origin, Severity: sev, Kind: kind, Want: want, Got: s.Value,
		})
	}

	// 2. Anything in the stored sum but not on disk.
	for p, want := range storedSum {
		if consumed[p] {
			continue
		}
		if p == AgentsLockFileName {
			got, err := sha256File(AgentsLockPath(agentsDir))
			if err != nil {
				findings = append(findings, Finding{Path: p, Severity: sevError, Kind: "lock-missing"})
			} else if got != want {
				// Hand-edited lock — the sum is the root of trust.
				findings = append(findings, Finding{Path: p, Severity: sevError, Kind: "lock-tampered", Want: want, Got: got})
			}
			continue
		}
		findings = append(findings, Finding{
			Path: p, Origin: originForStoredFile(stored, p), Severity: sevError,
			Kind: "missing-file", Want: want,
		})
	}

	// 3. Cross-check: a source: artifact's tree_hash must equal its
	// sources.lock content_hash at lock time (SPEC-008 invariant).
	srcLock, _ := source.LoadLock(agentsDir)
	freshByPath := map[string]lockedArtifact{}
	for _, la := range st.artifacts {
		freshByPath[la.Path] = la
	}
	for _, la := range st.artifacts {
		if !strings.HasPrefix(la.Origin, "source:") || la.ResolvedSHA == "" {
			continue
		}
		for _, le := range srcLock.Entries {
			if le.ResolvedSHA == la.ResolvedSHA && le.ContentHash != "" && le.ContentHash != la.TreeHash {
				findings = append(findings, Finding{
					Path: la.Path, Origin: la.Origin, Severity: sevError,
					Kind: "sources-lock-mismatch", Want: le.ContentHash, Got: la.TreeHash,
				})
			}
		}
	}

	sortFindings(findings)
	return findings
}

// driftSeverity: a linked checkout drifts by design (INFO, ERROR under
// --strict); anything else is ERROR.
func driftSeverity(origin string, strict bool) string {
	if strings.HasPrefix(origin, "linked:") {
		if strict {
			return sevError
		}
		return sevInfo
	}
	return sevError
}

// originForStoredFile finds the origin of the artifact that owns a
// stored sum path (best-effort; "" for root inputs).
func originForStoredFile(stored agentsLock, p string) string {
	best := ""
	for _, la := range stored.Artifacts {
		if la.Path == p || strings.HasPrefix(p, la.Path+"/") {
			if len(la.Path) > len(best) {
				best = la.Origin
			}
		}
	}
	return best
}

// sortFindings orders ERROR → WARN → INFO, then by path, for a stable
// most-severe-first report.
func sortFindings(f []Finding) {
	rank := map[string]int{sevError: 0, sevWarn: 1, sevInfo: 2}
	sort.SliceStable(f, func(i, j int) bool {
		if rank[f[i].Severity] != rank[f[j].Severity] {
			return rank[f[i].Severity] < rank[f[j].Severity]
		}
		return f[i].Path < f[j].Path
	})
}

// verifyExitError maps findings to the SPEC-008 exit-code policy: 0
// clean (INFO allowed), 1 on any WARN/ERROR.
func verifyExitError(findings []Finding) error {
	for _, f := range findings {
		if f.Severity == sevError || f.Severity == sevWarn {
			return &ExitError{Code: 1, Err: fmt.Errorf("verify failed: integrity drift detected")}
		}
	}
	return nil
}

func (a *App) emitVerifyText(findings []Finding) error {
	if len(findings) == 0 {
		a.Info("verify: clean — tree matches agents.lock + agents.sum")
		return nil
	}
	for _, f := range findings {
		msg := fmt.Sprintf("[%s] %s: %s", f.Severity, f.Kind, f.Path)
		if f.Want != "" || f.Got != "" {
			msg += fmt.Sprintf(" (want %s, got %s)", short(f.Want), short(f.Got))
		}
		switch f.Severity {
		case sevError:
			a.Error(msg)
		default:
			a.Info(msg)
		}
	}
	return verifyExitError(findings)
}

func (a *App) emitVerifyJSON(findings []Finding) error {
	counts := map[string]int{sevError: 0, sevWarn: 0, sevInfo: 0}
	for _, f := range findings {
		counts[f.Severity]++
	}
	status := "clean"
	if counts[sevError] > 0 || counts[sevWarn] > 0 {
		status = "drift"
	}
	if findings == nil {
		findings = []Finding{}
	}
	out := verifyResult{Status: status, Findings: findings, Counts: counts}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return &ExitError{Code: 2, Err: err}
	}
	fmt.Fprintln(a.Stdout, string(data))
	return verifyExitError(findings)
}

// explainOne answers "where did this come from and does it match?" for
// a single artifact path.
func (a *App) explainOne(agentsDir, target string, stored agentsLock, st integrityState) error {
	target = filepath.ToSlash(strings.TrimSuffix(target, "/"))
	var locked *lockedArtifact
	for i := range stored.Artifacts {
		if stored.Artifacts[i].Path == target {
			locked = &stored.Artifacts[i]
			break
		}
	}
	if locked == nil {
		a.Error(fmt.Sprintf("%s: not in agents.lock", target))
		return &ExitError{Code: 2, Err: fmt.Errorf("unknown artifact")}
	}
	var fresh *lockedArtifact
	for i := range st.artifacts {
		if st.artifacts[i].Path == target {
			fresh = &st.artifacts[i]
			break
		}
	}
	out := a.Stdout
	fmt.Fprintln(out, target)
	fmt.Fprintf(out, "  origin:       %s\n", locked.Origin)
	if locked.ResolvedSHA != "" {
		fmt.Fprintf(out, "  resolved_sha: %s\n", short(locked.ResolvedSHA))
	}
	fmt.Fprintf(out, "  locked:       %s\n", locked.TreeHash)
	if fresh == nil {
		fmt.Fprintln(out, "  on disk:      MISSING")
		return &ExitError{Code: 1, Err: fmt.Errorf("artifact missing")}
	}
	status := "match"
	if fresh.TreeHash != locked.TreeHash {
		status = "DRIFT"
	}
	fmt.Fprintf(out, "  on disk:      %s  → %s\n", fresh.TreeHash, status)

	// Per-file breakdown.
	storedSum, _ := loadAgentsSum(agentsDir)
	match, modified := 0, []string{}
	for _, f := range st.artFiles[target] {
		if storedSum[f.Path] == f.Value {
			match++
		} else {
			modified = append(modified, f.Path)
		}
	}
	fmt.Fprintf(out, "  files:        %d match, %d modified", match, len(modified))
	if len(modified) > 0 {
		fmt.Fprintf(out, " (%s)", strings.Join(modified, ", "))
	}
	fmt.Fprintln(out)
	return nil
}

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

func loadAgentsLock(agentsDir string) (agentsLock, error) {
	data, err := os.ReadFile(AgentsLockPath(agentsDir))
	if err != nil {
		return agentsLock{}, err
	}
	var l agentsLock
	if err := yaml.Unmarshal(data, &l); err != nil {
		return agentsLock{}, fmt.Errorf("%s: %w", AgentsLockPath(agentsDir), err)
	}
	return l, nil
}

// loadAgentsSum parses agents.sum into a path→value map.
func loadAgentsSum(agentsDir string) (map[string]string, error) {
	data, err := os.ReadFile(AgentsSumPath(agentsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.IndexByte(line, ' ')
		if i <= 0 {
			continue
		}
		out[line[:i]] = strings.TrimSpace(line[i+1:])
	}
	return out, nil
}

func artifactsEqual(a, b []lockedArtifact) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sha256Bytes(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", h)
}

func short(s string) string {
	s = strings.TrimPrefix(s, "sha256:")
	if len(s) > 12 {
		return s[:12] + "…"
	}
	return s
}

// writeFileAtomicAgent writes via temp file + rename in the target's
// directory (mirrors concat.go's helper; kept local to avoid widening
// any package's API surface).
func writeFileAtomicAgent(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}
