package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrConflict is returned by CreateSymlink when a real (non-symlink)
// file or directory occupies the link's path and App.Overwrite is
// not set. The caller decides how to surface it; CreateSymlink never
// removes the obstruction on its own.
var ErrConflict = errors.New("conflict: real file or directory in the way (resync with --overwrite to move it aside)")

// BackupSuffix is appended to a conflicting path when --overwrite
// moves it aside. The same suffix is used by global sync so users
// learn one recovery location for both scopes.
const BackupSuffix = ".replaced-by-sync-agents"

type linkOutcome int

const (
	linkNoop linkOutcome = iota
	linkCreated
	linkRepaired
	linkMovedAside
)

// BucketLinkResult tallies one linkBucket call. CmdSync sums
// Conflicts across targets to decide its exit status; CmdFix sums
// the change counters for its "Fixed N item(s)" summary.
type BucketLinkResult struct {
	// Linked counts symlinks created where nothing existed.
	Linked int
	// Repaired counts drifted symlinks that were re-pointed.
	Repaired int
	// Moved counts real files/dirs renamed aside under --overwrite.
	Moved int
	// Conflicts counts real files/dirs left in place because
	// --overwrite was not set.
	Conflicts int
}

func (r BucketLinkResult) changed() bool {
	return r.Linked+r.Repaired+r.Moved > 0
}

// deprecateForce maps the legacy --force flag onto --overwrite for
// the project-scope link commands (sync, fix). --force used to mean
// "os.RemoveAll whatever is in the way", which wiped real tool
// directories such as ~/.wave/skills; SPEC-010 §Phase 3 replaces
// that with a never-delete guarantee. The flag is kept as an alias
// so existing scripts keep working, but it now only moves conflicts
// aside.
func (a *App) deprecateForce() {
	if a.Force && !a.Overwrite {
		a.Warn("--force is deprecated for this command; it now behaves as --overwrite (nothing is deleted)")
		a.Overwrite = true
	}
}

// linkBucket materializes one bucket of .agents/ into one target tool
// directory using the fold-or-drill algorithm from SPEC-010 §Phase 3
// (GNU Stow's tree-folding model). dest is <targetDir>/<bucket.Dir>
// and agentsRel is the target's relative path back to .agents/
// (ResolveAgentsRel).
//
//   - Nothing at dest: one relative symlink dest -> agentsRel/<bucket>
//     (fold). This is the historical whole-bucket behavior.
//   - Symlink at dest: no-op when it already resolves to the bucket,
//     otherwise replaced (a symlink holds no data, so repair needs no
//     flag).
//   - Real directory at dest: drill. The directory is the tool's own
//     (e.g. a real ~/.wave/skills that the tool created) and is never
//     moved, renamed, or deleted under any flag. Each entry of
//     .agents/<bucket>/ is linked individually inside it as
//     <dest>/<name> -> <relative path to .agents/<bucket>/<name>>.
//     Entries the tool owns that .agents/ does not claim are foreign
//     and are neither touched nor reported. A real file or directory
//     at <dest>/<name> that shadows a claimed artifact is a conflict:
//     warned and counted, and only moved aside (never deleted) when
//     App.Overwrite is set.
//   - Real file at dest: conflict with the same rule as a per-entry
//     conflict; under Overwrite it is moved aside and the bucket is
//     folded.
//
// Both CmdSync and CmdFix call this so the two commands cannot
// diverge in what they consider conformant.
func (a *App) linkBucket(targetDir, agentsRel string, b Bucket) BucketLinkResult {
	var r BucketLinkResult
	dest := filepath.Join(targetDir, b.Dir)
	srcAbs := filepath.Join(a.ProjectRoot, ".agents", b.Dir)
	foldSource := agentsRel + "/" + b.Dir

	fi, err := os.Lstat(dest)
	if err == nil && fi.Mode()&os.ModeSymlink == 0 && fi.IsDir() {
		a.drillBucket(dest, srcAbs, b, &r)
		return r
	}

	a.placeClaimed(foldSource, dest, ".agents/"+b.Dir, &r)
	return r
}

// drillBucket links each entry of .agents/<bucket>/ as a whole
// (<dest>/<name> -> ../../.agents/skills/<name>), not its SKILL.md as
// global sync does, so a skill's supporting files resolve through the
// link too. filepath.Rel keeps the link text correct at any depth,
// including .github/copilot/.
func (a *App) drillBucket(dest, srcAbs string, b Bucket, r *BucketLinkResult) {
	entries, err := os.ReadDir(srcAbs)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		srcEntry := filepath.Join(srcAbs, name)
		rel, err := filepath.Rel(dest, srcEntry)
		if err != nil {
			a.Warn(fmt.Sprintf("cannot compute relative link for %s: %v", srcEntry, err))
			continue
		}
		a.placeClaimed(rel, filepath.Join(dest, name), ".agents/"+b.Dir+"/"+name, r)
	}
}

func (a *App) placeClaimed(source, target, claimedLabel string, r *BucketLinkResult) {
	outcome, err := a.placeLink(source, target, a.DryRun)
	if errors.Is(err, ErrConflict) {
		r.Conflicts++
		a.Warn(fmt.Sprintf("conflict: %s is a real %s shadowing %s; leaving it in place (resync with --overwrite to move it aside)",
			target, realKind(target), claimedLabel))
		return
	}
	if err != nil {
		a.Warn(fmt.Sprintf("link %s: %v", target, err))
		return
	}
	switch outcome {
	case linkCreated:
		r.Linked++
	case linkRepaired:
		r.Repaired++
	case linkMovedAside:
		r.Moved++
	}
}

func realKind(path string) string {
	if fi, err := os.Lstat(path); err == nil && fi.IsDir() {
		return "directory"
	}
	return "file"
}

// placeLink is the primitive behind CreateSymlink and the only place
// that touches what is already at target. The never-delete guarantee
// (SPEC-010 §Phase 3) lives here: a real file or directory is either
// left alone (ErrConflict) or renamed to a BackupSuffix sibling under
// App.Overwrite. Nothing in this package may call os.RemoveAll on a
// tool directory.
//
// A real path that already resolves to the same file as source (via
// an ancestor symlink, or because .agents/<x> itself links to it) is
// treated as conformant and skipped rather than reported as a
// conflict; renaming it would move the canonical copy out from under
// its own link (the issue-#90 self-referential symlink bug).
func (a *App) placeLink(source, target string, dryRun bool) (linkOutcome, error) {
	fi, err := os.Lstat(target)
	if err != nil && !os.IsNotExist(err) {
		return linkNoop, err
	}
	exists := err == nil

	if exists && fi.Mode()&os.ModeSymlink != 0 {
		if linkSatisfied(target, source) {
			return linkNoop, nil
		}
		existing, _ := os.Readlink(target)
		if dryRun {
			fmt.Fprintf(a.Stdout, "  would relink: %s -> %s (was %s)\n", target, source, existing)
			return linkRepaired, nil
		}
		if err := os.Remove(target); err != nil {
			return linkNoop, err
		}
		if err := os.Symlink(source, target); err != nil {
			return linkNoop, err
		}
		a.Info(fmt.Sprintf("Repaired: %s -> %s (was %s)", target, source, existing))
		return linkRepaired, nil
	}

	outcome := linkCreated
	if exists {
		if foldedResolves(target, resolveLinkSource(target, source)) {
			return linkNoop, nil
		}
		if !a.Overwrite {
			return linkNoop, ErrConflict
		}
		backup := backupPath(target)
		if dryRun {
			fmt.Fprintf(a.Stdout, "  would move aside: %s -> %s\n", target, backup)
		} else {
			if err := os.Rename(target, backup); err != nil {
				return linkNoop, err
			}
			a.Warn(fmt.Sprintf("moved existing %s to %s", target, backup))
		}
		outcome = linkMovedAside
	}

	if dryRun {
		fmt.Fprintf(a.Stdout, "  would link: %s -> %s\n", target, source)
		return outcome, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return linkNoop, err
	}
	if err := os.Symlink(source, target); err != nil {
		return linkNoop, err
	}
	a.Info(fmt.Sprintf("Linked: %s -> %s", target, source))
	return outcome, nil
}

// linkSatisfied treats a symlink that resolves to source as correct
// even when its text differs, so an absolute link a user placed by
// hand is not "repaired" into a relative one on every run.
func linkSatisfied(target, source string) bool {
	existing, err := os.Readlink(target)
	if err != nil {
		return false
	}
	if existing == source {
		return true
	}
	return foldedResolves(target, resolveLinkSource(target, source))
}

func resolveLinkSource(target, source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(filepath.Dir(target), source)
}

func backupPath(path string) string {
	backup := path + BackupSuffix
	if _, err := os.Lstat(backup); err == nil {
		backup = fmt.Sprintf("%s.%d", backup, time.Now().Unix())
	}
	return backup
}

// BucketMergeStats describes how far a real (drilled) bucket
// directory in a tool dir has been merged with .agents/<bucket>.
// CmdStatus renders it as "[merged] skills (2/3 linked, 1 conflict(s))".
type BucketMergeStats struct {
	// Total is the number of artifacts .agents/<bucket> claims
	// (dotfiles excluded).
	Total int
	// Linked is how many of those have a correct symlink inside the
	// real directory.
	Linked int
	// Conflicts is how many are shadowed by a real file/dir.
	Conflicts int
}

func bucketMergeStats(dest, srcAbs string) BucketMergeStats {
	var s BucketMergeStats
	entries, err := os.ReadDir(srcAbs)
	if err != nil {
		return s
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		s.Total++
		entryDest := filepath.Join(dest, name)
		fi, err := os.Lstat(entryDest)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(dest, filepath.Join(srcAbs, name))
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			if linkSatisfied(entryDest, rel) {
				s.Linked++
			}
			continue
		}
		if foldedResolves(entryDest, filepath.Join(srcAbs, name)) {
			s.Linked++
			continue
		}
		s.Conflicts++
	}
	return s
}
