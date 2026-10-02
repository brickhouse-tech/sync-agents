package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// This file removes the placements earlier versions made and SPEC-013
// retires (§Migration, step 4). Each removal needs a proof that the
// path is sync-agents'; anything else is reported and left.

// legacyFold is one tool-directory link the pre-SPEC-013 sync made:
// Path folded (or drilled) Source, the bucket directory in .agents/.
type legacyFold struct {
	Path, Source string
}

// legacyFoldPaths lists the folds linksBucket now withholds: every
// <tool dir>/<bucket> fold of a passive bucket into a tool that has a
// local channel (.cursor/rules, .github/copilot/rules, .codex/rules,
// .opencode/rules). It is derived from the same registry linksBucket
// reads, so the two cannot disagree.
func legacyFoldPaths(root string) []legacyFold {
	var folds []legacyFold
	for _, tool := range Tools {
		for _, b := range Buckets {
			if b.SyncsToTool(tool.ID) && !linksBucket(tool.ID, b) {
				folds = append(folds, legacyFold{
					Path:   filepath.Join(ResolveTargetDir(tool.ID, root), b.Dir),
					Source: filepath.Join(root, ".agents", b.Dir),
				})
			}
		}
	}
	sort.Slice(folds, func(i, j int) bool { return folds[i].Path < folds[j].Path })
	return folds
}

// removeLegacyPlacements removes the local legacy links (SPEC-013
// §Migration, step 4) and returns the paths it removed. The proof is
// the link itself: a symlink that resolves into the folded bucket
// (.agents/rules) is sync-agents' (the old fold). A real directory
// there is the tool's own: only the links inside it that resolve into
// the bucket (the old per-rule drill) are removed, and the directory
// goes only when that leaves it empty. Our own channel link in
// .cursor/rules points into .agents/index/, so it is never taken for a
// legacy one. Anything else is reported and left. It is idempotent and
// dry-run aware.
func (a *App) removeLegacyPlacements() []string {
	var removed []string
	for _, fold := range legacyFoldPaths(a.ProjectRoot) {
		path, tree := fold.Path, fold.Source
		fi, err := os.Lstat(path)
		if err != nil {
			continue
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			if !pointsInto(path, tree) {
				a.Info(fmt.Sprintf("left %s: a symlink outside %s is not sync-agents'", a.display(path), a.display(tree)))
				continue
			}
			if a.removePath(path) {
				removed = append(removed, path)
				a.pruneEmptyParents(filepath.Dir(path))
			}
		case fi.IsDir():
			entries, err := os.ReadDir(path)
			if err != nil {
				a.Warn(fmt.Sprintf("read %s: %v", a.display(path), err))
				continue
			}
			for _, e := range entries {
				p := filepath.Join(path, e.Name())
				if e.Type()&os.ModeSymlink != 0 && pointsInto(p, tree) && a.removePath(p) {
					removed = append(removed, p)
				}
			}
			a.pruneEmptyParents(path)
		default:
			a.Info(fmt.Sprintf("left %s: a real file is not sync-agents'", a.display(path)))
		}
	}
	if len(removed) > 0 && !a.DryRun {
		a.Info("Removed legacy rule links: rules now reach Cursor, Copilot, Codex, and opencode through .agents/index/")
	}
	return removed
}

// pointsInto reports whether the symlink at link resolves inside dir.
// A dangling link is judged by its text, so a fold whose target was
// deleted is still recognized.
func pointsInto(link, dir string) bool {
	within := func(p, root string) bool {
		return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
	}
	if real, err := filepath.EvalSymlinks(link); err == nil {
		if rdir, err := filepath.EvalSymlinks(dir); err == nil && within(real, rdir) {
			return true
		}
	}
	text, err := os.Readlink(link)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(text) {
		text = filepath.Join(filepath.Dir(link), text)
	}
	return within(filepath.Clean(text), filepath.Clean(dir))
}
