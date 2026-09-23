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

// linkOutcome reports what placeLink actually did, so callers can
// keep accurate "fixed N item(s)" tallies without re-inspecting the
// filesystem.
type linkOutcome int

const (
	// linkNoop means the link already pointed at the right place.
	linkNoop linkOutcome = iota
	// linkCreated means nothing was at the path and a link was made.
	linkCreated
	// linkRepaired means a drifted symlink was replaced.
	linkRepaired
	// linkMovedAside means a real file/dir was renamed to a backup
	// sibling and the link placed in its stead (Overwrite only).
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

// changed reports whether the call altered the filesystem (or would
// have, under dry-run).
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

// drillBucket links each artifact of .agents/<bucket>/ into the real
// directory dest one entry at a time. Dotfiles in the source are
// skipped (editor state, .DS_Store). Entries are linked as a whole:
// a skill directory becomes <dest>/<name> -> ../../.agents/skills/<name>,
// not a link to its SKILL.md, so supporting files inside the skill
// resolve too. The relative target is computed with filepath.Rel so
// the same code serves .claude/ and the deeper .github/copilot/.
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

// placeClaimed places one symlink for an artifact .agents/ claims and
// folds the outcome into r. claimed is the display path used in the
// conflict warning ("skills/foo" rather than the absolute link path).
func (a *App) placeClaimed(source, target, claimed string, r *BucketLinkResult) {
	outcome, err := a.placeLink(source, target, a.DryRun)
	if errors.Is(err, ErrConflict) {
		r.Conflicts++
		a.Warn(fmt.Sprintf("conflict: %s is a real %s shadowing %s; leaving it in place (resync with --overwrite to move it aside)",
			target, realKind(target), claimed))
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

// realKind names the obstruction at path for conflict messages:
// "directory" or "file".
func realKind(path string) string {
	if fi, err := os.Lstat(path); err == nil && fi.IsDir() {
		return "directory"
	}
	return "file"
}

// placeLink is the primitive behind CreateSymlink. source is the link
// text (usually relative to the link's own directory) and target is
// the link path. It reports what it did so bucket-level callers can
// tally changes.
//
// The never-delete guarantee lives here: a real file or directory at
// target is either left alone (ErrConflict) or renamed to a
// BackupSuffix sibling when App.Overwrite is set. os.RemoveAll is
// deliberately absent; the previous --force implementation used it
// and wiped a user's real ~/.wave/skills directory.
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

// linkSatisfied reports whether the symlink at target already points
// at source: either its link text is identical, or it resolves to
// the same file (an absolute link a user placed by hand is not
// "drifted" just because sync would have written a relative one).
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

// resolveLinkSource turns a symlink's text into a path that can be
// stat'ed: relative link text is interpreted from the link's own
// directory, as the kernel does.
func resolveLinkSource(target, source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(filepath.Dir(target), source)
}

// backupPath returns the sibling a conflicting path is renamed to
// under --overwrite: <path>.replaced-by-sync-agents, or with a unix
// timestamp appended when that name is already taken by an earlier
// move.
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

// bucketMergeStats inspects a real directory dest against the
// artifacts in srcAbs using the same conformance predicate sync uses
// (linkSatisfied), so status and sync agree on what counts as linked.
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
